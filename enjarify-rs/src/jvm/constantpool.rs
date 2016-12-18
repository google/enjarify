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
use dex::{FieldId, MethodId};
use error;

#[derive(PartialEq, Eq, Hash, Clone, Copy, Debug)]
pub struct ArgsUtf<'a>(&'a bstr);
#[derive(PartialEq, Eq, Hash, Clone, Copy, Debug)]
pub struct ArgsInd(u16);
#[derive(PartialEq, Eq, Hash, Clone, Copy, Debug)]
pub struct ArgsInd2(u16, u16);
#[derive(PartialEq, Eq, Hash, Clone, Copy, Debug)]
pub struct ArgsPrim(pub u64);

#[derive(PartialEq, Eq, Hash, Clone, Copy, Debug)]
pub enum Entry<'a> {
    Class(ArgsInd),
    Fieldref(ArgsInd2),
    Methodref(ArgsInd2),
    InterfaceMethodref(ArgsInd2),
    JString(ArgsInd),
    Integer(ArgsPrim),
    Float(ArgsPrim),
    Long(ArgsPrim),
    Double(ArgsPrim),
    NameAndType(ArgsInd2),
    Utf8(ArgsUtf<'a>),
}
pub use self::Entry::*;
impl<'a> Entry<'a> {
    fn width(&self) -> usize {
        match self {
            &Long(_) | &Double(_) => 2,
            _ => 1,
        }
    }

    // to sort matching the python order
    pub fn cmp_key(&self) -> (u8, u64) {
        match self {
            &Integer(ref data) => (3, data.0),
            &Float(ref data) => (4, data.0),
            &Long(ref data) => (5, data.0),
            &Double(ref data) => (6, data.0),
            _ => unreachable!(),
        }
    }
}

pub trait ConstantPool<'a> {
    // abstract methods
    fn space(&self) -> usize;
    fn lowspace(&self) -> usize;
    fn write(&self, stream: &mut Writer);
    fn get_ind(&mut self, low: bool, width: usize) -> u16;
    fn lookup(&mut self) -> &mut HashMap<Entry<'a>, u16>;
    fn vals(&mut self) -> &mut [Option<Entry<'a>>];

    // derived methods
    fn get(&mut self, entry: Entry<'a>) -> u16 {
        // println!("{} {} cp.get {:?}", self.lookup().len(), self.vals().len(), &entry);
        if let Some(val) = self.lookup().get(&entry) {
            return *val;
        }

        let low = match &entry {
            &Integer(_) | &Float(_) | &JString(_) => true,
            _ => false,
        };
        let index = self.get_ind(low, entry.width());
        self.lookup().insert(entry.clone(), index);
        self.vals()[index as usize] = Some(entry);
        index
    }

    fn insert_directly(&mut self, entry: Entry<'a>, low: bool) -> u16 {
        let index = self.get_ind(low, entry.width());
        self.lookup().insert(entry.clone(), index);
        self.vals()[index as usize] = Some(entry);
        index
    }

    fn try_get(&mut self, entry: Entry<'a>) -> Option<u16> {
        if let Some(val) = self.lookup().get(&entry) {
            return Some(*val);
        }

        if entry.width() > self.space() { return None; }
        Some(self.insert_directly(entry, true))
    }

    fn utf8(&mut self, s: &'a bstr) -> u16 {
        if s.len() > 65535 {
            error::classfile_limit_exceeded();
        }
        self.get(Utf8(ArgsUtf(s)))
    }

    fn class(&mut self, s: &'a bstr) -> u16 {
        let ind = self.utf8(s);
        self.get(Class(ArgsInd(ind)))
    }

    fn string(&mut self, s: &'a bstr) -> u16 {
        let ind = self.utf8(s);
        self.get(JString(ArgsInd(ind)))
    }

    fn _nat(&mut self, name: &'a bstr, desc: &'a bstr) -> u16 {
        let ind = self.utf8(name);
        let ind2 = self.utf8(desc);
        self.get(NameAndType(ArgsInd2(ind, ind2)))
    }

    fn field(&mut self, trip: &FieldId<'a>) -> u16 {
        let ind = self.class(trip.cname);
        let ind2 = self._nat(trip.name, trip.desc);
        self.get(Fieldref(ArgsInd2(ind, ind2)))
    }

    fn method(&mut self, trip: MethodId<'a>) -> u16 {
        let ind = self.class(trip.cname);
        let ind2 = self._nat(trip.name, trip.desc);
        self.get(Methodref(ArgsInd2(ind, ind2)))
    }

    fn imethod(&mut self, trip: MethodId<'a>) -> u16 {
        let ind = self.class(trip.cname);
        let ind2 = self._nat(trip.name, trip.desc);
        self.get(InterfaceMethodref(ArgsInd2(ind, ind2)))
    }

    fn int(&mut self, x: u32) -> u16 { self.get(Integer(ArgsPrim(x as u64))) }
    fn float(&mut self, x: u32) -> u16 { self.get(Float(ArgsPrim(x as u64))) }
    fn long(&mut self, x: u64) -> u16 { self.get(Long(ArgsPrim(x))) }
    fn double(&mut self, x: u64) -> u16 { self.get(Double(ArgsPrim(x))) }

    fn write_entry(&self, stream: &mut Writer, entry: &Option<Entry<'a>>) {
        if let Some(entry) = entry.as_ref() {
            match entry {
                &Class(ref args) => { stream.u8(7); stream.u16(args.0); },
                &Fieldref(ref args) => { stream.u8(9); stream.u16(args.0); stream.u16(args.1); },
                &Methodref(ref args) => { stream.u8(10); stream.u16(args.0); stream.u16(args.1); },
                &InterfaceMethodref(ref args) => { stream.u8(11); stream.u16(args.0); stream.u16(args.1); },
                &JString(ref args) => { stream.u8(8); stream.u16(args.0); },
                &Integer(ref args) => { stream.u8(3); stream.u32(args.0 as u32); },
                &Float(ref args) => { stream.u8(4); stream.u32(args.0 as u32); },
                &Long(ref args) => { stream.u8(5); stream.u64(args.0); },
                &Double(ref args) => { stream.u8(6); stream.u64(args.0); },
                &NameAndType(ref args) => { stream.u8(12); stream.u16(args.0); stream.u16(args.1); },
                &Utf8(ref args) => { stream.u8(1);
                    stream.u16(args.0.len() as u16);
                    stream.write(args.0);
                },
            }
        }
    }
}

// A simple constant pool that just allocates slots in increasing order.
struct SimplePool<'a> {
    lookup: HashMap<Entry<'a>, u16>,
    vals: Vec<Option<Entry<'a>>>,
}
impl<'a> ConstantPool<'a> for SimplePool<'a> {
    fn lookup(&mut self) -> &mut HashMap<Entry<'a>, u16> { &mut self.lookup }
    fn vals(&mut self) -> &mut [Option<Entry<'a>>] { &mut self.vals }

    fn space(&self) -> usize { 65535 - self.vals.len() }
    fn lowspace(&self) -> usize { 256usize.saturating_sub(self.vals.len()) }

    fn get_ind(&mut self, _low: bool, width: usize) -> u16 {
        if self.space() < width { error::classfile_limit_exceeded(); }
        let temp = self.vals.len();
        for _ in 0..width { self.vals.push(None); }
        temp as u16
    }
    fn write(&self, stream: &mut Writer) {
        stream.u16(self.vals.len() as u16);
        for item in &self.vals { self.write_entry(stream, item); }
    }
}

pub fn simple_pool<'a>() -> Box<ConstantPool<'a> + 'a> {
    Box::new(SimplePool{
        lookup: HashMap::new(),
        vals: vec![None],
    })
}

// Constant pool slots 1-255 are special because they can be referred to by the
// two byte ldc instruction (as opposed to 3 byte ldc_w/ldc2_w). Therefore, it is
// desireable to allocate constants which could use ldc in the first 255 slots,
// while not wasting these valuable low slots with pool entries that can't use
// ldc (utf8s, longs, etc.)
// One possible approach is to allocate the ldc entries starting at 1 and the
// others starting at 256, (possibly leaving a gap if there are less than 255 of
// the former). However, this is not ideal because the empty slots are not
// continguous. This means that you could end up in the sitatuation where there
// are exactly two free slots and you wish to add a long/double entry but the
// free slots are not continguous.
// To solve this, we take a different approach - always create the pool as the
// largest possible size (65534 entries) and allocate the non-ldc constants
// starting from the highest index and counting down. This ensures that the free
// slots are always contiguous. Since the classfile representation doesn't
// actually allow gaps like that, the empty spaces if any are filled in with
// dummy entries at the end.
// For simplicity, we always allocate ints, floats, and strings in the low entries
// and everything else in the high entries, regardless of whether they are actaully
// referenced by a ldc or not. (see ConstantPoolBase._get)

// Fill in unused space with shortest possible item (Utf8 ''), preencoded for efficiency
const PLACEHOLDER_ENTRY: &'static bstr = b"\x01\0\0";
struct SplitPool<'a> {
    lookup: HashMap<Entry<'a>, u16>,
    // vals: [Option<Entry<'a>>; 65535],
    vals: Vec<Option<Entry<'a>>>,
    bot: usize,
    top: usize,
}
impl<'a> ConstantPool<'a> for SplitPool<'a> {
    fn lookup(&mut self) -> &mut HashMap<Entry<'a>, u16> { &mut self.lookup }
    fn vals(&mut self) -> &mut [Option<Entry<'a>>] { &mut self.vals }

    fn space(&self) -> usize { self.top - self.bot }
    fn lowspace(&self) -> usize { 256usize.saturating_sub(self.bot) }

    fn get_ind(&mut self, low: bool, width: usize) -> u16 {
        if self.space() < width { error::classfile_limit_exceeded(); }
        (if low {
            self.bot += width;
            self.bot - width
        } else {
            self.top -= width;
            self.top
        }) as u16
    }
    fn write(&self, stream: &mut Writer) {
        stream.u16(65535);
        assert!(self.bot <= self.top);

        for item in &self.vals[..self.bot] { self.write_entry(stream, item); }

        stream.0.reserve(PLACEHOLDER_ENTRY.len() * (self.top - self.bot));
        for _ in 0..(self.top - self.bot) { stream.0.extend_from_slice(PLACEHOLDER_ENTRY); }

        for item in &self.vals[self.top..] { self.write_entry(stream, item); }
    }
}

pub fn split_pool<'a>() -> Box<ConstantPool<'a> + 'a> {
    Box::new(SplitPool{
        lookup: HashMap::new(),
        vals: vec![None; 65535],
        // vals: [None; 65535],
        bot: 1,
        top: 65535,
    })
}
