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
	"enjarify-go/util"
	"math"
)

// IR representation roughly corresponding to JVM bytecode instructions. Note that these
// may correspond to more than one instruction in the actual bytecode generated but they
// are useful logical units for the internal optimization passes.
type insTag uint8

const (
	INVALID_INS insTag = iota
	LABEL
	REGACCESS
	PRIMCONSTANT
	OTHERCONSTANT
	GOTO_TAG
	IF
	SWITCH
	OTHER
)

type Instruction struct {
	Bytecode string
	HasBC    bool

	Tag insTag
	Label
	RegAccess
	PrimConstant
	Goto
	If
	Switch
}

func (self *Instruction) Fallsthrough() bool {
	switch self.Tag {
	case GOTO_TAG, SWITCH:
		return false
	case OTHER:
		{
			op := self.Bytecode[0]
			return !(op == ATHROW || (IRETURN <= op && op <= RETURN))
		}
	default:
		return true
	}
}

func (self *Instruction) Targets() []uint32 {
	switch self.Tag {
	case GOTO_TAG:
		return []uint32{self.Goto.Target}
	case IF:
		return []uint32{self.If.Target}
	case SWITCH:
		{
			result := make([]uint32, 0, 1+len(self.Jumps))
			for _, v := range self.Jumps {
				result = append(result, v)
			}
			return append(result, self.Default)
		}
	default:
		return nil
	}
}

func (self *Instruction) IsJump() bool {
	return self.Tag == GOTO_TAG || self.Tag == IF || self.Tag == SWITCH
}
func (self *Instruction) IsConstant() bool {
	return self.Tag == PRIMCONSTANT || self.Tag == OTHERCONSTANT
}

func (self *Instruction) MinLen(pos uint32) uint32 {
	switch self.Tag {
	case GOTO_TAG:
		if self.Goto.Wide {
			return 5
		} else {
			return 3
		}
	case IF:
		if self.If.Wide {
			return 8
		} else {
			return 3
		}
	case SWITCH:
		return ((^pos) % 4) + self.NoPadSize
	default:
		return uint32(len(self.Bytecode))
	}
}

func (self *Instruction) UpperBound() int {
	if self.HasBC {
		return len(self.Bytecode)
	}
	switch self.Tag {
	case GOTO_TAG:
		return 5
	case IF:
		return 8
	case SWITCH:
		return 3 + int(self.NoPadSize)
	}
	panic(util.Unreachable)
}

// Used to mark locations in the IR instructions for various purposes. These are
// seperate IR 'instructions' since the optimization passes may remove or replace
// the other instructions.
type lblTag uint8

const (
	INVALID_LBL lblTag = iota
	DPOS
	ESTART
	EEND
	EHANDLER
)

type Label struct {
	Tag lblTag
	Pos uint32
}

func NewLabel(tag lblTag, pos uint32) Instruction {
	return Instruction{HasBC: true, Tag: LABEL, Label: Label{tag, pos}}
}

type RegKey struct {
	Reg uint16
	T   scalars.T
}

func (a RegKey) Less(b RegKey) bool {
	return a.Reg < b.Reg || (a.Reg == b.Reg && a.T < b.T)
}

type RegAccess struct {
	RegKey
	Store bool
}

func (self *RegAccess) CalcBytecode(local uint16) string {
	op_base := byte(ILOAD)
	if self.Store {
		op_base = ISTORE
	}

	switch {
	case local < 4:
		op_base += ILOAD_0 - ILOAD
		return (byteio.B(op_base + byte(local) + ops.IlfdaOrd[self.T]*4))
	case local < 256:
		return (byteio.BB(op_base+ops.IlfdaOrd[self.T], byte(local)))
	default:
		return (byteio.BBH(WIDE, op_base+ops.IlfdaOrd[self.T], local))
	}
}

func NewRegAccess(dreg uint16, st scalars.T, store bool) Instruction {
	return Instruction{Tag: REGACCESS, RegAccess: RegAccess{RegKey{dreg, st}, store}}
}

func RawRegAccess(local uint16, st scalars.T, store bool) Instruction {
	data := RegAccess{RegKey{0, st}, store}
	return Instruction{Bytecode: data.CalcBytecode(local), HasBC: true, Tag: REGACCESS, RegAccess: data}
}

type PrimConstant struct {
	scalars.T
	cpool.Pair
}

func (self *PrimConstant) FixWithPool(pool cpool.Pool, self2 *Instruction) {
	if len(self2.Bytecode) > 2 {
		if newbc, ok := fromPool(pool, self.Pair, self.T.Wide()); ok {
			self2.Bytecode = newbc
			self2.HasBC = true
		}
	}
}

func fromPool(pool cpool.Pool, key cpool.Pair, wide bool) (string, bool) {
	if index, ok := pool.TryGet(key); ok {
		if wide {
			return byteio.BH(LDC2_W, index), true
		} else if index >= 256 {
			return byteio.BH(LDC_W, index), true
		} else {
			return byteio.BB(LDC, uint8(index)), true
		}
	}
	return "", false
}

func cpoolKey(val uint64, st scalars.T) cpool.Pair {
	var tag byte
	switch st {
	case scalars.INT:
		tag = cpool.CONSTANT_Integer
	case scalars.FLOAT:
		tag = cpool.CONSTANT_Float
	case scalars.DOUBLE:
		tag = cpool.CONSTANT_Double
	case scalars.LONG:
		tag = cpool.CONSTANT_Long
	}
	return cpool.Pair{tag, cpool.Data{X: val}}
}

func NewPrimConstant(st scalars.T, val uint64, pool cpool.Pool) Instruction {
	util.Assert(st.Wide() || val == uint64(uint32(val)))
	val = constants.Normalize(st, val)
	key := cpoolKey(val, st)

	// If pool is passed in, just grab an entry greedily, otherwise calculate
	// a sequence of bytecode to generate the constant
	bytecode := ""
	hasbc := false

	if pool != nil {
		temp := constants.LookupOnly(st, val)
		if temp != nil {
			bytecode = *temp
			hasbc = true
		}

		if !hasbc {
			bytecode, hasbc = fromPool(pool, key, st.Wide())
		}
		if !hasbc {
			panic(&errors.ClassfileLimitExceeded{})
		}
	} else {
		bytecode = constants.Calc(st, val)
	}
	return Instruction{Bytecode: bytecode, HasBC: true, Tag: PRIMCONSTANT, PrimConstant: PrimConstant{st, key}}
}

func NewOtherConstant(bc string) Instruction {
	return Instruction{Bytecode: bc, HasBC: true, Tag: OTHERCONSTANT}
}

type Goto struct {
	Target uint32
	Wide   bool
}

func NewGoto(target uint32) Instruction {
	return Instruction{Tag: GOTO_TAG, Goto: Goto{target, false}}
}

type If struct {
	Target uint32
	Wide   bool
	Op     byte
}

func NewIf(op uint8, target uint32) Instruction {
	return Instruction{Tag: IF, If: If{target, false, op}}
}

type Switch struct {
	Default   uint32
	Jumps     map[int32]uint32
	Low, High int32
	IsTable   bool
	NoPadSize uint32
}

func NewSwitch(def uint32, jumps map[int32]uint32) Instruction {
	util.Assert(len(jumps) > 0)
	low := int64(math.MaxInt64)
	high := int64(math.MinInt64)
	for k, _ := range jumps {
		k := int64(k)
		if k < low {
			low = k
		}
		if k > high {
			high = k
		}
	}

	tableCount := high - low + 1
	tableSize := 4 * (tableCount + 1)
	jumpSize := 8 * int64(len(jumps))

	best := tableSize
	if best > jumpSize {
		best = jumpSize
	}

	return Instruction{Tag: SWITCH, Switch: Switch{
		Default:   def,
		Jumps:     jumps,
		Low:       int32(low),
		High:      int32(high),
		IsTable:   jumpSize > tableSize,
		NoPadSize: 9 + uint32(best),
	}}
}

func NewOther(bc string) Instruction { return Instruction{Bytecode: bc, HasBC: true, Tag: OTHER} }
