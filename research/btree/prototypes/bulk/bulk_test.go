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

import (
	"fmt"
	"math/rand"
	"runtime"
	"slices"
	"sort"
	"testing"
)

// checkInvariants verifies the B-tree shape: node sizes within limits, every
// leaf at the same depth, children count one more than items, and the item
// count equal to Len.
func checkInvariants[T any](t *testing.T, tr *BTree[T]) {
	t.Helper()
	if tr.root == nil {
		if tr.length != 0 {
			t.Fatalf("nil root with length %d", tr.length)
		}
		return
	}
	leafDepth := -1
	var walk func(n *node[T], depth int, root bool) int
	walk = func(n *node[T], depth int, root bool) int {
		if len(n.items) > tr.maxItems() {
			t.Fatalf("node with %d items, max %d", len(n.items), tr.maxItems())
		}
		if !root && len(n.items) < tr.minItems() {
			t.Fatalf("node with %d items, min %d", len(n.items), tr.minItems())
		}
		if len(n.children) == 0 {
			if leafDepth == -1 {
				leafDepth = depth
			} else if leafDepth != depth {
				t.Fatalf("leaf at depth %d, others at %d", depth, leafDepth)
			}
			return len(n.items)
		}
		if len(n.children) != len(n.items)+1 {
			t.Fatalf("node with %d items and %d children", len(n.items), len(n.children))
		}
		count := len(n.items)
		for _, c := range n.children {
			count += walk(c, depth+1, false)
		}
		return count
	}
	if got := walk(tr.root, 0, true); got != tr.length {
		t.Fatalf("tree holds %d items, Len says %d", got, tr.length)
	}
}

func TestNewFromSortedShape(t *testing.T) {
	for degree := 2; degree <= 40; degree++ {
		for _, n := range []int{0, 1, 2, 3, 5, 7, 10, 31, 32, 33, 62, 63, 64, 65, 100, 127, 128, 129, 1000, 1023, 1024, 1025, 4096, 10007} {
			sorted := intRange(n, false)
			tr := NewFromSorted(degree, Less[int](), sorted)
			checkInvariants(t, tr)
			if got := intAll(tr); !slices.Equal(got, sorted) {
				t.Fatalf("degree %d n %d: Ascend mismatch", degree, n)
			}
			if got := intAllRev(tr); !slices.Equal(got, intRange(n, true)) {
				t.Fatalf("degree %d n %d: Descend mismatch", degree, n)
			}
			for _, k := range []int{0, 1, n / 2, n - 1} {
				if k >= 0 && k < n {
					if v, ok := tr.Get(k); !ok || v != k {
						t.Fatalf("degree %d n %d: Get(%d) = %d %v", degree, n, k, v, ok)
					}
				}
			}
			if _, ok := tr.Get(n); ok {
				t.Fatalf("degree %d n %d: Get(%d) found", degree, n, n)
			}
		}
	}
}

// TestNewFromSortedFallback checks that input that is not strictly ascending
// is inserted one by one.
func TestNewFromSortedFallback(t *testing.T) {
	for _, in := range [][]int{{3, 1, 2}, {1, 1, 2}, {5, 4, 3, 2, 1}, {1, 2, 3, 3}} {
		tr := NewFromSorted(3, Less[int](), in)
		want := slices.Clone(in)
		sort.Ints(want)
		want = slices.Compact(want)
		if got := intAll(tr); !slices.Equal(got, want) {
			t.Fatalf("%v: got %v want %v", in, got, want)
		}
		checkInvariants(t, tr)
	}
}

// TestNewFromSortedThenMutate builds a large tree and then deletes and
// re-inserts half of it in random order, against a model.
func TestNewFromSortedThenMutate(t *testing.T) {
	for _, degree := range []int{2, 3, 16, 32} {
		const n = 20000
		tr := NewFromSorted(degree, Less[int](), intRange(n, false))
		checkInvariants(t, tr)
		model := make(map[int]bool, n)
		for i := range n {
			model[i] = true
		}
		r := rand.New(rand.NewSource(int64(degree)))
		for _, k := range r.Perm(n)[:n/2] {
			if _, ok := tr.Delete(k); !ok {
				t.Fatalf("Delete(%d) missed", k)
			}
			delete(model, k)
		}
		checkInvariants(t, tr)
		for _, k := range r.Perm(n)[:n/2] {
			_, had := tr.ReplaceOrInsert(k)
			if had != model[k] {
				t.Fatalf("ReplaceOrInsert(%d): had %v", k, had)
			}
			model[k] = true
		}
		checkInvariants(t, tr)
		want := make([]int, 0, len(model))
		for k := range model {
			want = append(want, k)
		}
		sort.Ints(want)
		if got := intAll(tr); !slices.Equal(got, want) {
			t.Fatalf("degree %d: contents differ", degree)
		}
	}
}

// treeStats reports the node count, depth, and average fill of a tree.
func treeStats[T any](tr *BTree[T]) (nodes, depth int, fill float64) {
	var walk func(n *node[T], d int)
	items := 0
	walk = func(n *node[T], d int) {
		nodes++
		items += len(n.items)
		if d > depth {
			depth = d
		}
		for _, c := range n.children {
			walk(c, d+1)
		}
	}
	if tr.root != nil {
		walk(tr.root, 1)
	}
	return nodes, depth, float64(items) / float64(nodes) / float64(tr.maxItems())
}

type statElement struct {
	Key    string
	Object any
	Labels map[string]string
	Fields map[string]string
}

// TestNewFromSortedStats prints the shape and the live heap bytes of trees
// built three ways from the same 100k and 1M string-keyed elements.
func TestNewFromSortedStats(t *testing.T) {
	less := func(a, b *statElement) bool { return a.Key < b.Key }
	for _, n := range []int{100_000, 1_000_000} {
		sorted := make([]*statElement, n)
		for i := range sorted {
			sorted[i] = &statElement{Key: fmt.Sprintf("/registry/pods/namespace-%05d/deployment-name-replicaset-%07d", i/200, i)}
		}
		random := slices.Clone(sorted)
		rand.New(rand.NewSource(1)).Shuffle(n, func(i, j int) { random[i], random[j] = random[j], random[i] })
		for _, degree := range []int{16, 32} {
			builders := []struct {
				name  string
				build func() *BTree[*statElement]
			}{
				{"insert-sorted", func() *BTree[*statElement] {
					tr := New(degree, less)
					for _, e := range sorted {
						tr.ReplaceOrInsert(e)
					}
					return tr
				}},
				{"insert-random", func() *BTree[*statElement] {
					tr := New(degree, less)
					for _, e := range random {
						tr.ReplaceOrInsert(e)
					}
					return tr
				}},
				{"bulk", func() *BTree[*statElement] { return NewFromSorted(degree, less, sorted) }},
			}
			for _, bld := range builders {
				var before, after runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&before)
				tr := bld.build()
				runtime.GC()
				runtime.ReadMemStats(&after)
				nodes, depth, fill := treeStats(tr)
				t.Logf("n=%d degree=%d %-14s nodes=%6d depth=%d fill=%.2f liveHeap=%.1f MB (%.1f B/item)",
					n, degree, bld.name, nodes, depth, fill,
					float64(after.HeapAlloc-before.HeapAlloc)/1e6, float64(after.HeapAlloc-before.HeapAlloc)/float64(n))
				runtime.KeepAlive(tr)
			}
		}
	}
}

// BenchmarkBuildUnchecked measures the bulk build without the order check,
// to show what the check costs.
func BenchmarkBuildUnchecked(b *testing.B) {
	less := func(a, b *statElement) bool { return a.Key < b.Key }
	for _, n := range []int{100_000, 1_000_000} {
		sorted := make([]*statElement, n)
		for i := range sorted {
			sorted[i] = &statElement{Key: fmt.Sprintf("/registry/pods/namespace-%05d/deployment-name-replicaset-%07d", i/200, i)}
		}
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				t := New(16, less)
				t.root = t.cow.buildFromSorted(sorted, nil, t.maxItems(), t.minItems())
				t.length = n
			}
		})
	}
}
