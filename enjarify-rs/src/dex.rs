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
use std::collections::HashSet;
// use std::fmt;
use std::str;

use strings::*;
use byteio::Reader;
use dalvik::{DalvikInstruction, parse_bytecode};

const NO_INDEX: u32 = 0xFFFFFFFF;

fn type_list<'a>(dex: &'a DexFile<'a>, off: u32, parse_cls_desc: bool) -> Vec<&'a bstr> {
    if off == 0 { return Vec::new(); }

    let mut st = dex.stream(off);
    let size = st.u32();
    let mut result = Vec::with_capacity(size as usize);
    for _ in 0..size {
        let idx = st.u16() as u32;
        result.push(if parse_cls_desc {dex.cls_type(idx)} else {dex.raw_type(idx)});
    }
    result
}

pub enum ConstantValue<'a> {
    None,
    Invalid,
    Const32(u32),
    Const64(u64),
    String(&'a bstr),
    Type(&'a bstr),
}

fn encoded_value<'a>(dex: &'a DexFile<'a>, stream: &mut Reader<'a>) -> ConstantValue<'a> {
    let tag = stream.u8() as u32;
    let (vtype, varg) = (tag & 31, tag >> 5);

    match vtype {
        0x1c => // ARRAY
            { for _ in 0..stream.uleb128() { encoded_value(dex, stream); }; return ConstantValue::Invalid; }
        0x1d => // ANNOTATION
            { stream.uleb128(); for _ in 0..stream.uleb128() { stream.uleb128(); encoded_value(dex, stream); }; return ConstantValue::Invalid; }
        0x1e => // NULL
            { return ConstantValue::None; }
        0x1f => // BOOLEAN
            { return ConstantValue::Const32(varg); }
        _ => {}
    };

    // the rest are an int encoded into varg + 1 bytes in some way
    let size = varg + 1;
    let mut val = 0u64;
    for i in 0..size {
        val += (stream.u8() as u64) << (i * 8);
    }

    match vtype {
        0x00 => ConstantValue::Const32(val as i8 as u32), // BYTE
        0x02 => ConstantValue::Const32(val as i16 as u32), // SHORT
        0x03 => ConstantValue::Const32(val as u16 as u32), // CHAR
        0x04 => ConstantValue::Const32(val as i32 as u32), // INT
        0x06 => ConstantValue::Const64(val), // LONG

        // floats are 0 extended to the right
        0x10 => ConstantValue::Const32((val << (32 - size*8)) as u32), // FLOAT
        0x11 => ConstantValue::Const64(val << (64 - size*8)), // DOUBLE

        0x17 => ConstantValue::String(dex.string(val as u32)), // STRING
        0x18 => ConstantValue::Type(dex.cls_type(val as u32)), // TYPE
        _ => ConstantValue::None
    }
}

pub struct FieldId<'a> {
    pub cname: &'a bstr,
    pub name: &'a bstr,
    pub desc: &'a bstr,
}
impl<'a> FieldId<'a> {
    fn new(dex: &'a DexFile<'a>, field_idx: u32) -> FieldId<'a> {
        let mut st = dex.stream(dex.field_ids.off + field_idx*8);
        FieldId{
            cname: dex.cls_type(st.u16() as u32),
            desc: dex.raw_type(st.u16() as u32),
            name: dex.string(st.u32()),
        }
    }
}

pub struct Field<'a> {
    pub id: FieldId<'a>,
    pub access: u32,
    pub constant_value: ConstantValue<'a>,
}
impl<'a> Field<'a> {
    fn new(dex: &'a DexFile<'a>, field_idx: u32, access: u32) -> Field<'a> {
        Field{
            id: FieldId::new(dex, field_idx),
            access: access,
            constant_value: ConstantValue::None,
        }
    }
}

#[derive(Clone, Copy)]
pub struct CatchItem<'a> {
    pub ctype: &'a bstr,
    pub target: u32,
}

pub struct TryItem<'a> {
    pub start: u32,
    pub end: u32,
    handler_off: u32,
    pub catches: Vec<CatchItem<'a>>,
}

pub struct CodeItem<'a> {
    pub nregs: u16,
    pub tries: Vec<TryItem<'a>>,
    pub bytecode: Vec<DalvikInstruction<'a>>,
}
impl<'a> CodeItem<'a> {
    fn new(dex: &'a DexFile<'a>, offset: u32) -> CodeItem<'a> {
        let mut stream = dex.stream(offset);
        let nregs = stream.u16();
        let _ins_size = stream.u16();
        let _outs_size = stream.u16();
        let tries_size = stream.u16();
        let _debug_off = stream.u32();
        let insns_size = stream.u32();
        let code_start_st = stream.clone();

        let shorts: Vec<u16> = (0..insns_size).map(|_| stream.u16()).collect();

        if tries_size != 0 && insns_size & 1 != 0 {
            stream.u16(); // padding
        }

        let mut tries = Vec::with_capacity(tries_size as usize);
        for _ in 0..tries_size {
            let (start, count, handler_off) = (stream.u32(), stream.u16(), stream.u16());
            tries.push(TryItem{
                start: start,
                end: start + count as u32,
                handler_off: handler_off as u32,
                catches: Vec::new(),
            });
        }

        // Now get catches
        let list_off_st = stream.clone();
        for item in &mut tries {
            let mut stream = list_off_st.offset(item.handler_off);
            let size = stream.sleb128();
            item.catches.reserve(size.abs() as usize + 1);
            for _ in 0..size.abs() {
                item.catches.push(CatchItem{
                    ctype: dex.cls_type(stream.uleb128()),
                    target: stream.uleb128(),
                });
            }
            if size <= 0 {
                item.catches.push(CatchItem{
                    ctype: &b"java/lang/Throwable"[..],
                    target: stream.uleb128(),
                });
            }
        }

        let mut catch_addrs = HashSet::new();
        for item in &tries { for catch in &item.catches { catch_addrs.insert(catch.target); } }

        CodeItem{
            nregs: nregs,
            tries: tries,
            bytecode: parse_bytecode(dex, &code_start_st, &shorts, &catch_addrs),
        }
    }
}

pub struct MethodId<'a> {
    pub cname: &'a bstr,
    pub name: &'a bstr,
    pub desc: BString,
    pub return_type: &'a bstr,
    pub method_idx: u32,

    cdesc: &'a bstr,
    param_types: Vec<&'a bstr>,
}
impl<'a> MethodId<'a> {
    fn new(dex: &'a DexFile<'a>, method_idx: u32) -> MethodId<'a> {
        let mut st = dex.stream(dex.method_ids.off + method_idx*8);
        let cname_idx = st.u16() as u32;
        let cname = dex.cls_type(cname_idx);
        let proto_idx = st.u16() as u32;
        let name = dex.string(st.u32());

        st = dex.stream(dex.proto_ids.off + proto_idx * 12);
        st.u32();
        let return_type = dex.raw_type(st.u32());
        let param_types = type_list(dex, st.u32(), false);

        let mut desc = vec![b'('];
        for part in &param_types {
            desc.extend_from_slice(part);
        }
        desc.push(b')');
        desc.extend_from_slice(return_type);

        MethodId{
            cname: cname,
            name: name,
            desc: desc,
            return_type: return_type,
            method_idx: method_idx,

            cdesc: dex.raw_type(cname_idx),
            param_types: param_types,
        }
    }

    pub fn spaced_param_types(&self, isstatic: bool) -> Vec<Option<&'a bstr>> {
        let mut res = Vec::with_capacity(self.param_types.len() + 1);
        if !isstatic { res.push(Some(self.cdesc)); }

        for param in &self.param_types {
            res.push(Some(param));
            if param[0] == b'J' || param[0] == b'D' { res.push(None); }
        }
        res
    }
}
// impl<'a> fmt::Debug for MethodId<'a> {
//     fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
//         write!(f, "{}.{}->{}", to_str(self.cname), to_str(self.name), to_string(self.desc.clone()))
//     }
// }

pub struct Method<'a> {
    pub id: MethodId<'a>,
    pub dex: &'a DexFile<'a>,
    pub access: u32,
    pub code: Option<CodeItem<'a>>,
}
impl<'a> Method<'a> {
    fn new(dex: &'a DexFile<'a>, method_idx: u32, access: u32, code_off: u32) -> Method<'a> {
        // if code_off != 0 { println!("parsing code for {:?}", MethodId::new(dex, method_idx)); }
        let code = if code_off != 0 { Some(CodeItem::new(dex, code_off)) } else { None };
        Method{
            id: MethodId::new(dex, method_idx),
            dex: dex,
            access: access,
            code: code,
        }
    }
}

pub struct DexClass<'a> {
    dex: &'a DexFile<'a>,
    pub name: &'a bstr,
    pub access: u32,
    pub super_: Option<&'a bstr>,
    pub interfaces: Vec<&'a bstr>,
    data_off: u32,
    constant_values_off: u32,
}
impl<'a> DexClass<'a> {
    fn new(dex: &'a DexFile<'a>, base_off: u32, i: u32) -> DexClass<'a> {
        let mut stream = dex.stream(base_off + i*32);
        let name = stream.u32();
        let access = stream.u32();
        let super_ = stream.u32();
        let interfaces = stream.u32();
        let _srcfile = stream.u32();
        let _annotations = stream.u32();
        let data_off = stream.u32();
        let constant_values_off = stream.u32();

        DexClass{
            dex: dex,
            name: dex.cls_type(name),
            access: access,
            super_: dex.cls_type_opt(super_),
            interfaces: type_list(dex, interfaces, true),

            data_off: data_off,
            constant_values_off: constant_values_off,
        }
    }

    pub fn parse_data(&self) -> (Vec<Field<'a>>, Vec<Method<'a>>) {
        if self.data_off == 0 {
            return (Vec::new(), Vec::new());
        }

        let mut stream = self.dex.stream(self.data_off);
        let numstatic = stream.uleb128();
        let numinstance = stream.uleb128();
        let numdirect = stream.uleb128();
        let numvirtual = stream.uleb128();

        let mut fields = Vec::with_capacity((numstatic + numinstance) as usize);
        for num in &[numstatic, numinstance] {
            let mut field_idx = 0;
            for _ in 0..*num {
                field_idx += stream.uleb128();
                fields.push(Field::new(self.dex, field_idx, stream.uleb128()));
            }
        }

        let mut methods = Vec::with_capacity((numdirect + numvirtual) as usize);
        for num in &[numdirect, numvirtual] {
            let mut method_idx = 0;
            for _ in 0..*num {
                method_idx += stream.uleb128();
                methods.push(Method::new(self.dex, method_idx, stream.uleb128(), stream.uleb128()));
            }
        }

        if self.constant_values_off != 0 {
            stream = self.dex.stream(self.constant_values_off);
            let size = stream.uleb128();
            for i in 0..size {
                fields[i as usize].constant_value = encoded_value(self.dex, &mut stream)
            }
        }

        (fields, methods)
    }
}

struct SizeOff { size: u32, off: u32 }
impl SizeOff {
    fn new(stream: &mut Reader) -> SizeOff {
        SizeOff{size: stream.u32(), off: stream.u32() }
    }
}

#[allow(dead_code)] //data is not used
pub struct DexFile<'a> {
    raw: &'a bstr,
    string_ids: SizeOff,
    type_ids: SizeOff,
    proto_ids: SizeOff,
    field_ids: SizeOff,
    method_ids: SizeOff,
    class_defs: SizeOff,
    data: SizeOff,
}
impl<'a> DexFile<'a> {
    pub fn new(data: &bstr) -> DexFile {
        let mut stream = Reader(data);
        stream.read(36);
        if stream.u32() != 0x70 {
            println!("Warning, unexpected header size!");
        }
        if stream.u32() != 0x12345678 {
            println!("Warning, unexpected endianess tag!");
        }

        SizeOff::new(&mut stream);
        stream.u32();

        DexFile {
            raw: data,
            string_ids: SizeOff::new(&mut stream),
            type_ids: SizeOff::new(&mut stream),
            proto_ids: SizeOff::new(&mut stream),
            field_ids: SizeOff::new(&mut stream),
            method_ids: SizeOff::new(&mut stream),
            class_defs: SizeOff::new(&mut stream),
            data: SizeOff::new(&mut stream),
        }
    }
    pub fn parse_classes(&'a self) -> Vec<DexClass<'a>> {
        let mut classes = Vec::with_capacity(self.class_defs.size as usize);
        for i in 0..self.class_defs.size {
            classes.push(DexClass::new(&self, self.class_defs.off, i));
        }
        classes
    }

    fn stream(&self, offset: u32) -> Reader<'a> {
        Reader(self.raw.split_at(offset as usize).1)
    }
    fn u32(&self, i: u32) -> u32 {
        self.stream(i).u32()
    }

    pub fn string(&self, i: u32) -> &bstr {
        let data_off = self.u32(self.string_ids.off + i*4);
        let mut stream = self.stream(data_off);
        stream.uleb128(); // Ignore decoded length
        stream.cstr()
    }

    pub fn raw_type(&self, i: u32) -> &bstr {
        assert!(i < self.type_ids.size);
        self.string(self.u32(self.type_ids.off + i*4))
    }

    pub fn cls_type(&self, i: u32) -> &bstr {
        let data = self.raw_type(i);
        if data[0] == b'L' {
            &data[1..data.len()-1]
        } else { data }
    }

    fn cls_type_opt(&self, i: u32) -> Option<&bstr> {
        if i == NO_INDEX { None } else { Some(self.cls_type(i)) }
    }

    pub fn field_id(&self, i: u32) -> FieldId { FieldId::new(self, i) }
    pub fn method_id(&self, i: u32) -> MethodId { MethodId::new(self, i) }
}
