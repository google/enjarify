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
use std::hash::Hash;

use strings::*;

use jvm::jvmops::*;
use pack;

// Create a precomputed lookup table giving the bytecode sequence to generate
// any primative constant of 3 bytes or less plus special float values (negative
// infinity requires 4 bytes but is included anyway to simplify things elsewhere)
//
// For example
// 128 -> sipush 128
// -65535 -> iconst_m1 i2c ineg
// 2147483647 -> iconst_m1 iconst_m1 iushr
// 1L -> lconst_1
// 127L -> bipush 127 i2l
// 42.0f -> bipush 42 i2f
// -Inf -> dconst_1 dneg dconst_0 ddiv
//
// Lookup table keys are s32/s64 for ints/longs and u32/u64 for floats/doubles
// There are multiple NaN representations, so we normalize NaNs to the
// representation of all 1s (e.g. float NaN = 0xFFFFFFFF)

pub const FLOAT_SIGN: u32 = 1<<31;
pub const FLOAT_NAN: u32 = !0;
pub const FLOAT_INF: u32 = 0xFF << 23;
pub const FLOAT_NINF: u32  = FLOAT_INF ^ FLOAT_SIGN;

fn i2f(x: i32) -> u32 {
    if x == 0 { return 0; }
    if x < 0 { return i2f(-x) ^ FLOAT_SIGN; }
    let x = x as u32;
    // Don't bother implementing rounding since we'll only convert small ints
    // that can be exactly represented anyway
    let shift = x.leading_zeros() - 8;
    let exponent = shift + 127;
    (exponent << 23) | (x << shift)
}

pub const DOUBLE_SIGN: u64 = 1<<63;
pub const DOUBLE_NAN: u64 = !0;
pub const DOUBLE_INF: u64 = 0x7FF << 52;
pub const DOUBLE_NINF: u64 = DOUBLE_INF ^ DOUBLE_SIGN;

fn i2d(x: i32) -> u64 {
    if x == 0 { return 0; }
    if x < 0 { return i2d(-x) ^ DOUBLE_SIGN; }
    let x = x as u64;
    let shift = x.leading_zeros() as u64 - 11;
    let exponent = shift + 1023;
    (exponent << 52) | (x << shift)
}

// add if value is shorter then current best
fn add<K: Eq + Hash>(d: &mut HashMap<K, BString>, k: K, v: BString) {
    // todo: avoid unnecessary extra lookup?
    if let Some(ref cur) = d.get(&k) {
        if cur.len() <= v.len() { return; }
    }
    d.insert(k, v);
}

fn concat(s1: &BString, s2: BString) -> BString { let mut t = s1.clone(); t.extend(s2); t }

pub fn create() -> (HashMap<i32, BString>, HashMap<u32, BString>, HashMap<i64, BString>, HashMap<u64, BString>){
    // println!("beginning create");
    // int constants
    let mut all_ints = HashMap::with_capacity(65671);

    // 1 byte ints
    for i in -1i32..6 {
        all_ints.insert(i, vec![(i + ICONST_0 as i32) as u8]);
    }
    let int_1s = -1i32..6;

    // 2 byte ints
    for i in -128i32..128 { add(&mut all_ints, i, vec![BIPUSH, i as u8]); }
    all_ints.insert(65535, vec![ICONST_M1, I2C]);
    // Sort for determinism. Otherwise -0x80000000 could be either
    // 1 << -1 or -1 << -1, for example
    let int_2s = {
        let mut t: Vec<_> = all_ints.iter().filter(|&(k, v)| v.len() == 2).map(|(k, v)| *k).collect();
        t.sort();
        t
    };

    // 3 byte ints
    for i in -32768i32..32768 { add(&mut all_ints, i, pack::Bh(SIPUSH, i as i16)); }
    for i in int_2s.clone() {
        let val = concat(&all_ints[&i], vec![I2C]);
        add(&mut all_ints, i as u16 as i32, val);
        let val = concat(&all_ints[&i], vec![INEG]);
        add(&mut all_ints, i.wrapping_neg(), val);
    }
    for x in int_1s.clone() { for y in int_1s.clone() {
        let xy = concat(&all_ints[&x], all_ints[&y].clone());
        add(&mut all_ints, x.wrapping_shl(y as u32), concat(&xy, vec![ISHL]));
        add(&mut all_ints, x.wrapping_shr(y as u32), concat(&xy, vec![ISHR]));
        add(&mut all_ints, (x as u32).wrapping_shr(y as u32) as i32, concat(&xy, vec![IUSHR]));
    }}

    // long constants
    let mut all_longs = HashMap::with_capacity(257);
    for i in 0..2 { all_longs.insert(i as i64, vec![LCONST_0 + i as u8]); }
    for i in int_1s.clone() { add(&mut all_longs, i as i64, concat(&all_ints[&i], vec![I2L])); }
    for i in int_2s.clone() { add(&mut all_longs, i as i64, concat(&all_ints[&i], vec![I2L])); }

    // float constants
    let mut all_floats = HashMap::with_capacity(177);
    for i in 0..2 { all_floats.insert(i2f(i), vec![FCONST_0 + i as u8]); }
    for i in int_1s.clone() { add(&mut all_floats, i2f(i), concat(&all_ints[&i], vec![I2F])); }
    for i in int_2s.clone() { add(&mut all_floats, i2f(i), concat(&all_ints[&i], vec![I2F])); }
    // hardcode unusual float values for simplicity
    all_floats.insert(FLOAT_SIGN, vec![FCONST_0, FNEG]); // -0.0
    all_floats.insert(FLOAT_NAN, vec![FCONST_0, FCONST_0, FDIV]);
    all_floats.insert(FLOAT_INF, vec![FCONST_1, FCONST_0, FDIV]);
    all_floats.insert(FLOAT_NINF, vec![FCONST_1, FNEG, FCONST_0, FDIV]);

    // double constants
    let mut all_doubles = HashMap::with_capacity(218);
    for i in 0..2 { all_doubles.insert(i2d(i), vec![DCONST_0 + i as u8]); }
    for i in int_1s.clone() { add(&mut all_doubles, i2d(i), concat(&all_ints[&i], vec![I2D])); }
    for i in int_2s.clone() { add(&mut all_doubles, i2d(i), concat(&all_ints[&i], vec![I2D])); }
    // hardcode unusual float values for simplicity
    all_doubles.insert(DOUBLE_SIGN, vec![DCONST_0, DNEG]); // -0.0
    all_doubles.insert(DOUBLE_NAN, vec![DCONST_0, DCONST_0, DDIV]);
    all_doubles.insert(DOUBLE_INF, vec![DCONST_1, DCONST_0, DDIV]);
    all_doubles.insert(DOUBLE_NINF, vec![DCONST_1, DNEG, DCONST_0, DDIV]);
    // println!("end create");
    (all_ints, all_floats, all_longs, all_doubles)
}
