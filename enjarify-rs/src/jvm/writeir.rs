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
use std::collections::{HashMap, HashSet};
use std::hash::Hash;
use std::mem::swap;

use dex;
use dalvik::DalvikType as dt;
use flags;
use typeinference::inference::do_inference;
use typeinference::scalar;
use super::constantpool::ConstantPool;
use super::ir;
use super::ir::LabelId::*;
use super::irbuilder::write_instruction;
use super::optimization::options::Options;

pub struct IRWriter {
    pub method_idx: u32,
    pub instructions: Vec<ir::JvmInstruction>,
    pub target_pred_counts: HashMap<ir::LabelId, u32>,
    pub excepts: Vec<(ir::LabelId, ir::LabelId, ir::LabelId, u16)>,

    pub initial_args: Vec<ir::RegKey>,
    pub numregs: Option<u16>, // will be set once registers are allocated (see registers.rs)
}
impl IRWriter {
    pub fn is_target(&self, target: Option<ir::LabelId>) -> bool {
        match target { Some(target) => self.target_pred_counts.contains_key(&target), None => false }
    }

    pub fn replace_instrs(&mut self, mut replace: HashMap<usize, Vec<ir::JvmInstruction>>) {
        if replace.is_empty() { return; }
        let mut old_instrs = Vec::with_capacity(self.instructions.len());
        swap(&mut self.instructions, &mut old_instrs);

        for (i, instr) in old_instrs.into_iter().enumerate() {
            if let Some(replacement) = replace.remove(&i) {
                self.instructions.extend(replacement);
            } else {
                self.instructions.push(instr);
            }
        }
        assert!(replace.is_empty());
    }

    pub fn upper_bound(&self) -> usize { self.instructions.iter().map(|ref ins| ins.upper_bound()).sum() }
}

fn increment<K: Eq + Hash>(map: &mut HashMap<K, u32>, key: K) {
    *map.entry(key).or_insert(0) += 1;
}

pub fn write_bytecode<'b, 'a>(pool: &'b mut (ConstantPool<'a> + 'a), method: &dex::Method<'a>, code: &dex::CodeItem<'a>, opts: Options) -> IRWriter {
    let dex = method.dex;

    let instr_d = {
        let mut instr_d = HashMap::with_capacity(code.bytecode.len());
        for instr in &code.bytecode { instr_d.insert(instr.pos, instr); }
        instr_d
    };
    // for instr in &code.bytecode {println!("ins {} {:?}", instr.pos, instr);}

    let (types, all_handlers) = do_inference(method, code, &instr_d);
    let instructions = {
        // Find places where we will need a redirect because target is not a MoveException
        let mut exception_redirects = HashSet::new();
        for (_, handlers) in &all_handlers {
            for item in handlers {
                if instr_d[&item.target].typ != dt::MoveResult {
                    exception_redirects.insert(item.target);
                }
            }
        }

        let blockiter = code.bytecode.iter()
            .filter(|&instr| types.contains_key(&instr.pos)) // skip unreachable instructions
            .map(|instr| write_instruction(
                &mut *pool, method, opts, dex, instr, &types[&instr.pos], &instr_d,
                all_handlers.contains_key(&instr.pos)
            ));

        // now flatten the instructions
        let mut instructions: Vec<ir::JvmInstruction> = Vec::with_capacity(code.bytecode.len() * 2);
        for (pos, block_instructions) in blockiter {
            if exception_redirects.contains(&pos) {
                // check if we can put handler pop in front of block
                // if not, leave it in dict to be redirected later
                let ft = instructions.last().map_or(true, |instr| instr.fallsthrough());
                if !ft {
                    instructions.push(ir::label(EHandler(pos)));
                    instructions.push(ir::pop());
                    exception_redirects.remove(&pos);
                }
            }
            // now add instructions for actual block
            instructions.extend(block_instructions);
        }

        // exception handler pops that couldn't be placed inline
        // in this case, just put them at the end with a goto back to the handler
        let mut redirects_needed: Vec<_> = exception_redirects.into_iter().collect();
        redirects_needed.sort();
        for target in redirects_needed {
            instructions.push(ir::label(EHandler(target)));
            instructions.push(ir::pop());
            instructions.push(ir::goto(target));
        }
        instructions
    };

    let mut target_pred_counts = HashMap::new();
    let mut excepts = Vec::new();
    for instr in &code.bytecode {
        if !types.contains_key(&instr.pos) { continue; }
        if let Some(items) = all_handlers.get(&instr.pos) {
            let start = EStart(instr.pos);
            let end = EEnd(instr.pos);

            for item in items {
                // If handler doesn't use the caught exception, we need to redirect to a pop instead
                let target_lbl = match instr_d[&item.target].typ {
                    dt::MoveResult => DPos(item.target),
                    _ => EHandler(item.target),
                };
                increment(&mut target_pred_counts, target_lbl);
                let jctype = if item.ctype == b"java/lang/Throwable" {0} else {pool.class(item.ctype)};
                excepts.push((start, end, target_lbl, jctype));
            }
        }
    }

    // find jump targets (in addition to exception handler targets)
    for instr in &instructions {
        for target in instr.targets() {
            increment(&mut target_pred_counts, DPos(target));
        }
    }

    let isstatic = method.access as u16 & flags::ACC_STATIC != 0;
    let arg_descs = method.id.spaced_param_types(isstatic);
    let regoff = code.nregs - arg_descs.len() as u16;
    let mut iargs = Vec::with_capacity(arg_descs.len());
    for (i, optdesc) in arg_descs.iter().enumerate() {
        iargs.push(match *optdesc {
            Some(desc) => (regoff + i as u16, scalar::T::from_desc(desc)),
            None => ir::INVALID_KEY
        });
    }


    IRWriter{
        method_idx: method.id.method_idx,
        instructions: instructions,
        target_pred_counts: target_pred_counts,
        excepts: excepts,

        initial_args: iargs,
        numregs: None,
    }
}
