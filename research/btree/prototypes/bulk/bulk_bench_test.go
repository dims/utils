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

package btree_test

import (
	"fmt"
	"testing"

	btree "btreeproto/bulk"
	"btreeproto/internal/bench"
)

func insertTree(degree int, elems []*bench.Element) *btree.BTree[*bench.Element] {
	tr := btree.New(degree, bench.ElementLess)
	for _, e := range elems {
		tr.ReplaceOrInsert(e)
	}
	return tr
}

// BenchmarkBuild builds a tree from a sorted list, as etcd's client cache
// Restore and the watch cache's initial list do: by inserting one by one,
// or with NewFromSorted. "ordercheck" is the strictly-ascending scan alone.
func BenchmarkBuild(b *testing.B) {
	for _, n := range bench.Sizes {
		sorted := bench.Elements(bench.SortedKeys(n))
		b.Run(fmt.Sprintf("ordercheck/%s", bench.SizeName(n)), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ok := true
				for j := 1; j < len(sorted); j++ {
					if !bench.ElementLess(sorted[j-1], sorted[j]) {
						ok = false
					}
				}
				if !ok {
					b.Fatal("not sorted")
				}
			}
		})
		for _, degree := range []int{16, 32} {
			b.Run(fmt.Sprintf("insert/%s/degree=%d", bench.SizeName(n), degree), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					bench.Sink += insertTree(degree, sorted).Len()
				}
			})
			b.Run(fmt.Sprintf("bulk/%s/degree=%d", bench.SizeName(n), degree), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					bench.Sink += btree.NewFromSorted(degree, bench.ElementLess, sorted).Len()
				}
			})
		}
	}
}

// builtTrees returns degree-16 trees with the same n items built by sorted
// inserts, random inserts, and NewFromSorted.
func builtTrees(n int) (names []string, trees []*btree.BTree[*bench.Element]) {
	sorted := bench.Elements(bench.SortedKeys(n))
	random := bench.Elements(bench.Keys(n))
	return []string{"insert-sorted", "insert-random", "bulk"}, []*btree.BTree[*bench.Element]{
		insertTree(16, sorted), insertTree(16, random), btree.NewFromSorted(16, bench.ElementLess, sorted),
	}
}

// BenchmarkBuiltGet looks up random keys in trees built the three ways.
func BenchmarkBuiltGet(b *testing.B) {
	for _, n := range bench.Sizes {
		keys := bench.Keys(n)
		names, trees := builtTrees(n)
		for i, tr := range trees {
			b.Run(fmt.Sprintf("%s/%s", names[i], bench.SizeName(n)), func(b *testing.B) {
				b.ReportAllocs()
				probe := &bench.Element{}
				for i := 0; i < b.N; i++ {
					probe.Key = keys[i%n]
					if e, ok := tr.Get(probe); ok {
						bench.Sink += len(e.Key)
					}
				}
			})
		}
	}
}

// BenchmarkBuiltScan reads 20k items upward from a namespace start in trees
// built the three ways, as a paginated list does, and walks the whole tree.
func BenchmarkBuiltScan(b *testing.B) {
	const scanLen = 20_000
	for _, n := range bench.Sizes {
		names, trees := builtTrees(n)
		starts := bench.Namespaces(n) - scanLen/bench.ItemsPerNamespace
		for i, tr := range trees {
			b.Run(fmt.Sprintf("scan20k/%s/%s", names[i], bench.SizeName(n)), func(b *testing.B) {
				b.ReportAllocs()
				probe := &bench.Element{}
				for i := 0; i < b.N; i++ {
					probe.Key = bench.NamespacePrefix(i % starts)
					count := 0
					tr.AscendGreaterOrEqual(probe, func(*bench.Element) bool {
						count++
						return count < scanLen
					})
					bench.Sink += count
				}
			})
			b.Run(fmt.Sprintf("ascendAll/%s/%s", names[i], bench.SizeName(n)), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					count := 0
					tr.Ascend(func(*bench.Element) bool {
						count++
						return true
					})
					bench.Sink += count
				}
			})
		}
	}
}
