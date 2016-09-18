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
	"strings"
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
func _calcInt(x int32) string {
	if res, ok := INTS[x]; ok {
		return res
	}
	// max required - 10 bytes
	// (high << 16) ^ low
	low := int32(int16(x))
	high := (x ^ low) >> 16
	if low == 0 {
		return _calcInt(high) + _calcInt(16) + string([]byte{ISHL})
	}
	return _calcInt(high) + _calcInt(16) + string([]byte{ISHL}) + _calcInt(low) + string([]byte{IXOR})
}
func _calcLong(x int64) string {
	if res, ok := LONGS[x]; ok {
		return res
	}
	// max required - 26 bytes
	// (high << 32) ^ low
	low := int32(x)
	high := int32((x ^ int64(low)) >> 32)
	if high == 0 {
		return _calcInt(low) + string([]byte{I2L})
	}
	result := _calcInt(high) + string([]byte{I2L}) + _calcInt(32) + string([]byte{LSHL})
	if low != 0 {
		result += _calcInt(low) + string([]byte{I2L, LXOR})
	}
	return result
}

func _calcFloat(x uint64) string {
	if res, ok := FLOATS[x]; ok {
		return res
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

	exponent_parts := []string{}
	for exponent >= 63 { // max 2 iterations since -149 <= exp <= 104
		exponent_parts = append(exponent_parts, string([]byte{LCONST_1, ICONST_M1, LSHL, L2F, ex_combine_op}))
		mantissa = -mantissa
		exponent -= 63
	}
	if exponent > 0 {
		exponent_parts = append(exponent_parts, string([]byte{LCONST_1}))
		exponent_parts = append(exponent_parts, _calcInt(exponent))
		exponent_parts = append(exponent_parts, string([]byte{LSHL, L2F, ex_combine_op}))
	}
	return _calcInt(mantissa) + string([]byte{I2F}) + strings.Join(exponent_parts, "")
}

func _calcDouble(x uint64) string {
	if res, ok := DOUBLES[x]; ok {
		return res
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
	exponent_parts := []string{}

	part63 := abs_exponent / 63
	if part63 > 0 { //create *63 part of exponent by repeated squaring
		// use 2^-x instead of calculating 2^x and dividing to avoid overflow in
		// case we need 2^-1071
		if exponent < 0 { // -2^-63
			exponent_parts = append(exponent_parts, string([]byte{DCONST_1, LCONST_1, ICONST_M1, LSHL, L2D, DDIV}))
		} else { // -2^63
			exponent_parts = append(exponent_parts, string([]byte{LCONST_1, ICONST_M1, LSHL, L2D}))
		}
		// adjust sign of mantissa for odd powers since we're actually using -2^63 rather than positive
		if part63&1 > 0 {
			mantissa = -mantissa
		}
		dmuls := []byte{DMUL} // include term from leading one in part63
		last_needed := part63 & 1
		for bi := 1; bi < big.NewInt(int64(part63)).BitLen(); bi++ {
			exponent_parts = append(exponent_parts, string([]byte{DUP2}))
			if last_needed > 0 {
				exponent_parts = append(exponent_parts, string([]byte{DUP2}))
				dmuls = append(dmuls, DMUL)
			}
			exponent_parts = append(exponent_parts, string([]byte{DMUL}))
			last_needed = part63 & (1 << uint32(bi))
		}
		exponent_parts = append(exponent_parts, string(dmuls))
	}

	// now handle the rest
	rest := int32(abs_exponent % 63)
	if rest > 0 {
		exponent_parts = append(exponent_parts, string([]byte{LCONST_1}))
		exponent_parts = append(exponent_parts, _calcInt(rest))
		exponent_parts = append(exponent_parts, string([]byte{LSHL, L2D}))
		exponent_parts = append(exponent_parts, string([]byte{ex_combine_op}))
	}
	return _calcLong(mantissa) + string([]byte{L2D}) + strings.Join(exponent_parts, "")
}

func calcInt(x uint64) string {
	return _calcInt(int32(x))
}
func calcLong(x uint64) string {
	return _calcLong(int64(x))
}
func calcFloat(x uint64) string {
	return _calcFloat(normalizeFloat(x))
}
func calcDouble(x uint64) string {
	return _calcDouble(normalizeDouble(x))

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
