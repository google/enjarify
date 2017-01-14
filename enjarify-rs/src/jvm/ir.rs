// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
use std::cmp::min;
use std::collections::HashMap;

use strings::*;

use error;
use pack;
use typeinference::scalar;
use super::constantpool::{ConstantPool, Entry, ArgsPrim, Integer, Float, Double, Long};
use super::constants::calc;
use super::jvmops::*;

#[derive(Clone, Debug)]
pub enum JvmInstructionSub {
    Label(LabelId),
    RegAccess(RAImpl),
    PrimConstant(PCImpl),
    OtherConstant,
    Goto(GotoImpl),
    If(IfImpl),
    Switch(Box<SwitchImpl>),
    Other,
}
pub use self::JvmInstructionSub::*;

#[derive(Clone, Debug)]
pub struct JvmInstruction {
    pub bytecode: Option<BString>,
    pub sub: JvmInstructionSub,
}
impl JvmInstruction {
    pub fn fallsthrough(&self) -> bool {
        match self.sub {
            Goto(_) | Switch(_) => false,
            Other => {
                let op = self.bytecode.as_ref().unwrap()[0];
                !(op == ATHROW || (IRETURN <= op && op <= RETURN))
            }
            _ => true,
        }
    }

    // nondeterministic
    pub fn targets(&self) -> Vec<u32> {
        match self.sub {
            Goto(ref data) => vec![data.target],
            If(ref data) => vec![data.target],
            Switch(ref data) => {
                let mut res: Vec<_> = data.jumps.values().map(|&x| x).collect();
                res.push(data.default);
                res
            }
            _ => Vec::new(),
        }
    }

    pub fn lbl(&self) -> Option<LabelId> { if let Label(id) = self.sub {Some(id)} else {None} }

    pub fn is_jump(&self) -> bool { match self.sub {
            Goto(_) | If(_) | Switch(_) => true, _ => false,
    }}
    pub fn is_constant(&self) -> bool { match self.sub {
            PrimConstant(_) | OtherConstant => true, _ => false,
    }}

    // used by jumps -> todo refactor?
    pub fn minlen(&self, pos: u32) -> u32 {
        match self.sub {
            Goto(ref data) => if data.wide {5} else {3},
            If(ref data) => if data.wide {8} else {3},
            Switch(ref data) => {
                let pad = (!pos) % 4;
                pad + data.nopad_size
            },
            _ => self.bytecode.as_ref().unwrap().len() as u32,
        }
    }

    // Get an uppper bound on the size of the bytecode
    pub fn upper_bound(&self) -> usize {
        match self.bytecode { Some(ref bc) => bc.len(), None => {
            match self.sub {
                // RegAccess(_) => 4,
                Goto(_) => 5,
                If(_) => 8,
                Switch(ref data) => 3 + data.nopad_size as usize,
                _ => unreachable!(),
            }
        }}
    }
}

#[derive(Clone, Copy, PartialEq, Eq, Hash, Debug)]
pub enum LabelId {
    DPos(u32), EStart(u32), EEnd(u32), EHandler(u32),
}
pub fn label(lbl: LabelId) -> JvmInstruction { JvmInstruction{bytecode: Some(Vec::new()), sub: Label(lbl)} }

pub type RegKey = (u16, scalar::T);
pub const INVALID_KEY: RegKey = (0, scalar::INVALID);

#[derive(Clone, Debug)]
pub struct RAImpl {
    pub key: RegKey,
    pub store: bool,
}
impl RAImpl {
    pub fn calc_bytecode(&self, local: u16) -> BString {
        let st = self.key.1;
        let opoff = if self.store {ISTORE - ILOAD} else {0};
        if local < 4 {
            vec![ILOAD_0 + opoff + local as u8 + st.ilfda()*4]
        } else if local < 256 {
            vec![ILOAD + opoff + st.ilfda(), local as u8]
        } else {
            pack::BBH(WIDE, ILOAD + opoff + st.ilfda(), local)
        }
    }
}

pub fn reg_access(dreg: u16, st: scalar::T, store: bool) -> JvmInstruction { JvmInstruction{bytecode: None, sub: RegAccess(RAImpl{
    key: (dreg, st),
    store: store,
})} }
pub fn raw_access(local: u16, st: scalar::T, store: bool) -> JvmInstruction {
    let data = RAImpl{key: (0, st), store: store};
    JvmInstruction{bytecode: Some(data.calc_bytecode(local)), sub:RegAccess(data)}
}

#[derive(Clone, Debug)]
pub struct PCImpl {
    pub st: scalar::T,
    pub key: Entry<'static>,
}
impl PCImpl {
    pub fn fix_with_pool(&self, pool: &mut ConstantPool, bc: &mut Option<BString>) {
        if bc.as_ref().unwrap().len() <= 2 { return; }
        let newbc = from_pool(pool, self.key.clone(), self.st.is_wide());
        if newbc.is_some() { *bc = newbc; }
    }
}

fn from_pool<'a>(pool: &mut ConstantPool<'a>, key: Entry<'a>, wide: bool) -> Option<BString> {
    pool.try_get(key).map(|index|
        if wide {
            pack::BH(LDC2_W, index)
        } else if index >= 256 {
            pack::BH(LDC_W, index)
        } else {
            vec![LDC, index as u8]
        }
    )
}

pub fn prim_const<'a>(st: scalar::T, val: u64, pool: Option<&mut (ConstantPool<'a> + 'a)>) -> JvmInstruction {
    assert!(st.is_wide() || val == (val as u32 as u64));
    let val = calc::normalize(st, val);
    let args = ArgsPrim(val);
    let key = match st {
        scalar::INT => Integer(args),
        scalar::FLOAT => Float(args),
        scalar::LONG => Long(args),
        scalar::DOUBLE => Double(args),
        _ => unreachable!()
    };

    // If pool is passed in, just grab an entry greedily, otherwise calculate
    // a sequence of bytecode to generate the constant
    let bytecode = match pool {
        Some(pool) => calc::lookup(st, val).unwrap_or_else(||
            from_pool(pool, key.clone(), st.is_wide()).unwrap_or_else(|| error::classfile_limit_exceeded())
        ),
        None => calc::calc(st, val)
    };

    JvmInstruction{bytecode: Some(bytecode), sub: PrimConstant(PCImpl{st: st, key: key})}
}
pub fn other_const(bc: BString) -> JvmInstruction { JvmInstruction{bytecode: Some(bc), sub: OtherConstant} }

#[derive(Clone, Debug)]
pub struct GotoImpl {
    pub target: u32, pub wide: bool,
}
pub fn goto(target: u32) -> JvmInstruction { JvmInstruction{bytecode: None, sub: Goto(GotoImpl{target: target, wide: false})} }

#[derive(Clone, Debug)]
pub struct IfImpl {
    pub op: u8, pub target: u32, pub wide: bool,
}
pub fn if_(op: u8, target: u32) -> JvmInstruction { JvmInstruction{bytecode: None, sub: If(IfImpl{op:op, target: target, wide: false})} }

#[derive(Clone, Debug)]
pub struct SwitchImpl {
    pub default: u32,
    pub jumps: HashMap<i32, u32>,
    pub low: i32,
    pub high: i32,
    pub is_table: bool,
    nopad_size: u32,
}
pub fn switch(default: u32, jumps: HashMap<i32, u32>) -> JvmInstruction {
    assert!(!jumps.is_empty()); // otherwise it is turned into a goto
    let low = *jumps.keys().min().unwrap() as i64; // convert to i64 to prevent overflow in table count calc
    let high = *jumps.keys().max().unwrap() as i64;
    let table_count = high - low + 1;
    let table_size = 4*(table_count+1);
    let jump_size = 8*(jumps.len() as i64);

    JvmInstruction{bytecode: None, sub: Switch(Box::new(
        SwitchImpl{
            default: default,
            jumps: jumps,

            low: low as i32,
            high: high as i32,
            is_table: jump_size > table_size,
            nopad_size: 9 + min(jump_size, table_size) as u32,
        }
    ))}
}

pub fn other(bc: BString) -> JvmInstruction { JvmInstruction{bytecode: Some(bc), sub: Other} }

// convienence funcs
pub fn pop() -> JvmInstruction { other(vec![POP]) }
pub fn pop2() -> JvmInstruction { other(vec![POP2]) }
pub fn dup() -> JvmInstruction { other(vec![DUP]) }
pub fn dup2() -> JvmInstruction { other(vec![DUP2]) }
