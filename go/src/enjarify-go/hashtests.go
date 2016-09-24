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

import (
	"crypto/sha256"
	"enjarify-go/jvm"
	"enjarify-go/util"
	"fmt"
	"path"
)

func hash(s string) string {
	digest := sha256.Sum256([]byte(s))
	return string(digest[:])
}

func hashTests() {
	// defer profile.Start().Stop()
	fullhash := ""
	for test := 1; test < 8; test++ {
		name := fmt.Sprintf("test%d", test)
		fmt.Print(name + "\n")
		dir := path.Join("..", "tests", name)
		rawdex := Read(path.Join(dir, "classes.dex"))

		for bits := 0; bits < 256; bits++ {

			opts := jvm.Options{bits&1 == 1, bits&2 == 2, bits&4 == 4, bits&8 == 8, bits&16 == 16, bits&32 == 32, bits&64 == 64, bits&128 == 128}
			classes, ordkeys, errors := translate(opts, rawdex)
			util.Assert(len(errors) == 0)

			for _, k := range ordkeys {
				// for i, k := range ordkeys {
				cls := classes[k]

				// fname := fmt.Sprintf("../../rsout/%d_%d_%d.class", test, bits, i)
				// Write(fname, cls)

				fmt.Printf("%08b %x\n", uint8(bits), hash(cls))
				fullhash = hash(fullhash + cls)
			}
		}
	}

	fmt.Printf("done!\nFinal hash: %x\n", hash(fullhash))
}
