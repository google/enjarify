// Copyright 2016 Google Inc. All Rights Reserved.
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
use std::collections::HashMap;

use strings::*;

use dex;
use dalvik::*;
use dalvik::DalvikType as dt;
use pack;
use typeinference::inference::TypeInfo;
use typeinference::{array, scalar};
use typeinference::array::Base;

use super::constantpool::ConstantPool;
use super::ir;
use super::ir::LabelId::*;
use super::jvmops::*;
use super::mathops;
use super::optimization::options::Options;
use super::optimization::stack::gen_dups;

// add some helper methods T -> jvmop
impl array::T {
    fn load_op(self) -> u8 {
        match self {
            array::T::Array(dim, base) => {
                if dim == 1 {
                    match base {
                        Base::I => IALOAD,
                        Base::J => LALOAD,
                        Base::F => FALOAD,
                        Base::D => DALOAD,
                        Base::B => BALOAD,
                        Base::C => CALOAD,
                        Base::S => SALOAD,
                    }
                } else { AALOAD }
            }
            array::INVALID | array::NULL => AALOAD,
        }
    }

    fn store_op(self) -> u8 { self.load_op() + (IASTORE - IALOAD) }
}

struct IRBlock<'b, 'a: 'b> {
    pos: u32,
    pool: &'b mut (ConstantPool<'a> + 'a),
    type_info: TypeInfo,
    instructions: Vec<ir::JvmInstruction>,
    delay_consts: bool,
}
impl <'b, 'a> IRBlock<'b, 'a> {
    fn add(&mut self, ins: ir::JvmInstruction) { self.instructions.push(ins); }
    fn other(&mut self, bc: BString) { self.add(ir::other(bc)); }
    fn u8(&mut self, op: u8) { self.other(vec![op]); }
    fn u8u8(&mut self, op: u8, x: u8) { self.other(vec![op, x]); }
    fn u8u16(&mut self, op: u8, x: u16) { self.other(pack::BH(op, x)); }

    fn load(&mut self, reg: u16, st: scalar::T) {
        // if we know the register to be 0/null, don't bother loading
        if self.type_info.arrs.get(reg) == array::NULL {
            self.const_(0, st);
        } else {
            self.add(ir::reg_access(reg, st, false));
        }
    }

    fn load_as(&mut self, reg: u16, cname: &'a bstr) {
        self.load(reg, scalar::OBJ);
        if self.type_info.arrs.get(reg) != array::NULL {
            if self.type_info.tainted.get(reg) {
                if cname != b"java/lang/Object" {
                    let ind = self.pool.class(cname);
                    self.u8u16(CHECKCAST, ind);
                }
            }
        }
    }

    fn load_desc(&mut self, reg: u16, desc: &'a bstr) {
        let st = scalar::T::from_desc(desc);
        if st == scalar::OBJ {
            // remember to handle arrays
            let desc = if desc[0] == b'L' {&desc[1..desc.len()-1]} else {desc};
            self.load_as(reg, desc);
        } else {
            self.load(reg, st);
        }
    }

    fn load_as_array(&mut self, reg: u16) {
        let at = self.type_info.arrs.get(reg);
        if at == array::NULL {
            self.const_null();
        } else {
            self.add(ir::reg_access(reg, scalar::OBJ, false));
            // cast to appropriate type if tainted
            if self.type_info.tainted.get(reg) {
                let ind = if at == array::INVALID {
                    // needs to be some type of object array, so just cast to Object[]
                    self.pool.class(b"[Ljava/lang/Object;")
                } else {
                    // note - will throw if actual type is boolean[] but there's not
                    // much we can do in this case
                    self.pool._class(at.to_desc().into())
                };
                self.u8u16(CHECKCAST, ind);
            }
        }
    }

    fn store(&mut self, reg: u16, st: scalar::T) {
        self.add(ir::reg_access(reg, st, true));
    }

    fn const_(&mut self, val: u64, st: scalar::T) {
        if st == scalar::OBJ { self.const_null(); return; }
        let ins = {
            let pool = if self.delay_consts { None } else { Some(&mut *self.pool) };
            ir::prim_const(st, val, pool)
        };
        self.add(ins);
    }

    fn const_null(&mut self) { self.add(ir::other_const(vec![ACONST_NULL])) }

    fn ldc(&mut self, ind: u16) {
        self.add(ir::other_const(
            if ind < 256 { vec![LDC, ind as u8] } else { pack::BH(LDC_W, ind) }
        ));
    }

    fn new_array(&mut self, desc: &'a bstr) {
        match desc {
            b"[Z" => self.u8u8(NEWARRAY, 4),
            b"[C" => self.u8u8(NEWARRAY, 5),
            b"[F" => self.u8u8(NEWARRAY, 6),
            b"[D" => self.u8u8(NEWARRAY, 7),
            b"[B" => self.u8u8(NEWARRAY, 8),
            b"[S" => self.u8u8(NEWARRAY, 9),
            b"[I" => self.u8u8(NEWARRAY, 10),
            b"[J" => self.u8u8(NEWARRAY, 11),
            _ => {
                // can be either multidim array or object array descriptor
                let desc = &desc[1..];
                let desc = if desc[0] == b'L' { &desc[1..desc.len()-1] } else {desc};
                let ind = self.pool.class(desc);
                self.u8u16(ANEWARRAY, ind);
            }
        };
    }

    fn goto(&mut self, target: u32) { self.add(ir::goto(target)); }

    fn add_except_labels(&mut self) -> (ir::LabelId, ir::LabelId) {
        let (mut s, mut e) = (0, self.instructions.len());
        // assume only Other instructions can throw
        while s < e { if let ir::Other = self.instructions[s].sub { break; } else { s+=1; } }
        while s < e { if let ir::Other = self.instructions[e-1].sub { break; } else { e-=1; } }
        assert!(s < e);
        let (stag, etag) = (EStart(self.pos), EEnd(self.pos));
        self.instructions.insert(s, ir::label(stag));
        self.instructions.insert(e+1, ir::label(etag));
        (stag, etag)
    }
}

pub fn write_instruction<'b, 'a>(pool: &'b mut (ConstantPool<'a> + 'a), method: &dex::Method<'a>, opts: Options, dex: &'a dex::DexFile<'a>, instr: &DalvikInstruction<'a>, type_info: &TypeInfo, instr_d: &HashMap<u32, &DalvikInstruction<'a>>, can_throw: bool) -> (u32, Vec<ir::JvmInstruction>) {

    let mut block = IRBlock{
        pos: instr.pos,
        pool: pool,
        type_info: type_info.clone(),
        instructions: vec![ir::label(DPos(instr.pos))],
        delay_consts: opts.delay_consts
    };

    match instr.typ {
        dt::Nop => {}
        dt::Move => {
            for st in vec![scalar::INT, scalar::OBJ, scalar::FLOAT] {
                if type_info.prims.get(instr.rb).includes(st) {
                    block.load(instr.rb, st);
                    block.store(instr.ra, st);
                }
            }
        }
        dt::MoveWide => {
            for st in vec![scalar::LONG, scalar::DOUBLE] {
                if type_info.prims.get(instr.rb).includes(st) {
                    block.load(instr.rb, st);
                    block.store(instr.ra, st);
                }
            }
        }
        dt::MoveResult => {
            block.store(instr.ra, scalar::T::from_desc(instr.prev_result.unwrap()));
        }
        dt::Return => {
            if method.id.return_type == b"V" { block.u8(RETURN); }
            else {
                let st = scalar::T::from_desc(method.id.return_type);
                block.load_desc(instr.ra, method.id.return_type);
                block.u8(IRETURN + st.ilfda());
            }
        }
        dt::Const32 => {
            let val = instr.b as u64;
            block.const_(val, scalar::INT);
            block.store(instr.ra, scalar::INT);
            block.const_(val, scalar::FLOAT);
            block.store(instr.ra, scalar::FLOAT);
            if val == 0 {
                block.const_(val, scalar::OBJ);
                block.store(instr.ra, scalar::OBJ);
            }
        }
        dt::Const64 => {
            let val = instr.long;
            block.const_(val, scalar::LONG);
            block.store(instr.ra, scalar::LONG);
            block.const_(val, scalar::DOUBLE);
            block.store(instr.ra, scalar::DOUBLE);
        }
        dt::ConstString => {
            let ind = block.pool.string(dex.string(instr.b)); block.ldc(ind);
            block.store(instr.ra, scalar::OBJ);
        }
        dt::ConstClass => {
            // Could use dex.raw_type here since the JVM doesn't care, but this is cleaner
            let ind = block.pool.class(dex.cls_type(instr.b)); block.ldc(ind);
            block.store(instr.ra, scalar::OBJ);
        }
        dt::MonitorEnter => { block.load(instr.ra, scalar::OBJ); block.u8(MONITORENTER); }
        dt::MonitorExit => { block.load(instr.ra, scalar::OBJ); block.u8(MONITOREXIT); }
        dt::CheckCast => {
            block.load(instr.ra, scalar::OBJ);
            let ind = block.pool.class(dex.cls_type(instr.b));
            block.u8u16(CHECKCAST, ind);
            block.store(instr.ra, scalar::OBJ);
        }
        dt::InstanceOf => {
            block.load(instr.rb, scalar::OBJ);
            let ind = block.pool.class(dex.cls_type(instr.c));
            block.u8u16(INSTANCEOF, ind);
            block.store(instr.ra, scalar::INT);
        }
        dt::ArrayLen => {
            block.load_as_array(instr.rb);
            block.u8(ARRAYLENGTH);
            block.store(instr.ra, scalar::INT);
        }
        dt::NewInstance => {
            let ind = block.pool.class(dex.cls_type(instr.b));
            block.u8u16(NEW, ind);
            block.store(instr.ra, scalar::OBJ);
        }
        dt::NewArray => {
            block.load(instr.rb, scalar::INT);
            block.new_array(dex.raw_type(instr.c));
            block.store(instr.ra, scalar::OBJ);
        }
        dt::FilledNewArray => {
            let args = instr.args.as_ref().unwrap();
            block.const_(args.len() as u64, scalar::INT);
            block.new_array(dex.raw_type(instr.a));
            let at = array::T::from_desc(dex.raw_type(instr.a));
            let (st, elet) = at.eletpair();
            let op = at.store_op();

            let mustpop = instr_d[&instr.pos2].typ != dt::MoveResult;
            let mut dupiter = gen_dups(args.len(), if mustpop {0} else {1});
            for (i, val) in args.iter().enumerate() {
                block.instructions.extend(dupiter.next().unwrap());
                block.const_(i as u64, scalar::INT);
                block.load(*val, st);
                block.u8(op);
            }

            // may need to pop at end
            block.instructions.extend(dupiter.next().unwrap());
        }
        dt::FillArrayData => {
            let arrdata = instr_d[&instr.b].array_data.as_ref().unwrap();
            let at = type_info.arrs.get(instr.ra);
            block.load_as_array(instr.ra);

            if at == array::NULL {
                block.u8(ATHROW);
            } else if arrdata.count == 0 {
                // fill-array-data throws a NPE if array is null even when
                // there is 0 data, so we need to add an instruction that
                // throws a NPE in this case
                block.u8(ARRAYLENGTH);
                block.u8(POP);
            } else {
                let (st, elet) = at.eletpair();
                let op = at.store_op();
                let base = if let array::T::Array(1, base) = at {base} else {panic!("bad array")};
                let mut stream = arrdata.stream.clone();

                let mut dupiter = gen_dups(arrdata.count as usize, 0);
                for i in 0..arrdata.count {
                    let val = match base {
                        Base::B => stream.u8() as i8 as u32 as u64,
                        Base::S => stream.u16() as i16 as u32 as u64,
                        Base::C => stream.u16() as u64,
                        Base::I => stream.u32() as u64,
                        Base::F => stream.u32() as u64,
                        Base::J => stream.u64(),
                        Base::D => stream.u64(),
                    };

                    block.instructions.extend(dupiter.next().unwrap());
                    block.const_(i as u64, scalar::INT);
                    block.const_(val, st);
                    block.u8(op);
                }
                assert!(dupiter.next().unwrap().len() == 0);
            }
        }
        dt::Throw => {
            block.load_as(instr.ra, b"java/lang/Throwable");
            block.u8(ATHROW);
        }
        dt::Goto => { block.goto(instr.a); }
        dt::Switch => {
            block.load(instr.ra, scalar::INT);
            let data = instr_d[&instr.b].switch_data.as_ref().unwrap();
            let default = instr.pos2;
            let mut jumps = HashMap::new();

            for (k, offset) in data.parse() {
                let target = instr.pos.wrapping_add(offset);
                if target != default {
                    jumps.insert(k as i32, target);
                }
            }
            if jumps.is_empty() {
                block.goto(default);
            } else {
                block.add(ir::switch(default, jumps));
            }
        }
        dt::Cmp => {
            let kind = instr.opcode as usize - 0x2d;
            let op = [FCMPL, FCMPG, DCMPL, DCMPG, LCMP][kind];
            let st = [scalar::FLOAT, scalar::FLOAT, scalar::DOUBLE, scalar::DOUBLE, scalar::LONG][kind];
            block.load(instr.rb, st);
            block.load(instr.rc, st);
            block.u8(op);
            block.store(instr.ra, scalar::INT);
        }
        dt::If => {
            let kind = instr.opcode as usize - 0x32;
            let prims = &type_info.prims;
            let st = prims.get(instr.ra) & prims.get(instr.rb);
            let op = if st.includes(scalar::INT) {
                block.load(instr.ra, scalar::INT);
                block.load(instr.rb, scalar::INT);
                [IF_ICMPEQ, IF_ICMPNE, IF_ICMPLT, IF_ICMPGE, IF_ICMPGT, IF_ICMPLE][kind]
            } else {
                block.load(instr.ra, scalar::OBJ);
                block.load(instr.rb, scalar::OBJ);
                [IF_ACMPEQ, IF_ACMPNE][kind]
            };
            block.add(ir::if_(op, instr.c));
        }
        dt::IfZ => {
            let kind = instr.opcode as usize - 0x38;
            let st = type_info.prims.get(instr.ra);
            let op = if st.includes(scalar::INT) {
                block.load(instr.ra, scalar::INT);
                [IFEQ, IFNE, IFLT, IFGE, IFGT, IFLE][kind]
            } else {
                block.load(instr.ra, scalar::OBJ);
                [IFNULL, IFNONNULL][kind]
            };
            block.add(ir::if_(op, instr.b));
        }
        dt::ArrayGet => {
            let at = type_info.arrs.get(instr.rb);
            if at == array::NULL {
                block.const_null();
                block.u8(ATHROW);
            } else {
                block.load_as_array(instr.rb);
                block.load(instr.rc, scalar::INT);
                block.u8(at.load_op());
                block.store(instr.ra, at.eletpair().0);
            }
        }
        dt::ArrayPut => {
            let at = type_info.arrs.get(instr.rb);
            if at == array::NULL {
                block.const_null();
                block.u8(ATHROW);
            } else {
                block.load_as_array(instr.rb);
                block.load(instr.rc, scalar::INT);
                block.load(instr.ra, at.eletpair().0);
                block.u8(at.store_op());
            }
        }
        dt::InstanceGet => {
            let field_id = dex.field_id(instr.c);
            block.load_as(instr.rb, field_id.cname);
            let ind = block.pool.field(&field_id);
            block.u8u16(GETFIELD, ind);
            block.store(instr.ra, scalar::T::from_desc(field_id.desc));
        }
        dt::InstancePut => {
            let field_id = dex.field_id(instr.c);
            block.load_as(instr.rb, field_id.cname);
            block.load_desc(instr.ra, field_id.desc);
            let ind = block.pool.field(&field_id);
            block.u8u16(PUTFIELD, ind);
        }
        dt::StaticGet => {
            let field_id = dex.field_id(instr.b);
            let ind = block.pool.field(&field_id);
            block.u8u16(GETSTATIC, ind);
            block.store(instr.ra, scalar::T::from_desc(field_id.desc));
        }
        dt::StaticPut => {
            let field_id = dex.field_id(instr.b);
            block.load_desc(instr.ra, field_id.desc);
            let ind = block.pool.field(&field_id);
            block.u8u16(PUTSTATIC, ind);
        }
        dt::InvokeVirtual | dt::InvokeSuper | dt::InvokeDirect | dt::InvokeStatic | dt::InvokeInterface => {
            let isstatic = instr.typ == dt::InvokeStatic;
            let called_id = dex.method_id(instr.a);
            let args = instr.args.as_ref().unwrap();
            let rtype = called_id.return_type;

            for (reg, desc) in args.iter().zip(called_id.spaced_param_types(isstatic)) {
                if let Some(desc) = desc { // skip long/double tops
                    block.load_desc(*reg, desc);
                }
            }

            let op = match instr.typ {
                dt::InvokeVirtual => INVOKEVIRTUAL,
                dt::InvokeSuper => INVOKESPECIAL,
                dt::InvokeDirect => INVOKESPECIAL,
                dt::InvokeStatic => INVOKESTATIC,
                dt::InvokeInterface => INVOKEINTERFACE,
                _ => unreachable!()
            };

            let bc = if instr.typ == dt::InvokeInterface {
                pack::BHBB(op, block.pool.imethod(called_id), args.len() as u8, 0)
            } else {
                pack::BH(op, block.pool.method(called_id))
            };
            block.other(bc);

            // check if we need to pop result instead of leaving on stack
            if instr_d[&instr.pos2].typ != dt::MoveResult && rtype != b"V" {
                block.u8(if scalar::T::from_desc(rtype).is_wide() {POP2} else {POP});
            }
        }
        dt::UnaryOp => {
            let data = mathops::unary(instr.opcode);
            block.load(instr.rb, data.src);
            // *not requires special handling since there's no direct Java equivalent. Instead we have to do x ^ -1
            if data.op == IXOR {
                block.u8(ICONST_M1);
            } else if data.op == LXOR {
                block.u8(ICONST_M1); block.u8(I2L);
            }
            block.u8(data.op);
            block.store(instr.ra, data.dest);
        }
        dt::BinaryOp => {
            let data = mathops::binary(instr.opcode);
            if instr.opcode >= 0xB0 { //2addr form
                block.load(instr.ra, data.src);
                block.load(instr.rb, data.src2);
            } else {
                block.load(instr.rb, data.src);
                block.load(instr.rc, data.src2);
            }
            block.u8(data.op);
            block.store(instr.ra, data.src);
        }
        dt::BinaryOpConst => {
            let data = mathops::binary_lit(instr.opcode);
            if data.op == ISUB { // rsub
                block.const_(instr.c as u64, scalar::INT);
                block.load(instr.rb, scalar::INT);
            } else {
                block.load(instr.rb, scalar::INT);
                block.const_(instr.c as u64, scalar::INT);
            }
            block.u8(data.op);
            block.store(instr.ra, scalar::INT);
        }
    }

    if can_throw { block.add_except_labels(); }
    (instr.pos, block.instructions)
}
