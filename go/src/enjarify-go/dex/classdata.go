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
	"strings"
)

type Triple struct {
	Cname, Name, Desc string
}

type Field struct {
	Triple
	Access        uint32
	ConstantValue interface{}
}

func fieldId(dex *DexFile, field_idx uint32) Triple {
	st := dex.stream(dex.field_ids.off + field_idx*8)
	return Triple{
		Cname: dex.ClsType(uint32(st.U16())),
		Desc:  dex.Type(uint32(st.U16())),
		Name:  dex.String(st.U32())}
}

func newField(dex *DexFile, field_idx uint32, access uint32) Field {
	return Field{fieldId(dex, field_idx), access, nil} // ConstantValue will be set later
}

type CatchItem struct {
	Type   string
	Target uint32
}

type TryItem struct {
	Start, End, handler_off uint32
	Catches                 []CatchItem
}

func newTry(st *byteio.Reader) TryItem {
	self := TryItem{}
	self.Start = st.U32()
	self.End = self.Start + uint32(st.U16())
	self.handler_off = uint32(st.U16())
	return self
}

func (self *TryItem) finish(dex *DexFile, list_off uint32) {
	st := dex.stream(self.handler_off + list_off)
	size := st.Sleb128()
	abs := size
	if abs < 0 {
		abs = -abs
	}

	self.Catches = make([]CatchItem, abs, abs+1)
	for i := int32(0); i < abs; i++ {
		self.Catches[i] = CatchItem{dex.ClsType(st.Uleb128()), st.Uleb128()}
	}
	if size <= 0 {
		self.Catches = append(self.Catches, CatchItem{"java/lang/Throwable", st.Uleb128()})
	}
}

type CodeItem struct {
	Nregs    uint16
	Tries    []TryItem
	Bytecode []Instruction
}

func newCode(dex *DexFile, offset uint32) *CodeItem {
	st := dex.stream(offset)
	self := CodeItem{Nregs: st.U16()}
	_ = st.U16()
	_ = st.U16()
	tries_size := st.U16()
	_ = st.U32()
	insns_size := st.U32()
	insns_start_pos := st.Pos

	insns := make([]uint16, insns_size)
	for i := uint32(0); i < insns_size; i++ {
		insns[i] = st.U16()
	}

	if tries_size > 0 && (insns_size&1) != 0 {
		_ = st.U16() // padding
	}

	self.Tries = make([]TryItem, tries_size)
	for i := uint16(0); i < tries_size; i++ {
		self.Tries[i] = newTry(st)
	}

	list_off := st.Pos
	for i := uint16(0); i < tries_size; i++ {
		self.Tries[i].finish(dex, list_off)
	}

	catch_addrs := make(map[uint32]bool)
	for _, try := range self.Tries {
		for _, catch := range try.Catches {
			catch_addrs[catch.Target] = true
		}
	}

	self.Bytecode = parseBytecode(dex, insns_start_pos, insns, catch_addrs)
	return &self
}

type MethodId struct {
	Triple
	param_types []string
	ReturnType  string
}

func methodId(dex *DexFile, method_idx uint32) MethodId {
	st := dex.stream(dex.method_ids.off + method_idx*8)
	self := MethodId{Triple{Cname: dex.ClsType(uint32(st.U16()))}, nil, ""}
	proto_idx := uint32(st.U16())
	self.Name = dex.String(st.U32())

	st = dex.stream(dex.proto_ids.off + proto_idx*12)
	_ = st.U32()
	self.ReturnType = dex.Type(st.U32())
	self.param_types = typeList(dex, st.U32(), false)

	parts := append([]string{"("}, self.param_types...)
	parts = append(parts, ")", self.ReturnType)
	self.Desc = strings.Join(parts, "")
	return self
}

func (self *MethodId) GetSpacedParamTypes(isstatic bool) []*string {
	results := make([]*string, 0, len(self.param_types)+1)
	if !isstatic {
		if self.Cname[0] == '[' {
			results = append(results, &self.Cname)
		} else {
			temp := "L" + self.Cname + ";"
			results = append(results, &temp)
		}
	}

	for _, ptype := range self.param_types {
		temp := ptype
		results = append(results, &temp)
		if ptype == "J" || ptype == "D" {
			results = append(results, nil)
		}
	}
	return results
}

type Method struct {
	Dex *DexFile
	MethodId
	Access, code_off uint32
	Code             *CodeItem
}

func newMethod(dex *DexFile, method_idx, access, code_off uint32) Method {
	m := Method{dex, methodId(dex, method_idx), access, code_off, nil}
	if code_off != 0 {
		m.Code = newCode(dex, code_off)
	}
	return m
}

type ClassData struct {
	Fields  []Field
	Methods []Method
}

func newClassData(dex *DexFile, offset uint32) *ClassData {
	self := ClassData{}
	if offset == 0 {
		return &self
	}

	stream := dex.stream(offset)
	numstatic := stream.Uleb128()
	numinstance := stream.Uleb128()
	numdirect := stream.Uleb128()
	numvirtual := stream.Uleb128()

	field_idx := uint32(0)
	for i := uint32(0); i < numstatic+numinstance; i++ {
		if i == numstatic {
			field_idx = uint32(0)
		}
		field_idx += stream.Uleb128()
		self.Fields = append(self.Fields, newField(dex, field_idx, stream.Uleb128()))
	}

	method_idx := uint32(0)
	for i := uint32(0); i < numdirect+numvirtual; i++ {
		if i == numdirect {
			method_idx = uint32(0)
		}
		method_idx += stream.Uleb128()
		self.Methods = append(self.Methods, newMethod(dex, method_idx, stream.Uleb128(), stream.Uleb128()))
	}

	return &self
}
