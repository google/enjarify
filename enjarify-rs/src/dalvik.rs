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
use std::collections::{BTreeSet, HashSet};
// use std::fmt;
use std::ops::Deref;

use strings::*;
use byteio::Reader;
use dex::DexFile;
use dalvikformats::{DalvikArgs, decode};

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum DalvikType {
    Nop,
    Move,
    MoveWide,
    MoveResult,
    Return,
    Const32,
    Const64,
    ConstString,
    ConstClass,
    MonitorEnter,
    MonitorExit,
    CheckCast,
    InstanceOf,
    ArrayLen,
    NewInstance,
    NewArray,
    FilledNewArray,
    FillArrayData,
    Throw,
    Goto,
    Switch,
    Cmp,
    If,
    IfZ,
    ArrayGet,
    ArrayPut,
    InstanceGet,
    InstancePut,
    StaticGet,
    StaticPut,
    InvokeVirtual,
    InvokeSuper,
    InvokeDirect,
    InvokeStatic,
    InvokeInterface,
    UnaryOp,
    BinaryOp,
    BinaryOpConst,
}
use self::DalvikType::*;
impl DalvikType {
    pub fn is_pruned_throw(&self) -> bool {
        match *self {
            InvokeVirtual | InvokeSuper | InvokeDirect | InvokeStatic | InvokeInterface | MonitorEnter | MonitorExit | CheckCast | ArrayLen | NewArray | NewInstance | FilledNewArray | FillArrayData | Throw | ArrayGet | ArrayPut | InstanceGet | InstancePut | StaticGet | StaticPut | BinaryOp | BinaryOpConst => true,
            _ => false
        }
    }
}

#[allow(dead_code)] //width is not used
pub struct ArrayData<'a>{width: u8, pub count: u32, pub stream: Reader<'a>}
pub struct SwitchData<'a>{packed: bool, count: u32, stream: Reader<'a>}
impl<'a> SwitchData<'a> {
    pub fn parse(&self) -> Vec<(u32, u32)> {
        let mut st = self.stream.clone();
        if self.packed {
            let first_key = st.u32();
            (0..self.count).map(|i| (first_key.wrapping_add(i), st.u32())).collect()
        } else {
            let mut st2 = st.offset(4 * self.count);
            (0..self.count).map(|_| (st.u32(), st2.u32())).collect()
        }
    }
}

pub struct DalvikInstruction<'a> {
    pub typ: DalvikType,
    pub pos: u32,
    pub pos2: u32,

    pub opcode: u8,
    arguments: DalvikArgs,

    pub prev_result: Option<&'a bstr>, //for move-result/exception
    pub implicit_casts: Option<(u32, Vec<u16>)>,
    pub array_data: Option<ArrayData<'a>>,
    pub switch_data: Option<SwitchData<'a>>,
}
// To make accessing arguments more convienent
impl<'a> Deref for DalvikInstruction<'a> {
    type Target = DalvikArgs;
    fn deref(&self) -> &DalvikArgs { &self.arguments }
}
// impl<'a> fmt::Debug for DalvikInstruction<'a> {
//     fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
//         write!(f, "DIns{{typ: {:?}}}", self.typ)
//     }
// }
fn op_to_type(opcode: u8) -> DalvikType {
    match opcode {
        0x00 => Nop,
        0x01...0x03 => Move,
        0x04...0x06 => MoveWide,
        0x07...0x09 => Move,
        0x0a...0x0d => MoveResult,
        0x0e...0x11 => Return,
        0x12...0x15 => Const32,
        0x16...0x19 => Const64,
        0x1a...0x1b => ConstString,
        0x1c => ConstClass,
        0x1d => MonitorEnter,
        0x1e => MonitorExit,
        0x1f => CheckCast,
        0x20 => InstanceOf,
        0x21 => ArrayLen,
        0x22 => NewInstance,
        0x23 => NewArray,
        0x24...0x25 => FilledNewArray,
        0x26 => FillArrayData,
        0x27 => Throw,
        0x28...0x2a => Goto,
        0x2b...0x2c => Switch,
        0x2d...0x31 => Cmp,
        0x32...0x37 => If,
        0x38...0x3d => IfZ,
        0x3e...0x43 => Nop,
        0x44...0x4a => ArrayGet,
        0x4b...0x51 => ArrayPut,
        0x52...0x58 => InstanceGet,
        0x59...0x5f => InstancePut,
        0x60...0x66 => StaticGet,
        0x67...0x6d => StaticPut,
        0x6e => InvokeVirtual,
        0x6f => InvokeSuper,
        0x70 => InvokeDirect,
        0x71 => InvokeStatic,
        0x72 => InvokeInterface,
        0x73 => Nop,
        0x74 => InvokeVirtual,
        0x75 => InvokeSuper,
        0x76 => InvokeDirect,
        0x77 => InvokeStatic,
        0x78 => InvokeInterface,
        0x79...0x7a => Nop,
        0x7b...0x8f => UnaryOp,
        0x90...0xcf => BinaryOp,
        0xd0...0xe2 => BinaryOpConst,
        0xe3...0xff => Nop,
        _ => unreachable!()
    }
}

fn parse_instruction<'a>(start_st: &Reader<'a>, shorts: &[u16], pos: usize) -> (usize, DalvikInstruction<'a>) {
    let word = shorts[pos];
    let opcode = word as u8;
    let (mut newpos, args) = decode(shorts, pos, opcode);

    let switch_data = match word {
        0x100 | 0x200 => {
            let packed = word == 0x100;
            let count = shorts[pos+1] as u32;
            let keysize = if packed {1} else {count};
            newpos = pos + (2 + (keysize + count) * 2) as usize;

            Some(SwitchData{packed: packed, count: count,
                stream: start_st.offset(pos as u32*2 + 4),
        })},
        _ => None
    };

    let array_data = if word == 0x300 {
        let width = (shorts[pos+1] % 16) as u32;
        let count = (shorts[pos+2] as u32) ^ (shorts[pos+3] as u32) << 16;
        newpos = pos + ((count*width+1)/2 + 4) as usize;

        Some(ArrayData{width: width as u8, count: count,
            stream: start_st.offset(pos as u32*2 + 8)
        })
    } else { None };

    // warning, this must go below the special data handling that calculates newpos
    (newpos, DalvikInstruction{
        typ: op_to_type(opcode),
        pos: pos as u32,
        pos2: newpos as u32,
        opcode: opcode,
        arguments: args,

        prev_result: None,
        implicit_casts: None,
        array_data: array_data,
        switch_data: switch_data,
    })
}


pub fn parse_bytecode<'a>(dex: &'a DexFile<'a>, start_st: &Reader<'a>, shorts: &[u16], catch_addrs: &HashSet<u32>) -> Vec<DalvikInstruction<'a>> {
    let mut ops = Vec::with_capacity(shorts.len());
    let mut pos = 0;
    while pos < shorts.len() {
        let (newpos, op) = parse_instruction(start_st, shorts, pos);
        pos = newpos;
        ops.push(op);
    }

    // Fill in data for move-result
    {
        let mut prev = None;
        for instr in &mut ops {
            if instr.typ == MoveResult {
                if catch_addrs.contains(&instr.pos) {
                    prev = Some(&b"Ljava/lang/Throwable;"[..]);
                }
                // prev may be None if instruction is unreachable
                instr.prev_result = prev;
            };

            prev = match instr.typ {
                InvokeVirtual | InvokeSuper | InvokeDirect | InvokeStatic | InvokeInterface =>
                    Some(dex.method_id(instr.a).return_type),
                FilledNewArray => Some(dex.raw_type(instr.a)),
                _ => None,
            };
        }
    }

    // Fill in implicit cast data
    {
        let mut prev2 = (Nop, 0, 0, 0);
        let mut prev = (Nop, 0, 0, 0);
        for instr in &mut ops {
            if instr.opcode == 0x38 || instr.opcode == 0x39 { // if-eqz, if-nez
                if prev.0 == InstanceOf {
                    let desc_ind = prev.3;
                    let mut regs = BTreeSet::new();
                    regs.insert(prev.2);

                    if prev2.0 == Move && prev2.1 == prev.2 {
                        regs.insert(prev2.2);
                    }
                    regs.remove(&prev.1);

                    instr.implicit_casts = Some((desc_ind, regs.into_iter().collect()));
                }
            }

            prev2 = prev;
            prev = (instr.typ, instr.ra, instr.rb, instr.c);
        }
    }

    ops
}
