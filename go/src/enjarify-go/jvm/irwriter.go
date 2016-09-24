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
	"enjarify-go/byteio"
	"enjarify-go/dex"
	"enjarify-go/jvm/cpool"
	"enjarify-go/jvm/ir"
	"enjarify-go/jvm/scalars"
)

type exceptInfo struct {
	start, end ir.Instruction
	target     ir.Label
	ctype      uint16
}

type IRWriter struct {
	pool   cpool.Pool
	method dex.Method
	types  map[uint32]TypeInfo
	opts   Options

	iblocks map[uint32]*irBlock

	Instructions []ir.Instruction
	excepts      []exceptInfo

	initial_args []ir.RegKey

	exception_redirects map[uint32]ir.Instruction
	target_pred_counts  map[ir.Label]uint32

	numregs uint16 // will be set once registers are allocated (see registers.py)
}

func newIRWriter(pool cpool.Pool, method dex.Method, types map[uint32]TypeInfo, opts Options) *IRWriter {
	return &IRWriter{
		pool,
		method,
		types,
		opts,

		make(map[uint32]*irBlock),

		nil,
		nil,

		nil,

		make(map[uint32]ir.Instruction),
		make(map[ir.Label]uint32),

		0xDEAD,
	}
}

func (self *IRWriter) calcInitialArgs(nregs uint16, scalar_ptypes []scalars.T) {
	regoff := nregs - uint16(len(scalar_ptypes))
	args := make([]ir.RegKey, len(scalar_ptypes))
	for i, st := range scalar_ptypes {
		if st == scalars.INVALID {
			args[i] = ir.RegKey{}
		} else {
			args[i] = ir.RegKey{uint16(i) + regoff, st}
		}
	}
	self.initial_args = args
}

func (self *IRWriter) addExceptionRedirect(target uint32) ir.Label {
	if val, ok := self.exception_redirects[target]; ok {
		return val.Label
	}
	self.exception_redirects[target] = ir.NewLabel(ir.EHANDLER, target)
	return self.exception_redirects[target].Label
}

func (self *IRWriter) createBlock(pos uint32) *irBlock {
	block := newIRBlock(self, pos)
	self.iblocks[pos] = block
	return block
}
func (self *IRWriter) flatten() {
	size := 3 * len(self.exception_redirects)
	for _, block := range self.iblocks {
		size += len(block.instructions)
	}

	instrs := make([]ir.Instruction, 0, size)
	for _, pos := range keys1(self.iblocks).Sort() {
		if _, ok := self.exception_redirects[pos]; ok {
			// check if we can put handler pop in front of block
			if len(instrs) > 0 && !instrs[len(instrs)-1].Fallsthrough() {
				instrs = append(instrs, self.exception_redirects[pos])
				delete(self.exception_redirects, pos)
				instrs = append(instrs, ir.NewOther(byteio.B(POP)))
			} // if not, leave it in dict to be redirected later
		}
		// now add instructions for actual block
		instrs = append(instrs, self.iblocks[pos].instructions...)
	}

	// exception handler pops that couldn't be placed inline
	// in this case, just put them at the end with a goto back to the handler
	for _, target := range keys2(self.exception_redirects).Sort() {
		instrs = append(instrs, self.exception_redirects[target])
		instrs = append(instrs, ir.NewOther((byteio.B(POP))))
		instrs = append(instrs, ir.NewGoto(target))
	}

	self.Instructions = instrs
	self.iblocks = nil
	self.exception_redirects = nil
}
func (self *IRWriter) ReplaceInstrs(replace map[int][]ir.Instruction) {
	if len(replace) == 0 {
		return
	}

	old := make([]ir.Instruction, 0, len(self.Instructions))
	self.Instructions, old = old, self.Instructions

	for i := range old {
		if replacement, ok := replace[i]; ok {
			self.Instructions = append(self.Instructions, replacement...)
		} else {
			self.Instructions = append(self.Instructions, old[i])
		}
	}
}
func (self *IRWriter) CalcUpperBound() int {
	// Get an uppper bound on the size of the bytecode
	size := 0
	for i := range self.Instructions {
		size += self.Instructions[i].UpperBound()
	}
	return size
}

func (self *IRWriter) IsTarget(target ir.Label) bool {
	_, ok := self.target_pred_counts[target]
	return ok
}
