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

// NewFromSorted creates a new B-Tree with the given degree that holds the
// items of sorted, which must be in strictly ascending order under less.
//
// It checks the order with one pass over sorted and then builds the tree
// from the bottom up without comparing items again. Every node is about
// three quarters full, so later inserts rarely split a node and the tree
// uses less memory than one built by inserting the items one by one. If
// sorted is not strictly ascending, the items are inserted one by one
// instead, which yields the same tree as a loop over ReplaceOrInsert.
//
// The tree copies the items and keeps no reference to sorted.
func NewFromSorted[T any](degree int, less LessFunc[T], sorted []T) *BTree[T] {
	t := New(degree, less)
	for i := 1; i < len(sorted); i++ {
		if !less(sorted[i-1], sorted[i]) {
			for _, item := range sorted {
				t.ReplaceOrInsert(item)
			}
			return t
		}
	}
	if len(sorted) > 0 {
		t.root = t.cow.buildFromSorted(sorted, nil, t.maxItems(), t.minItems())
		t.length = len(sorted)
	}
	return t
}

// buildFromSorted builds the subtree that holds items in order and, between
// consecutive items, the subtrees in children, which is nil for leaves.
// The nodes it creates hold between minItems and maxItems items, about
// three quarters of maxItems where it has the choice, and every leaf is at
// the same depth.
func (c *copyOnWriteContext[T]) buildFromSorted(items []T, children []*node[T], maxItems, minItems int) *node[T] {
	if len(items) <= maxItems {
		n := c.newNode()
		n.items = append(n.items, items...)
		n.children = append(n.children, children...)
		return n
	}
	// Spread the items over count nodes separated by count-1 items, which
	// move up a level. The slots are len(items)+1: one per item plus one
	// closing slot, so that count = slots/(fill+1) gives nodes of fill items.
	slots := len(items) + 1
	fill := maxItems * 3 / 4
	count := (slots + fill) / (fill + 1)
	if lo := (slots + maxItems) / (maxItems + 1); count < lo {
		count = lo
	}
	if hi := slots / (minItems + 1); count > hi {
		count = hi
	}
	perNode, extra := (len(items)-count+1)/count, (len(items)-count+1)%count
	separators := make([]T, 0, count-1)
	nodes := make([]*node[T], 0, count)
	offset := 0
	for j := 0; j < count; j++ {
		k := perNode
		if j < extra {
			k++
		}
		n := c.newNode()
		n.items = append(n.items, items[offset:offset+k]...)
		if children != nil {
			n.children = append(n.children, children[offset:offset+k+1]...)
		}
		nodes = append(nodes, n)
		if j < count-1 {
			separators = append(separators, items[offset+k])
		}
		offset += k + 1
	}
	return c.buildFromSorted(separators, nodes, maxItems, minItems)
}
