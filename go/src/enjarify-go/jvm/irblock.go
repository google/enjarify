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
	"enjarify-go/jvm/arrays"
	"enjarify-go/jvm/cpool"
	"enjarify-go/jvm/ir"
	"enjarify-go/jvm/ops"
	"enjarify-go/jvm/scalars"
)

// Code for converting dalvik bytecode to intermediate representation
// effectively this is just Java bytecode instructions with some abstractions for
// later optimization
var newArrayCodes = map[string]uint8{"[Z": 4, "[C": 5, "[F": 6, "[D": 7, "[B": 8, "[S": 9, "[I": 10, "[J": 11}

// For generating IR instructions corresponding to a single Dalvik instruction
type irBlock struct {
	type_data    TypeInfo
	Pool         cpool.Pool
	instructions []ir.Instruction
	delay_consts bool
}

func newIRBlock(parent *IRWriter, pos uint32) *irBlock {
	self := &irBlock{
		parent.types[pos],
		parent.pool,
		[]ir.Instruction{ir.NewLabel(pos)},
		parent.opts.DelayConsts,
	}
	return self
}

func (self *irBlock) other(bytecode string) {
	self.add(ir.NewOther_(bytecode))
}
func (self *irBlock) U8(op uint8) {
	self.add(ir.NewOther(op))
}
func (self *irBlock) U8U8(op, x uint8) {
	self.add(ir.NewOther(op, x))
}
func (self *irBlock) U8U16(op uint8, x uint16) {
	self.add(ir.NewOther_(byteio.BH(op, x)))
}

// wide non iinc
func (self *irBlock) U8U8U16(op, x uint8, y uint16) {
	self.add(ir.NewOther_(byteio.BBH(op, x, y)))
}

// invokeinterface
func (self *irBlock) U8U16U8U8(op uint8, x uint16, y, z uint8) {
	self.add(ir.NewOther_(byteio.BHBB(op, x, y, z)))
}
func (self *irBlock) add(jvm_instr ir.Instruction) {
	self.instructions = append(self.instructions, jvm_instr)
}

func (self *irBlock) Ldc(index uint16) {
	if index < 256 {
		self.add(ir.NewOtherConstant(byteio.BB(LDC, uint8(index))))
	} else {
		self.add(ir.NewOtherConstant(byteio.BH(LDC_W, index)))
	}
}

func (self *irBlock) Load(reg uint16, stype scalars.T) {
	// if we know the register to be 0/null, don't bother loading
	if self.type_data.at(reg) == arrays.NULL {
		self.Const(0, stype)
	} else {
		self.add(ir.NewRegAccess(reg, stype, false))
	}
}
func (self *irBlock) LoadAsCls(reg uint16, stype scalars.T, clsname string) {
	self.Load(reg, stype)
	if self.type_data.at(reg) != arrays.NULL {
		if stype == scalars.OBJ && self.type_data.taint(reg) {
			if clsname != "java/lang/Object" {
				self.U8U16(CHECKCAST, self.Pool.Class(clsname))
			}
		}
	}
}
func (self *irBlock) LoadDesc(reg uint16, stype scalars.T, desc string) {
	// remember to handle arrays
	if desc[0] == 'L' {
		desc = desc[1 : len(desc)-1]
	}
	self.LoadAsCls(reg, stype, desc)
}
func (self *irBlock) LoadAsArray(reg uint16) {
	if at := self.type_data.at(reg); at == arrays.NULL {
		self.ConstNull()
	} else {
		self.add(ir.NewRegAccess(reg, scalars.OBJ, false))
		if self.type_data.taint(reg) {
			if at == arrays.INVALID {
				// needs to be some type of object array, so just cast to Object[]
				self.U8U16(CHECKCAST, self.Pool.Class("[Ljava/lang/Object;"))
			} else {
				// note - will throw if actual type is boolean[] but there's not
				// much we can do in this case
				self.U8U16(CHECKCAST, self.Pool.Class(string(at)))

			}
		}
	}
}

func (self *irBlock) Store(reg uint16, stype scalars.T) {
	self.add(ir.NewRegAccess(reg, stype, true))
}

func (self *irBlock) Return_() {
	self.U8(RETURN)
}
func (self *irBlock) Return(stype scalars.T) {
	self.U8(IRETURN + ops.IlfdaOrd[stype])
}
func (self *irBlock) Const(val uint64, stype scalars.T) {
	if stype == scalars.OBJ {
		self.ConstNull()
	} else {
		// If constant pool is simple, assume we're in non-opt mode and only use
		// the constant pool for generating constants instead of calculating
		// bytecode sequences for them. If we're in opt mode, pass None for pool
		// to generate bytecode instead
		pool := self.Pool
		if self.delay_consts {
			pool = nil
		}
		self.add(ir.NewPrimConstant(stype, val, pool))
	}
}
func (self *irBlock) ConstNull() {
	self.add(ir.NewOtherConstant(byteio.B(ACONST_NULL)))
}

func (self *irBlock) FillArraySub(op byte, cbs []func(), pop bool) {
	needed_after := 0
	if !pop {
		needed_after++
	}

	gen := genDups(len(cbs), needed_after)
	for i, cb := range cbs {
		for _, bytecode := range gen[i] {
			self.other(bytecode)
		}
		self.Const(uint64(i), scalars.INT)
		cb()
		self.U8(op)
	}

	// may need to pop at end
	for _, bytecode := range gen[len(cbs)] {
		self.other(bytecode)
	}
}

func (self *irBlock) NewArray(desc string) {
	if code, ok := newArrayCodes[desc]; ok {
		self.U8U8(NEWARRAY, code)
	} else {
		// can be either multidim array or object array descriptor
		desc = desc[1:]
		if desc[0] == 'L' {
			desc = desc[1 : len(desc)-1]
		}
		self.U8U16(ANEWARRAY, self.Pool.Class(desc))
	}
}

func (self *irBlock) Cast(dex_ *dex.DexFile, reg uint16, index uint32) {
	self.Load(reg, scalars.OBJ)
	self.U8U16(CHECKCAST, self.Pool.Class(dex_.ClsType(index)))
	self.Store(reg, scalars.OBJ)
}
func (self *irBlock) Goto(target uint32) {
	self.add(ir.NewGoto(target))
}
func (self *irBlock) If(op byte, target uint32) {
	self.add(ir.NewIf(op, target))
}
func (self *irBlock) Switch(def uint32, jumps map[uint32]uint32) {
	jumps2 := make(map[int32]uint32)
	for k, v := range jumps {
		if v != def {
			jumps2[int32(k)] = v
		}
	}
	if len(jumps2) > 0 {
		self.add(ir.NewSwitch(def, jumps2))
	} else {
		self.Goto(def)
	}
}

func (self *irBlock) AddExceptLabels() (start_lbl, end_lbl ir.Instruction) {
	s_ind := 0
	e_ind := len(self.instructions)
	// assume only Other instructions can throw
	for s_ind < e_ind {
		if _, ok := self.instructions[s_ind].(*ir.Other); ok {
			break
		}
		s_ind += 1
	}
	for s_ind < e_ind {
		if _, ok := self.instructions[e_ind-1].(*ir.Other); ok {
			break
		}
		e_ind -= 1
	}

	start_lbl, end_lbl = ir.NewLabel_(), ir.NewLabel_()

	s := append(self.instructions, nil)
	copy(s[s_ind+1:], s[s_ind:])
	s[s_ind] = start_lbl
	s = append(s, nil)
	copy(s[e_ind+1+1:], s[e_ind+1:])
	s[e_ind+1] = end_lbl
	self.instructions = s
	return
}
