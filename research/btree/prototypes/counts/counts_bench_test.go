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

// These benchmarks run the same write workloads on the baseline tree and
// on the tree with subtree counts, so that the overhead of maintaining the
// counts shows directly. The workloads are written twice on purpose: a
// shared interface or type parameter would add an indirect call to both.
package btree_test

import (
	"fmt"
	"testing"

	baseline "btreeproto/baseline"
	counts "btreeproto/counts"
	"btreeproto/internal/bench"
)

func baselineTree(elems []*bench.Element) *baseline.BTree[*bench.Element] {
	tr := baseline.New(16, bench.ElementLess)
	for _, e := range elems {
		tr.ReplaceOrInsert(e)
	}
	return tr
}

func countsTree(elems []*bench.Element) *counts.BTree[*bench.Element] {
	tr := counts.New(16, bench.ElementLess)
	for _, e := range elems {
		tr.ReplaceOrInsert(e)
	}
	return tr
}

// BenchmarkWrite runs the write workloads: replace an existing item, delete
// and re-insert an item in an owned tree, and the watch cache's pattern of a
// Clone before every delete and re-insert.
func BenchmarkWrite(b *testing.B) {
	for _, n := range bench.Sizes {
		elems := bench.Elements(bench.Keys(n))
		size := bench.SizeName(n)

		b.Run("replace/baseline/"+size, func(b *testing.B) {
			tr := baselineTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tr.ReplaceOrInsert(elems[i%n])
			}
		})
		b.Run("replace/counts/"+size, func(b *testing.B) {
			tr := countsTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tr.ReplaceOrInsert(elems[i%n])
			}
		})

		b.Run("deleteInsert/baseline/"+size, func(b *testing.B) {
			tr := baselineTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tr.Delete(elems[i%n])
				tr.ReplaceOrInsert(elems[i%n])
			}
		})
		b.Run("deleteInsert/counts/"+size, func(b *testing.B) {
			tr := countsTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tr.Delete(elems[i%n])
				tr.ReplaceOrInsert(elems[i%n])
			}
		})

		b.Run("cloneDeleteInsert/baseline/"+size, func(b *testing.B) {
			tr := baselineTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = tr.Clone()
				tr.Delete(elems[i%n])
				tr.ReplaceOrInsert(elems[i%n])
			}
		})
		b.Run("cloneDeleteInsert/counts/"+size, func(b *testing.B) {
			tr := countsTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = tr.Clone()
				tr.Delete(elems[i%n])
				tr.ReplaceOrInsert(elems[i%n])
			}
		})
	}
}

// BenchmarkCount compares CountRange with the walk the consumers do today:
// one namespace of 200 items (a LIST with a limit needs the total), and the
// whole tree (etcd's CountRevisions).
func BenchmarkCount(b *testing.B) {
	for _, n := range bench.Sizes {
		elems := bench.Elements(bench.Keys(n))
		size := bench.SizeName(n)
		ns := bench.Namespaces(n)
		lo, hi := &bench.Element{}, &bench.Element{}
		first, last := &bench.Element{Key: ""}, &bench.Element{Key: "\xff"}

		b.Run("namespace/walk/"+size, func(b *testing.B) {
			tr := baselineTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				prefix := bench.NamespacePrefix(i % ns)
				lo.Key, hi.Key = prefix, prefix+"\xff"
				count := 0
				tr.AscendRange(lo, hi, func(*bench.Element) bool {
					count++
					return true
				})
				bench.Sink += count
			}
		})
		b.Run("namespace/CountRange/"+size, func(b *testing.B) {
			tr := countsTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				prefix := bench.NamespacePrefix(i % ns)
				lo.Key, hi.Key = prefix, prefix+"\xff"
				bench.Sink += tr.CountRange(lo, hi)
			}
		})

		b.Run("all/walk/"+size, func(b *testing.B) {
			tr := baselineTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				count := 0
				tr.Ascend(func(*bench.Element) bool {
					count++
					return true
				})
				bench.Sink += count
			}
		})
		b.Run("all/CountRange/"+size, func(b *testing.B) {
			tr := countsTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				bench.Sink += tr.CountRange(first, last)
			}
		})
	}
}

// TestCountRangeMatchesWalk checks CountRange against a walk on a 100k
// string-keyed tree for every namespace and for the whole tree.
func TestCountRangeMatchesWalk(t *testing.T) {
	n := 100_000
	tr := countsTree(bench.Elements(bench.Keys(n)))
	for ns := 0; ns < bench.Namespaces(n); ns++ {
		prefix := bench.NamespacePrefix(ns)
		lo, hi := &bench.Element{Key: prefix}, &bench.Element{Key: prefix + "\xff"}
		want := 0
		tr.AscendRange(lo, hi, func(*bench.Element) bool {
			want++
			return true
		})
		if got := tr.CountRange(lo, hi); got != want || want != bench.ItemsPerNamespace {
			t.Fatalf("namespace %d: CountRange %d, walk %d", ns, got, want)
		}
	}
	if got := tr.CountRange(&bench.Element{Key: ""}, &bench.Element{Key: "\xff"}); got != n {
		t.Fatalf("all: CountRange %d, want %d", got, n)
	}
	fmt.Println("CountRange agrees with the walk for", bench.Namespaces(n), "namespaces")
}
