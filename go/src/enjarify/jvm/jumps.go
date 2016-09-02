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
	"enjarify/byteio"
	"enjarify/jvm/ir"
	"enjarify/util"
	"fmt"
	"strings"
)

func _calcMinimumPositions(instrs []ir.Instruction) map[ir.Instruction]int32 {
	posd := map[ir.Instruction]int32{}
	pos := int32(0)
	for _, ins := range instrs {
		posd[ins] = pos

		switch ins := ins.(type) {
		case *ir.Goto:
			pos += ins.Min()
		case *ir.If:
			pos += ins.Min()
		case *ir.Switch:
			pad := (((-pos - 1) % 4) + 4) % 4
			pos += pad + ins.NoPadSize
		default:
			pos += int32(len(ins.Bytecode()))
		}
	}
	return posd
}
func optimizeJumps(irdata *IRWriter) {
	// For jump offsets of more than +-32767, a longer form of the jump instruction
	// is required. This function finds the optimal jump widths by optimistically
	// starting with everything narrow and then iteratively marking instructions
	// as wide if their offset is too large (in rare cases, this can in turn cause
	// other jumps to become wide, hence iterating until convergence)
	instrs := irdata.Instructions

	done := false
	for !done {
		done = true
		posd := _calcMinimumPositions(instrs)

		for _, ins := range instrs {
			if ins, ok := ins.(ir.LazyJump); ok {
				ins := ins.X()
				if !ins.Wide && ins.WidenIfNecessary(irdata.Labels, posd) {
					done = false
				}
			}
		}
	}
}

func createBytecode(irdata *IRWriter) (string, []string) {
	instrs := irdata.Instructions

	posd := _calcMinimumPositions(instrs)
	parts := make([]string, len(instrs))
	for i, ins := range instrs {
		switch ins := ins.(type) {
		case *ir.Goto:
			ins.CalcBytecode(posd, irdata.Labels)
		case *ir.If:
			ins.CalcBytecode(posd, irdata.Labels)
		case *ir.Switch:
			ins.CalcBytecode(posd, irdata.Labels)
		}

		parts[i] = ins.Bytecode()
	}

	bytecode := strings.Join(parts, "")
	prev_instr_map := map[ir.Instruction]ir.Instruction{}
	for i, instr := range instrs {
		if i > 0 {
			prev_instr_map[instr] = instrs[i-1]
		}
	}
	prev_instr_map[instrs[0]] = instrs[0]

	excepts := []string{}
	for _, info := range irdata.excepts {
		// There appears to be a bug in the JVM where in rare cases, it throws
		// the exception at the address of the instruction _before_ the instruction
		// that actually caused the exception, triggering the wrong handler
		// therefore we include the previous (IR) instruction too
		// Note that this cannot cause an overlap because in that case the previous
		// instruction would just be a label and hence not change anything
		s_off := uint16(posd[prev_instr_map[info.start]])
		e_off := uint16(posd[info.end])
		h_off := uint16(posd[info.target])
		if s_off < e_off {
			excepts = append(excepts, byteio.HHHH(s_off, e_off, h_off, info.ctype))
		} else {
			fmt.Printf("Skipping zero width exception!\n")
			panic(util.Unreachable)
		}
	}

	return bytecode, excepts
}
