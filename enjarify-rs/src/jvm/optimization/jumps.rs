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
use std::collections::HashMap;

use strings::*;

use byteio::Writer;
use jvm::ir;
use jvm::ir::LabelId::*;
use jvm::jvmops::*;
use jvm::writeir::IRWriter;
use pack;

// todo - make this cleaner (avoid calculating all these unnecsssary vectors)
fn calc_min_positions(instrs: &Vec<ir::JvmInstruction>) -> (Vec<u32>, u32) {
    let mut pos = 0;
    (instrs.iter().map(|ref ins| {
        let old=pos; pos += ins.minlen(old); old
        // let old=pos; pos += ins.minlen(old);
        // println!("old {} pos {}", old, pos);
        // old
    }).collect(), pos)
}

// #[derive(Clone, Copy)]
struct PosInfo<'a>(&'a HashMap<ir::LabelId, usize>, &'a Vec<u32>);
impl<'a> PosInfo<'a> {
    fn getlbl(&self, lbl: ir::LabelId) -> u32 { self.1[self.0[&lbl]] }
    fn get(&self, target: u32) -> u32 { self.getlbl(DPos(target)) }
    fn offset(&self, pos: u32, target: u32) -> i32 { self.get(target).wrapping_sub(pos) as i32 }
}

fn widen_if_necessary(ins: &mut ir::JvmInstruction, pos: u32, info: PosInfo) -> bool {
    match ins.sub {
        ir::Goto(ref mut data) => {
            !data.wide && {
                let offset = info.offset(pos, data.target);
                data.wide = offset != (offset as i16 as i32);
                data.wide
            }
        },
        ir::If(ref mut data) => {
            // println!("pos {} target {} l2v {:?}", pos, data.target, info.0);
            !data.wide && {
                let offset = info.offset(pos, data.target);
                data.wide = offset != (offset as i16 as i32);
                data.wide
            }
        },
        _ => false,
    }
}

pub fn optimize_jumps(irdata: &mut IRWriter) {
    // For jump offsets of more than +-32767, a longer form of the jump instruction
    // is required. This function finds the optimal jump widths by optimistically
    // starting with everything narrow and then iteratively marking instructions
    // as wide if their offset is too large (in rare cases, this can in turn cause
    // other jumps to become wide, hence iterating until convergence)
    let lbl_to_vind: HashMap<ir::LabelId, usize> = irdata.instructions.iter().enumerate()
        .filter(|&(_, ref ins)| ins.lbl().is_some())
        .map(|(i, ref ins)| (ins.lbl().unwrap(), i)).collect();

    loop {
        let mut done = true;
        let mins = calc_min_positions(&irdata.instructions).0;
        for (ins, pos) in irdata.instructions.iter_mut().zip(mins.iter()) {
            // note, not short circuit
            done = done & !widen_if_necessary(ins, *pos, PosInfo(&lbl_to_vind, &mins));
        }

        if done { break; }
    }
}

fn opposite_op(op: u8) -> u8 {
    // todo - add tests for clarity?
    if op >= IFNULL {op ^ 1} else {((op+1)^1)-1}
}

pub fn create_bytecode(irdata: IRWriter) -> (BString, Vec<(u16, u16, u16, u16)>) {
    // todo - avoid repetition?
    let lbl_to_vind: HashMap<ir::LabelId, usize> = irdata.instructions.iter().enumerate()
        .filter(|&(_, ref ins)| ins.lbl().is_some())
        .map(|(i, ref ins)| (ins.lbl().unwrap(), i)).collect();
    let (positions, endpos) = calc_min_positions(&irdata.instructions);
    let info = PosInfo(&lbl_to_vind, &positions);

    let mut stream = Writer(Vec::with_capacity(endpos as usize));
    for (ins, pos) in irdata.instructions.into_iter().zip(positions.iter()) {
        assert!(*pos == stream.0.len() as u32);
        match ins.sub {
            ir::Goto(data) => {
                let offset = info.offset(*pos, data.target);
                stream.0.extend(
                    if data.wide {pack::Bi(GOTO_W, offset)} else {pack::Bh(GOTO, offset as i16)}
                );
            }
            ir::If(data) => {
                let offset = info.offset(*pos, data.target);
                if !data.wide {
                    stream.0.extend(pack::Bh(data.op, offset as i16));
                } else {
                    // Unlike with goto, if instructions are limited to a 16 bit jump offset.
                    // Therefore, for larger jumps, we have to substitute a different sequence
                    //
                    // if x goto A
                    // B: whatever
                    //
                    // becomes
                    //
                    // if !x goto B
                    // goto A
                    // B: whatever
                    let offset = offset - 3;
                    let op = opposite_op(data.op);
                    stream.0.extend(pack::BhBi(op, 8, GOTO_W, offset));
                }
            }
            ir::Switch(data) => {
                let offset = info.offset(*pos, data.default);
                let pad = (!*pos) % 4;
                if data.is_table {
                    stream.u8(TABLESWITCH); for _ in 0..pad { stream.u8(0); }
                    stream.i32(offset); stream.i32(data.low); stream.i32(data.high);
                    for k in data.low...data.high {
                        let target = *data.jumps.get(&k).unwrap_or(&data.default);
                        stream.i32(info.offset(*pos, target));
                    }
                } else {
                    stream.u8(LOOKUPSWITCH); for _ in 0..pad { stream.u8(0); }
                    stream.i32(offset); stream.u32(data.jumps.len() as u32);
                    let mut items: Vec<_> = data.jumps.into_iter().collect();
                    items.sort();
                    for (k, target) in items {
                        stream.i32(k);
                        stream.i32(info.offset(*pos, target));
                    }
                }
            }
            _ => { stream.0.extend(ins.bytecode.unwrap()); }
        };
    }
    assert!(stream.0.len() as u32 == endpos);

    let execpts = irdata.excepts.into_iter().map(|(s, e, h, c)| {
        // There appears to be a bug in the JVM where in rare cases, it throws
        // the exception at the address of the instruction _before_ the instruction
        // that actually caused the exception, triggering the wrong handler
        // therefore we include the previous (IR) instruction too
        // Note that this cannot cause an overlap because in that case the previous
        // instruction would just be a label and hence not change anything
        let sind = lbl_to_vind[&s].saturating_sub(1);
        let soff = positions[sind];
        (soff as u16, info.getlbl(e) as u16, info.getlbl(h) as u16, c)
    }).collect();

    (stream.0, execpts)
}
