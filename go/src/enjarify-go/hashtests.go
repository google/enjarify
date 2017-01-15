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
	sem := make(chan struct{}, 9999)
	outputs := [256 * 7][][2]string{}

	for test := 1; test < 8; test++ {
		name := fmt.Sprintf("test%d", test)
		dir := path.Join("..", "tests", name)
		rawdex := Read(path.Join(dir, "classes.dex"))

		for bits := 0; bits < 256; bits++ {
			go func(test, bits int, rawdex string) {
				opts := jvm.Options{bits&1 == 1, bits&2 == 2, bits&4 == 4, bits&8 == 8, bits&16 == 16, bits&32 == 32, bits&64 == 64, bits&128 == 128}
				results := translate(opts, rawdex)

				output := make([][2]string, len(results))
				outputs[(test-1)*256+bits] = output

				for i := range results {
					cls := results[i][1]
					util.Assert(cls != "")
					output[i][0] = cls
					output[i][1] = hash(cls)
				}
				sem <- struct{}{}
			}(test, bits, rawdex)
		}
	}

	for i := 0; i < 7*256; i++ {
		<-sem
	}

	fullhash := ""
	for test := 1; test < 8; test++ {
		name := fmt.Sprintf("test%d", test)
		fmt.Print(name + "\n")
		for bits := 0; bits < 256; bits++ {
			for _, pair := range outputs[(test-1)*256+bits] {
				fmt.Printf("%08b %x\n", uint8(bits), pair[1])
				fullhash = hash(fullhash + pair[0])
			}
		}
	}

	fmt.Printf("done!\nFinal hash: %x\n", hash(fullhash))
}
