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

// These benchmarks compare the baseline Clone (three allocations) with the
// Clone that allocates both contexts in one array (two allocations), alone
// and in the watch cache's pattern of a Clone around every write.
package btree_test

import (
	"runtime"
	"testing"

	baseline "btreeproto/baseline"
	clone "btreeproto/clone"
	"btreeproto/internal/bench"
)

func baselineTree(elems []*bench.Element) *baseline.BTree[*bench.Element] {
	tr := baseline.New(16, bench.ElementLess)
	for _, e := range elems {
		tr.ReplaceOrInsert(e)
	}
	return tr
}

func cloneTree(elems []*bench.Element) *clone.BTree[*bench.Element] {
	tr := clone.New(16, bench.ElementLess)
	for _, e := range elems {
		tr.ReplaceOrInsert(e)
	}
	return tr
}

func BenchmarkClone(b *testing.B) {
	for _, n := range bench.Sizes {
		elems := bench.Elements(bench.Keys(n))
		size := bench.SizeName(n)

		// The clone is kept, so that escape analysis cannot put it on the
		// stack as it does when the result is dropped.
		b.Run("cloneOnly/baseline/"+size, func(b *testing.B) {
			tr := baselineTree(elems)
			snapshots := make([]*baseline.BTree[*bench.Element], 1024)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				snapshots[i%len(snapshots)] = tr.Clone()
			}
		})
		b.Run("cloneOnly/cowsArray/"+size, func(b *testing.B) {
			tr := cloneTree(elems)
			snapshots := make([]*clone.BTree[*bench.Element], 1024)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				snapshots[i%len(snapshots)] = tr.Clone()
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
		b.Run("cloneDeleteInsert/cowsArray/"+size, func(b *testing.B) {
			tr := cloneTree(elems)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = tr.Clone()
				tr.Delete(elems[i%n])
				tr.ReplaceOrInsert(elems[i%n])
			}
		})

		// A write and a Clone per write, with the Clone kept as the watch
		// cache keeps snapshots: the single ReplaceOrInsert the watch cache
		// does per event, then Clone.
		b.Run("insertThenClone/baseline/"+size, func(b *testing.B) {
			tr := baselineTree(elems)
			snapshots := make([]*baseline.BTree[*bench.Element], 1024)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tr.ReplaceOrInsert(elems[i%n])
				snapshots[i%len(snapshots)] = tr.Clone()
			}
		})
		b.Run("insertThenClone/cowsArray/"+size, func(b *testing.B) {
			tr := cloneTree(elems)
			snapshots := make([]*clone.BTree[*bench.Element], 1024)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tr.ReplaceOrInsert(elems[i%n])
				snapshots[i%len(snapshots)] = tr.Clone()
			}
		})
	}
}

// TestCloneRetention shows what each Clone layout keeps alive after 200k
// writes with a dropped Clone before each: the live heap should hold one
// tree, not its history.
func TestCloneRetention(t *testing.T) {
	const n, writes = 100_000, 200_000
	elems := bench.Elements(bench.Keys(n))
	measure := func(name string, run func() any) {
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		kept := run()
		runtime.GC()
		runtime.ReadMemStats(&after)
		t.Logf("%-10s live heap with the tree kept, after %d write+Clone: %6.1f MB", name, writes,
			float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/1e6)
		runtime.KeepAlive(kept)
	}
	measure("baseline", func() any {
		tr := baselineTree(elems)
		for i := 0; i < writes; i++ {
			_ = tr.Clone()
			tr.ReplaceOrInsert(elems[i%n])
		}
		return tr
	})
	measure("cowsArray", func() any {
		tr := cloneTree(elems)
		for i := 0; i < writes; i++ {
			_ = tr.Clone()
			tr.ReplaceOrInsert(elems[i%n])
		}
		return tr
	})
	measure("oneBlock", func() any {
		tr := cloneTree(elems)
		for i := 0; i < writes; i++ {
			_ = tr.CloneOneBlock()
			tr.ReplaceOrInsert(elems[i%n])
		}
		return tr
	})
}
