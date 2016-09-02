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
package byteio

import (
	"bytes"
	"encoding/binary"
)

type Reader struct {
	Data []byte
	Pos  uint32
}

func (self *Reader) U8() (data uint8) {
	self.Pos += 1
	return self.Data[self.Pos-1]
}

func (self *Reader) U16() (data uint16) {
	result := uint16(self.U8()) + (uint16(self.U8()) << 8)

	// fmt.Printf("u16 %v\n", result)
	return result
	// return uint16(self.U8()) + (uint16(self.U8()) << 8)
}

func (self *Reader) U32() (data uint32) {
	// fmt.Printf("u32 pos %v bytes %v\n", self.Pos, self.Data[self.Pos:self.Pos+4])
	result := uint32(self.U16()) + (uint32(self.U16()) << 16)
	// fmt.Printf("u32 %v\n", result)
	return result
	// return uint32(self.U16()) + (uint32(self.U16()) << 16)
}

func (self *Reader) U64() (data uint64) {
	return uint64(self.U32()) + (uint64(self.U32()) << 32)
}

func (self *Reader) leb128() (result, size uint32) {
	b := self.U8()
	for b > 127 {
		result ^= uint32(b&0x7f) << size
		size += 7
		b = self.U8()
	}
	result ^= uint32(b&0x7f) << size
	size += 7
	// fmt.Printf("leb %v s%v\n", result, size)
	return
}

func (self *Reader) Uleb128() uint32 {
	result, _ := self.leb128()
	return result
}

func (self *Reader) Sleb128() int32 {
	result, size := self.leb128()
	val := int32(result)
	if val >= 1<<(size-1) {
		val -= 1 << size
	}
	return val
}

func (self *Reader) CStr() []byte {
	result := []byte{}
	b := self.U8()
	for b != 0 {
		result = append(result, b)
		b = self.U8()
	}
	return result
}

type Writer struct {
	bytes.Buffer
	Endianess binary.ByteOrder
}

func (self *Writer) write(data interface{}) {
	if err := binary.Write(self, self.Endianess, data); err != nil {
		panic(err)
	}
}

func (self *Writer) U8(data uint8) {
	self.write(&data)
}
func (self *Writer) S8(data int8) { self.U8(uint8(data)) }
func (self *Writer) U16(data uint16) {
	self.write(&data)
}
func (self *Writer) S16(data int16) { self.U16(uint16(data)) }
func (self *Writer) U32(data uint32) {
	self.write(&data)
}
func (self *Writer) S32(data int32) { self.U32(uint32(data)) }
func (self *Writer) U64(data uint64) {
	self.write(&data)
}

func (self *Writer) Append(other *Writer) {
	if _, err := self.Write(other.Bytes()); err != nil {
		panic(err)
	}
}

func NewWriter() *Writer {
	return &Writer{Endianess: binary.BigEndian}
}

// Replacement for Python's struct module
func Bytes(x ...byte) string   { return string(x) }
func B(x byte) string          { return Bytes(x) }
func BB(x byte, y byte) string { return Bytes(x, y) }
func BH(x byte, y uint16) string {
	w := Writer{Endianess: binary.BigEndian}
	w.U8(x)
	w.U16(y)
	return w.String()
}
func Bh(x byte, y int16) string { return BH(x, uint16(y)) }
func Bi(x byte, y int32) string {
	w := Writer{Endianess: binary.BigEndian}
	w.U8(x)
	w.U32(uint32(y))
	return w.String()
}
func BhBi(x byte, y int16, z byte, w int32) string { return Bh(x, y) + Bi(z, w) }

func BBH(x, y byte, z uint16) string {
	w := Writer{Endianess: binary.BigEndian}
	w.U8(x)
	w.U8(y)
	w.U16(z)
	return w.String()
}
func BHBB(x byte, y uint16, z byte, z2 byte) string {
	w := Writer{Endianess: binary.BigEndian}
	w.U8(x)
	w.U16(y)
	w.U8(z)
	w.U8(z2)
	return w.String()
}
func HHHH(x, y, z, z2 uint16) string {
	w := Writer{Endianess: binary.BigEndian}
	w.U16(x)
	w.U16(y)
	w.U16(z)
	w.U16(z2)
	return w.String()
}
