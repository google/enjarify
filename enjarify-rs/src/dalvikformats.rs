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
#[derive(Default, Debug)]
pub struct DalvikArgs {
    pub a: u32,
    pub b: u32,
    pub c: u32,
    pub long: u64,
    pub args: Option<Vec<u16>>,

    pub ra: u16,
    pub rb: u16,
    pub rc: u16,
}

#[derive(Clone, Copy, Debug)]
enum ArgsCount { N0, N1, N2, N3, N5, R }
use self::ArgsCount::*;

#[derive(Clone, Copy, Debug)]
enum ArgsType { B, C, H, I, L, N, S, T, X }
use self::ArgsType::*;

pub fn decode(shorts: &[u16], pos: usize, opcode: u8) -> (usize, DalvikArgs) {
    let (size, cnt, typ) = match opcode {
        0x00 => (1, N0, X),
        0x01 => (1, N2, X),
        0x02 => (2, N2, X),
        0x03 => (3, N2, X),
        0x04 => (1, N2, X),
        0x05 => (2, N2, X),
        0x06 => (3, N2, X),
        0x07 => (1, N2, X),
        0x08 => (2, N2, X),
        0x09 => (3, N2, X),
        0x0a...0x0d => (1, N1, X),
        0x0e => (1, N0, X),
        0x0f...0x11 => (1, N1, X),
        0x12 => (1, N1, N),
        0x13 => (2, N1, S),
        0x14 => (3, N1, I),
        0x15 => (2, N1, H),
        0x16 => (2, N1, S),
        0x17 => (3, N1, I),
        0x18 => (5, N1, L),
        0x19 => (2, N1, H),
        0x1a => (2, N1, C),
        0x1b => (3, N1, C),
        0x1c => (2, N1, C),
        0x1d...0x1e => (1, N1, X),
        0x1f => (2, N1, C),
        0x20 => (2, N2, C),
        0x21 => (1, N2, X),
        0x22 => (2, N1, C),
        0x23 => (2, N2, C),
        0x24 => (3, N5, C),
        0x25 => (3, R, C),
        0x26 => (3, N1, T),
        0x27 => (1, N1, X),
        0x28 => (1, N0, T),
        0x29 => (2, N0, T),
        0x2a => (3, N0, T),
        0x2b...0x2c => (3, N1, T),
        0x2d...0x31 => (2, N3, X),
        0x32...0x37 => (2, N2, T),
        0x38...0x3d => (2, N1, T),
        0x3e...0x43 => (1, N0, X),
        0x44...0x51 => (2, N3, X),
        0x52...0x5f => (2, N2, C),
        0x60...0x6d => (2, N1, C),
        0x6e...0x72 => (3, N5, C),
        0x73 => (1, N0, X),
        0x74...0x78 => (3, R, C),
        0x79...0x7a => (1, N0, X),
        0x7b...0x8f => (1, N2, X),
        0x90...0xaf => (2, N3, X),
        0xb0...0xcf => (1, N2, X),
        0xd0...0xd7 => (2, N2, S),
        0xd8...0xe2 => (2, N2, B),
        0xe3...0xff => (1, N0, X),
        _ => unreachable!()
    };

    let mut d = DalvikArgs::default();

    match size {
        1 => {
            let w = shorts[pos] as u32;
            match (cnt, typ) {
                (N2, X) | (N1, N) => {
                    d.a = (w >> 8) & 0xF;
                    d.b = w >> 12;
                }
                (N1, X) | (N0, T) => {
                    d.a = w >> 8;
                }
                (N0, X) => {}
                _ => unreachable!()
            }
        }
        2 => {
            let w = shorts[pos] as u32;
            let w2 = shorts[pos+1] as u32;
            match (cnt, typ) {
                (N0, T) => {
                    d.a = w2;
                }
                (N2, X) | (N1, T) | (N1, S) | (N1, H) | (N1, C) => {
                    d.a = w >> 8;
                    d.b = w2;
                }
                (N3, X) | (N2, B) => {
                    d.a = w >> 8;
                    d.b = w2 & 0xFF;
                    d.c = w2 >> 8;
                }
                (N2, T) | (N2, S) | (N2, C) => {
                    d.a = (w >> 8) & 0xF;
                    d.b = w >> 12;
                    d.c = w2;
                }
                _ => unreachable!()
            }
        }
        3 => {
            let w = shorts[pos] as u32;
            let w2 = shorts[pos+1] as u32;
            let w3 = shorts[pos+2] as u32;
            match (cnt, typ) {
                (N0, T) => {
                    d.a = w2 ^ (w3 << 16);
                }
                (N2, X) => {
                    d.a = w2;
                    d.b = w3;
                }
                (N1, I) | (N1, T) | (N1, C) => {
                    d.a = w >> 8;
                    d.b = w2 ^ (w3 << 16);
                }
                (N5, C) => {
                    d.a = w2;

                    let w = w as u16;
                    let w3 = w3 as u16;
                    let nibs = [w3&0xF, (w3>>4)&0xF, (w3>>8)&0xF, (w3>>12)&0xF, (w>>8)&0xF];
                    let nibcnt = (w >> 12) as usize;
                    d.args = Some((&nibs[..nibcnt]).to_vec());
                }
                (R, C) => {
                    d.a = w2;

                    let w = w as u16;
                    let w3 = w3 as u16;
                    d.args = Some((w3..w3+(w>>8)).collect());
                }
                _ => unreachable!()
            }
        }
        5 => {
            d.a = (shorts[pos] as u32) >> 8;
            for i in 0..4 {
                d.long ^= (shorts[pos+1+i] as u64) << (16 * i);
            }
        }
        _ => unreachable!()
    };

    // Check if we need to sign extend
    match (size, cnt, typ) {
        (1, N1, N) => { d.b = (((d.b << 4) as i8) >> 4) as u32; },
        (1, N0, T) => { d.a = d.a as i8 as u32; },
        (2, N2, B) => { d.c = d.c as i8 as u32; },
        (2, N0, T) => { d.a = d.a as i16 as u32; },
        (2, N1, T) | (2, N1, S) => { d.b = d.b as i16 as u32; },
        (2, N2, T) | (2, N2, S) => { d.c = d.c as i16 as u32; },
        _ => {}
    };

    // Hats depend on actual size expected, so we rely on opcode as a hack
    if let H = typ {
        if opcode == 0x15 {
            d.b = d.b << 16;
        } else {
            d.long = (d.b as u64) << 48;
        }
    }

    // Make sure const-wide is always stored in d.Long, even if it's short
    if opcode == 0x16 || opcode == 0x17 {
        d.long = d.b as u64;
    }

    // Convert code offsets to actual code position
    if let T = typ {
        let pos = pos as u32;
        match cnt {
            N0 => { d.a = d.a.wrapping_add(pos); },
            N1 => { d.b = d.b.wrapping_add(pos); },
            N2 => { d.c = d.c.wrapping_add(pos); },
            _ => unreachable!()
        }
    }

    d.ra = d.a as u16;
    d.rb = d.b as u16;
    d.rc = d.c as u16;
    (pos + size as usize, d)
}
