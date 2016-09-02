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
package ir

import (
	"encoding/binary"
	"enjarify/byteio"
	"enjarify/util"
)

type lazyJumpBase struct {
	self Instruction
	instruction
	target uint32

	Wide bool
}

func newljb(target uint32) lazyJumpBase {
	return lazyJumpBase{target: target}
}

func (self *lazyJumpBase) Targets() []uint32 { return []uint32{self.target} }
func (self *lazyJumpBase) WidenIfNecessary(labels map[uint32]*Label, posd map[Instruction]int32) bool {
	_, ok := posd[self.self]
	util.Assert(ok)

	offset := posd[labels[self.target]] - posd[self.self]
	if !(-32768 <= offset && offset < 32768) {
		self.Wide = true
		return true
	}
	return false
}

// For upcasting
type LazyJump interface {
	X() *lazyJumpBase
}

func (self *lazyJumpBase) X() *lazyJumpBase { return self }

type Goto struct {
	lazyJumpBase
}

func NewGoto(target uint32) Instruction {
	ins := &Goto{newljb(target)}
	ins.self = ins
	return ins
}
func (self *Goto) Fallsthrough() bool { return false }
func (self *Goto) Max() int32         { return 5 }
func (self *Goto) Min() int32 {
	if self.Wide {
		return 5
	}
	return 3
}

func (self *Goto) CalcBytecode(posd map[Instruction]int32, labels map[uint32]*Label) {
	offset := posd[labels[self.target]] - posd[self]
	if self.Wide {
		self.SetBytecode(byteio.Bi(GOTO_W, offset))
	} else {
		util.Assert(int32(int16(offset)) == offset)
		self.SetBytecode(byteio.Bh(GOTO, int16(offset)))
	}
}

type If struct {
	lazyJumpBase
	op byte
}

func NewIf(op byte, target uint32) Instruction {
	ins := &If{newljb(target), op}
	ins.self = ins
	return ins
}
func (self *If) Max() int32 { return 8 }
func (self *If) Min() int32 {
	if self.Wide {
		return 8
	}
	return 3
}

var ifOpposite = map[byte]byte{
	IFEQ:      IFNE,
	IFNE:      IFEQ,
	IFLT:      IFGE,
	IFGE:      IFLT,
	IFGT:      IFLE,
	IFLE:      IFGT,
	IF_ICMPEQ: IF_ICMPNE,
	IF_ICMPNE: IF_ICMPEQ,
	IF_ICMPLT: IF_ICMPGE,
	IF_ICMPGE: IF_ICMPLT,
	IF_ICMPGT: IF_ICMPLE,
	IF_ICMPLE: IF_ICMPGT,
	IFNULL:    IFNONNULL,
	IFNONNULL: IFNULL,
	IF_ACMPEQ: IF_ACMPNE,
	IF_ACMPNE: IF_ACMPEQ,
}

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
func (self *If) CalcBytecode(posd map[Instruction]int32, labels map[uint32]*Label) {
	if self.Wide {
		offset := posd[labels[self.target]] - posd[self] - 3
		self.SetBytecode(byteio.BhBi(ifOpposite[self.op], 8, GOTO_W, offset))
	} else {
		offset := posd[labels[self.target]] - posd[self]
		self.SetBytecode(byteio.Bh(self.op, int16(offset)))
	}
}

type Switch struct {
	instruction

	def       uint32
	jumps     map[int32]uint32
	low, high int32
	istable   bool
	NoPadSize int32
}

func NewSwitch(def uint32, jumps map[int32]uint32) Instruction {
	min := int32((1 << 31) - 1)
	max := int32(-(1 << 31))
	for k, _ := range jumps {
		if k < min {
			min = k
		}
		if k > max {
			max = k
		}
	}
	table_count := int64(max) - int64(min) + 1
	table_size := 4 * (int64(table_count) + 1)
	jump_size := 8 * int64(len(jumps))
	size := int32(jump_size)
	if jump_size > table_size {
		size = int32(table_size)
	}
	return &Switch{
		super(),
		def,
		jumps,
		min,
		max,
		jump_size > table_size,
		9 + size,
	}
}

func (self *Switch) Fallsthrough() bool { return false }
func (self *Switch) Max() int32         { return self.NoPadSize + 3 }

func (self *Switch) CalcBytecode(posd map[Instruction]int32, labels map[uint32]*Label) {
	pos := posd[self]
	offset := posd[labels[self.def]] - pos
	pad := uint32(-pos-1) % 4

	writer := &byteio.Writer{Endianess: binary.BigEndian}
	if self.istable {
		writer.U8(TABLESWITCH)
	} else {
		writer.U8(LOOKUPSWITCH)
	}

	for i := 0; i < int(pad); i++ {
		writer.U8(0)
	}

	if self.istable {
		writer.S32(offset)
		writer.S32(self.low)
		writer.S32(self.high)

		for k := self.low; k <= self.high; k++ {
			target, ok := self.jumps[k]
			if !ok {
				target = self.def
			}
			writer.S32(posd[labels[target]] - pos)
		}
	} else {
		writer.S32(offset)
		writer.U32(uint32(len(self.jumps)))
		for _, k := range keys1(self.jumps).Sort() {
			writer.S32(k)
			writer.S32(posd[labels[self.jumps[k]]] - pos)
		}
	}

	self.SetBytecode(writer.String())
}

func (self *Switch) Targets() []uint32 {
	set := make(map[uint32]bool)
	for _, v := range self.jumps {
		set[v] = true
	}
	return append(keys2(set).Sort(), self.def)
}
