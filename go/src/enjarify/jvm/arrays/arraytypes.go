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
package arrays

import (
	"enjarify/jvm/scalars"
	"enjarify/util"
	"strings"
)

type T string

const INVALID T = "INVALID"
const NULL T = "NULL"

func (t1 T) Merge(t2 T) T {
	if t1 == NULL {
		return t2
	}
	if t2 == NULL {
		return t1
	}
	if t1 == t2 {
		return t1
	}
	return INVALID
}

// intersect types
func (t1 T) Narrow(t2 T) T {
	if t1 == INVALID {
		return t2
	}
	if t2 == INVALID {
		return t1
	}
	if t1 == t2 {
		return t1
	}
	return NULL
}

func (t T) EletPair() (scalars.T, T) {
	if t == INVALID {
		return scalars.OBJ, t
	}

	util.Assert(t[0] == '[')
	t = t[1:]
	return scalars.FromDesc(string(t)), t
}

func FromDesc(desc string) T {
	if desc[0] != '[' || desc[len(desc)-1] == ';' {
		return INVALID
	}
	return T(strings.Replace(desc, "Z", "B", -1)) // treat bool arrays as byte arrays
}
