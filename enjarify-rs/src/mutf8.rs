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
use std::borrow::Cow;
use std::char;
use std::iter::FromIterator;
use std::slice;
use std::str;

use strings::*;

// decode arbitrary utf8 codepoints, tolerating surrogate pairs, nonstandard encodings, etc.
struct Utf8Iter<'a>(slice::Iter<'a, u8>);
impl<'a> Iterator for Utf8Iter<'a> {
    type Item = u32;

    fn next(&mut self) -> Option<u32> {
        let x = match self.0.next() {
            None => { return None; }
            Some(v) => *v as u32
        };
        if x < 128 { return Some(x); }

        // figure out how many bytes
        let mut extra = 0;
        for i in 0..3 {
            if x & (1 << (6-i)) != 0 {
                extra += 1;
            } else { break; }
        }

        let mut bits = (x as u32) % (1 << (6-extra));
        for _ in 0..extra {
            let x = match self.0.next() {
                None => { return None; }
                Some(v) => *v as u32
            };
            bits = (bits << 6) ^ (x & 63);
        }

        Some(bits)
    }
}

struct FixPairsIter<'a>(Utf8Iter<'a>);
impl<'a> Iterator for FixPairsIter<'a> {
    type Item = char;

    fn next(&mut self) -> Option<char> {
        let x = match self.0.next() {
            None => { return None; }
            Some(v) => v
        };
        if 0xD800 <= x && x < 0xDC00 {
            let high = x - 0xD800;
            let low = match self.0.next() {
                None => { return None; }
                Some(v) => v
            } - 0xDC00;
            char::from_u32(0x10000 + (high << 10) + (low & 1023))
        } else {
            char::from_u32(x)
        }
    }
}

pub fn decode(s: &bstr) -> Cow<str> {
    match str::from_utf8(s) {
        Ok(s) => Cow::Borrowed(s),
        Err(_) => Cow::Owned(String::from_iter(FixPairsIter(Utf8Iter(s.iter())))),
    }
}
