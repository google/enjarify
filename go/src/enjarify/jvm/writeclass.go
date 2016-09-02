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
package jvm

import (
	"enjarify/byteio"
	"enjarify/dex"
	"enjarify/jvm/cpool"
	"enjarify/jvm/errors"
	"runtime/debug"
)

func writeField(pool cpool.Pool, stream *byteio.Writer, field dex.Field) {
	stream.U16(uint16(field.Access & FIELD_FLAGS))
	stream.U16(pool.Utf8(field.Name))
	stream.U16(pool.Utf8(field.Desc))

	if field.ConstantValue != nil {
		stream.U16(1)
		stream.U16(pool.Utf8("ConstantValue"))
		stream.U32(2)

		// Ignore dalvik constant type and use actual field type instead
		index := uint16(0)
		switch field.Desc {
		case "Z", "B", "S", "C", "I":
			index = pool.Int(field.ConstantValue.(uint32))
		case "F":
			index = pool.Float(field.ConstantValue.(uint32))
		case "J":
			index = pool.Long(field.ConstantValue.(uint64))
		case "D":
			index = pool.Double(field.ConstantValue.(uint64))
		case "Ljava/lang/String;":
			index = pool.String(field.ConstantValue.(string))
		case "Ljava/lang/Class;":
			index = pool.Class(field.ConstantValue.(string))
		}

		stream.U16(index)
	} else {
		stream.U16(0) // no attributes
	}
}
func writeMethod(pool cpool.Pool, stream *byteio.Writer, method dex.Method, code_attrs map[dex.Triple]string) {
	stream.U16(uint16(method.Access & METHOD_FLAGS))
	stream.U16(pool.Utf8(method.Name))
	stream.U16(pool.Utf8(method.Desc))

	if code_attr_data, ok := code_attrs[method.Triple]; ok {
		stream.U16(1)
		stream.U16(pool.Utf8("Code"))
		stream.U32(uint32(len(code_attr_data)))
		stream.WriteString(code_attr_data)
	} else {
		stream.U16(0)
	}
}
func writeMethods(pool cpool.Pool, stream *byteio.Writer, methods []dex.Method, opts Options) {
	code_irs := make([]*IRWriter, 0, len(methods))
	for _, m := range methods {
		code_irs = append(code_irs, getCodeIR(pool, m, opts))
	}

	code_attrs := finishCodeAttrs(pool, code_irs, opts)
	stream.U16(uint16(len(methods)))
	for _, method := range methods {
		writeMethod(pool, stream, method, code_attrs)
	}
}
func classFileAfterPool(cls dex.DexClass, opts Options, canfail bool) (pool cpool.Pool, stream *byteio.Writer, failed bool) {
	if canfail {
		defer func() {
			recovered := recover()
			_, ok := recovered.(*errors.ClassfileLimitExceeded)
			if recovered != nil && !ok {
				debug.PrintStack()
				panic(recovered)
			}
			failed = ok
		}()
	}

	stream = byteio.NewWriter()
	if opts.SplitPool {
		pool = cpool.Split()
	} else {
		pool = cpool.Simple()
	}
	cls.ParseData()

	stream.U16(uint16(cls.Access & CLASS_FLAGS))
	stream.U16(pool.Class(cls.Name))
	if cls.Super == nil {
		stream.U16(0)
	} else {
		stream.U16(pool.Class(*cls.Super))
	}

	// interfaces
	stream.U16(uint16(len(cls.Interfaces)))
	for _, i := range cls.Interfaces {
		stream.U16(pool.Class(i))
	}

	// fields
	stream.U16(uint16(len(cls.Data.Fields)))
	for _, i := range cls.Data.Fields {
		writeField(pool, stream, i)
	}

	// methods
	writeMethods(pool, stream, cls.Data.Methods, opts)

	// attributes
	stream.U16(0)
	return
}

func ToClassFile(cls dex.DexClass, opts Options) (result string, err error) {
	defer func() {
		temp := recover()
		if temp != nil {
			err = temp.(error)
		}
	}()

	stream := byteio.NewWriter()
	stream.U32(0xCAFEBABE)
	// bytecode version 49.0
	stream.U16(0)
	stream.U16(49)

	// Optimistically try translating without optimization to speed things up
	// if the resulting code is too big, retry with optimization
	pool, rest_stream, failed := classFileAfterPool(cls, opts, true)
	if failed {
		// fmt.Printf("Retrying %s with optimization enabled\n", cls.Name)
		pool, rest_stream, _ = classFileAfterPool(cls, ALL, false)
	}

	// write constant pool
	pool.Write(stream)
	// write rest of file
	stream.Append(rest_stream)
	result = stream.String()
	return
}
