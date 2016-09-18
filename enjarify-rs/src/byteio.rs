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
extern crate byteorder;
use self::byteorder::{ByteOrder, BigEndian, LittleEndian, WriteBytesExt};

use strings::*;

#[derive(Clone)]
pub struct Reader<'a>(pub &'a bstr);
impl<'a> Reader<'a> {
    pub fn read(&mut self, size: usize) -> &'a bstr {
        let (first, rest) = self.0.split_at(size);
        self.0 = rest;
        first
    }

    pub fn offset(&self, offset: u32) -> Reader<'a> { Reader(self.0.split_at(offset as usize).1) }

    pub fn u8(&mut self) -> u8 { self.read(1)[0] }
    pub fn u16(&mut self) -> u16 { LittleEndian::read_u16(self.read(2)) }
    pub fn u32(&mut self) -> u32 { LittleEndian::read_u32(self.read(4)) }
    pub fn u64(&mut self) -> u64 { LittleEndian::read_u64(self.read(8)) }

    fn leb128(&mut self) -> (u32, u8) {
        let (mut result, mut size) = (0u32, 0);
        while self.0[0] >> 7 != 0 {
            result ^= ((self.u8() & 0x7f) as u32) << size;
            size += 7;
        }
        result ^= ((self.u8() & 0x7f) as u32) << size;
        size += 7;
        (result, size)
    }

    pub fn uleb128(&mut self) -> u32 { self.leb128().0 }
    pub fn sleb128(&mut self) -> i32 {
        let (result, size) = self.leb128();
        let val = result as i32;
        if val >= 1i32 << (size-1) {
            val - (1i32 << size)
        } else { val }
    }

    pub fn cstr(&mut self) -> &'a bstr {
        let index = self.0.iter().position(|&b| b == b'\0').unwrap();
        self.read(index)
    }
}

#[derive(Default)]
pub struct Writer(pub BString);
impl Writer {
    pub fn write(&mut self, s: &bstr) { self.0.extend_from_slice(s); }

    pub fn u8(&mut self, x: u8) { self.0.push(x); }
    pub fn u16(&mut self, x: u16) { self.0.write_u16::<BigEndian>(x).unwrap(); }
    pub fn u32(&mut self, x: u32) { self.0.write_u32::<BigEndian>(x).unwrap(); }
    pub fn u64(&mut self, x: u64) { self.0.write_u64::<BigEndian>(x).unwrap(); }

    pub fn i32(&mut self, x: i32) { self.u32(x as u32); }
}
