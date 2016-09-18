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
package dex

import (
	"enjarify-go/byteio"
	"fmt"
)

const NO_INDEX = 0xFFFFFFFF

func typeList(dex *DexFile, off uint32, parseClsDesc bool) (result []string) {
	if off != 0 {
		f := dex.Type
		if parseClsDesc {
			f = dex.ClsType
		}

		st := dex.stream(off)
		size := st.U32()
		for i := uint32(0); i < size; i++ {
			result = append(result, f(uint32(st.U16())))
		}
	}
	return
}

func encodedValue(dex *DexFile, stream *byteio.Reader) interface{} {
	tag := uint32(stream.U8())
	vtype, varg := tag&31, tag>>5

	switch vtype {
	case 0x1c: // ARRAY
		res := make([]interface{}, stream.Uleb128())
		for i := range res {
			res[i] = encodedValue(dex, stream)
		}
		return res
	case 0x1d: // ANNOTATION
		// We don't actually care about annotations but still need to read it to
		// find out how much data is taken up
		stream.Uleb128()
		res := make([]interface{}, stream.Uleb128())
		for i := range res {
			stream.Uleb128()
			res[i] = encodedValue(dex, stream)
		}
		return nil
	case 0x1e: // NULL
		return nil
	case 0x1f: // BOOLEAN
		return uint32(varg)
	}

	// the rest are an int encoded into varg + 1 bytes in some way
	size := varg + 1
	val := uint64(0)
	for i := uint32(0); i < size; i++ {
		val += uint64(stream.U8()) << (i * 8)
	}

	switch vtype {
	case 0x00: // BYTE
		return uint32(int8(val))
	case 0x02: // SHORT
		return uint32(int16(val))
	case 0x03: // CHAR
		return uint32(uint16(val))
	case 0x04: // INT
		return uint32(int32(val))
	case 0x06: // LONG
		return val

	// floats are 0 extended to the right
	case 0x10: // FLOAT
		return uint32(val << (32 - size*8))
	case 0x11: // DOUBLE
		return val << (64 - size*8)

	case 0x17: // STRING
		return string(dex.String(uint32(val)))
	case 0x18: // TYPE
		return string(dex.ClsType(uint32(val)))
	}
	return nil
}

type DexClass struct {
	dex                 *DexFile
	Name                string
	Access              uint32
	Super               *string
	Interfaces          []string
	sourcefile          uint32
	annotations         uint32
	data_off            uint32
	constant_values_off uint32

	Data *ClassData
}

func (self *DexClass) ParseData() {
	if self.Data == nil {
		self.Data = newClassData(self.dex, self.data_off)
		if self.constant_values_off > 0 {
			st := self.dex.stream(self.constant_values_off)
			size := st.Uleb128()
			for i := range self.Data.Fields[:size] {
				self.Data.Fields[i].ConstantValue = encodedValue(self.dex, st)
			}
		}
	}
}

func newDexClass(dex *DexFile, base_off, i uint32) DexClass {
	st := dex.stream(base_off + i*32)
	self := DexClass{
		dex,
		dex.ClsType(st.U32()),
		st.U32(),
		dex.optClsType(st.U32()),
		typeList(dex, st.U32(), true),
		st.U32(),
		st.U32(),
		st.U32(),
		st.U32(),
		nil,
	}
	return self
}

type sizeOff struct {
	size, off uint32
}

func newSizeOff(stream *byteio.Reader) sizeOff {
	return sizeOff{stream.U32(), stream.U32()}
}

type DexFile struct {
	raw                                                    string
	string_ids, type_ids, proto_ids, field_ids, method_ids sizeOff
	Classes                                                []DexClass
}

func (self *DexFile) stream(i uint32) *byteio.Reader {
	return &byteio.Reader{self.raw, i}
}

func (self *DexFile) u32(i uint32) uint32 {
	return self.stream(i).U32()
}

func (self *DexFile) String(i uint32) string {
	data_off := self.u32(self.string_ids.off + i*4)
	stream := self.stream(data_off)
	_ = stream.Uleb128() // Ignore decoded length
	return string(stream.CStr())
}

func (self *DexFile) Type(i uint32) (data string) {
	si := self.u32(self.type_ids.off + i*4)
	return self.String(si)
}

func (self *DexFile) ClsType(i uint32) string {
	data := self.Type(i)
	if data[0] == '[' {
		return data
	}
	return data[1 : len(data)-1]
}

func (self *DexFile) optClsType(i uint32) *string {
	if i != NO_INDEX {
		s := self.ClsType(i)
		return &s
	}
	return nil
}

func (self *DexFile) GetFieldId(i uint32) Triple {
	return fieldId(self, i)
}

func (self *DexFile) GetMethodId(i uint32) MethodId {
	return methodId(self, i)
}

func Parse(data string) *DexFile {
	dex := DexFile{raw: data}

	// parse header
	stream := &byteio.Reader{data, 36}
	if stream.U32() != 0x70 {
		fmt.Printf("Warning, unexpected header size!\n")
	}
	if stream.U32() != 0x12345678 {
		fmt.Printf("Warning, unexpected endianess tag!\n")
	}

	_ = newSizeOff(stream)
	_ = stream.U32()
	dex.string_ids = newSizeOff(stream)
	dex.type_ids = newSizeOff(stream)
	dex.proto_ids = newSizeOff(stream)
	dex.field_ids = newSizeOff(stream)
	dex.method_ids = newSizeOff(stream)
	class_defs := newSizeOff(stream)
	_ = newSizeOff(stream)

	classes := make([]DexClass, class_defs.size)
	for i := uint32(0); i < class_defs.size; i++ {
		classes[i] = newDexClass(&dex, class_defs.off, i)
	}
	dex.Classes = classes

	return &dex
}
