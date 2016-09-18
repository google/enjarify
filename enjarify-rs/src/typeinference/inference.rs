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
use std::collections::{HashMap,HashSet};

use strings::*;

use dex;
use dalvik::*;
use dalvik::DalvikType as dt;
use flags;
use jvm::jvmops::*;
use jvm::mathops;
use treelist::TreePtr;
use super::array;
use super::scalar;

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

#[derive(Clone, Default)]
pub struct TypeInfo {
    pub prims: TreePtr<scalar::T>,
    pub arrs: TreePtr<array::T>,
    pub tainted: TreePtr<bool>,
}
impl PartialEq for TypeInfo {
  fn eq(&self, rhs: &Self) -> bool { self.prims.is(&rhs.prims) && self.arrs.is(&rhs.arrs) && self.tainted.is(&rhs.tainted) }
}
// impl Eq for TypeInfo {}
impl TypeInfo {
    fn move_(&mut self, src: u16, dest: u16, wide: bool) {
        if wide { self.move_(src + 1, dest + 1, false); }
        let t = self.prims.get(src); self.prims.set(dest, t);
        let t = self.arrs.get(src); self.arrs.set(dest, t);
        let t = self.tainted.get(src); self.tainted.set(dest, t);
    }

    fn assign(&mut self, reg: u16, st: scalar::T) { self.assign_(reg, st, array::INVALID) }
    fn assign_(&mut self, reg: u16, st: scalar::T, at: array::T) { self.assign__(reg, st, at, false) }
    fn assign__(&mut self, reg: u16, st: scalar::T, at: array::T, taint: bool) {
        self.prims.set(reg, st);
        self.arrs.set(reg, at);
        self.tainted.set(reg, taint);
    }
    fn assign2(&mut self, reg: u16, st: scalar::T) {
        self.assign_(reg, st, array::INVALID);
        self.assign_(reg+1, scalar::INVALID, array::INVALID);
    }

    fn assign_from_desc(&mut self, reg: u16, desc: &bstr) {
        let st = scalar::T::from_desc(desc);
        if st.is_wide() {
            self.assign2(reg, st);
        } else {
            self.assign_(reg, st, array::T::from_desc(desc));
        }
    }

    fn merge(&mut self, rhs: &Self) -> bool {
        self.prims.merge(&rhs.prims, &|x, y| x & y, true) |
        self.arrs.merge(&rhs.arrs, &|x, y| x.merge(y), false) |
        self.tainted.merge(&rhs.tainted, &|x, y| x || y, false)
    }

    fn from_params<'a>(method: &dex::Method<'a>, nregs: u16) -> Self {
        let mut res = TypeInfo::default();
        let isstatic = method.access as u16 & flags::ACC_STATIC != 0;
        let full_ptypes = method.id.spaced_param_types(isstatic);
        let offset = nregs - full_ptypes.len() as u16;

        for (i, desc) in full_ptypes.iter().enumerate() {
            if let Some(desc) = *desc {
                res.assign_(offset + i as u16, scalar::T::from_desc(desc), array::T::from_desc(desc));
            }
        }
        res
    }
}

fn math_throws(jvmop: u8) -> bool { match jvmop { IDIV | IREM | LDIV | LREM => true,  _ => false, } }
fn prune_handlers<'a>(instr_d: &HashMap<u32, &DalvikInstruction<'a>>, all_handlers: HashMap<u32, Vec<dex::CatchItem<'a>>>) -> HashMap<u32, Vec<dex::CatchItem<'a>>> {
    let mut result = HashMap::new();
    for (pos, handlers) in all_handlers {
        let instr = instr_d[&pos];
        if !instr.typ.is_pruned_throw() { continue; }
        // if math op, make sure it is int div/rem
        if instr.typ == dt::BinaryOp && !math_throws(mathops::binary(instr.opcode).op) { continue; }
        if instr.typ == dt::BinaryOpConst && !math_throws(mathops::binary_lit(instr.opcode).op) { continue; }

        let mut types = HashSet::new();
        let mut pruned = Vec::new();
        for citem in handlers {
            // if multiple handlers with same catch type, only include the first
            if types.insert(citem.ctype) {
                pruned.push(citem);
            }
            // stop as soon as we reach a catch all handler
            if citem.ctype == &b"java/lang/Throwable"[..] { break; }
        }

        if pruned.len() > 0 { result.insert(pos, pruned); }
    }
    result
}

////////////////////////////////////////////////////////////////////////////////
fn visit_normal<'a>(dex: &dex::DexFile<'a>, instr: &DalvikInstruction<'a>, cur: &mut TypeInfo) {
    match instr.typ {
        dt::ConstString | dt::ConstClass | dt::NewInstance => { cur.assign(instr.ra, scalar::OBJ); }
        dt::InstanceOf | dt::ArrayLen | dt::Cmp | dt::BinaryOpConst => { cur.assign(instr.ra, scalar::INT); }
        dt::Move => { cur.move_(instr.rb, instr.ra, false); }
        dt::MoveWide => { cur.move_(instr.rb, instr.ra, true); }
        dt::MoveResult => { cur.assign_from_desc(instr.ra, instr.prev_result.unwrap()); }
        dt::Const32 => {
            if instr.b == 0 {
                cur.assign_(instr.ra, scalar::ZERO, array::NULL);
            } else {
                cur.assign(instr.ra, scalar::C32);
            }
        }
        dt::Const64 => { cur.assign2(instr.ra, scalar::C64); }
        dt::CheckCast => {
            let at = array::T::from_desc(dex.raw_type(instr.b));
            let at = at.narrow(cur.arrs.get(instr.ra));
            cur.assign_(instr.ra, scalar::OBJ, at);
        }
        dt::NewArray => { cur.assign_(instr.ra, scalar::OBJ, array::T::from_desc(dex.raw_type(instr.c))); }
        dt::ArrayGet => {
            let (st, at) = cur.arrs.get(instr.rb).eletpair();
            cur.assign_(instr.ra, st, at);
        }
        dt::InstanceGet => { cur.assign_from_desc(instr.ra, dex.field_id(instr.c).desc); }
        dt::StaticGet => { cur.assign_from_desc(instr.ra, dex.field_id(instr.b).desc); }
        dt::UnaryOp => {
            let st = mathops::unary(instr.opcode).dest;
            if st.is_wide() { cur.assign2(instr.ra, st); } else { cur.assign(instr.ra, st); }
        }
        dt::BinaryOp => {
            let st = mathops::binary(instr.opcode).src;
            if st.is_wide() { cur.assign2(instr.ra, st); } else { cur.assign(instr.ra, st); }
        }
        _ => {}
    }
}

struct State<'a>(HashMap<u32, TypeInfo>, HashSet<u32>, &'a HashMap<u32, &'a DalvikInstruction<'a>>);
impl<'a> State<'a> {
    fn do_merge(&mut self, pos: u32, new: &TypeInfo) {
        // println!("domerge {} {}", pos, self.2.contains_key(&pos));
        if !self.2.contains_key(&pos) { return; }

        // unnecessary extra lookup?
        if self.0.contains_key(&pos) {
            if self.0.get_mut(&pos).unwrap().merge(new) {
                self.1.insert(pos);
            }
        } else {
            self.0.insert(pos, new.clone());
            self.1.insert(pos);
        }
        // println!("[{}] tainted[13] {}", pos, self.0[&pos].tainted.get(13));
    }
}

pub fn do_inference<'a>(method: &dex::Method<'a>, code: &dex::CodeItem<'a>, instr_d: &HashMap<u32, &DalvikInstruction<'a>>) -> (HashMap<u32, TypeInfo>, HashMap<u32, Vec<dex::CatchItem<'a>>>) {

    let all_handlers = {
        let mut all_handlers = HashMap::new();
        for tryi in &code.tries {
            for instr in &code.bytecode {
                if tryi.start < instr.pos2 && tryi.end > instr.pos {
                    let val = all_handlers.entry(instr.pos).or_insert_with(|| Vec::new());
                    val.extend(tryi.catches.clone()); // unnecessary copy?
                }
            }
        }
        prune_handlers(instr_d, all_handlers)
    };

    let mut state = State(HashMap::with_capacity(code.bytecode.len()), HashSet::new(), instr_d);
    state.0.insert(0, TypeInfo::from_params(method, code.nregs));
    state.1.insert(0);

    // iterate until convergence
    while !state.1.is_empty() {
        for instr in &code.bytecode {
            if !state.1.remove(&instr.pos) { continue; }
            // println!("instr {} {:?}", instr.pos, instr);

            let cur = state.0.get(&instr.pos).unwrap().clone();
            let mut after = cur.clone();
            visit_normal(method.dex, instr, &mut after);

            // handle control flow
            assert!(instr.implicit_casts.is_none() || instr.typ == dt::IfZ);
            match instr.typ {
                dt::Goto => { state.do_merge(instr.a, &after); }
                dt::If => {
                    state.do_merge(instr.c, &after);
                    state.do_merge(instr.pos2, &after);
                }
                dt::IfZ => {
                    let mut after2 = after.clone();
                    // implicit casts
                    if let Some((desc_ind, ref regs)) = instr.implicit_casts {
                        let at = array::T::from_desc(method.dex.raw_type(desc_ind));
                        let mut result = after.clone();
                        for reg in regs {
                            let st = after.prims.get(*reg);
                            let at = after.arrs.get(*reg).narrow(at);
                            result.assign__(*reg, st, at, true);
                        }

                        match instr.opcode {
                            0x38 => { after = result; }
                            0x39 => { after2 = result; }
                            _ => unreachable!()
                        }
                    }

                    // println!("pos2 {} args {:?}", instr.pos2, instr.deref());
                    state.do_merge(instr.b, &after2);
                    state.do_merge(instr.pos2, &after);
                }
                dt::Switch => {
                    let switch_data = &instr_d[&instr.b].switch_data;
                    for (_, offset) in switch_data.as_ref().unwrap().parse() {
                        state.do_merge(instr.pos.wrapping_add(offset), &after);
                    }
                    state.do_merge(instr.pos2, &after);
                }
                dt::Return | dt::Throw => {}
                // regular instructions
                _ => { state.do_merge(instr.pos2, &after); }
            }

            if let Some(handlers) = all_handlers.get(&instr.pos) {
                for item in handlers {
                    state.do_merge(item.target, &cur);
                }
            }
        }
    }
    (state.0, all_handlers)
}
