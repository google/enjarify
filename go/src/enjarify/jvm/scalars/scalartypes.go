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
package scalars

import "enjarify/dex"

type T uint32

const INVALID T = 0
const INT T = 1 << 0
const FLOAT T = 1 << 1
const OBJ T = 1 << 2
const LONG T = 1 << 3
const DOUBLE T = 1 << 4

const ZERO T = INT | FLOAT | OBJ
const C32 T = INT | FLOAT
const C64 T = LONG | DOUBLE
const ALL T = ZERO | C64

func FromDesc(desc string) T {
	switch desc[0] {
	case 'Z', 'B', 'S', 'C', 'I':
		return INT
	case 'F':
		return FLOAT
	case 'J':
		return LONG
	case 'D':
		return DOUBLE
	default:
		return OBJ
	}
}

func (st T) Wide() bool { return st&C64 != 0 }

func ParamTypes(method_id dex.MethodId, static bool) []T {
	temp := method_id.GetSpacedParamTypes(static)
	ptypes := make([]T, len(temp))
	for i, desc := range temp {
		if desc == nil {
			ptypes[i] = INVALID
		} else {
			ptypes[i] = FromDesc(*desc)
		}
	}
	return ptypes
}
