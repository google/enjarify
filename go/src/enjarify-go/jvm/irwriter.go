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
	"enjarify-go/dex"
	"enjarify-go/jvm/cpool"
	"enjarify-go/jvm/ir"
	"enjarify-go/jvm/scalars"
)

type exceptInfo struct {
	start, end, target ir.Instruction
	ctype              uint16
}

type IRWriter struct {
	pool   cpool.Pool
	method dex.Method
	types  map[uint32]TypeInfo
	opts   Options

	iblocks map[uint32]*irBlock

	Instructions []ir.Instruction
	excepts      []exceptInfo
	Labels       map[uint32]*ir.Label

	initial_args        []ir.RegKey
	exception_redirects map[uint32]*ir.Label

	except_starts, except_ends, jump_targets map[ir.Instruction]bool
	target_pred_counts                       map[ir.Instruction]uint32

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
		make(map[uint32]*ir.Label),

		nil,
		make(map[uint32]*ir.Label),

		make(map[ir.Instruction]bool),
		make(map[ir.Instruction]bool),
		make(map[ir.Instruction]bool),
		make(map[ir.Instruction]uint32),

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

func (self *IRWriter) addExceptionRedirect(target uint32) *ir.Label {
	if val, ok := self.exception_redirects[target]; ok {
		return val
	}
	self.exception_redirects[target] = ir.NewLabel_()
	return self.exception_redirects[target]
}

func (self *IRWriter) createBlock(pos uint32) *irBlock {
	block := newIRBlock(self, pos)
	self.iblocks[pos] = block
	self.Labels[pos] = block.instructions[0].(*ir.Label)
	return block
}
func (self *IRWriter) flatten() {
	instrs := self.Instructions
	// fmt.Printf("keys %v %v\n", keys1(self.iblocks).Sort(), keys2(self.exception_redirects).Sort())
	for _, pos := range keys1(self.iblocks).Sort() {
		if _, ok := self.exception_redirects[pos]; ok {
			// check if we can put handler pop in front of block
			if len(instrs) > 0 && !instrs[len(instrs)-1].Fallsthrough() {
				instrs = append(instrs, self.exception_redirects[pos])
				delete(self.exception_redirects, pos)
				instrs = append(instrs, ir.NewOther(POP))
			} // if not, leave it in dict to be redirected later
		}
		// now add instructions for actual block
		instrs = append(instrs, self.iblocks[pos].instructions...)
	}

	// exception handler pops that couldn't be placed inline
	// in this case, just put them at the end with a goto back to the handler
	for _, target := range keys2(self.exception_redirects).Sort() {
		instrs = append(instrs, self.exception_redirects[target])
		instrs = append(instrs, ir.NewOther(POP))
		instrs = append(instrs, ir.NewGoto(target))
	}

	self.Instructions = instrs
	self.iblocks = nil
	self.exception_redirects = nil
}
func (self *IRWriter) ReplaceInstrs(replace map[ir.Instruction][]ir.Instruction) {
	if len(replace) > 0 {
		instructions := make([]ir.Instruction, 0, len(self.Instructions))
		// for i, instr := range self.Instructions {
		for _, instr := range self.Instructions {
			if v, ok := replace[instr]; ok {
				// fmt.Printf("Replace[%d]: %T %p %v\n", i, instr, instr, instr)
				// fmt.Printf("-> %v\n", v)
				instructions = append(instructions, v...)
			} else {
				instructions = append(instructions, instr)
			}
		}

		// fmt.Printf("ReplaceInstrs m %v old %d new %d\n", self.method.MethodId, len(self.Instructions), len(instructions))
		self.Instructions = instructions
	} else {
		// fmt.Printf("ReplaceInstrs m %v -------\n", self.method.MethodId)
	}
}
func (self *IRWriter) CalcUpperBound() int32 {
	// Get an uppper bound on the size of the bytecode
	size := int32(0)
	for _, instr := range self.Instructions {
		if instr.HasBytecode() {
			size += int32(len(instr.Bytecode()))
		} else {
			size += instr.(interface {
				Max() int32
			}).Max()
		}
	}
	return size
}
