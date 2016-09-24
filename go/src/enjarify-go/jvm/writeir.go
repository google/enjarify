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
	"enjarify-go/jvm/arrays"
	"enjarify-go/jvm/cpool"
	"enjarify-go/jvm/ir"
	"enjarify-go/jvm/ops"
	"enjarify-go/jvm/scalars"
	"enjarify-go/util"
)

func visit(method dex.Method, dex_ *dex.DexFile, instr_d map[uint32]dex.Instruction, type_data TypeInfo, block *irBlock, instr dex.Instruction) {
	switch instr.Type {
	case dex.Nop:
	case dex.Move:
		for _, st := range []scalars.T{scalars.INT, scalars.OBJ, scalars.FLOAT} {
			if st&type_data.st(instr.Rb) > 0 {
				block.Load(instr.Rb, st)
				block.Store(instr.Ra, st)
			}
		}
	case dex.MoveWide:
		for _, st := range []scalars.T{scalars.LONG, scalars.DOUBLE} {
			if st&type_data.st(instr.Rb) > 0 {
				block.Load(instr.Rb, st)
				block.Store(instr.Ra, st)
			}
		}
	case dex.MoveResult:
		block.Store(instr.Ra, scalars.FromDesc(instr.PrevResult))
	case dex.Return:
		if method.ReturnType == "V" {
			block.Return_()
		} else {
			st := scalars.FromDesc(method.ReturnType)
			block.LoadDesc(instr.Ra, st, method.ReturnType)
			block.Return(st)
		}
	case dex.Const32:
		val := uint64(instr.B)
		block.Const(val, scalars.INT)
		block.Store(instr.Ra, scalars.INT)
		block.Const(val, scalars.FLOAT)
		block.Store(instr.Ra, scalars.FLOAT)
		if val == 0 {
			block.Const(val, scalars.OBJ)
			block.Store(instr.Ra, scalars.OBJ)
		}
	case dex.Const64:
		val := instr.Long
		block.Const(val, scalars.LONG)
		block.Store(instr.Ra, scalars.LONG)
		block.Const(val, scalars.DOUBLE)
		block.Store(instr.Ra, scalars.DOUBLE)
	case dex.ConstString:
		val := dex_.String(instr.B)
		block.Ldc(block.Pool.String(val))
		block.Store(instr.Ra, scalars.OBJ)
	case dex.ConstClass:
		// Could use dex.type here since the JVM doesn't care, but this is cleaner
		val := dex_.ClsType(instr.B)
		block.Ldc(block.Pool.Class(val))
		block.Store(instr.Ra, scalars.OBJ)
	case dex.MonitorEnter:
		block.Load(instr.Ra, scalars.OBJ)
		block.U8(MONITORENTER)
	case dex.MonitorExit:
		block.Load(instr.Ra, scalars.OBJ)
		block.U8(MONITOREXIT)
	case dex.CheckCast:
		block.Cast(dex_, instr.Ra, instr.B)
	case dex.InstanceOf:
		block.Load(instr.Rb, scalars.OBJ)
		block.U8U16(INSTANCEOF, block.Pool.Class(dex_.ClsType(instr.C)))
		block.Store(instr.Ra, scalars.INT)
	case dex.ArrayLen:
		block.LoadAsArray(instr.Rb)
		block.U8(ARRAYLENGTH)
		block.Store(instr.Ra, scalars.INT)

	case dex.NewInstance:
		block.U8U16(NEW, block.Pool.Class(dex_.ClsType(instr.B)))
		block.Store(instr.Ra, scalars.OBJ)
	case dex.NewArray:
		block.Load(instr.Rb, scalars.INT)
		block.NewArray(dex_.Type(instr.C))
		block.Store(instr.Ra, scalars.OBJ)

	case dex.FilledNewArray:
		block.Const(uint64(len(instr.Args)), scalars.INT)
		block.NewArray(dex_.Type(instr.A))
		st, elet := arrays.FromDesc(dex_.Type(instr.A)).EletPair()
		op := ops.ArrStoreOp(elet)
		cbs := make([]func(), len(instr.Args))
		for i, x := range instr.Args {
			x, st := x, st // make copy of vars for closure
			cbs[i] = func() { block.Load(x, st) }
		}
		mustpop := instr_d[instr.Pos2].Type != dex.MoveResult
		block.FillArraySub(op, cbs, mustpop)

	case dex.FillArrayData:
		arrdata := instr_d[instr.B].Fillarrdata
		at := type_data.at(instr.Ra)

		block.LoadAsArray(instr.Ra)
		if at == arrays.NULL {
			block.U8(ATHROW)
		} else {
			if len(arrdata) == 0 {
				// fill-array-data throws a NPE if array is null even when
				// there is 0 data, so we need to add an instruction that
				// throws a NPE in this case
				block.U8(ARRAYLENGTH)
				block.U8(POP)
			} else {
				st, elet := at.EletPair()
				cbs := make([]func(), len(arrdata))
				for i, val := range arrdata {
					// check if we need to sign extend
					if elet == "B" {
						val = uint64(uint32(int8(val)))
					} else if elet == "S" {
						val = uint64(uint32(int16(val)))
					}
					util.Assert(st != scalars.OBJ)
					val, st := val, st // make copy of changing variables for closure
					cbs[i] = func() { block.Const(val, st) }
				}
				block.FillArraySub(ops.ArrStoreOp(elet), cbs, true)
			}
		}
	case dex.Throw:
		block.LoadAsCls(instr.Ra, scalars.OBJ, "java/lang/Throwable")
		block.U8(ATHROW)
	case dex.Goto:
		block.Goto(instr.A)
	case dex.Switch:
		block.Load(instr.Ra, scalars.INT)
		switchdata := instr_d[instr.B].Switchdata
		def := instr.Pos2
		jumps := make(map[uint32]uint32, len(switchdata))
		for k, offset := range switchdata {
			jumps[k] = offset + instr.Pos
		}
		block.Switch(def, jumps)
	case dex.Cmp:
		op := []byte{FCMPL, FCMPG, DCMPL, DCMPG, LCMP}[instr.Opcode-0x2d]
		st := []scalars.T{scalars.FLOAT, scalars.FLOAT, scalars.DOUBLE, scalars.DOUBLE, scalars.LONG}[instr.Opcode-0x2d]
		block.Load(instr.Rb, st)
		block.Load(instr.Rc, st)
		block.U8(op)
		block.Store(instr.Ra, scalars.INT)
	case dex.If:
		st := type_data.st(instr.Ra) & type_data.st(instr.Rb)
		op := byte(0)
		if st&scalars.INT > 0 {
			block.Load(instr.Ra, scalars.INT)
			block.Load(instr.Rb, scalars.INT)
			op = []byte{IF_ICMPEQ, IF_ICMPNE, IF_ICMPLT, IF_ICMPGE, IF_ICMPGT, IF_ICMPLE}[instr.Opcode-0x32]
		} else {
			block.Load(instr.Ra, scalars.OBJ)
			block.Load(instr.Rb, scalars.OBJ)
			op = []byte{IF_ACMPEQ, IF_ACMPNE}[instr.Opcode-0x32]
		}
		block.If(op, instr.C)
	case dex.IfZ:
		op := byte(0)
		if type_data.st(instr.Ra)&scalars.INT > 0 {
			block.Load(instr.Ra, scalars.INT)
			op = []byte{IFEQ, IFNE, IFLT, IFGE, IFGT, IFLE}[instr.Opcode-0x38]
		} else {
			block.Load(instr.Ra, scalars.OBJ)
			op = []byte{IFNULL, IFNONNULL}[instr.Opcode-0x38]
		}
		block.If(op, instr.B)

	case dex.ArrayGet:
		at := type_data.at(instr.Rb)
		if at == arrays.NULL {
			block.ConstNull()
			block.U8(ATHROW)
		} else {
			block.LoadAsArray(instr.Rb)
			block.Load(instr.Rc, scalars.INT)
			st, elet := at.EletPair()
			block.U8(ops.ArrLoadOp(elet))
			block.Store(instr.Ra, st)
		}
	case dex.ArrayPut:
		at := type_data.at(instr.Rb)
		if at == arrays.NULL {
			block.ConstNull()
			block.U8(ATHROW)
		} else {
			block.LoadAsArray(instr.Rb)
			block.Load(instr.Rc, scalars.INT)
			st, elet := at.EletPair()
			block.Load(instr.Ra, st)
			block.U8(ops.ArrStoreOp(elet))
		}

	case dex.InstanceGet:
		field_id := dex_.GetFieldId(instr.C)
		st := scalars.FromDesc(field_id.Desc)
		block.LoadAsCls(instr.Rb, scalars.OBJ, field_id.Cname)
		block.U8U16(GETFIELD, block.Pool.Field(field_id))
		block.Store(instr.Ra, st)

	case dex.InstancePut:
		field_id := dex_.GetFieldId(instr.C)
		st := scalars.FromDesc(field_id.Desc)
		block.LoadAsCls(instr.Rb, scalars.OBJ, field_id.Cname)
		block.LoadDesc(instr.Ra, st, field_id.Desc)
		block.U8U16(PUTFIELD, block.Pool.Field(field_id))

	case dex.StaticGet:
		field_id := dex_.GetFieldId(instr.B)
		st := scalars.FromDesc(field_id.Desc)
		block.U8U16(GETSTATIC, block.Pool.Field(field_id))
		block.Store(instr.Ra, st)

	case dex.StaticPut:
		field_id := dex_.GetFieldId(instr.B)
		st := scalars.FromDesc(field_id.Desc)
		block.LoadDesc(instr.Ra, st, field_id.Desc)
		block.U8U16(PUTSTATIC, block.Pool.Field(field_id))

	case dex.InvokeVirtual, dex.InvokeSuper, dex.InvokeDirect, dex.InvokeStatic, dex.InvokeInterface:
		isstatic := instr.Type == dex.InvokeStatic
		called_id := dex_.GetMethodId(instr.A)
		sts := scalars.ParamTypes(called_id, isstatic)
		descs := called_id.GetSpacedParamTypes(isstatic)

		for i, reg := range instr.Args {
			st, desc := sts[i], descs[i]
			if st != scalars.INVALID { // skip long/double tops
				block.LoadDesc(reg, st, *desc)
			}
		}

		op := map[dex.DalvikType]byte{
			dex.InvokeVirtual:   INVOKEVIRTUAL,
			dex.InvokeSuper:     INVOKESPECIAL,
			dex.InvokeDirect:    INVOKESPECIAL,
			dex.InvokeStatic:    INVOKESTATIC,
			dex.InvokeInterface: INVOKEINTERFACE,
		}[instr.Type]

		if instr.Type == dex.InvokeInterface {
			block.U8U16U8U8(op, block.Pool.IMethod(called_id.Triple), uint8(len(descs)), 0)
		} else {
			block.U8U16(op, block.Pool.Method(called_id.Triple))
		}

		// check if we need to pop result instead of leaving on stack
		if instr_d[instr.Pos2].Type != dex.MoveResult {
			if called_id.ReturnType != "V" {
				if scalars.FromDesc(called_id.ReturnType).Wide() {
					block.U8(POP2)
				} else {
					block.U8(POP)
				}
			}
		}

	case dex.UnaryOp:
		data := UNARY[instr.Opcode]
		block.Load(instr.Rb, data.SrcT)
		// *not requires special handling since there's no direct Java equivalent. Instead we have to do x ^ -1
		switch data.Op {
		case IXOR:
			block.U8(ICONST_M1)
		case LXOR:
			block.U8(ICONST_M1)
			block.U8(I2L)
		}

		block.U8(data.Op)
		block.Store(instr.Ra, data.DestT)

	case dex.BinaryOp:
		data := BINARY[instr.Opcode]
		if instr.Opcode >= 0xB0 { // 2addr form
			block.Load(instr.Ra, data.SrcT)
			block.Load(instr.Rb, data.Src2T)
		} else {
			block.Load(instr.Rb, data.SrcT)
			block.Load(instr.Rc, data.Src2T)
		}

		block.U8(data.Op)
		block.Store(instr.Ra, data.SrcT)

	case dex.BinaryOpConst:
		data := BINARY_LIT[instr.Opcode]
		if data.Op == ISUB { // rsub
			block.Const(uint64(instr.C), scalars.INT)
			block.Load(instr.Rb, scalars.INT)
		} else {
			block.Load(instr.Rb, scalars.INT)
			block.Const(uint64(instr.C), scalars.INT)
		}
		block.U8(data.Op)
		block.Store(instr.Ra, scalars.INT)
	}
}

func writeBytecode(pool cpool.Pool, method dex.Method, opts Options) *IRWriter {
	code := method.Code
	instr_d := make(map[uint32]dex.Instruction, len(code.Bytecode))
	for _, ins := range code.Bytecode {
		instr_d[ins.Pos] = ins
	}

	types, all_handlers := doInference(method, instr_d)
	scalar_ptypes := scalars.ParamTypes(method.MethodId, (method.Access&ACC_STATIC) > 0)

	writer := newIRWriter(pool, method, types, opts)
	writer.calcInitialArgs(code.Nregs, scalar_ptypes)

	for _, instr := range code.Bytecode {
		if type_data, ok := types[instr.Pos]; ok {
			block := writer.createBlock(instr.Pos)
			visit(method, method.Dex, instr_d, type_data, block, instr)
		}
	}

	for _, instr := range code.Bytecode {
		if _, ok := types[instr.Pos]; !ok {
			continue // skip unreachable instructions
		}

		if handlers, ok := all_handlers[instr.Pos]; ok {
			_ = handlers[0]
			start, end := writer.iblocks[instr.Pos].AddExceptLabels()

			for _, item := range handlers {
				target := ir.Label{ir.DPOS, item.Target}
				// If handler doesn't use the caught exception, we need to redirect to a pop instead
				if instr_d[item.Target].Type != dex.MoveResult {
					target = writer.addExceptionRedirect(item.Target)
				}
				writer.target_pred_counts[target]++

				// When catching Throwable, we can use the special index 0 instead,
				// potentially saving a constant pool entry or two
				jctype := uint16(0)
				if item.Type != "java/lang/Throwable" {
					jctype = pool.Class(item.Type)
				}
				writer.excepts = append(writer.excepts, exceptInfo{start, end, target, jctype})
			}
		}
	}
	writer.flatten()

	// find jump targets (in addition to exception handler targets)
	for i := range writer.Instructions {
		instr := &writer.Instructions[i]
		for _, target := range instr.Targets() {
			writer.target_pred_counts[ir.Label{ir.DPOS, target}]++
		}
	}

	return writer
}
