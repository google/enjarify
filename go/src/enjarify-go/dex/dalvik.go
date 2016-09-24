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
package dex

type DalvikType int

type Instruction struct {
	Type      DalvikType
	Pos, Pos2 uint32
	Opcode    uint8
	DalvikArgs

	ImplicitCasts *ImplicitCastData
	PrevResult    string // for move-result/exception
	Switchdata    map[uint32]uint32
	Fillarrdata   []uint64
}

type ImplicitCastData struct {
	DescInd uint32
	Regs    []uint16
}

const (
	INVALID DalvikType = iota

	Nop
	Move
	MoveWide
	MoveResult
	Return
	Const32
	Const64
	ConstString
	ConstClass
	MonitorEnter
	MonitorExit
	CheckCast
	InstanceOf
	ArrayLen
	NewInstance
	NewArray
	FilledNewArray
	FillArrayData
	Throw
	Goto
	Switch
	Cmp
	If
	IfZ
	ArrayGet
	ArrayPut
	InstanceGet
	InstancePut
	StaticGet
	StaticPut
	InvokeVirtual
	InvokeSuper
	InvokeDirect
	InvokeStatic
	InvokeInterface
	UnaryOp
	BinaryOp
	BinaryOpConst
)

var THROW_TYPES = map[DalvikType]bool{ConstString: true, ConstClass: true, MonitorEnter: true, MonitorExit: true, CheckCast: true, InstanceOf: true, ArrayLen: true, NewArray: true, NewInstance: true, FilledNewArray: true, FillArrayData: true, Throw: true, ArrayGet: true, ArrayPut: true, InstanceGet: true, InstancePut: true, StaticGet: true, StaticPut: true, InvokeVirtual: true, InvokeSuper: true, InvokeDirect: true, InvokeStatic: true, InvokeInterface: true, BinaryOp: true, BinaryOpConst: true}

var PRUNED_THROW_TYPES = map[DalvikType]bool{MonitorEnter: true, MonitorExit: true, CheckCast: true, ArrayLen: true, NewArray: true, NewInstance: true, FilledNewArray: true, FillArrayData: true, Throw: true, ArrayGet: true, ArrayPut: true, InstanceGet: true, InstancePut: true, StaticGet: true, StaticPut: true, InvokeVirtual: true, InvokeSuper: true, InvokeDirect: true, InvokeStatic: true, InvokeInterface: true, BinaryOp: true, BinaryOpConst: true}

func getOpcode(op uint8) DalvikType {
	switch {
	case 0 <= op && op <= 0:
		return Nop
	case 1 <= op && op <= 3:
		return Move
	case 4 <= op && op <= 6:
		return MoveWide
	case 7 <= op && op <= 9:
		return Move
	case 10 <= op && op <= 13:
		return MoveResult
	case 14 <= op && op <= 17:
		return Return
	case 18 <= op && op <= 21:
		return Const32
	case 22 <= op && op <= 25:
		return Const64
	case 26 <= op && op <= 27:
		return ConstString
	case 28 <= op && op <= 28:
		return ConstClass
	case 29 <= op && op <= 29:
		return MonitorEnter
	case 30 <= op && op <= 30:
		return MonitorExit
	case 31 <= op && op <= 31:
		return CheckCast
	case 32 <= op && op <= 32:
		return InstanceOf
	case 33 <= op && op <= 33:
		return ArrayLen
	case 34 <= op && op <= 34:
		return NewInstance
	case 35 <= op && op <= 35:
		return NewArray
	case 36 <= op && op <= 37:
		return FilledNewArray
	case 38 <= op && op <= 38:
		return FillArrayData
	case 39 <= op && op <= 39:
		return Throw
	case 40 <= op && op <= 42:
		return Goto
	case 43 <= op && op <= 44:
		return Switch
	case 45 <= op && op <= 49:
		return Cmp
	case 50 <= op && op <= 55:
		return If
	case 56 <= op && op <= 61:
		return IfZ
	case 62 <= op && op <= 67:
		return Nop
	case 68 <= op && op <= 74:
		return ArrayGet
	case 75 <= op && op <= 81:
		return ArrayPut
	case 82 <= op && op <= 88:
		return InstanceGet
	case 89 <= op && op <= 95:
		return InstancePut
	case 96 <= op && op <= 102:
		return StaticGet
	case 103 <= op && op <= 109:
		return StaticPut
	case 110 <= op && op <= 110:
		return InvokeVirtual
	case 111 <= op && op <= 111:
		return InvokeSuper
	case 112 <= op && op <= 112:
		return InvokeDirect
	case 113 <= op && op <= 113:
		return InvokeStatic
	case 114 <= op && op <= 114:
		return InvokeInterface
	case 115 <= op && op <= 115:
		return Nop
	case 116 <= op && op <= 116:
		return InvokeVirtual
	case 117 <= op && op <= 117:
		return InvokeSuper
	case 118 <= op && op <= 118:
		return InvokeDirect
	case 119 <= op && op <= 119:
		return InvokeStatic
	case 120 <= op && op <= 120:
		return InvokeInterface
	case 121 <= op && op <= 122:
		return Nop
	case 123 <= op && op <= 143:
		return UnaryOp
	case 144 <= op && op <= 207:
		return BinaryOp
	case 208 <= op && op <= 226:
		return BinaryOpConst
	}
	return Nop
}

func parseInstruction(dex *DexFile, insns_start_pos uint32, shorts []uint16, pos uint32) (newpos uint32, instr Instruction) {
	word := shorts[pos]
	opcode := uint8(word) & 0xFF
	newpos, args := decode(shorts, pos, opcode)

	instr.Type = getOpcode(opcode)
	instr.Pos = pos
	instr.Pos2 = newpos
	instr.Opcode = opcode
	instr.DalvikArgs = args

	// parse special data instructions
	if word == 0x100 || word == 0x200 { //switch
		size := uint32(shorts[pos+1])
		st := dex.stream(insns_start_pos + pos*2 + 4)

		instr.Switchdata = make(map[uint32]uint32, size)
		if word == 0x100 { //packed
			first_key := st.U32()
			for i := uint32(0); i < size; i++ {
				instr.Switchdata[i+first_key] = st.U32()
			}
			newpos = pos + 2 + (1+size)*2
		} else { //packed
			keys := make([]uint32, size)
			for i := uint32(0); i < size; i++ {
				keys[i] = st.U32()
			}
			for i := uint32(0); i < size; i++ {
				instr.Switchdata[keys[i]] = st.U32()
			}
			newpos = pos + 2 + (size+size)*2
		}
	}

	if word == 0x300 {
		width := uint32(shorts[pos+1]) % 16
		size := uint32(shorts[pos+2]) ^ (uint32(shorts[pos+3]) << 16)
		newpos = pos + ((size*width+1)/2 + 4)

		// get array data
		st := dex.stream(insns_start_pos + pos*2 + 8)
		vals := make([]uint64, size)
		for i := uint32(0); i < size; i++ {
			switch width {
			case 1:
				vals[i] = uint64(st.U8())
			case 2:
				vals[i] = uint64(st.U16())
			case 4:
				vals[i] = uint64(st.U32())
			case 8:
				vals[i] = uint64(st.U64())
			}
		}
		instr.Fillarrdata = vals
	}
	return
}

func parseBytecode(dex *DexFile, insns_start_pos uint32, shorts []uint16, catch_addrs map[uint32]bool) (ops []Instruction) {
	op := Instruction{} // make compiler happy
	pos := uint32(0)
	for pos < uint32(len(shorts)) {
		pos, op = parseInstruction(dex, insns_start_pos, shorts, pos)
		ops = append(ops, op)
	}

	// Fill in data for move-result
	for i := range ops[1:] {
		instr := &ops[i]
		instr2 := &ops[i+1]
		if instr2.Type != MoveResult {
			continue
		}

		if catch_addrs[instr2.Pos] {
			instr2.PrevResult = "Ljava/lang/Throwable;"
		}

		switch instr.Type {
		case InvokeVirtual, InvokeSuper, InvokeDirect, InvokeStatic, InvokeInterface:
			instr2.PrevResult = dex.GetMethodId(instr.A).ReturnType
		case FilledNewArray:
			instr2.PrevResult = dex.Type(instr.A)
		}
	}

	for i := range ops {
		switch ops[i].Opcode {
		case 0x38, 0x39:
			if i > 0 && ops[i-1].Type == InstanceOf {
				prev := ops[i-1]
				desc_ind := prev.C
				set := map[uint16]bool{prev.Rb: true}

				if i > 1 && ops[i-2].Type == Move {
					prev2 := ops[i-2]
					if prev2.Ra == prev.Rb {
						set[prev2.Rb] = true
					}
				}

				set[prev.Ra] = false
				regs := make([]uint16, 0, len(set))
				for k, v := range set {
					if v {
						regs = append(regs, k)
					}
				}

				for i := 0; i < len(regs); i++ {
					for j := 0; j < i; j++ {
						if regs[j] > regs[i] {
							regs[i], regs[j] = regs[j], regs[i]
						}
					}
				}

				if len(regs) > 0 {
					ops[i].ImplicitCasts = &ImplicitCastData{desc_ind, regs}
				}
			}
		}
	}

	return
}
