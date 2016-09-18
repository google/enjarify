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

use byteio::Writer;
use dex;
use error;
use super::constantpool::ConstantPool;
use super::ir::PrimConstant;
use super::optimization::{consts, jumps, registers, stack};
use super::optimization::options::Options;
use super::writeir::{IRWriter, write_bytecode};

pub fn get_code_ir<'b, 'a>(pool: &'b mut (ConstantPool<'a> + 'a), method: &dex::Method<'a>, opts: Options) -> Option<IRWriter> {
    method.code.as_ref().map(|ref code| {
        let mut irdata = write_bytecode(pool, method, code, opts);

        if opts.inline_consts { stack::inline_consts(&mut irdata); }
        if opts.copy_propagation { registers::copy_propagation(&mut irdata); }
        if opts.remove_unused_regs { registers::remove_unused_registers(&mut irdata); }
        if opts.dup2ize { stack::dup2ize(&mut irdata); }

        if opts.prune_store_loads {
            stack::prune_store_loads(&mut irdata);
            if opts.remove_unused_regs { registers::remove_unused_registers(&mut irdata); }
        }

        if opts.sort_registers {
            registers::sort_allocate_registers(&mut irdata);
        } else {
            registers::simple_allocate_registers(&mut irdata);
        }
        irdata
    })
}

pub fn finish_code_attrs<'a>(pool: &mut (ConstantPool<'a> + 'a), code_irs: Vec<Option<IRWriter>>, opts: Options) -> HashMap<u32, BString> {
    let mut code_irs = {
        let mut v = Vec::with_capacity(code_irs.len());
        for x in code_irs { if let Some(irw) = x {v.push(irw);} }
        v
    };

    // if we have any code, make sure to reserve pool slot for attr name
    if !code_irs.is_empty() { pool.utf8(b"Code"); }

    if opts.delay_consts {
        // In the rare case where the class references too many constants to fit in
        // the constant pool, we can workaround this by replacing primative constants
        // e.g. ints, longs, floats, and doubles, with a sequence of bytecode instructions
        // to generate that constant. This obviously increases the size of the method's
        // bytecode, so we ideally only want to do it to constants in short methods.

        // First off, we find which methods are potentially too long. If a method
        // will be under 65536 bytes even with all constants replaced, then it
        // will be ok no matter what we do.
        {
        let longirs = code_irs.iter().filter(|&irw| irw.upper_bound() >= 65536).collect();

        // Now allocate constants used by potentially long methods
        consts::allocate_required_constants(pool, longirs);
        }

        // If there's space left in the constant pool, allocate constants used by short methods
        for irdata in code_irs.iter_mut() {
            for ins in irdata.instructions.iter_mut() {
                if let PrimConstant(ref data) = ins.sub {
                    data.fix_with_pool(pool, &mut ins.bytecode);
                }
            }
        }
    }

    code_irs.into_iter().map(
        |irdata| (irdata.method_idx, write_code_attr(irdata, opts))
    ).collect()
}

fn write_code_attr<'a>(mut irdata: IRWriter, opts: Options) -> BString {
    let nregs = irdata.numregs.unwrap();
    jumps::optimize_jumps(&mut irdata);
    let (mut bytecode, excepts) = jumps::create_bytecode(irdata);

    // If code is too long and optimization is off, raise exception so we can
    // retry with optimization. If it is still too long with optimization,
    // don't raise an error, since a class with illegally long code is better
    // than no output at all.
    if bytecode.len() > 65535 && opts != Options::all() {
        error::classfile_limit_exceeded();
    }

    let expectlen = 12 + bytecode.len() + 8 * excepts.len();
    let mut stream = Writer(Vec::with_capacity(expectlen));
    // For simplicity, don't bother calculating the actual maximum stack height
    // of the generated code. Instead, just use a value that will always be high
    // enough. Note that just setting this to 65535 is a bad idea since it tends
    // to cause StackOverflowErrors under default JVM memory settings
    stream.u16(300); stream.u16(nregs);

    stream.u32(bytecode.len() as u32);
    stream.0.append(&mut bytecode);
    stream.u16(excepts.len() as u16);
    for (s, e, h, c) in excepts {
        stream.u16(s); stream.u16(e); stream.u16(h); stream.u16(c);
    }

    // attributes
    stream.u16(0);
    assert!(stream.0.len() == expectlen);
    stream.0
}
