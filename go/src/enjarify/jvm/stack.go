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
	"enjarify/jvm/ir"
	"enjarify/util"
	"sort"
)

var dup = string([]byte{DUP})
var dup2 = string([]byte{DUP2})
var pop = string([]byte{POP})
var pop2 = string([]byte{POP2})

type visitorInterface interface {
	reset()
	visitReturn()
	visit(instr ir.Instruction)
}

func visitLinearCode(irdata *IRWriter, visitor visitorInterface) {
	// Visit linear sections of code, pessimistically treating all exception
	// handler ranges as jumps.
	except_level := 0
	for _, instr := range irdata.Instructions {
		if _, ok := irdata.except_starts[instr]; ok {
			except_level += 1
			visitor.reset()
		} else if _, ok := irdata.except_ends[instr]; ok {
			except_level -= 1
		}

		if except_level > 0 {
			continue
		}

		_, ok := irdata.jump_targets[instr]
		switch instr.(type) {
		case *ir.Goto, *ir.If, *ir.Switch:
			ok = true
		}

		if ok {
			visitor.reset()
		} else if !instr.Fallsthrough() {
			visitor.visitReturn()
		} else {
			visitor.visit(instr)
		}
	}
	util.Assert(except_level == 0)
}

type ConstInliner struct {
	uses         map[ir.Instruction]ir.Instruction
	notmultiused map[ir.Instruction]bool
	current      map[ir.RegKey]ir.Instruction
}

func newConstInliner() *ConstInliner {
	return &ConstInliner{make(map[ir.Instruction]ir.Instruction), make(map[ir.Instruction]bool), make(map[ir.RegKey]ir.Instruction)}
}

func (self *ConstInliner) reset() { self.current = make(map[ir.RegKey]ir.Instruction) }
func (self *ConstInliner) visitReturn() {
	for _, v := range self.current {
		self.notmultiused[v] = true
	}
	self.reset()
}
func (self *ConstInliner) visit(instr ir.Instruction) {
	if ins, ok := instr.(*ir.RegAccess); ok {
		key := ins.RegKey
		if ins.Store {
			if v, ok := self.current[key]; ok {
				self.notmultiused[v] = true
			}
			self.current[key] = instr
		} else if v, ok := self.current[key]; ok {
			// if currently used 0, mark it used once
			// if used once already, mark it as multiused
			if _, ok := self.uses[v]; ok {
				delete(self.current, key)
			} else {
				self.uses[v] = instr
			}
		}
	}
}

func InlineConsts(irdata *IRWriter) {
	// Inline constants which are only used once or not at all. This only covers
	// linear sections of code and pessimistically assumes everything is used
	// when it reaches a jump or exception range. Essentially, this means that
	// the value can only be considered unused if it is either overwritten by a
	// store or reaches a return or throw before any jumps.
	// As usual, assume no iinc.
	instrs := irdata.Instructions
	visitor := newConstInliner()
	visitLinearCode(irdata, visitor)

	replace := make(map[ir.Instruction][]ir.Instruction)
	for i, ins2 := range instrs[1:] {
		ins1 := instrs[i]

		ok := false
		switch ins1.(type) {
		case *ir.PrimConstant, *ir.OtherConstant:
			ok = true
		}
		if visitor.notmultiused[ins2] && ok {
			replace[ins1] = nil
			replace[ins2] = nil
			if v, ok := visitor.uses[ins2]; ok {
				replace[v] = []ir.Instruction{ins1}
			}
		}
	}

	irdata.ReplaceInstrs(replace)
}

type StoreLoadPruner struct {
	current map[ir.RegKey][2]ir.Instruction
	last    *ir.RegAccess
	removed map[ir.Instruction]bool
}

func newStoreLoadPruner() *StoreLoadPruner {
	return &StoreLoadPruner{make(map[ir.RegKey][2]ir.Instruction), nil, make(map[ir.Instruction]bool)}
}

func (self *StoreLoadPruner) reset() {
	self.current = make(map[ir.RegKey][2]ir.Instruction)
	self.last = nil
}
func (self *StoreLoadPruner) visitReturn() {
	for _, pair := range self.current {
		self.removed[pair[0]] = true
		self.removed[pair[1]] = true
	}
	self.reset()
}
func (self *StoreLoadPruner) visit(instr ir.Instruction) {
	if ins, ok := instr.(*ir.RegAccess); ok {
		key := ins.RegKey
		if ins.Store {
			if pair, ok := self.current[key]; ok {
				self.removed[pair[0]] = true
				self.removed[pair[1]] = true
				delete(self.current, key)
			}
			self.last = ins
		} else {
			delete(self.current, key)
			if self.last != nil && self.last.RegKey == key {
				self.current[key] = [2]ir.Instruction{self.last, instr}
			}
			self.last = nil
		}
	} else if _, ok := instr.(*ir.Label); !ok {
		self.last = nil
	}
}

func PruneStoreLoads(irdata *IRWriter) {
	// Remove a store immediately followed by a load from the same register
	// (potentially with a label in between) if it can be proven that this
	// register isn't read again. As above, this only considers linear sections of code.
	// Must not be run before dup2ize!
	visitor := newStoreLoadPruner()
	visitLinearCode(irdata, visitor)

	replace := make(map[ir.Instruction][]ir.Instruction)
	for k, _ := range visitor.removed {
		replace[k] = nil
	}
	irdata.ReplaceInstrs(replace)
}

// used by writeir too
func genDups(needed, needed_after int) [][]string {
	// Generate a sequence of dup and dup2 instructions to duplicate the given
	// value. This keeps up to 4 copies of the value on the stack. Thanks to dup2
	// this asymptotically takes only half a byte per access.
	results := make([][]string, needed+1)
	have := 1
	ele_count := needed
	needed += needed_after

	for i := 0; i < ele_count; i++ {
		cur := []string{}
		if have < needed {
			if have == 1 && needed >= 2 {
				cur = append(cur, dup)
				have += 1
			}
			if have == 2 && needed >= 4 {
				cur = append(cur, dup2)
				have += 2
			}
		}
		have -= 1
		needed -= 1
		results[i] = cur
	}

	// check if we have to pop at end
	cur := []string{}
	for ; needed < have; needed++ {
		cur = append(cur, pop)
	}
	results[ele_count] = cur
	return results
}

// Range of instruction indexes at which a given register is read (in linear code)
type UseRange struct {
	uses []int
}

func (self *UseRange) add(i int)  { self.uses = append(self.uses, i) }
func (self *UseRange) start() int { return self.uses[0] }
func (self *UseRange) end() int   { return self.uses[len(self.uses)-1] }
func (self *UseRange) subtract(other *UseRange) (results []*UseRange) {
	s, e := other.start(), other.end()
	left := make([]int, 0, len(self.uses))
	right := make([]int, 0, len(self.uses))
	for _, i := range self.uses {
		if i < s {
			left = append(left, i)
		}
		if i > e {
			right = append(right, i)
		}
	}

	if len(left) >= 2 {
		results = append(results, &UseRange{left})
	}
	if len(right) >= 2 {
		results = append(results, &UseRange{right})
	}
	return
}

type URSlice []*UseRange

func (p URSlice) Len() int      { return len(p) }
func (p URSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p URSlice) Less(i, j int) bool {
	x, y := p[i], p[j]
	return len(x.uses) < len(y.uses) || (len(x.uses) == len(y.uses) && x.uses[0] < y.uses[0])
}

func (p URSlice) Sort() URSlice {
	sort.Sort(p)
	return p
}

func Dup2ize(irdata *IRWriter) {
	instrs := irdata.Instructions

	ranges := URSlice{}
	current := map[ir.RegKey]*UseRange{}
	at_head := false
	for i, instr := range instrs {
		// if not linear section of bytecode, reset everything. Exceptions are ok
		// since they clear the stack, but jumps obviously aren't.
		_, ok := irdata.jump_targets[instr]
		switch instr.(type) {
		case *ir.If, *ir.Switch:
			ok = true
		}

		if ok {
			for _, v := range current {
				ranges = append(ranges, v)
			}
			current = map[ir.RegKey]*UseRange{}
		}

		if ins, ok := instr.(*ir.RegAccess); ok {
			key := ins.RegKey
			if !key.T.Wide() {
				if ins.Store {
					if v, ok := current[key]; ok {
						ranges = append(ranges, v)
						delete(current, key)
					}
				} else if at_head {
					if _, ok := current[key]; !ok {
						current[key] = &UseRange{}
					}
					current[key].add(i)
				}
			}
		}

		if ins, ok := instr.(*ir.Label); ok {
			at_head = ins.Haspos
		} else {
			at_head = false
		}
	}

	for _, v := range current {
		ranges = append(ranges, v)
	}

	ranges2 := make(URSlice, 0, len(ranges))
	for _, ur := range ranges {
		if len(ur.uses) >= 2 {
			ranges2 = append(ranges2, ur)
		}
	}
	ranges = ranges2.Sort()

	// Greedily choose a set of disjoint ranges to dup2ize.
	chosen := URSlice{}
	for len(ranges) > 0 {
		best := ranges[len(ranges)-1]
		chosen = append(chosen, best)
		newranges := URSlice{}
		for _, ur := range ranges[:len(ranges)-1] {
			newranges = append(newranges, ur.subtract(best)...)
		}
		ranges = newranges.Sort()
	}

	replace := make(map[ir.Instruction][]ir.Instruction)
	for _, ur := range chosen {
		gen := genDups(len(ur.uses), 0)
		for i, pos := range ur.uses {
			ops := []ir.Instruction{}
			for _, bytecode := range gen[i] {
				ops = append(ops, ir.NewOther_(bytecode))
			}

			// remember to include initial load!
			if pos == ur.start() {
				ops = append([]ir.Instruction{instrs[pos]}, ops...)
			}

			replace[instrs[pos]] = ops
		}
	}

	irdata.ReplaceInstrs(replace)
}
