// Copyright 2015 Google Inc. All Rights Reserved.
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
package jvm

import (
	"enjarify-go/jvm/ir"
	"enjarify-go/util"
	"sort"
)

// As usual, assume no iincs

// A set of registers that currently are copies of each other.
type copySet struct {
	root ir.RegKey
	set  map[ir.RegKey]bool
	q    []ir.RegKey // keep track of insertion order in case root is overwritten
}

func newCopySet(key ir.RegKey) *copySet {
	return &copySet{key, map[ir.RegKey]bool{key: true}, []ir.RegKey{key}}
}
func (self *copySet) add(key ir.RegKey) {
	util.Assert(len(self.set) > 0)
	self.set[key] = true
	self.q = append(self.q, key)
}
func (self *copySet) remove(key ir.RegKey) {
	delete(self.set, key)
	// Heuristic - use oldest element still in set as new root
	for len(self.q) > 0 && !self.set[self.root] {
		self.root = self.q[0]
		self.q = self.q[1:]
	}
}
func (self *copySet) copy() *copySet {
	new := newCopySet(self.root)
	new.set = make(map[ir.RegKey]bool)
	for k, v := range self.set {
		new.set[k] = v
	}
	new.q = make([]ir.RegKey, 0, len(self.q))
	copy(new.q, self.q)
	return new
}

// Map registers to CopySets
type copySetsMap struct {
	lookup map[ir.RegKey]*copySet
}

func newCopySetsMap() *copySetsMap { return &copySetsMap{make(map[ir.RegKey]*copySet)} }

func (self *copySetsMap) get(key ir.RegKey) *copySet {
	if v, ok := self.lookup[key]; !ok {
		new := newCopySet(key)
		self.lookup[key] = new
		return new
	} else {
		return v
	}
}

func (self *copySetsMap) clobber(key ir.RegKey) {
	self.get(key).remove(key)
	delete(self.lookup, key)
}

func (self *copySetsMap) move(dest, src ir.RegKey) bool {
	// return false if the corresponding instructions should be removed
	s_set := self.get(src)
	d_set := self.get(dest)

	if s_set == d_set {
		// src and dest are copies of same value, so we can remove
		return false
	}
	d_set.remove(dest)
	s_set.add(dest)
	self.lookup[dest] = s_set
	return true
}
func (self *copySetsMap) load(key ir.RegKey) ir.RegKey {
	return self.get(key).root
}
func (self *copySetsMap) copy() *copySetsMap {
	copies := make(map[*copySet]*copySet)
	new := newCopySetsMap()
	for k, v := range self.lookup {
		if copies[v] == nil {
			copies[v] = v.copy()
		}
		new.lookup[k] = copies[v]
	}
	return new
}

func CopyPropagation(irdata *IRWriter) {
	instrs := irdata.Instructions

	replace := make(map[ir.Instruction][]ir.Instruction)
	single_pred_infos := make(map[ir.Instruction]*copySetsMap)
	prev := ir.Instruction(nil)

	current := newCopySetsMap()
	for _, instr := range instrs {
		// reset all info when control flow is merged
		if irdata.jump_targets[instr] {
			// try to use info if this was a single predecessor forward jump
			if prev != nil && !prev.Fallsthrough() && irdata.target_pred_counts[instr] == 1 {
				current = single_pred_infos[instr]
				if current == nil {
					current = newCopySetsMap()
				}
			} else {
				current = newCopySetsMap()
			}
		} else if ins, ok := instr.(*ir.RegAccess); ok {
			key := ins.RegKey
			if ins.Store {
				// check if previous instr was a load
				if prev, ok := prev.(*ir.RegAccess); ok && !prev.Store {
					if !current.move(key, prev.RegKey) {
						replace[prev] = nil
						replace[instr] = nil
					}
				} else {
					current.clobber(key)
				}
			} else {
				root_key := current.load(key)
				if key != root_key {
					_, ok := replace[instr]
					util.Assert(!ok)
					// replace with load from root register instead
					replace[instr] = []ir.Instruction{ir.NewRegAccess(root_key.Reg, root_key.T, false)}
				}
			}
		} else {
			for _, target := range instr.Targets() {
				label := irdata.Labels[target]
				if irdata.target_pred_counts[label] == 1 {
					single_pred_infos[label] = current.copy()
				}
			}
		}

		prev = instr
	}

	irdata.ReplaceInstrs(replace)
}

func isRemoveable(instr ir.Instruction) bool {
	// can remove if load or const since we know there are no side effects
	// note - instr may be nil
	switch instr := instr.(type) {
	case *ir.RegAccess:
		return !instr.Store
	case *ir.PrimConstant, *ir.OtherConstant:
		return true
	default:
		return false
	}
}

func RemoveUnusedRegisters(irdata *IRWriter) {
	// Remove stores to registers that are not read from anywhere in the method
	instrs := irdata.Instructions

	used := make(map[ir.RegKey]bool)
	for _, instr := range instrs {
		if ins, ok := instr.(*ir.RegAccess); ok && !ins.Store {
			used[ins.RegKey] = true
		}
	}

	replace := make(map[ir.Instruction][]ir.Instruction)
	prev := ir.Instruction(nil)
	for _, instr := range instrs {
		if ins, ok := instr.(*ir.RegAccess); ok && !used[ins.RegKey] {
			util.Assert(ins.Store)
			// if prev instruction is load or const, just remove it and the store
			// otherwise, replace the store with a pop
			if isRemoveable(prev) {
				replace[prev] = nil
				replace[instr] = nil
			} else {
				if ins.T.Wide() {
					replace[instr] = []ir.Instruction{ir.NewOther(POP2)}
				} else {
					replace[instr] = []ir.Instruction{ir.NewOther(POP)}
				}
			}

		}
		prev = instr
	}
	irdata.ReplaceInstrs(replace)
}

// Allocate registers to JVM registers on a first come, first serve basis
// For simplicity, parameter registers are preserved as is
func SimpleAllocateRegisters(irdata *IRWriter) {
	instrs := irdata.Instructions
	regmap := map[ir.RegKey]int{}
	for i, v := range irdata.initial_args {
		regmap[v] = i
	}
	next := len(irdata.initial_args)

	for _, instr := range instrs {
		if instr, ok := instr.(*ir.RegAccess); ok {
			if _, ok := regmap[instr.RegKey]; !ok {
				regmap[instr.RegKey] = next
				next++
				if instr.T.Wide() {
					next++
				}
			}
			_, ok = regmap[instr.RegKey]
			util.Assert(ok)
			instr.CalcBytecode(regmap[instr.RegKey])
		}
	}
	irdata.numregs = uint16(next)
}

type TempSlice struct {
	regs       []ir.RegKey
	use_counts map[ir.RegKey]int
}

func (p TempSlice) Len() int { return len(p.regs) }
func (p TempSlice) Less(i, j int) bool {
	k1, k2 := p.regs[i], p.regs[j]
	u1 := p.use_counts[k1]
	u2 := p.use_counts[k2]
	return u1 > u2 || (u1 == u2 && k1.Less(k2))
}
func (p TempSlice) Swap(i, j int) { p.regs[i], p.regs[j] = p.regs[j], p.regs[i] }

func (p TempSlice) Sort() []ir.RegKey {
	sort.Sort(p)
	return p.regs
}

// Sort registers by number of uses so that more frequently used registers will
// end up in slots 0-3 or 4-255 and benefit from the shorter instruction forms
// For simplicity, parameter registers are still preserved as is with one exception
func SortAllocateRegisters(irdata *IRWriter) {
	NONE := ir.RegKey{}
	instrs := irdata.Instructions

	use_counts := make(map[ir.RegKey]int)
	for _, instr := range instrs {
		if ins, ok := instr.(*ir.RegAccess); ok {
			use_counts[ins.RegKey] += 1
		}
	}

	regs := append([]ir.RegKey(nil), irdata.initial_args...)

	keys := []ir.RegKey{}
	for k, _ := range use_counts {
		keys = append(keys, k)
	}
	for _, key := range (TempSlice{keys, use_counts}.Sort()) {
		// If key is a param, it was already added at the beginning
		isparam := false
		for _, param := range irdata.initial_args {
			if param == key {
				isparam = true
				break
			}
		}

		if !isparam {
			regs = append(regs, key)
			if key.T.Wide() {
				regs = append(regs, NONE)
			}
		}
	}

	// Sometimes the non-param regsisters are used more times than the param registers
	// and it is beneificial to swap them (which requires inserting code at the
	// beginning of the method to move the value if the param is not unused)
	// This is very complicated to do in general, so the following code only does
	// this in one specific circumstance which should nevertheless be sufficient
	// to capture the majority of the benefit
	// Specificially, it only swaps at most one register, and only in the case that
	// it is nonwide and there is a nonwide parameter in the first 4 slots that
	// it can be swapped with. Also, it doesn't bother to check if param is unused.
	candidate_i := len(irdata.initial_args)
	if candidate_i < 4 {
		candidate_i = 4
	}
	// make sure candidate is valid, nonwide register
	if len(regs) > candidate_i && regs[candidate_i] != NONE {
		candidate := regs[candidate_i]
		if !candidate.T.Wide() && use_counts[candidate] >= 3 {

			for i := 0; i < 4 && i < len(irdata.initial_args); i++ {
				// make sure target is not wide
				if regs[i] == NONE || regs[i+1] == NONE {
					continue
				}

				target := regs[i]
				if use_counts[candidate] > use_counts[target]+3 {
					// swap register assignments
					regs[i], regs[candidate_i] = candidate, target
					// add move instructions at beginning of method
					load := ir.RawRegAccess(i, target.T, false)
					store := ir.NewRegAccess(target.Reg, target.T, true)
					instrs = append([]ir.Instruction{load, store}, instrs...)
					irdata.Instructions = instrs
					break
				}
			}
		}
	}

	// Now generate bytecode from the selected register allocations
	irdata.numregs = uint16(len(regs))
	regmap := map[ir.RegKey]int{}
	for i, v := range regs {
		if v != NONE {
			regmap[v] = i
		}
	}

	for _, instr := range instrs {
		if ins, ok := instr.(*ir.RegAccess); ok && !instr.HasBytecode() {
			ins.CalcBytecode(regmap[ins.RegKey])
		}
	}
}
