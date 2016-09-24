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

var dup = string([]byte{DUP})
var dup2 = string([]byte{DUP2})
var pop = string([]byte{POP})
var pop2 = string([]byte{POP2})

type visitorInterface interface {
	reset()
	visitReturn()
	visit(i int, instr *ir.Instruction)
}

func visitLinearCode(irdata *IRWriter, visitor visitorInterface) {
	// Visit linear sections of code, pessimistically treating all exception
	// handler ranges as jumps.
	except_level := 0
	for i := range irdata.Instructions {
		instr := &irdata.Instructions[i]
		lbl := instr.Label
		if lbl.Tag == ir.ESTART {
			except_level += 1
			visitor.reset()
		} else if lbl.Tag == ir.EEND {
			except_level -= 1
		}

		if except_level > 0 {
			continue
		}

		if irdata.IsTarget(lbl) || instr.IsJump() {
			visitor.reset()
		} else if !instr.Fallsthrough() {
			visitor.visitReturn()
		} else {
			visitor.visit(i, instr)
		}
	}
	util.Assert(except_level == 0)
}

type ConstInliner struct {
	uses         map[int]int
	notmultiused map[int]bool
	current      map[ir.RegKey]int
}

func newConstInliner() *ConstInliner {
	return &ConstInliner{make(map[int]int), make(map[int]bool), make(map[ir.RegKey]int)}
}

func (self *ConstInliner) reset() { self.current = make(map[ir.RegKey]int) }
func (self *ConstInliner) visitReturn() {
	for _, v := range self.current {
		self.notmultiused[v] = true
	}
	self.reset()
}
func (self *ConstInliner) visit(i int, instr *ir.Instruction) {
	if instr.Tag == ir.REGACCESS {
		key := instr.RegKey
		if instr.RegAccess.Store {
			if v, ok := self.current[key]; ok {
				self.notmultiused[v] = true
			}
			self.current[key] = i
		} else if v, ok := self.current[key]; ok {
			// if currently used 0, mark it used once
			// if used once already, mark it as multiused
			if _, ok := self.uses[v]; ok {
				delete(self.current, key)
			} else {
				self.uses[v] = i
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

	replace := make(map[int][]ir.Instruction)
	for i := range instrs[1:] {
		ins1 := &instrs[i]
		if visitor.notmultiused[i+1] && ins1.IsConstant() {
			replace[i] = nil
			replace[i+1] = nil
			if v, ok := visitor.uses[i+1]; ok {
				replace[v] = []ir.Instruction{*ins1}
			}
		}
	}

	irdata.ReplaceInstrs(replace)
}

type StoreLoadPruner struct {
	current map[ir.RegKey][2]int
	lastInd int
	last    *ir.RegAccess
	removed map[int]bool
}

func newStoreLoadPruner() *StoreLoadPruner {
	return &StoreLoadPruner{make(map[ir.RegKey][2]int), -1, nil, make(map[int]bool)}
}

func (self *StoreLoadPruner) reset() {
	self.current = make(map[ir.RegKey][2]int)
	self.last = nil
}
func (self *StoreLoadPruner) visitReturn() {
	for _, pair := range self.current {
		self.removed[pair[0]] = true
		self.removed[pair[1]] = true
	}
	self.reset()
}
func (self *StoreLoadPruner) visit(i int, instr *ir.Instruction) {
	if instr.Tag == ir.REGACCESS {
		key := instr.RegKey
		if instr.RegAccess.Store {
			if pair, ok := self.current[key]; ok {
				self.removed[pair[0]] = true
				self.removed[pair[1]] = true
				delete(self.current, key)
			}
			self.lastInd = i
			self.last = &instr.RegAccess
		} else {
			delete(self.current, key)
			if self.last != nil && self.last.RegKey == key {
				self.current[key] = [2]int{self.lastInd, i}
			}
			self.last = nil
		}
	} else if instr.Tag != ir.LABEL {
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

	replace := make(map[int][]ir.Instruction, len(visitor.removed))
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
	for i := range instrs {
		instr := &instrs[i]
		// if not linear section of bytecode, reset everything. Exceptions are ok
		// since they clear the stack, but jumps obviously aren't.
		if instr.IsJump() || irdata.IsTarget(instr.Label) {
			for _, v := range current {
				ranges = append(ranges, v)
			}
			current = map[ir.RegKey]*UseRange{}
		}

		if instr.Tag == ir.REGACCESS {
			key := instr.RegKey
			if !key.T.Wide() {
				if instr.RegAccess.Store {
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

		if instr.Tag == ir.LABEL {
			at_head = instr.Label.Tag == ir.DPOS
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

	replace := make(map[int][]ir.Instruction)
	for _, ur := range chosen {
		gen := genDups(len(ur.uses), 0)
		for i, pos := range ur.uses {
			ops := []ir.Instruction{}
			for _, bytecode := range gen[i] {
				ops = append(ops, ir.NewOther(bytecode))
			}

			// remember to include initial load!
			if pos == ur.start() {
				ops = append([]ir.Instruction{instrs[pos]}, ops...)
			}

			replace[pos] = ops
		}
	}

	irdata.ReplaceInstrs(replace)
}
