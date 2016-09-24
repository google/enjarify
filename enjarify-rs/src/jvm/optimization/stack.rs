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
use std::mem::swap;

// use strings::*;

use jvm::ir;
use jvm::ir::LabelId::*;
use jvm::writeir::IRWriter;

trait Visitor {
    fn reset(&mut self);
    fn visit_return(&mut self);
    fn visit(&mut self, i: usize, instr: &ir::JvmInstruction);
}
fn visit_linear_code<V: Visitor>(irdata: &IRWriter, visitor: &mut V) {
    // Visit linear sections of code, pessimistically treating all exception
    // handler ranges as jumps.

    let mut except_level = 0;
    for (i, instr) in irdata.instructions.iter().enumerate() {
        let lbl = instr.lbl();
        match lbl {
            Some(EStart(_)) => { except_level += 1; visitor.reset(); }
            Some(EEnd(_)) => { except_level -= 1; }
            _ => {}
        }

        if except_level > 0 { continue; }

        if irdata.is_target(lbl) || instr.is_jump() {
            visitor.reset();
        } else if !instr.fallsthrough() {
            visitor.visit_return();
        } else {
            visitor.visit(i, instr);
        }
    }
    assert!(except_level == 0);
}

#[derive(Default)]
struct ConstInliner {
    uses: HashMap<usize, usize>,
    notmultiused: HashSet<usize>,
    current: HashMap<ir::RegKey, usize>,
}
impl Visitor for ConstInliner {
    fn reset(&mut self) { self.current.clear(); }

    fn visit_return(&mut self) {
        for (_, val) in self.current.drain() {
            self.notmultiused.insert(val);
        }
    }

    fn visit(&mut self, i: usize, instr: &ir::JvmInstruction) {
        if let ir::RegAccess(ref data) = instr.sub {
            let key = data.key;
            if data.store {
                if let Some(existing) = self.current.get(&key) {
                    self.notmultiused.insert(*existing);
                }
                self.current.insert(key, i);
            } else if self.current.contains_key(&key) {
                // if currently used 0, mark it used once
                // if used once already, mark it as multiused
                let existing = self.current[&key];
                if self.uses.contains_key(&existing) {
                    self.current.remove(&key);
                } else {
                    self.uses.insert(existing, i);
                }
            }
        }
    }
}

pub fn inline_consts(irdata: &mut IRWriter) {
    // Inline constants which are only used once or not at all. This only covers
    // linear sections of code and pessimistically assumes everything is used
    // when it reaches a jump or exception range. Essentially, this means that
    // the value can only be considered unused if it is either overwritten by a
    // store or reaches a return or throw before any jumps.
    // As usual, assume no iinc.
    let mut visitor = ConstInliner::default();
    visit_linear_code(irdata, &mut visitor);

    let mut replace = HashMap::new();
    for (i, ins1) in irdata.instructions.iter().enumerate() {
        let i2 = i+1;
        if visitor.notmultiused.contains(&i2) && ins1.is_constant() {
            replace.insert(i, Vec::new());
            replace.insert(i2, Vec::new());
            if let Some(u) = visitor.uses.remove(&i2) {
                replace.insert(u, vec![ins1.clone()]);
            }
        }
    }
    irdata.replace_instrs(replace);
}

#[derive(Default)]
struct StoreLoadPruner {
    current: HashMap<ir::RegKey, (usize, usize)>,
    last: Option<(usize, ir::RegKey)>,
    removed: HashSet<usize>,
}
impl Visitor for StoreLoadPruner {
    fn reset(&mut self) { self.current.clear(); self.last = None; }

    fn visit_return(&mut self) {
        for (_, pair) in self.current.drain() {
            self.removed.insert(pair.0);
            self.removed.insert(pair.1);
        }
        self.last = None;
    }

    fn visit(&mut self, i: usize, instr: &ir::JvmInstruction) {
        if let ir::RegAccess(ref data) = instr.sub {
            let key = data.key;
            if data.store {
                if let Some(pair) = self.current.remove(&key) {
                    self.removed.insert(pair.0);
                    self.removed.insert(pair.1);
                }
                self.last = Some((i, key));
            } else {
                self.current.remove(&key);
                if let Some((lasti, lastkey)) = self.last {
                    if lastkey == key {
                        self.current.insert(key, (lasti, i));
                    }
                }
                self.last = None;
            }
        } else if instr.lbl().is_none() {
            self.last = None;
        }
    }
}

pub fn prune_store_loads(irdata: &mut IRWriter) {
    // Remove a store immediately followed by a load from the same register
    // (potentially with a label in between) if it can be proven that this
    // register isn't read again. As above, this only considers linear sections of code.
    // Must not be run before dup2ize!
    let mut visitor = StoreLoadPruner::default();
    visit_linear_code(irdata, &mut visitor);

    let replace = visitor.removed.into_iter().map(|k| (k, vec![])).collect();
    irdata.replace_instrs(replace);
}

pub struct GenDupIter{n: usize, i:usize, have:usize, needed:usize}
impl Iterator for GenDupIter {
    type Item = Vec<ir::JvmInstruction>;
    fn next(&mut self) -> Option<Self::Item> {
        let mut res = Vec::new(); // todo: remove indirection by using scratch space?
        if self.i < self.n {
            if self.have == 1 && self.needed >= 2 { res.push(ir::dup()); self.have += 1; }
            if self.have == 2 && self.needed >= 4 { res.push(ir::dup2()); self.have += 2; }
            self.have -= 1; self.needed -= 1;
            self.i += 1;
        } else {
            assert!(self.i == self.n);
            if self.have > self.needed {
                assert!(self.have == self.needed + 1);
                res.push(ir::pop());
            }
        }
        Some(res)
    }
}

// used by writeir too
pub fn gen_dups(needed: usize, need_after: usize) -> GenDupIter {
    // Generate a sequence of dup and dup2 instructions to duplicate the given
    // value. This keeps up to 4 copies of the value on the stack. Thanks to dup2
    // this asymptotically takes only half a byte per access.
    GenDupIter{n:needed, i:0, have:1, needed:needed + need_after}
}

#[derive(Default)]
struct UseRange(Vec<usize>);
impl UseRange {
    fn start(&self) -> usize { self.0[0] }
    fn end(&self) -> usize { self.0[self.0.len()-1] }

    fn subtract(&self, other: &Self, out: &mut Vec<Self>) {
        let (s, e) = (other.start(), other.end());
        let left: Vec<_> = self.0.iter().map(|&x| x).filter(|&x| x < s).collect();
        let right: Vec<_> = self.0.iter().map(|&x| x).filter(|&x| x > e).collect();
        if left.len() >= 2 {out.push(UseRange(left));}
        if right.len() >= 2 {out.push(UseRange(right));}
    }
}

pub fn dup2ize(irdata: &mut IRWriter) {
    // This optimization replaces narrow registers which are frequently read at
    // stack height 0 with a single read followed by the more efficient dup and
    // dup2 instructions. This asymptotically uses only half a byte per access.
    // For simplicity, instead of explicitly keeping track of which locations
    // have stack height 0, we take advantage of the invariant that ranges of code
    // corresponding to a single Dalvik instruction always begin with empty stack.
    // These can be recognized by labels with a non-None id.
    // This isn't true for move-result instructions, but in that case the range
    // won't begin with a register load so it doesn't matter.
    // Note that pruneStoreLoads breaks this invariant, so dup2ize must be run first.
    // Also, for simplicity, we only keep at most one such value on the stack at
    // a time (duplicated up to 4 times).
    let mut ranges = Vec::new();
    let mut current = HashMap::new();
    let mut at_head = false;
    for (i, instr) in irdata.instructions.iter().enumerate() {
        // if not linear section of bytecode, reset everything. Exceptions are ok
        // since they clear the stack, but jumps obviously aren't.
        let lbl = instr.lbl();
        if instr.is_jump() || irdata.is_target(lbl) {
            ranges.extend(current.drain().map(|(_, v)| v));
        }

        if let ir::RegAccess(ref data) = instr.sub {
            let key = data.key;
            if !key.1.is_wide() {
                if data.store {
                    if let Some(ur) = current.remove(&key) {
                        ranges.push(ur);
                    }
                } else if at_head {
                    current.entry(key).or_insert(UseRange::default()).0.push(i);
                }
            }
        }

        at_head = if let Some(DPos(_)) = lbl {true} else {false};
    }
    ranges.extend(current.drain().map(|(_, v)| v));

    let mut ranges: Vec<_> = ranges.into_iter().filter(|ref ur| ur.0.len() >= 2).collect();
    ranges.sort_by_key(|ur| (ur.0.len(), ur.start()));

    // Greedily choose a set of disjoint ranges to dup2ize.
    let mut chosen = Vec::new();
    while let Some(best) = ranges.pop() {
        let mut oldranges = Vec::new();
        swap(&mut ranges, &mut oldranges);
        for ur in oldranges.into_iter() {
            ur.subtract(&best, &mut ranges);
        }

        chosen.push(best);
        ranges.sort_by_key(|ur| (ur.0.len(), ur.start()));
    }

    let mut replace = HashMap::new();
    for ur in chosen.into_iter() {
        let mut gen = gen_dups(ur.0.len(), 0);
        let mut first = true;
        for pos in ur.0 {
            let mut ops = gen.next().unwrap();
            // remember to include initial load!
            if first { ops.insert(0, irdata.instructions[pos].clone()); first = false; }
            replace.insert(pos, ops);
        }
    }
    irdata.replace_instrs(replace);
}
