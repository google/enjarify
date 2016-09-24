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
package constants

import (
	"enjarify-go/jvm/scalars"
	"enjarify-go/util"
	"math/big"
)

// Calculate a sequence of bytecode instructions to generate the given constant
// to be used in the rare case that the constant pool is full.

// NaN has multiple representations, so normalize Floats to a single NaN representation
func normalizeFloat(x uint64) uint64 {
	if x|FLOAT_SIGN > FLOAT_NINF {
		return FLOAT_NAN
	}
	return x
}
func normalizeDouble(x uint64) uint64 {
	if x|DOUBLE_SIGN > DOUBLE_NINF {
		return DOUBLE_NAN
	}
	return x
}

type buffer []byte

func (self *buffer) append(s string)  { *self = append(*self, s...) }
func (self *buffer) appendb(s []byte) { *self = append(*self, s...) }
func (self *buffer) push(s ...byte)   { *self = append(*self, s...) }

func (self *buffer) calcInt(x int32) {
	if res, ok := INTS[x]; ok {
		self.append(res)
		return
	}
	// max required - 10 bytes
	// (high << 16) ^ low
	low := int32(int16(x))
	high := (x ^ low) >> 16
	if low == 0 {
		self.calcInt(high)
		self.calcInt(16)
		self.push(ISHL)
	} else {
		self.calcInt(high)
		self.calcInt(16)
		self.push(ISHL)
		self.calcInt(low)
		self.push(IXOR)
	}
}
func (self *buffer) calcLong(x int64) {
	if res, ok := LONGS[x]; ok {
		self.append(res)
		return
	}
	// max required - 26 bytes
	// (high << 32) ^ low
	low := int32(x)
	high := int32((x ^ int64(low)) >> 32)
	if high == 0 {
		self.calcInt(low)
		self.push(I2L)
		return
	}
	self.calcInt(high)
	self.push(I2L)
	self.calcInt(32)
	self.push(LSHL)
	if low != 0 {
		self.calcInt(low)
		self.push(I2L, LXOR)
	}
}

func (self *buffer) calcFloat(x uint64) {
	if res, ok := FLOATS[x]; ok {
		self.append(res)
		return
	}
	// max required - 27 bytes
	exponent := int32((x>>23)&0xFF) - 127
	mantissa := int32(x % (1 << 23))
	// check for denormals!
	if exponent == -127 {
		exponent += 1
	} else {
		mantissa += 1 << 23
	}
	exponent -= 23

	if x&FLOAT_SIGN > 0 {
		mantissa = -mantissa
	}
	ex_combine_op := byte(FMUL)
	if exponent < 0 {
		ex_combine_op = FDIV
		exponent = -exponent
	}

	exponent_part := buffer{}
	for exponent >= 63 { // max 2 iterations since -149 <= exp <= 104
		exponent_part.push(LCONST_1, ICONST_M1, LSHL, L2F, ex_combine_op)
		mantissa = -mantissa
		exponent -= 63
	}
	if exponent > 0 {
		exponent_part.push(LCONST_1)
		exponent_part.calcInt(exponent)
		exponent_part.push(LSHL, L2F, ex_combine_op)
	}
	self.calcInt(mantissa)
	self.push(I2F)
	self.appendb(exponent_part)
}

func (self *buffer) calcDouble(x uint64) {
	if res, ok := DOUBLES[x]; ok {
		self.append(res)
		return
	}
	// max required - 55 bytes
	exponent := int32((x>>52)&0x7FF) - 1023
	mantissa := int64(x % (1 << 52))
	// check for denormals!
	if exponent == -1023 {
		exponent += 1
	} else {
		mantissa += 1 << 52
	}
	exponent -= 52

	if x&DOUBLE_SIGN > 0 {
		mantissa = -mantissa
	}

	ex_combine_op := byte(DMUL)
	abs_exponent := exponent
	if exponent < 0 {
		ex_combine_op = DDIV
		abs_exponent = -exponent
	}

	exponent_part := buffer{}
	part63 := abs_exponent / 63
	if part63 > 0 { //create *63 part of exponent by repeated squaring
		// use 2^-x instead of calculating 2^x and dividing to avoid overflow in
		// case we need 2^-1071
		if exponent < 0 { // -2^-63
			exponent_part.push(DCONST_1, LCONST_1, ICONST_M1, LSHL, L2D, DDIV)
		} else { // -2^63
			exponent_part.push(LCONST_1, ICONST_M1, LSHL, L2D)
		}
		// adjust sign of mantissa for odd powers since we're actually using -2^63 rather than positive
		if part63&1 > 0 {
			mantissa = -mantissa
		}
		dmuls := []byte{DMUL} // include term from leading one in part63
		last_needed := part63 & 1
		for bi := 1; bi < big.NewInt(int64(part63)).BitLen(); bi++ {
			exponent_part.push(DUP2)
			if last_needed > 0 {
				exponent_part.push(DUP2)
				dmuls = append(dmuls, DMUL)
			}
			exponent_part.push(DMUL)
			last_needed = part63 & (1 << uint32(bi))
		}
		exponent_part.appendb(dmuls)
	}

	// now handle the rest
	rest := int32(abs_exponent % 63)
	if rest > 0 {
		exponent_part.push(LCONST_1)
		exponent_part.calcInt(rest)
		exponent_part.push(LSHL, L2D)
		exponent_part.push(ex_combine_op)
	}
	self.calcLong(mantissa)
	self.push(L2D)
	self.appendb(exponent_part)
}

func calcInt(x uint64) string {
	b := buffer{}
	b.calcInt(int32(x))
	return string(b)
}
func calcLong(x uint64) string {
	b := buffer{}
	b.calcLong(int64(x))
	return string(b)
}
func calcFloat(x uint64) string {
	b := buffer{}
	b.calcFloat(normalizeFloat(x))
	return string(b)
}
func calcDouble(x uint64) string {
	b := buffer{}
	b.calcDouble(normalizeDouble(x))
	return string(b)
}

func Normalize(st scalars.T, val uint64) uint64 {
	if st == scalars.FLOAT {
		return normalizeFloat(val)
	} else if st == scalars.DOUBLE {
		return normalizeDouble(val)
	}
	return val
}

func Calc(st scalars.T, val uint64) string {
	if st == scalars.INT {
		return calcInt(val)
	} else if st == scalars.FLOAT {
		return calcFloat(val)
	} else if st == scalars.LONG {
		return calcLong(val)
	} else if st == scalars.DOUBLE {
		return calcDouble(val)
	}
	panic(util.Unreachable)
}

func lookupOnly(st scalars.T, val uint64) (ret string, ok bool) {
	// assume floats and double have already been normalized but int/longs haven't
	if st == scalars.INT {
		ret, ok = INTS[int32(val)]
	} else if st == scalars.FLOAT {
		ret, ok = FLOATS[val]
	} else if st == scalars.LONG {
		ret, ok = LONGS[int64(val)]
	} else if st == scalars.DOUBLE {
		ret, ok = DOUBLES[val]
	}
	return
}
func LookupOnly(st scalars.T, val uint64) *string {
	if val, ok := lookupOnly(st, val); ok {
		return &val
	}
	return nil
}
