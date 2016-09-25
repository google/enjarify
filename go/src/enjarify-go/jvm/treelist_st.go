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

// This class represents a list as a persistent n-ary tree
// This has much slower access and updates than a real list but has the advantage
// of sharing memory with previous versions of the list when only a few elements
// are changed. See http://en.wikipedia.org/wiki/Persistent_data_structure//Trees
// Also, default values are not stored, so this is good for sparse arrays
type ImmutableTreeListᐸScalarTᐳ struct {
	missing  ScalarT
	direct   [16]ScalarT
	children [16]*ImmutableTreeListᐸScalarTᐳ
}

func newTreeListᐸScalarTᐳ(missing ScalarT) *ImmutableTreeListᐸScalarTᐳ {
	self := ImmutableTreeListᐸScalarTᐳ{missing: missing}
	for i := 0; i < 16; i++ {
		self.direct[i] = missing
	}
	// Subtrees allocated lazily
	return &self
}

func (self *ImmutableTreeListᐸScalarTᐳ) get(i uint16) ScalarT {
	if i < 16 {
		return self.direct[i]
	}
	i -= 16

	ci := i % 16
	i = i / 16
	child := self.children[ci]
	if child == nil {
		return self.missing
	}
	return child.get(i)
}

func (self *ImmutableTreeListᐸScalarTᐳ) set(i uint16, val ScalarT) *ImmutableTreeListᐸScalarTᐳ {
	if i < 16 {
		if val == self.direct[i] {
			return self
		}

		temp := self.direct
		temp[i] = val
		return &ImmutableTreeListᐸScalarTᐳ{self.missing, temp, self.children}
	}

	i -= 16

	ci := i % 16
	i = i / 16
	child := self.children[ci]

	if child == nil {
		if val == self.missing {
			return self
		}
		child = newTreeListᐸScalarTᐳ(self.missing).set(i, val)
	} else {
		if val == child.get(i) {
			return self
		}
		child = child.set(i, val)
	}

	temp := self.children
	temp[ci] = child
	return &ImmutableTreeListᐸScalarTᐳ{self.missing, self.direct, temp}
}

func (left *ImmutableTreeListᐸScalarTᐳ) merge(right *ImmutableTreeListᐸScalarTᐳ, f func(ScalarT, ScalarT) ScalarT) *ImmutableTreeListᐸScalarTᐳ {
	// Effectively computes [func(x, y) for x, y in zip(left, right)]
	// Assume func(x, x) == x
	if left == right {
		return left
	}

	if left == nil {
		left, right = right, left
	}

	missing := left.missing
	direct := [16]ScalarT{}
	children := [16]*ImmutableTreeListᐸScalarTᐳ{}

	if right == nil {
		for i, x := range left.direct {
			direct[i] = f(x, missing)
		}
		for i, child := range left.children {
			children[i] = child.merge(nil, f)
		}
	} else {
		for i, x := range left.direct {
			direct[i] = f(x, right.direct[i])
		}
		for i, child := range left.children {
			children[i] = child.merge(right.children[i], f)
		}

		if direct == right.direct && children == right.children {
			return right
		}
	}

	if direct == left.direct && children == left.children {
		return left
	}
	return &ImmutableTreeListᐸScalarTᐳ{missing, direct, children}
}
