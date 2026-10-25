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

// CountRange returns the number of items in the range [greaterOrEqual,
// lessThan). It descends the tree twice and visits no item in the range, so
// it costs O(degree * log n) where a walk with AscendRange costs O(n).
func (t *BTree[T]) CountRange(greaterOrEqual, lessThan T) int {
	if t.root == nil || !t.cow.less(greaterOrEqual, lessThan) {
		return 0
	}
	return t.root.rank(lessThan) - t.root.rank(greaterOrEqual)
}

// rank returns the number of items in the subtree that sort before key.
func (n *node[T]) rank(key T) int {
	rank := 0
	for {
		i, found := n.items.find(key, n.cow.less)
		rank += i
		if len(n.children) == 0 {
			return rank
		}
		for _, child := range n.children[:i] {
			rank += child.count
		}
		if found {
			return rank + n.children[i].count
		}
		n = n.children[i]
	}
}
