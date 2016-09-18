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
package ops

import (
	"enjarify-go/jvm/arrays"
	"enjarify-go/jvm/scalars"
)

func ArrStoreOp(t arrays.T) byte {
	switch t {
	case "I":
		return IASTORE
	case "J":
		return LASTORE
	case "F":
		return FASTORE
	case "D":
		return DASTORE
	case "B":
		return BASTORE
	case "C":
		return CASTORE
	case "S":
		return SASTORE
	default:
		return AASTORE
	}
}
func ArrLoadOp(t arrays.T) byte { return ArrStoreOp(t) - (-IALOAD + IASTORE) }

var IlfdaOrd = map[scalars.T]uint8{scalars.INT: 0, scalars.LONG: 1, scalars.FLOAT: 2, scalars.DOUBLE: 3, scalars.OBJ: 4}
