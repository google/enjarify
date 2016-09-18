// Copyright 2015 Google Inc. All Rights Reserved.
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
package cpool

import (
	"enjarify-go/byteio"
	"enjarify-go/dex"
	"enjarify-go/jvm/errors"
	"enjarify-go/util"
)

const CONSTANT_Class = 7
const CONSTANT_Fieldref = 9
const CONSTANT_Methodref = 10
const CONSTANT_InterfaceMethodref = 11
const CONSTANT_String = 8
const CONSTANT_Integer = 3
const CONSTANT_Float = 4
const CONSTANT_Long = 5
const CONSTANT_Double = 6
const CONSTANT_NameAndType = 12
const CONSTANT_Utf8 = 1

// const CONSTANT_MethodHandle = 15
// const CONSTANT_MethodType = 16
// const CONSTANT_InvokeDynamic = 18
const MAX_CONST = CONSTANT_NameAndType

func width(tag byte) int {
	if tag == CONSTANT_Double || tag == CONSTANT_Long {
		return 2
	}
	return 1
}

type Data struct {
	s      string
	p1, p2 uint16
	X      uint64
}

type Pair struct {
	Tag byte
	Data
}

type subclassImpl interface {
	Vals() []Pair
	getInd(low bool, width int) uint16
	space() int
	lowspace() int
}

type constantPoolBase struct {
	sub    subclassImpl
	lookup [MAX_CONST + 1]map[Data]uint16
}

func newConstantPoolBase() constantPoolBase {
	self := constantPoolBase{}
	for i := 0; i < len(self.lookup); i++ {
		self.lookup[i] = make(map[Data]uint16)
	}
	return self
}

func (self *constantPoolBase) get(tag byte, data Data) uint16 {
	d := self.lookup[tag]
	if val, ok := d[data]; ok {
		return val
	}

	low := (tag == CONSTANT_Integer || tag == CONSTANT_Float || tag == CONSTANT_String)
	index := self.sub.getInd(low, width(tag))
	d[data] = index
	self.sub.Vals()[index] = Pair{tag, data}
	return index
}

func (self *constantPoolBase) InsertDirectly(pair Pair, low bool) {
	d := self.lookup[pair.Tag]
	index := self.sub.getInd(low, width(pair.Tag))
	d[pair.Data] = index
	self.sub.Vals()[index] = pair
}

func (self *constantPoolBase) TryGet(pair Pair) (index uint16, ok bool) {
	tag := pair.Tag
	data := pair.Data

	d := self.lookup[tag]
	if val, ok := d[data]; ok {
		return val, true
	}

	width := width(tag)
	if width > self.sub.space() {
		return 0, false
	}

	index = self.sub.getInd(true, width)
	d[data] = index
	self.sub.Vals()[index] = pair
	return index, true
}

func (self *constantPoolBase) Space() int {
	return self.sub.space()
}

func (self *constantPoolBase) LowSpace() int {
	return self.sub.lowspace()
}
func (self *constantPoolBase) Utf8(s string) uint16 {
	if len(s) > 65535 {
		panic(&errors.ClassfileLimitExceeded{})
	}
	return self.get(CONSTANT_Utf8, Data{s: s})
}

func (self *constantPoolBase) Class(s string) uint16 {
	return self.get(CONSTANT_Class, Data{p1: self.Utf8(s)})
}

func (self *constantPoolBase) String(s string) uint16 {
	return self.get(CONSTANT_String, Data{p1: self.Utf8(s)})
}

func (self *constantPoolBase) Nat(name, desc string) uint16 {
	return self.get(CONSTANT_NameAndType, Data{p1: self.Utf8(name), p2: self.Utf8(desc)})
}

func (self *constantPoolBase) triple(tag byte, trip dex.Triple) uint16 {
	return self.get(tag, Data{p1: self.Class(trip.Cname), p2: self.Nat(trip.Name, trip.Desc)})
}

func (self *constantPoolBase) Field(trip dex.Triple) uint16 {
	return self.triple(CONSTANT_Fieldref, trip)
}

func (self *constantPoolBase) Method(trip dex.Triple) uint16 {
	return self.triple(CONSTANT_Methodref, trip)
}

func (self *constantPoolBase) IMethod(trip dex.Triple) uint16 {
	return self.triple(CONSTANT_InterfaceMethodref, trip)
}

func (self *constantPoolBase) Int(x uint32) uint16 {
	return self.get(CONSTANT_Integer, Data{X: uint64(x)})
}

func (self *constantPoolBase) Float(x uint32) uint16 {
	return self.get(CONSTANT_Float, Data{X: uint64(x)})
}

func (self *constantPoolBase) Long(x uint64) uint16 {
	return self.get(CONSTANT_Long, Data{X: x})
}

func (self *constantPoolBase) Double(x uint64) uint16 {
	return self.get(CONSTANT_Double, Data{X: x})
}

func (self *constantPoolBase) writeEntry(stream *byteio.Writer, item Pair) {
	if item.Tag == 0 {
		return
	}

	stream.U8(item.Tag)
	switch item.Tag {
	case CONSTANT_Utf8:
		stream.U16(uint16(len(item.s)))
		stream.WriteString(item.s)
	case CONSTANT_Integer, CONSTANT_Float:
		stream.U32(uint32(item.X))
	case CONSTANT_Long, CONSTANT_Double:
		stream.U64(item.X)
	case CONSTANT_Class, CONSTANT_String:
		stream.U16(item.p1)
	default:
		stream.U16(item.p1)
		stream.U16(item.p2)
	}
}

type simpleConstantPool struct {
	constantPoolBase
	_vals []Pair
}

func (self *simpleConstantPool) Vals() []Pair {
	return self._vals
}

func (self *simpleConstantPool) space() int {
	return 65535 - len(self._vals)
}

func (self *simpleConstantPool) lowspace() int {
	return 256 - len(self._vals)
}

func (self *simpleConstantPool) getInd(low bool, width int) (index uint16) {
	if self.space() < width {
		panic(&errors.ClassfileLimitExceeded{})
	}

	index = uint16(len(self._vals))
	for i := 0; i < width; i++ {
		self._vals = append(self._vals, Pair{})
	}
	return
}

func (self *simpleConstantPool) Write(stream *byteio.Writer) {
	util.Assert(len(self._vals) <= 65535)
	stream.U16(uint16(len(self._vals)))
	for _, item := range self._vals {
		self.writeEntry(stream, item)
	}
}

var PLACEHOLDER_ENTRY = byteio.BH(CONSTANT_Utf8, 0)

type splitConstantPool struct {
	constantPoolBase
	bot, top int
	_vals    [65535]Pair
}

func (self *splitConstantPool) Vals() []Pair {
	return self._vals[:]
}

func (self *splitConstantPool) space() int {
	return self.top - self.bot
}

func (self *splitConstantPool) lowspace() int {
	return 256 - self.bot
}

func (self *splitConstantPool) getInd(low bool, width int) (index uint16) {
	if self.space() < width {
		panic(&errors.ClassfileLimitExceeded{})
	}

	if low {
		self.bot += width
		return uint16(self.bot - width)
	}
	self.top -= width
	return uint16(self.top)
}

func (self *splitConstantPool) Write(stream *byteio.Writer) {
	stream.U16(uint16(len(self._vals)))

	for _, item := range self._vals[:self.bot] {
		self.writeEntry(stream, item)
	}

	for _, _ = range self._vals[self.bot:self.top] {
		if _, err := stream.WriteString(PLACEHOLDER_ENTRY); err != nil {
			panic(err)
		}
	}

	for _, item := range self._vals[self.top:] {
		self.writeEntry(stream, item)
	}
}

type Pool interface {
	Vals() []Pair
	InsertDirectly(pair Pair, low bool)
	TryGet(pair Pair) (index uint16, ok bool)
	Space() int
	LowSpace() int

	Utf8(s string) uint16
	Class(s string) uint16
	String(s string) uint16
	Nat(name, desc string) uint16
	Field(trip dex.Triple) uint16
	Method(trip dex.Triple) uint16
	IMethod(trip dex.Triple) uint16
	Int(x uint32) uint16
	Float(x uint32) uint16
	Long(x uint64) uint16
	Double(x uint64) uint16

	Write(stream *byteio.Writer)
}

func Simple() Pool {
	pool := simpleConstantPool{newConstantPoolBase(), make([]Pair, 1)}
	pool.sub = &pool
	return &pool
}

func Split() Pool {
	pool := splitConstantPool{constantPoolBase: newConstantPoolBase(), bot: 1, top: 65535}
	pool.sub = &pool
	return &pool
}
