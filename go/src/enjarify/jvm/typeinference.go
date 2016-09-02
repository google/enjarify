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
	"enjarify/dex"
	"enjarify/jvm/arrays"
	"enjarify/jvm/scalars"
)

// The two main things we need type inference for are determining the types of
// primative values and arrays. Luckily, we don't care about actual classes in
// these cases, we just need to know whether it is int,float,reference, etc. to
// generate the correct bytecode instructions, which are typed in Java.
//
// One additional problem is that ART's implicit casts narrow the type instead of
// replacing it like regular checkcasts do. This means that there is no way to
// replicate the behavior in Java using normal casts unless you know which class
// is a subclass of another and which classes are interfaces. However, we want to
// be able to translate code without knowing about every other class that could be
// referenced by the application, so we make do with a hack.
//
// Variables subjected to implicit casting are marked as tainted. Whenever a
// tained value is used, it is explcitly checkcasted to the expected type. This
// isn't ideal since it will incorrectly throw in the cast of bad interface casts,
// but it's the best we can do without requiring knowledge of the whole inheritance
// hierarchy.

type TypeInfo struct {
	prims, arrs, tainted *ImmutableTreeList
}

func (self TypeInfo) st(reg uint16) scalars.T {
	return self.prims.get(reg).(scalars.T)
}

func (self TypeInfo) at(reg uint16) arrays.T {
	return self.arrs.get(reg).(arrays.T)
}

func (self TypeInfo) taint(reg uint16) bool {
	return self.tainted.get(reg).(bool)
}

func (self TypeInfo) set(reg uint16, st scalars.T, at arrays.T, taint bool) TypeInfo {
	self.prims = self.prims.set(reg, st)
	self.arrs = self.arrs.set(reg, at)
	self.tainted = self.tainted.set(reg, taint)
	return self
}

func (self TypeInfo) move(src, dest uint16, wide bool) TypeInfo {
	self = self.set(dest, self.st(src), self.at(src), self.taint(src))
	if wide {
		src++
		self = self.set(dest+1, self.st(src), self.at(src), self.taint(src))
	}
	return self
}

func (self TypeInfo) assign(reg uint16, st scalars.T) TypeInfo {
	return self.set(reg, st, arrays.INVALID, false)
}

func (self TypeInfo) assign_(reg uint16, st scalars.T, at arrays.T) TypeInfo {
	return self.set(reg, st, at, false)
}

func (self TypeInfo) assignTaint(reg uint16, st scalars.T, at arrays.T) TypeInfo {
	return self.set(reg, st, at, true)
}

func (self TypeInfo) assign2(reg uint16, st scalars.T) TypeInfo {
	return self.set(reg, st, arrays.INVALID, false).set(reg+1, scalars.INVALID, arrays.INVALID, false)
}

func (self TypeInfo) assignFromDesc(reg uint16, desc string) TypeInfo {
	st := scalars.FromDesc(desc)
	at := arrays.FromDesc(desc)
	if st.Wide() {
		return self.assign2(reg, st)
	} else {
		return self.assign_(reg, st, at)
	}
}

func (self TypeInfo) merge(other TypeInfo) TypeInfo {
	self.prims = self.prims.merge(other.prims, func(a, b interface{}) interface{} { return a.(scalars.T) & b.(scalars.T) })
	self.arrs = self.arrs.merge(other.arrs, func(a, b interface{}) interface{} { return a.(arrays.T).Merge(b.(arrays.T)) })
	self.tainted = self.tainted.merge(other.tainted, func(a, b interface{}) interface{} { return a.(bool) || b.(bool) })
	return self
}

func fromParams(method dex.Method, nregs uint16) TypeInfo {
	isstatic := method.Access&ACC_STATIC != 0
	full_ptypes := method.GetSpacedParamTypes(isstatic)
	offset := nregs - uint16(len(full_ptypes))

	prims := newTreeList(scalars.INVALID)
	arrs := newTreeList(arrays.INVALID)
	tainted := newTreeList(false)

	for i, desc := range full_ptypes {
		if desc != nil {
			prims = prims.set(offset+uint16(i), scalars.FromDesc(*desc))
			arrs = arrs.set(offset+uint16(i), arrays.FromDesc(*desc))
		}
	}
	return TypeInfo{prims, arrs, tainted}
}

func isMathThrowOp(opcode uint8) bool {
	switch opcode {
	case IDIV, IREM, LDIV, LREM:
		return true
	default:
		return false
	}
}

func pruneHandlers(instr_d map[uint32]dex.Instruction, all_handlers map[uint32][]dex.CatchItem) map[uint32][]dex.CatchItem {
	result := make(map[uint32][]dex.CatchItem)
	for pos, handlers := range all_handlers {
		instr := instr_d[pos]
		if !dex.PRUNED_THROW_TYPES[instr.Type] {
			continue
		}

		// if math op, make sure it is int div/rem
		if instr.Type == dex.BinaryOp && !isMathThrowOp(BINARY[instr.Opcode].Op) {
			continue
		} else if instr.Type == dex.BinaryOpConst && !isMathThrowOp(BINARY_LIT[instr.Opcode].Op) {
			continue
		}

		types := make(map[string]bool)
		for _, item := range handlers {
			// if multiple handlers with same catch type, only include the first
			if !types[item.Type] {
				result[instr.Pos] = append(result[instr.Pos], item)
				types[item.Type] = true
			}
			// stop as soon as we reach a catch all handler
			if item.Type == "java/lang/Throwable" {
				break
			}
		}
	}
	return result
}

func visitNormal(dex_ *dex.DexFile, instr dex.Instruction, cur TypeInfo) TypeInfo {
	switch instr.Type {
	case dex.ConstString, dex.ConstClass, dex.NewInstance:
		return cur.assign(instr.Ra, scalars.OBJ)
	case dex.InstanceOf, dex.ArrayLen, dex.Cmp, dex.BinaryOpConst:
		return cur.assign(instr.Ra, scalars.INT)
	case dex.Move:
		return cur.move(instr.Rb, instr.Ra, false)
	case dex.MoveWide:
		return cur.move(instr.Rb, instr.Ra, true)
	case dex.MoveResult:
		return cur.assignFromDesc(instr.Ra, instr.PrevResult)
	case dex.Const32:
		if instr.B == 0 {
			return cur.assign_(instr.Ra, scalars.ZERO, arrays.NULL)
		} else {
			return cur.assign(instr.Ra, scalars.C32)
		}
	case dex.Const64:
		return cur.assign2(instr.Ra, scalars.C64)
	case dex.CheckCast:
		at := arrays.FromDesc(dex_.Type(instr.B))
		at = at.Narrow(cur.at(instr.Ra))
		return cur.assign_(instr.Ra, scalars.OBJ, at)
	case dex.NewArray:
		at := arrays.FromDesc(dex_.Type(instr.C))
		return cur.assign_(instr.Ra, scalars.OBJ, at)
	case dex.ArrayGet:
		arr_at := cur.at(instr.Rb)
		if arr_at == arrays.NULL {
			// This is unreachable, so use (ALL, NULL), which can be merged with anything
			return cur.assign_(instr.Ra, scalars.ALL, arrays.NULL)
		}
		st, at := arr_at.EletPair()
		return cur.assign_(instr.Ra, st, at)
	case dex.InstanceGet:
		field_id := dex_.GetFieldId(instr.C)
		return cur.assignFromDesc(instr.Ra, field_id.Desc)
	case dex.StaticGet:
		field_id := dex_.GetFieldId(instr.B)
		return cur.assignFromDesc(instr.Ra, field_id.Desc)
	case dex.UnaryOp:
		st := UNARY[instr.Opcode].DestT
		if st.Wide() {
			return cur.assign2(instr.Ra, st)
		} else {
			return cur.assign(instr.Ra, st)
		}
	case dex.BinaryOp:
		st := BINARY[instr.Opcode].SrcT
		if st.Wide() {
			return cur.assign2(instr.Ra, st)
		} else {
			return cur.assign(instr.Ra, st)
		}
	}
	return cur
}

func doInference(method dex.Method, instr_d map[uint32]dex.Instruction) (map[uint32]TypeInfo, map[uint32][]dex.CatchItem) {
	// fmt.Printf("TI %v\n", method.Triple)
	code := method.Code
	all_handlers := make(map[uint32][]dex.CatchItem)
	for _, tryi := range code.Tries {
		for _, instr := range code.Bytecode {
			if tryi.Start < instr.Pos2 && tryi.End > instr.Pos {
				all_handlers[instr.Pos] = append(all_handlers[instr.Pos], tryi.Catches...)
			}
		}
	}

	all_handlers = pruneHandlers(instr_d, all_handlers)
	types := make(map[uint32]TypeInfo)
	types[0] = fromParams(method, code.Nregs)
	dirty := map[uint32]bool{0: true}

	doMerge := func(pos uint32, newv TypeInfo) {
		// prevent infinite loops
		if _, ok := instr_d[pos]; !ok {
			return
		}

		if old, ok := types[pos]; ok {
			newv := newv.merge(old)
			if newv != old {
				types[pos] = newv
				dirty[pos] = true
			}
		} else {
			types[pos] = newv
			dirty[pos] = true
		}
	}

	for len(dirty) > 0 {
		for _, instr := range code.Bytecode {
			if !dirty[instr.Pos] {
				continue
			}
			delete(dirty, instr.Pos)

			cur := types[instr.Pos]
			// fmt.Printf("Inference at %d: %v %v %v\n", instr.Pos, cur.st(13), cur.at(13), cur.taint(13))
			// fmt.Printf("Inference at %d: %v\n", instr.Pos, cur.prims.direct)
			after := visitNormal(method.Dex, instr, cur)
			// if instr.Pos == 12 {
			// 	fmt.Printf("instr %v\n", instr)
			// 	fmt.Printf("after: %v\n", after.prims.direct)
			// }

			result, after2 := after, after

			// implicit casts
			if instr.ImplicitCasts != nil {
				mask := arrays.FromDesc(method.Dex.Type(instr.ImplicitCasts.DescInd))
				for _, reg := range instr.ImplicitCasts.Regs {
					st := cur.st(reg) // could != OBJ if null
					at := cur.at(reg).Narrow(mask)
					result = result.assignTaint(reg, st, at)
				}
				// merge into branch if op = if-nez else merge into fallthrough
				if instr.Opcode == 0x39 {
					after2 = result
				} else {
					after = result
				}
			}

			switch instr.Type {
			case dex.Goto:
				doMerge(instr.A, after2)
			case dex.If:
				doMerge(instr.C, after2)
			case dex.IfZ:
				doMerge(instr.B, after2)
			case dex.Switch:
				for _, offset := range instr_d[instr.B].Switchdata {
					doMerge(instr.Pos+offset, cur)
				}
			}

			// these instructions don't fallthrough
			switch instr.Type {
			case dex.Return, dex.Throw, dex.Goto:
			default:
				doMerge(instr.Pos2, after)
			}

			// exception handlers
			if handlers, ok := all_handlers[instr.Pos]; ok {
				for _, item := range handlers {
					doMerge(item.Target, cur)
				}
			}
		}
	}

	return types, all_handlers
}
