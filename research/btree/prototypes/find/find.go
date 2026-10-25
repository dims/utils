// Copyright 2025 The Kubernetes Authors.
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

package btree

import "sort"

// Find looks for the item that matches key and returns it, or (zeroValue,
// false) if there is none. cmp compares a key with an item: it returns a
// negative number when the key sorts before the item, zero when they match,
// and a positive number when the key sorts after the item. Its order must
// agree with the tree's LessFunc.
//
// Unlike Get, Find needs no probe item: a caller with a string key can look
// up an item whose key is that string without allocating an item for it.
func Find[T, K any](t *BTree[T], key K, cmp func(K, T) int) (_ T, _ bool) {
	for n := t.root; n != nil; {
		i := sort.Search(len(n.items), func(i int) bool { return cmp(key, n.items[i]) <= 0 })
		if i < len(n.items) && cmp(key, n.items[i]) == 0 {
			return n.items[i], true
		}
		if len(n.children) == 0 {
			return
		}
		n = n.children[i]
	}
	return
}

// FindBisect is Find with a hand-rolled three-way binary search instead of
// sort.Search. It is here only to measure the search loop separately from
// the probe removal; a hand-rolled loop in find gained nothing on x86.
func FindBisect[T, K any](t *BTree[T], key K, cmp func(K, T) int) (_ T, _ bool) {
	for n := t.root; n != nil; {
		lo, hi := 0, len(n.items)
		for lo < hi {
			m := int(uint(lo+hi) >> 1)
			c := cmp(key, n.items[m])
			if c == 0 {
				return n.items[m], true
			}
			if c < 0 {
				hi = m
			} else {
				lo = m + 1
			}
		}
		if len(n.children) == 0 {
			return
		}
		n = n.children[lo]
	}
	return
}

// GetFunc looks for the item for which cmp returns zero and returns it, or
// (zeroValue, false) if there is none. cmp compares an item with the key
// the caller looks for: negative when the item sorts before the key, zero
// when they match, positive when it sorts after. Its order must agree with
// the tree's LessFunc.
func (t *BTree[T]) GetFunc(cmp func(T) int) (_ T, _ bool) {
	for n := t.root; n != nil; {
		i := sort.Search(len(n.items), func(i int) bool { return cmp(n.items[i]) >= 0 })
		if i < len(n.items) && cmp(n.items[i]) == 0 {
			return n.items[i], true
		}
		if len(n.children) == 0 {
			return
		}
		n = n.children[i]
	}
	return
}
