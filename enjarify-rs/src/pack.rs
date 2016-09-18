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
use byteio::Writer;

// Replacement for Python's struct.pack
#[allow(non_snake_case)]
pub fn BH(x: u8, y: u16) -> BString { let mut w = Writer(Vec::with_capacity(3)); w.u8(x); w.u16(y); w.0 }
#[allow(non_snake_case)]
pub fn Bh(x: u8, y: i16) -> BString { BH(x, y as u16) }
#[allow(non_snake_case)]
pub fn Bi(x: u8, y: i32) -> BString { let mut w = Writer(Vec::with_capacity(5)); w.u8(x); w.u32(y as u32); w.0 }
#[allow(non_snake_case)]
pub fn BhBi(x: u8, y: i16, z: u8, z2: i32) -> BString {
    let mut w = Writer(Vec::with_capacity(8));
    w.u8(x); w.u16(y as u16); w.u8(z); w.u32(z2 as u32);
    w.0
}
#[allow(non_snake_case)]
pub fn BBH(x: u8, y: u8, z: u16) -> BString { let mut w = Writer(Vec::with_capacity(4)); w.u8(x); w.u8(y); w.u16(z); w.0 }
#[allow(non_snake_case)]
pub fn BHBB(x: u8, y: u16, z: u8, z2: u8) -> BString {
    let mut w = Writer(Vec::with_capacity(5));
    w.u8(x); w.u16(y); w.u8(z); w.u8(z2);
    w.0
}
