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
use std::ops::BitAnd;

use strings::*;

#[derive(Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Default, Hash, Debug)]
pub struct T(pub u8);
impl BitAnd for T { type Output = Self; fn bitand(self, rhs: Self) -> Self { T(self.0 & rhs.0) } }
impl T {
    pub fn is_wide(self) -> bool { self & C64 != INVALID }
    pub fn includes(self, rhs: T) -> bool { self & rhs != INVALID }

    pub fn from_desc(desc: &bstr) -> Self {
        match desc[0] {
            b'Z' => INT,
            b'B' => INT,
            b'C' => INT,
            b'S' => INT,
            b'I' => INT,
            b'F' => FLOAT,
            b'J' => LONG,
            b'D' => DOUBLE,
            b'L' => OBJ,
            b'[' => OBJ,
            _ => panic!("invalid desc")
        }
    }

    // for converting to jvmops, most of which use this ordering
    pub fn ilfda(self) -> u8 {
        match self {
            INT => 0,
            LONG => 1,
            FLOAT => 2,
            DOUBLE => 3,
            OBJ => 4,
            _ => panic!("bad scalar type"),
        }
    }
}

pub const INVALID: T = T(0);
pub const INT: T = T(1 << 0);
pub const FLOAT: T = T(1 << 1);
pub const OBJ: T = T(1 << 2);
pub const LONG: T = T(1 << 3);
pub const DOUBLE: T = T(1 << 4);

pub const ZERO: T = T(INT.0 | FLOAT.0 | OBJ.0);
pub const C32: T = T(INT.0 | FLOAT.0);
pub const C64: T = T(LONG.0 | DOUBLE.0);
pub const ALL: T = T(ZERO.0 | C64.0);
