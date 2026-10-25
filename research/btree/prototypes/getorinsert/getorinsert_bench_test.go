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

// These benchmarks model etcd's treeIndex.Put: a Get with a probe keyIndex,
// then ReplaceOrInsert of that probe when the key is new. Degree 32,
// []byte keys compared with bytes.Compare.
package btree_test

import (
	"testing"

	btree "btreeproto/getorinsert"
	"btreeproto/internal/bench"
)

func keyIndexTree(items []*bench.KeyIndex) *btree.BTree[*bench.KeyIndex] {
	tr := btree.New(32, bench.KeyIndexLess)
	for _, k := range items {
		tr.ReplaceOrInsert(k)
	}
	return tr
}

func byteKeys(keys []string) [][]byte {
	out := make([][]byte, len(keys))
	for i, k := range keys {
		out[i] = []byte(k)
	}
	return out
}

// BenchmarkPutNew inserts keys that are not in the tree. Every b.N/n
// iterations the tree is rebuilt outside the timer, so it holds between n
// and 2n items.
func BenchmarkPutNew(b *testing.B) {
	for _, n := range bench.Sizes {
		all := bench.Keys(2 * n)
		existing := bench.KeyIndexes(all[:n])
		fresh := byteKeys(all[n:])
		size := bench.SizeName(n)

		b.Run("GetThenReplaceOrInsert/"+size, func(b *testing.B) {
			tr := keyIndexTree(existing)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i > 0 && i%n == 0 {
					b.StopTimer()
					tr = keyIndexTree(existing)
					b.StartTimer()
				}
				keyi := &bench.KeyIndex{Key: fresh[i%n]}
				if _, ok := tr.Get(keyi); !ok {
					tr.ReplaceOrInsert(keyi)
				} else {
					b.Fatal("key present")
				}
			}
		})
		b.Run("GetOrInsert/"+size, func(b *testing.B) {
			tr := keyIndexTree(existing)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i > 0 && i%n == 0 {
					b.StopTimer()
					tr = keyIndexTree(existing)
					b.StartTimer()
				}
				if _, inserted := tr.GetOrInsert(&bench.KeyIndex{Key: fresh[i%n]}); !inserted {
					b.Fatal("key present")
				}
			}
		})
	}
}

// BenchmarkPutExisting updates keys that are in the tree: a Get that finds
// the item, against a GetOrInsert that finds it.
func BenchmarkPutExisting(b *testing.B) {
	for _, n := range bench.Sizes {
		keys := bench.Keys(n)
		existing := bench.KeyIndexes(keys)
		probes := byteKeys(keys)
		size := bench.SizeName(n)

		b.Run("Get/"+size, func(b *testing.B) {
			tr := keyIndexTree(existing)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				keyi := &bench.KeyIndex{Key: probes[i%n]}
				if _, ok := tr.Get(keyi); !ok {
					b.Fatal("key missing")
				}
			}
		})
		b.Run("GetOrInsert/"+size, func(b *testing.B) {
			tr := keyIndexTree(existing)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, inserted := tr.GetOrInsert(&bench.KeyIndex{Key: probes[i%n]}); inserted {
					b.Fatal("key missing")
				}
			}
		})
		// The watch cache clones after every write, so a GetOrInsert that
		// finds its key copies the nodes on its path for nothing. This is
		// that worst case: a Clone before every GetOrInsert of an existing key.
		b.Run("GetAfterClone/"+size, func(b *testing.B) {
			tr := keyIndexTree(existing)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = tr.Clone()
				keyi := &bench.KeyIndex{Key: probes[i%n]}
				if _, ok := tr.Get(keyi); !ok {
					b.Fatal("key missing")
				}
			}
		})
		b.Run("GetOrInsertAfterClone/"+size, func(b *testing.B) {
			tr := keyIndexTree(existing)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = tr.Clone()
				if _, inserted := tr.GetOrInsert(&bench.KeyIndex{Key: probes[i%n]}); inserted {
					b.Fatal("key missing")
				}
			}
		})
	}
}
