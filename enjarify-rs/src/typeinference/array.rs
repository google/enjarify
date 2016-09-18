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
use strings::*;
use super::scalar;

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum Base { B, C, S, I, F, J, D }

#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum T {
    Invalid,
    Null,
    Array(u8, Base),
}
impl Default for T {
    fn default() -> Self { T::Invalid }
}
impl T {
    pub fn from_desc(desc: &bstr) -> Self {
        let mut dim = 0;
        for byte in desc {
            if *byte == b'[' { dim += 1; continue; }
            if dim < 1 { return INVALID; }

            return match *byte {
                b'Z' | b'B' => T::Array(dim, Base::B),
                b'C' => T::Array(dim, Base::C),
                b'S' => T::Array(dim, Base::S),
                b'I' => T::Array(dim, Base::I),
                b'F' => T::Array(dim, Base::F),
                b'J' => T::Array(dim, Base::J),
                b'D' => T::Array(dim, Base::D),
                b'L' => T::Invalid,
                _ => panic!("invalid desc")
            }
        }
        panic!("invalid desc");
    }

    pub fn merge(self, rhs: Self) -> Self {
        if rhs == T::Null { return self; }
        if self == T::Null { return rhs; }
        if self == rhs { return self; }
        T::Invalid
    }

    // intersect types
    pub fn narrow(self, rhs: Self) -> Self {
        if rhs == T::Invalid { return self; }
        if self == T::Invalid { return rhs; }
        if self == rhs { return self; }
        T::Null
    }

    pub fn eletpair(self) -> (scalar::T, Self) {
        match self {
            T::Invalid => (scalar::OBJ, self),
            // This is unreachable, so use (ALL, NULL), which can be merged with anything
            T::Null => (scalar::ALL, T::Null),
            T::Array(dim, base) => {
                if dim > 1 {
                    (scalar::OBJ, T::Array(dim-1, base))
                } else {
                    (match base {
                        Base::B | Base::C | Base::S | Base::I => scalar::INT,
                        Base::F => scalar::FLOAT,
                        Base::J => scalar::LONG,
                        Base::D => scalar::DOUBLE,
                    }, T::Invalid)
                }
            }
        }
    }

    pub fn to_desc(self) -> BString {
        // todo: return static slice?
        if let T::Array(dim, base) = self {
            let mut res = vec![b'['; dim as usize];
            res.push(match base {
                Base::B => b'B',
                Base::C => b'C',
                Base::S => b'S',
                Base::I => b'I',
                Base::F => b'F',
                Base::J => b'J',
                Base::D => b'D',
            });
            res
        } else { unreachable!(); }
    }
}

pub const INVALID: T = T::Invalid;
pub const NULL: T = T::Null;
