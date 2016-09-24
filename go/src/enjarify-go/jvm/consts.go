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
	"enjarify-go/jvm/cpool"
	"enjarify-go/jvm/ir"
)

func AllocateRequiredConstants(pool cpool.Pool, long_irs []*IRWriter) {
	// see comments in writebytecode.finishCodeAttrs
	// We allocate the constants pretty much greedily. This is far from optimal,
	// but it shouldn't be a big deal since this code is almost never required
	// in the first place. In fact, there are no known real world classes that
	// even come close to exhausting the constant pool.

	narrow_pairs := map[cpool.Pair]int{}
	wide_pairs := map[cpool.Pair]int{}
	alt_lens := map[cpool.Pair]int{}
	for _, irw := range long_irs {
		for _, instr := range irw.Instructions {
			if instr.Tag == ir.PRIMCONSTANT {
				key := instr.PrimConstant.Pair
				alt_lens[key] = len(instr.Bytecode)
				if instr.PrimConstant.T.Wide() {
					if len(instr.Bytecode) > 3 {
						wide_pairs[key] += 1
					}
				} else {
					if len(instr.Bytecode) > 2 {
						narrow_pairs[key] += 1
					}
				}
			}
		}
	}
	// see if already in the constant pool
	for _, x := range pool.Vals() {
		delete(narrow_pairs, x)
		delete(wide_pairs, x)
	}

	// if we have enough space for all required constants, preferentially allocate
	// most commonly used constants to first 255 slots
	if pool.Space() >= len(narrow_pairs)+2*len(wide_pairs) && pool.LowSpace() > 0 {
		// Sort by negative count, then by key
		// Make sure this is determinstic in the case of ties.
		items := make(pislice, 0, len(narrow_pairs))
		for p, count := range narrow_pairs {
			items = append(items, pairint{-count, p})
		}

		for _, item := range items.Sort()[:pool.LowSpace()] {
			pool.InsertDirectly(item.key, true)
			delete(narrow_pairs, item.key)
		}
	}

	scores := map[cpool.Pair]int{}
	for p, count := range narrow_pairs {
		scores[p] = (alt_lens[p] - 3) * count
	}
	for p, count := range wide_pairs {
		scores[p] = (alt_lens[p] - 3) * count
	}

	// sort by score, then by key
	temp := func(m map[cpool.Pair]int) (res pislice) {
		for p, _ := range m {
			res = append(res, pairint{scores[p], p})
		}
		return
	}

	narrowq := temp(narrow_pairs).Sort()
	wideq := temp(wide_pairs).Sort()
	for pool.Space() >= 1 && len(narrowq)+len(wideq) > 0 {
		if len(narrowq) == 0 && pool.Space() < 2 {
			break
		}
		nscore := 0
		if len(narrowq) >= 1 {
			nscore += scores[narrowq[len(narrowq)-1].key]
		}
		if len(narrowq) >= 2 {
			nscore += scores[narrowq[len(narrowq)-2].key]
		}
		wscore := 0
		if len(wideq) >= 1 {
			wscore += scores[wideq[len(wideq)-1].key]
		}

		if pool.Space() >= 2 && wscore > nscore && wscore > 0 {
			pool.InsertDirectly(wideq[len(wideq)-1].key, false)
			wideq = wideq[:len(wideq)-1]
		} else if nscore > 0 {
			pool.InsertDirectly(narrowq[len(narrowq)-1].key, true)
			narrowq = narrowq[:len(narrowq)-1]
		} else {
			break
		}
	}
}
