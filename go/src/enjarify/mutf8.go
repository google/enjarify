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
package main

import "unicode/utf8"

func decode(b []byte) (out []rune) {
	// decode arbitrary utf8 codepoints, tolerating surrogate pairs, nonstandard encodings, etc.
	ind := 0
	for ind < len(b) {
		x := b[ind]
		ind++
		if x < 128 {
			out = append(out, rune(x))
		} else {
			// figure out how many bytes
			extra := 0
			for i := byte(6); i >= 0 && (x&(1<<i) > 0); i-- {
				extra++
			}

			bits := rune(x % (1 << uint(6-extra)))
			for i := 0; i < extra; i++ {
				bits = (bits << 6) ^ (rune(b[ind]) & 63)
				ind++
			}
			out = append(out, bits)
		}
	}
	return
}

func fixPairs(codes []rune) (out []rune) {
	// convert surrogate pairs to single code points
	ind := 0
	for ind < len(codes) {
		x := codes[ind]
		ind++
		if 0xD800 <= x && x < 0xDC00 {
			high := x - 0xD800
			low := codes[ind] - 0xDC00
			ind++
			x = 0x10000 + (high << 10) + (low & 1023)
		}
		out = append(out, rune(x))
	}
	return
}

func Decode(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	return string(fixPairs(decode([]byte(s))))
}
