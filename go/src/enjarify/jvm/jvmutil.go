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
	"enjarify/jvm/cpool"
	"enjarify/jvm/ir"
	"sort"
)

func keys1(m map[uint32]*irBlock) (res USlice) {
	for k, _ := range m {
		res = append(res, k)
	}
	return
}
func keys2(m map[uint32]*ir.Label) (res USlice) {
	for k, _ := range m {
		res = append(res, k)
	}
	return
}

type USlice []uint32

func (p USlice) Len() int           { return len(p) }
func (p USlice) Less(i, j int) bool { return p[i] < p[j] }
func (p USlice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func (p USlice) Sort() USlice {
	sort.Sort(p)
	return p
}

type pairint struct {
	count int
	key   cpool.Pair
}
type pislice []pairint

// will only be used with const keys (set X) anyway
func ckless(a, b cpool.Pair) bool { return a.Tag < b.Tag || a.Tag == b.Tag && a.X < b.X }

func (p pislice) Len() int { return len(p) }
func (p pislice) Less(i, j int) bool {
	return p[i].count < p[j].count || p[i].count == p[j].count && ckless(p[i].key, p[j].key)
}

func (p pislice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }
func (p pislice) Sort() pislice {
	sort.Sort(p)
	return p
}
