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
	"enjarify-go/byteio"
	"enjarify-go/dex"
	"enjarify-go/jvm/cpool"
	"enjarify-go/jvm/errors"
	"enjarify-go/jvm/ir"
	"strings"
)

func getCodeIR(pool cpool.Pool, method dex.Method, opts Options) *IRWriter {
	if method.Code == nil {
		return nil
	}

	irdata := writeBytecode(pool, method, opts)

	if opts.InlineConsts {
		InlineConsts(irdata)
	}

	if opts.CopyPropagation {
		CopyPropagation(irdata)
	}

	if opts.RemoveUnusedRegs {
		RemoveUnusedRegisters(irdata)
	}

	if opts.Dup2ize {
		Dup2ize(irdata)
	}

	if opts.PruneStoreLoads {
		PruneStoreLoads(irdata)
		if opts.RemoveUnusedRegs {
			RemoveUnusedRegisters(irdata)
		}
	}

	if opts.SortRegisters {
		SortAllocateRegisters(irdata)
	} else {
		SimpleAllocateRegisters(irdata)
	}

	return irdata
}

func finishCodeAttrs(pool cpool.Pool, code_irs []*IRWriter, opts Options) map[dex.Triple]string {
	irs := make([]*IRWriter, 0, len(code_irs))
	for _, w := range code_irs {
		if w != nil {
			irs = append(irs, w)
		}
	}

	// if we have any code, make sure to reserve pool slot for attr name
	if len(irs) > 0 {
		pool.Utf8("Code")
	}

	if opts.DelayConsts {
		// In the rare case where the class references too many constants to fit in
		// the constant pool, we can workaround this by replacing primative constants
		// e.g. ints, longs, floats, and doubles, with a sequence of bytecode instructions
		// to generate that constant. This obviously increases the size of the method's
		// bytecode, so we ideally only want to do it to constants in short methods.

		// First off, we find which methods are potentially too long. If a method
		// will be under 65536 bytes even with all constants replaced, then it
		// will be ok no matter what we do.
		long_irs := []*IRWriter{}
		for _, irw := range irs {
			if irw.CalcUpperBound() >= 65536 {
				long_irs = append(long_irs, irw)
			}
		}

		// Now allocate constants used by potentially long methods
		if len(long_irs) > 0 {
			AllocateRequiredConstants(pool, long_irs)
		}

		// If there's space left in the constant pool, allocate constants used by short methods
		for _, irw := range irs {
			for i, instr := range irw.Instructions {
				if instr.Tag == ir.PRIMCONSTANT {
					instr.FixWithPool(pool, &irw.Instructions[i])
				}
			}
		}
	}

	res := map[dex.Triple]string{}
	for _, irdata := range irs {
		res[irdata.method.Triple] = writeCodeAttributeTail(pool, irdata, opts)
	}
	return res
}

func writeCodeAttributeTail(pool cpool.Pool, irdata *IRWriter, opts Options) string {
	optimizeJumps(irdata)
	bytecode, excepts := createBytecode(irdata)

	stream := byteio.NewWriter()
	// For simplicity, don't bother calculating the actual maximum stack height
	// of the generated code. Instead, just use a value that will always be high
	// enough. Note that just setting this to 65535 is a bad idea since it tends
	// to cause StackOverflowErrors under default JVM memory settings
	stream.U16(300)            // stack
	stream.U16(irdata.numregs) // locals

	stream.U32(uint32(len(bytecode)))
	stream.WriteString(bytecode)

	if len(bytecode) > 65535 {
		// If code is too long and optimization is off, raise exception so we can
		// retry with optimization. If it is still too long with optimization,
		// don't raise an error, since a class with illegally long code is better
		// than no output at all.
		if opts != ALL {
			panic(&errors.ClassfileLimitExceeded{})
		}
	}
	// exceptions
	stream.U16(uint16(len(excepts)))
	stream.WriteString(strings.Join(excepts, ""))

	// attributes
	stream.U16(0)
	return stream.String()
}
