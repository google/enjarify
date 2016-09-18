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

use jvm::jvmops::*;
use typeinference::scalar;
use super::genlookup::*;

fn normalize_float(x: u32) -> u32 { if (x | FLOAT_SIGN) > FLOAT_NINF {FLOAT_NAN} else {x} }
fn normalize_double(x: u64) -> u64 { if (x | DOUBLE_SIGN) > DOUBLE_NINF {DOUBLE_NAN} else {x} }

type ITable = HashMap<i32, BString>;
type FTable = HashMap<u32, BString>;
type LTable = HashMap<i64, BString>;
type DTable = HashMap<u64, BString>;
struct Tables{ints: ITable, floats: FTable, longs: LTable, doubles: DTable}
impl Tables{
    fn new() -> Tables { let t = create(); Tables{ints: t.0, floats: t.1, longs: t.2, doubles: t.3} }

    fn int(&self, x: i32) -> BString {
        if let Some(v) = self.ints.get(&x) { return v.clone(); }
        let low = x as i16 as i32;
        let high = (x ^ low) >> 16;
        let mut res = self.int(high);
        res.extend(self.int(16)); res.push(ISHL);
        if low != 0 { res.extend(self.int(low)); res.push(IXOR); }
        res
    }

    fn long(&self, x: i64) -> BString {
        if let Some(v) = self.longs.get(&x) { return v.clone(); }
        let low = x as i32;
        let high = ((x ^ low as i64) >> 32) as i32;
        if high == 0 {
            let mut res = self.int(low); res.push(I2L); return res;
        }

        let mut res = self.int(high); res.push(I2L);
        res.extend(self.int(32)); res.push(LSHL);
        if low != 0 { res.extend(self.int(low)); res.push(I2L); res.push(LXOR); }
        res
    }

    fn float(&self, x: u32) -> BString {
        assert!(x == normalize_float(x));
        if let Some(v) = self.floats.get(&x) { return v.clone(); }

        let mut exponent = (((x >> 23) & 0xFF) as i32) - 127;
        let mut mantissa = (x % (1 << 23)) as i32;
        // check for denormals!
        if exponent == -127 { exponent += 1; }
        else { mantissa += 1 << 23; }
        exponent -= 23;

        if x & FLOAT_SIGN != 0 { mantissa = -mantissa; }
        let combine_op = if exponent < 0 {FDIV} else {FMUL};
        exponent = exponent.abs();

        let mut afterm = vec![];
        while exponent >= 63 { // max 2 iterations since -149 <= exp <= 104
            afterm.extend(vec![LCONST_1, ICONST_M1, LSHL, L2F, combine_op]);
            mantissa = -mantissa;
            exponent -= 63;
        }
        if exponent > 0 {
            afterm.push(LCONST_1);
            afterm.extend(self.int(exponent));
            afterm.extend(vec![LSHL, L2F, combine_op]);
        }
        let mut res = self.int(mantissa); res.push(I2F); res.extend(afterm);
        res
    }

    fn double(&self, x: u64) -> BString {
        assert!(x == normalize_double(x));
        if let Some(v) = self.doubles.get(&x) { return v.clone(); }

        let mut exponent = (((x >> 52) & 0x7FF) as i32) - 1023;
        let mut mantissa = (x % (1 << 52)) as i64;
        // check for denormals!
        if exponent == -1023 { exponent += 1; }
        else { mantissa += 1 << 52; }
        let exponent = exponent - 52;

        if x & DOUBLE_SIGN != 0 { mantissa = -mantissa; }

        let mut afterm = vec![];
        let part63 = exponent.abs() as u32 / 63;
        if part63 > 0 { //create *63 part of exponent by repeated squaring
            // use 2^-x instead of calculating 2^x and dividing to avoid overflow in
            // case we need 2^-1071
            if exponent < 0 { // -2^63
                afterm.extend(vec![DCONST_1, LCONST_1, ICONST_M1, LSHL, L2D, DDIV]);
            } else { // 2^63
                afterm.extend(vec![LCONST_1, ICONST_M1, LSHL, L2D]);
            }

            // adjust sign of mantissa for odd powers since we're actually using -2^63 rather than positive
            if part63&1 > 0 { mantissa = -mantissa; }

            let mut last_needed = part63 & 1;
            for bi in 1..(32 - part63.leading_zeros()) {
                afterm.push(DUP2);
                if last_needed > 0 {
                    afterm.push(DUP2);
                }
                afterm.push(DMUL);
                last_needed = part63 & (1<<bi);
            }
            afterm.extend(vec![DMUL; part63.count_ones() as usize]);
        }
        // now handle the rest
        let rest = exponent.abs() % 63;
        if rest > 0 {
            afterm.push(LCONST_1);
            afterm.extend(self.int(rest as i32));
            afterm.push(LSHL);
            afterm.push(L2D);
            afterm.push(if exponent < 0 {DDIV} else {DMUL});
        }
        let mut res = self.long(mantissa); res.push(L2D); res.extend(afterm);
        res
    }
}

lazy_static! {
    static ref TABLE: Tables = Tables::new();
}

pub fn normalize(st: scalar::T, val: u64) -> u64 {
    match st {
        scalar::FLOAT => normalize_float(val as u32) as u64,
        scalar::DOUBLE => normalize_double(val),
        _ => val
    }
}

pub fn calc(st: scalar::T, val: u64) -> BString {
    match st {
        scalar::INT => TABLE.int(val as i32),
        scalar::FLOAT => TABLE.float(val as u32),
        scalar::LONG => TABLE.long(val as i64),
        scalar::DOUBLE => TABLE.double(val),
        _ => unreachable!(),
    }
}

pub fn lookup(st: scalar::T, val: u64) -> Option<BString> {
    match st {
        scalar::INT => TABLE.ints.get(&(val as i32)).map(|v| v.clone()),
        scalar::FLOAT => TABLE.floats.get(&(val as u32)).map(|v| v.clone()),
        scalar::LONG => TABLE.longs.get(&(val as i64)).map(|v| v.clone()),
        scalar::DOUBLE => TABLE.doubles.get(&(val as u64)).map(|v| v.clone()),
        _ => unreachable!(),
    }
}
