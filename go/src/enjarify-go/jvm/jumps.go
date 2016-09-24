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
	"enjarify-go/jvm/ir"
	"enjarify-go/util"
)

func calcMinimumPositions(instrs []ir.Instruction) (res []uint32, pos uint32) {
	for _, ins := range instrs {
		old := pos
		pos += ins.MinLen(old)
		res = append(res, old)
	}
	return
}

type PosInfo struct {
	m map[ir.Label]int
	v []uint32
}

func (self *PosInfo) getlbl(lbl ir.Label) uint32             { return self.v[self.m[lbl]] }
func (self *PosInfo) get(target uint32) uint32               { return self.getlbl(ir.Label{ir.DPOS, target}) }
func (self *PosInfo) offset(pos uint32, target uint32) int32 { return int32(self.get(target) - pos) }

func widenIfNecessary(ins *ir.Instruction, pos uint32, info PosInfo) bool {
	switch ins.Tag {
	case ir.GOTO_TAG:
		{
			data := &ins.Goto
			if data.Wide {
				return false
			}
			offset := info.offset(pos, data.Target)
			data.Wide = offset != int32(int16(offset))
			return data.Wide
		}
	case ir.IF:
		{
			data := &ins.If
			if data.Wide {
				return false
			}
			offset := info.offset(pos, data.Target)
			data.Wide = offset != int32(int16(offset))
			return data.Wide
		}
	default:
		return false
	}
}

func optimizeJumps(irdata *IRWriter) {
	// For jump offsets of more than +-32767, a longer form of the jump instruction
	// is required. This function finds the optimal jump widths by optimistically
	// starting with everything narrow and then iteratively marking instructions
	// as wide if their offset is too large (in rare cases, this can in turn cause
	// other jumps to become wide, hence iterating until convergence)
	instrs := irdata.Instructions

	lblToVind := make(map[ir.Label]int)
	for i, ins := range instrs {
		if ins.Tag == ir.LABEL {
			lblToVind[ins.Label] = i
		}
	}

	done := false
	for !done {
		done = true
		mins, _ := calcMinimumPositions(instrs)
		info := PosInfo{lblToVind, mins}

		for i, _ := range instrs {
			pos := mins[i]
			temp := !widenIfNecessary(&instrs[i], pos, info)
			done = done && temp
		}
	}
}

func oppositeOp(op uint8) uint8 {
	if op >= IFNULL {
		return op ^ 1
	} else {
		return ((op + 1) ^ 1) - 1
	}
}

func createBytecode(irdata *IRWriter) (string, []string) {
	instrs := irdata.Instructions

	lblToVind := make(map[ir.Label]int)
	for i, ins := range instrs {
		if ins.Tag == ir.LABEL {
			lblToVind[ins.Label] = i
		}
	}

	positions, endpos := calcMinimumPositions(instrs)
	info := PosInfo{lblToVind, positions}

	stream := byteio.NewWriter()

	// parts := make([]string, len(instrs))
	for i, ins := range instrs {
		pos := positions[i]
		switch ins.Tag {
		case ir.GOTO_TAG:
			{
				data := ins.Goto
				offset := info.offset(pos, data.Target)
				if data.Wide {
					stream.WriteString(byteio.Bi(GOTO_W, offset))
				} else {
					stream.WriteString(byteio.Bh(GOTO, int16(offset)))
				}
			}
		case ir.IF:
			{
				data := ins.If
				offset := info.offset(pos, data.Target)
				if data.Wide {
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
					offset -= 3
					stream.WriteString(byteio.BhBi(oppositeOp(data.Op), 8, GOTO_W, offset))
				} else {
					stream.WriteString(byteio.Bh(data.Op, int16(offset)))
				}
			}
		case ir.SWITCH:
			{
				data := ins.Switch
				offset := info.offset(pos, data.Default)
				pad := (^pos) % 4

				if data.IsTable {
					stream.U8(TABLESWITCH)
					for pad > 0 {
						stream.U8(0)
						pad--
					}
					stream.S32(offset)
					stream.S32(data.Low)
					stream.S32(data.High)
					for k := data.Low; k <= data.High; k++ {
						target, ok := data.Jumps[k]
						if !ok {
							target = data.Default
						}
						stream.S32(info.offset(pos, target))
					}
				} else {
					stream.U8(LOOKUPSWITCH)
					for pad > 0 {
						stream.U8(0)
						pad--
					}
					stream.S32(offset)
					stream.U32(uint32(len(data.Jumps)))
					for _, k := range keys3(data.Jumps).Sort() {
						stream.S32(k)
						stream.S32(info.offset(pos, data.Jumps[k]))
					}
				}
			}

		default:
			stream.WriteString(ins.Bytecode)
		}
	}
	util.Assert(int(endpos) == stream.Len())

	excepts := []string{}
	for _, item := range irdata.excepts {
		// There appears to be a bug in the JVM where in rare cases, it throws
		// the exception at the address of the instruction _before_ the instruction
		// that actually caused the exception, triggering the wrong handler
		// therefore we include the previous (IR) instruction too
		// Note that this cannot cause an overlap because in that case the previous
		// instruction would just be a label and hence not change anything
		sind := lblToVind[item.start.Label]
		if sind > 0 {
			sind--
		}
		soff := positions[sind]
		excepts = append(excepts, byteio.HHHH(uint16(soff), uint16(info.getlbl(item.end.Label)), uint16(info.getlbl(item.target)), item.ctype))
	}

	return stream.String(), excepts
}
