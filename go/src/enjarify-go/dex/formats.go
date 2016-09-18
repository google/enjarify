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

var formats = [256]string{"10x", "12x", "22x", "32x", "12x", "22x", "32x", "12x", "22x", "32x", "11x", "11x", "11x", "11x", "10x", "11x", "11x", "11x", "11n", "21s", "31i", "21h", "21s", "31i", "51l", "21h", "21c", "31c", "21c", "11x", "11x", "21c", "22c", "12x", "21c", "22c", "35c", "3rc", "31t", "11x", "10t", "20t", "30t", "31t", "31t", "23x", "23x", "23x", "23x", "23x", "22t", "22t", "22t", "22t", "22t", "22t", "21t", "21t", "21t", "21t", "21t", "21t", "10x", "10x", "10x", "10x", "10x", "10x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "22c", "22c", "22c", "22c", "22c", "22c", "22c", "22c", "22c", "22c", "22c", "22c", "22c", "22c", "21c", "21c", "21c", "21c", "21c", "21c", "21c", "21c", "21c", "21c", "21c", "21c", "21c", "21c", "35c", "35c", "35c", "35c", "35c", "10x", "3rc", "3rc", "3rc", "3rc", "3rc", "10x", "10x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "23x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "12x", "22s", "22s", "22s", "22s", "22s", "22s", "22s", "22s", "22b", "22b", "22b", "22b", "22b", "22b", "22b", "22b", "22b", "22b", "22b", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x", "10x"}

type DalvikArgs struct {
	A, B, C    uint32
	Ra, Rb, Rc uint16
	Long       uint64
	Args       []uint16
}

func decode(shorts []uint16, pos uint32, opcode uint8) (uint32, DalvikArgs) {
	format := formats[opcode]
	d := DalvikArgs{}
	size := format[0] - '0'

	switch format[0] {
	case '1':
		w := uint32(shorts[pos])
		switch format {
		case "12x", "11n":
			d.A = (w >> 8) & 0xF
			d.B = w >> 12
		case "11x", "10t":
			d.A = w >> 8
		}

	case '2':
		w := uint32(shorts[pos])
		w2 := uint32(shorts[pos+1])
		switch format {
		case "20t":
			d.A = w2
		case "22x", "21t", "21s", "21h", "21c":
			d.A = w >> 8
			d.B = w2
		case "23x", "22b":
			d.A = w >> 8
			d.B = w2 & 0xFF
			d.C = w2 >> 8
		case "22t", "22s", "22c":
			d.A = (w >> 8) & 0xF
			d.B = w >> 12
			d.C = w2
		}

	case '3':
		w := uint32(shorts[pos])
		w2 := uint32(shorts[pos+1])
		w3 := uint32(shorts[pos+2])

		switch format {
		case "30t":
			d.A = w2 ^ (w3 << 16)
		case "32x":
			d.A = w2
			d.B = w3
		case "31i", "31t", "31c":
			d.A = w >> 8
			d.B = w2 ^ (w3 << 16)
		case "35c":
			a := w >> 12
			d.A = w2
			c, d_, e, f := uint16(w3)&0xF, uint16(w3>>4)&0xF, uint16(w3>>8)&0xF, uint16(w3>>12)&0xF
			g := uint16(w>>8) & 0xF
			d.Args = []uint16{c, d_, e, f, g}[:a]
		case "3rc":
			a := w >> 8
			d.A = w2
			for i := w3; i < w3+a; i++ {
				d.Args = append(d.Args, uint16(i))
			}
		}
	case '5':
		d.A = uint32(shorts[pos]) >> 8
		for i := uint32(0); i < 4; i++ {
			d.Long ^= uint64(shorts[pos+1+i]) << (16 * i)
		}
	}

	// Check if we need to sign extend
	switch format {
	case "11n":
		d.B = uint32(int8(d.B<<4) >> 4)
	case "10t":
		d.A = uint32(int8(d.A))
	case "22b":
		d.C = uint32(int8(d.C))
	case "20t":
		d.A = uint32(int16(d.A))
	case "21t", "21s":
		d.B = uint32(int16(d.B))
	case "22t", "22s":
		d.C = uint32(int16(d.C))
	}

	// Hats depend on actual size expected, so we rely on opcode as a hack
	if format[2] == 'h' {
		if opcode == 0x15 {
			d.B = d.B << 16
		} else {
			d.Long = uint64(d.B) << 48
		}
	}

	// Make sure const-wide is always stored in d.Long, even if it's short
	if opcode == 0x16 || opcode == 0x17 {
		d.Long = uint64(d.B)
	}

	// Convert code offsets to actual code position
	if format[2] == 't' {
		switch format[1] {
		case '0':
			d.A += pos
		case '1':
			d.B += pos
		case '2':
			d.C += pos
		}
	}

	d.Ra = uint16(d.A)
	d.Rb = uint16(d.B)
	d.Rc = uint16(d.C)
	return pos + uint32(size), d
}
