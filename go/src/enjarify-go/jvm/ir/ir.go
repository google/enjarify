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
	"enjarify-go/byteio"
	"enjarify-go/jvm/constants"
	"enjarify-go/jvm/cpool"
	"enjarify-go/jvm/errors"
	"enjarify-go/jvm/ops"
	"enjarify-go/jvm/scalars"
)

// IR representation roughly corresponding to JVM bytecode instructions. Note that these
// may correspond to more than one instruction in the actual bytecode generated but they
// are useful logical units for the internal optimization passes.
type Instruction interface {
	Fallsthrough() bool
	Targets() []uint32

	Bytecode() string
	HasBytecode() bool
}

type instruction struct {
	bytecode *string
}

func (self *instruction) Fallsthrough() bool { return true }
func (self *instruction) Targets() []uint32  { return nil }

func (self *instruction) HasBytecode() bool { return self.bytecode != nil }
func (self *instruction) Bytecode() string  { return *self.bytecode }
func (self *instruction) SetBytecode(s string) {
	self.bytecode = &s
}

func super() instruction { return instruction{} }
func new(bytes ...byte) instruction {
	bytecode := string(bytes)
	return instruction{&bytecode}
}

// Used to mark locations in the IR instructions for various purposes. These are
// seperate IR 'instructions' since the optimization passes may remove or replace
// the other instructions.
type Label struct {
	instruction
	Pos    uint32
	Haspos bool
}

func NewLabel(id uint32) Instruction {
	return &Label{new(), id, true}
}
func NewLabel_() *Label {
	return &Label{new(), 0, false}
}

type RegKey struct {
	Reg uint16
	T   scalars.T
}

func (a RegKey) Less(b RegKey) bool {
	return a.Reg < b.Reg || (a.Reg == b.Reg && a.T < b.T)
}

type RegAccess struct {
	instruction
	RegKey
	Store bool
}

func NewRegAccess(dreg uint16, st scalars.T, store bool) Instruction {
	return &RegAccess{super(), RegKey{dreg, st}, store}
}
func RawRegAccess(local int, st scalars.T, store bool) Instruction {
	new := NewRegAccess(0, st, store).(*RegAccess)
	new.CalcBytecode(local)
	return new
}

func (self *RegAccess) CalcBytecode(x int) {
	local := uint16(x)
	op_base := byte(ILOAD)
	if self.Store {
		op_base = ISTORE
	}

	switch {
	case local < 4:
		op_base += ILOAD_0 - ILOAD
		self.SetBytecode(byteio.B(op_base + byte(local) + ops.IlfdaOrd[self.T]*4))
	case local < 256:
		self.SetBytecode(byteio.BB(op_base+ops.IlfdaOrd[self.T], byte(local)))
	default:
		self.SetBytecode(byteio.BBH(WIDE, op_base+ops.IlfdaOrd[self.T], local))
	}
}

type PrimConstant struct {
	instruction
	scalars.T
	val  uint64
	wide bool
}

func NewPrimConstant(st scalars.T, val uint64, pool cpool.Pool) Instruction {
	self := PrimConstant{
		instruction{},
		st,
		constants.Normalize(st, val),
		st.Wide(),
	}

	// If pool is passed in, just grab an entry greedily, otherwise calculate
	// a sequence of bytecode to generate the constant
	if pool != nil {
		self.bytecode = constants.LookupOnly(st, self.val)
		if self.bytecode == nil {
			self.fromPool(pool)
		}
		if self.bytecode == nil {
			panic(&errors.ClassfileLimitExceeded{})
		}
	} else {
		self.SetBytecode(constants.Calc(st, self.val))
	}
	return &self
}

func (self *PrimConstant) CpoolKey() cpool.Pair {
	var tag byte
	switch self.T {
	case scalars.INT:
		tag = cpool.CONSTANT_Integer
	case scalars.FLOAT:
		tag = cpool.CONSTANT_Float
	case scalars.DOUBLE:
		tag = cpool.CONSTANT_Double
	case scalars.LONG:
		tag = cpool.CONSTANT_Long
	}
	return cpool.Pair{tag, cpool.Data{X: self.val}}
}

func (self *PrimConstant) fromPool(pool cpool.Pool) {
	if index, ok := pool.TryGet(self.CpoolKey()); ok {
		if self.T.Wide() {
			self.SetBytecode(byteio.BH(LDC2_W, index))
		} else if index >= 256 {
			self.SetBytecode(byteio.BH(LDC_W, index))
		} else {
			self.SetBytecode(byteio.BB(LDC, uint8(index)))
		}
	}
}

func (self *PrimConstant) FixWithPool(pool cpool.Pool) {
	if len(self.Bytecode()) > 2 {
		self.fromPool(pool)
	}
}

type OtherConstant struct {
	instruction
}

func NewOtherConstant(s string) Instruction {
	t := &OtherConstant{}
	t.SetBytecode(s)
	return t
}

type Other struct {
	instruction
}

func NewOther(bytes ...byte) Instruction {
	return &Other{new(bytes...)}
}

func NewOther_(s string) Instruction {
	t := &Other{}
	t.SetBytecode(s)
	return t
}
func (self *Other) Fallsthrough() bool {
	if len(*self.bytecode) != 1 {
		return true
	}
	c := (*self.bytecode)[0]
	return c != ATHROW && (c < IRETURN || c > RETURN)
}
