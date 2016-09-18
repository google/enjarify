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
use std::panic;

use strings::*;

use byteio::Writer;
use dex::*;
use error;
use flags;

use super::constantpool::{ConstantPool, simple_pool, split_pool};
use super::optimization::options::Options;
use super::writebytecode::{get_code_ir, finish_code_attrs};

fn write_field<'a>(pool: &mut ConstantPool<'a>, stream: &mut Writer, field: &Field<'a>) {
    stream.u16(field.access as u16 & flags::FIELD_FLAGS);
    stream.u16(pool.utf8(field.id.name));
    stream.u16(pool.utf8(field.id.desc));

    let cval = &field.constant_value;
    if let ConstantValue::None = *cval {
        stream.u16(0); // No attributes
    } else {
        stream.u16(1);
        stream.u16(pool.utf8(&b"ConstantValue"[..]));
        stream.u32(2);

        let index = match field.id.desc {
            b"Z" | b"B" | b"C" | b"S" | b"I" =>
                { if let ConstantValue::Const32(x) = *cval {pool.int(x)} else {panic!("")} },
            b"F" =>
                { if let ConstantValue::Const32(x) = *cval {pool.float(x)} else {panic!("")} },
            b"J" =>
                { if let ConstantValue::Const64(x) = *cval {pool.long(x)} else {panic!("")} },
            b"D" =>
                { if let ConstantValue::Const64(x) = *cval {pool.double(x)} else {panic!("")} },
            b"Ljava/lang/String;" =>
                { if let ConstantValue::String(x) = *cval {pool.string(x)} else {panic!("")} },
            b"Ljava/lang/Class;" =>
                { if let ConstantValue::Type(x) = *cval {pool.class(x)} else {panic!("")} },
            _ => {0},
        };
        stream.u16(index);
    }
}

fn write_methods<'a>(pool: &mut (ConstantPool<'a> + 'a), stream: &mut Writer, methods: Vec<Method<'a>>, opts: Options) {
    let code_irs = methods.iter().map(|ref m| get_code_ir(pool, &m, opts)).collect();
    let code_attrs = finish_code_attrs(pool, code_irs, opts);

    stream.u16(methods.len() as u16);
    for method in methods {
        stream.u16(method.access as u16 & flags::METHOD_FLAGS);
        stream.u16(pool.utf8(method.id.name));
        stream.u16(pool._utf8(method.id.desc.into()));

        match code_attrs.get(&method.id.method_idx) {
            Some(data) => {
                stream.u16(1);
                stream.u16(pool.utf8(&b"Code"[..]));
                stream.u32(data.len() as u32);
                stream.write(&data);
            },
            None => {
                stream.u16(0); // no attributes
            }
        }
    }
}

fn after_pool<'a, 'b>(cls: &'b DexClass<'a>, opts: Options) -> (Box<ConstantPool<'a> + 'a>, Writer) {
    let mut stream = Writer::default();
    let mut pool = if opts.split_pool {
        split_pool()
    } else {
        simple_pool()
    };

    stream.u16(cls.access as u16 & flags::CLASS_FLAGS);
    stream.u16(pool.class(cls.name));
    stream.u16(match cls.super_ {
        Some(v) => pool.class(v),
        None => 0,
    });

    stream.u16(cls.interfaces.len() as u16);
    for interface in &cls.interfaces { stream.u16(pool.class(interface)); }

    let (fields, methods) = cls.parse_data();
    stream.u16(fields.len() as u16);
    for field in &fields { write_field(&mut *pool, &mut stream, field); }

    write_methods(&mut *pool, &mut stream, methods, opts);

    // attributes
    stream.u16(0);
    (pool, stream)
}

pub fn to_class_file<'a>(cls: &DexClass<'a>, opts: Options) -> BString {
    let mut stream = Writer::default();
    stream.u32(0xCAFEBABE);
    // bytecode version 49.0
    stream.u16(0);
    stream.u16(49);

    // Optimistically try translating without optimization to speed things up
    // if the resulting code is too big, retry with optimization
    // let (pool, rest_stream) = after_pool(cls, opts);
    // todo: error handling

    let (pool, rest_stream) = panic::catch_unwind(|| {
        after_pool(cls, opts)
    }).unwrap_or_else(|err| {
        // rethrow the panic if it isn't a ClassfileLimitExceeded
        if let Err(err) = err.downcast::<error::ClassfileLimitExceeded>() {
            panic::resume_unwind(err);
        }

        after_pool(cls, Options::all())
    });

    // write constant pool
    pool.write(&mut stream);
    // write rest of file
    stream.write(&rest_stream.0);
    stream.0
}
