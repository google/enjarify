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
use std::cell::{RefCell, RefMut};
use std::cmp::{max, min};
use std::collections::{HashMap, HashSet};
use std::collections::vec_deque::VecDeque;
use std::ops::Deref;
use std::ptr::null;
use std::rc::Rc;


use jvm::ir;
use jvm::writeir::IRWriter;

// Copy propagation - when one register is moved to another, keep track and replace
// all loads with loads from the original register (as long as it hasn't since been
// overwritten). Note that stores won't be removed, since they may still be needed
// in some cases, but if they are unused, they'll be removed in a subsequent pass
// As usual, assume no iincs

// A set of registers that currently are copies of each other.
#[derive(Clone)]
struct CopySet{
    root: ir::RegKey,
    set: HashSet<ir::RegKey>,
    q: VecDeque<ir::RegKey>, // keep track of insertion order in case root is overwritten
}
impl CopySet {
    fn new(key: ir::RegKey) -> CopySet {
        let mut t = CopySet{root: key, set: HashSet::new(), q: VecDeque::new()};
        t.set.insert(key);
        t
    }

    fn add(&mut self, key: ir::RegKey) {
        assert!(self.set.len() > 0);
        self.set.insert(key);
        self.q.push_back(key);
    }

    fn remove(&mut self, key: ir::RegKey) {
        self.set.remove(&key);
        // Heuristic - use oldest element still in set as new root
        while self.q.len() > 0 && !self.set.contains(&self.root) {
            self.root = self.q.pop_front().unwrap();
        }
    }
}

fn ptr(p: Option<&Rc<RefCell<CopySet>>>) -> *const CopySet {
    match p {
        Some(p) => p.deref().borrow().deref() as *const _,
        None => null(),
    }
}

// Map registers to CopySets
#[derive(Default)]
struct CopySetsMap(HashMap<ir::RegKey, Rc<RefCell<CopySet>>>);
impl CopySetsMap {
    fn get(&mut self, key: ir::RegKey) -> RefMut<CopySet> {
        self.0.entry(key).or_insert_with(|| Rc::new(RefCell::new(CopySet::new(key)))).borrow_mut()
    }

    fn load(&mut self, key: ir::RegKey) -> ir::RegKey { self.get(key).root }

    fn clobber(&mut self, key: ir::RegKey) {
        if let Some(v) = self.0.remove(&key) {
            v.borrow_mut().remove(key);
        }
    }

    fn move_(&mut self, src: ir::RegKey, dest: ir::RegKey) -> bool {
        // return false if the corresponding instructions should be removed
        let s_set = ptr(self.0.get(&src));
        let d_set = ptr(self.0.get(&dest));
        if !s_set.is_null() && s_set == d_set {
            // src and dest are copies of same value, so we can remove
            return false;
        }
        if !d_set.is_null() { self.get(dest).remove(dest); }
        self.get(src).add(dest);

        // copy src set over to dest
        let r = self.0.get(&src).unwrap().clone();
        self.0.insert(dest, r);
        true
    }

    fn clone(&self) -> CopySetsMap {
        // overriden to perform deep copy while maintaining aliases
        let mut copies = HashMap::new();
        let mut refs = HashMap::new();
        for (k, v) in &self.0 {
            let c = copies.entry(ptr(Some(&v))).or_insert_with(||
                Rc::new(RefCell::new(v.deref().borrow().clone()))
            );
            refs.insert(*k, c.clone());
        }
        CopySetsMap(refs)
    }
}


pub fn copy_propagation(irdata: &mut IRWriter) {
    let mut replace = HashMap::new();
    let mut single_pred_infos = HashMap::new();

    let mut current = CopySetsMap::default();
    let mut prev: (_,_,Option<ir::RAImpl>) = (0, true, None);

    for (i, instr) in irdata.instructions.iter().enumerate() {
        // reset all info when control flow is merged
        if irdata.is_target(instr.lbl()) {
            let lbl = instr.lbl().unwrap();
            // try to use info if this was a single predecessor forward jump
            let pred_counts = irdata.target_pred_counts[&lbl];
            if !prev.1 && pred_counts == 1 {
                current = single_pred_infos.remove(&lbl).unwrap_or(CopySetsMap::default());
            } else {
                current = CopySetsMap::default();
            }
        } else if let ir::RegAccess(ref data) = instr.sub {
            let key = data.key;
            if data.store {
                // check if previous instr was a load
                if let Some(data2) = prev.2.take() {
                    if !data2.store {
                        if !current.move_(data2.key, key) {
                            replace.insert(prev.0, Vec::new());
                            replace.insert(i, Vec::new());
                        }
                    } else { current.clobber(key); }
                } else {
                    current.clobber(key);
                }
            } else {
                let root_key = current.load(key);
                if key != root_key {
                    assert!(!replace.contains_key(&i));
                    // replace with load from root register instead
                    replace.insert(i, vec![ir::reg_access(root_key.0, root_key.1, false)]);
                }
            }
        } else {
            for target in instr.targets() {
                let lbl = ir::LabelId::DPos(target);
                if irdata.target_pred_counts[&lbl] == 1 {
                    single_pred_infos.insert(lbl, current.clone());
                }
            }
        }

        prev = (i, instr.fallsthrough(),
            if let ir::RegAccess(ref data) = instr.sub {Some(data.clone())} else {None}
        );
    }

    irdata.replace_instrs(replace);
}


pub fn remove_unused_registers(irdata: &mut IRWriter) {
    // Remove stores to registers that are not read from anywhere in the method
    let mut used = HashSet::new();
    for instr in irdata.instructions.iter() {
        if let ir::RegAccess(ref data) = instr.sub {
            if !data.store {
                used.insert(data.key);
            }
        }
    }

    let mut replace = HashMap::new();
    let mut prev_was_replaceable = false;
    for (i, instr) in irdata.instructions.iter().enumerate() {
        if let ir::RegAccess(ref data) = instr.sub {
            if !used.contains(&data.key) {
                assert!(data.store);

                // if prev instruction is load or const, just remove it and the store
                // otherwise, replace the store with a pop
                if prev_was_replaceable {
                    replace.insert(i-1, Vec::new());
                    replace.insert(i, Vec::new());
                } else {
                    let wide = data.key.1.is_wide();
                    replace.insert(i, vec![if wide {ir::pop2()} else {ir::pop()}]);
                }
            }

            prev_was_replaceable = !data.store; // loads are ok
        } else { prev_was_replaceable = instr.is_constant(); }
    }
    irdata.replace_instrs(replace);
}

// Allocate registers to JVM registers on a first come, first serve basis
// For simplicity, parameter registers are preserved as is
pub fn simple_allocate_registers(irdata: &mut IRWriter) {
    let mut regmap: HashMap<_,_> = irdata.initial_args.iter().enumerate().map(|(i, v)| (v, i as u16)).collect();
    let mut nextreg = irdata.initial_args.len() as u16;

    for instr in &mut irdata.instructions {
        if let ir::RegAccess(ref data) = instr.sub {
            let reg = *regmap.entry(&data.key).or_insert_with(|| {
                let t = nextreg;
                nextreg += 1;
                if data.key.1.is_wide() { nextreg += 1; }
                t
            });
            instr.bytecode = Some(data.calc_bytecode(reg));
        }
    }
    irdata.numregs = Some(nextreg);
}

// Sort registers by number of uses so that more frequently used registers will
// end up in slots 0-3 or 4-255 and benefit from the shorter instruction forms
// For simplicity, parameter registers are still preserved as is with one exception
pub fn sort_allocate_registers(irdata: &mut IRWriter) {
    let mut use_counts = HashMap::new();
    {
        for instr in &mut irdata.instructions {
            if let ir::RegAccess(ref data) = instr.sub {
                *use_counts.entry(data.key).or_insert(0) += 1;
            }
        }
    }

    let mut regs = irdata.initial_args.clone();
    let mut rest: Vec<_> = use_counts.keys().map(|&k| k).collect();
    rest.sort_by_key(|k| (!use_counts[k], *k)); // ! -> bitwise not -> sort decreasing
    for key in rest {
        // If key is a param, it was already added at the beginning
        if !irdata.initial_args.contains(&key) {
            regs.push(key);
            if key.1.is_wide() {
                regs.push(ir::INVALID_KEY);
            }
        }
    }

    // make sure any key we might query is in the dict
    for key in &irdata.initial_args { use_counts.entry(*key).or_insert(0); }

    // Sometimes the non-param regsisters are used more times than the param registers
    // and it is beneificial to swap them (which requires inserting code at the
    // beginning of the method to move the value if the param is not unused)
    // This is very complicated to do in general, so the following code only does
    // this in one specific circumstance which should nevertheless be sufficient
    // to capture the majority of the benefit
    // Specificially, it only swaps at most one register, and only in the case that
    // it is nonwide and there is a nonwide parameter in the first 4 slots that
    // it can be swapped with. Also, it doesn't bother to check if param is unused.
    let nargs = irdata.initial_args.len();
    let candidate_i = max(4, nargs);
    // make sure candidate is valid, nonwide register
    if candidate_i < regs.len() && regs[candidate_i] != ir::INVALID_KEY {
        let candidate = regs[candidate_i];
        if !candidate.1.is_wide() && use_counts[&candidate] >= 3 {
            for i in 0..min(4, nargs) {
                // make sure target is not wide
                if regs[i] == ir::INVALID_KEY || regs[i+1] == ir::INVALID_KEY { continue; }

                let target = regs[i];
                if use_counts[&candidate] > use_counts[&target] + 3 {
                    // swap register assignments
                    regs[i] = candidate;
                    regs[candidate_i] = target;
                    // add move instructions at beginning of method
                    let load = ir::raw_access(i as u16, target.1, false);
                    let store = ir::reg_access(target.0, target.1, true);
                    // todo - can these be inserted simulatenously to avoid extra shift?
                    irdata.instructions.insert(0, store);
                    irdata.instructions.insert(0, load);
                    // println!("moving arg {} -> {}", i, target.0);
                    break;
                }
            }
        }
    }
    // println!("regs {:?}", regs);

    // Now generate bytecode from the selected register allocations
    let regmap: HashMap<_,_> = regs.iter().enumerate().map(|(i, v)| (v, i as u16)).collect();
    for instr in &mut irdata.instructions {
        if instr.bytecode.is_none() {
            if let ir::RegAccess(ref data) = instr.sub {
                instr.bytecode = Some(data.calc_bytecode(regmap[&data.key]));
            }
        }
    }
    irdata.numregs = Some(regs.len() as u16);
}
