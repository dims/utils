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

// Point lookups from an external package, as the consumers make them. The
// probe variants are what the watch cache and etcd do today; Find and
// GetFunc need no probe.
package btree_test

import (
	"bytes"
	"strings"
	"testing"

	btree "btreeproto/find"
	"btreeproto/internal/bench"
)

// BenchmarkLookupString is the watch cache's lookup: degree 16, string keys.
func BenchmarkLookupString(b *testing.B) {
	for _, n := range bench.Sizes {
		keys := bench.Keys(n)
		tr := btree.New(16, bench.ElementLess)
		for _, e := range bench.Elements(keys) {
			tr.ReplaceOrInsert(e)
		}
		size := bench.SizeName(n)
		b.Run("Get-probe-alloc/"+size, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if e, ok := tr.Get(&bench.Element{Key: keys[i%n]}); ok {
					bench.Sink += len(e.Key)
				}
			}
		})
		b.Run("Get-probe-reused/"+size, func(b *testing.B) {
			b.ReportAllocs()
			probe := &bench.Element{}
			for i := 0; i < b.N; i++ {
				probe.Key = keys[i%n]
				if e, ok := tr.Get(probe); ok {
					bench.Sink += len(e.Key)
				}
			}
		})
		b.Run("Find/"+size, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if e, ok := btree.Find(tr, keys[i%n], bench.ElementCmp); ok {
					bench.Sink += len(e.Key)
				}
			}
		})
		b.Run("FindBisect/"+size, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if e, ok := btree.FindBisect(tr, keys[i%n], bench.ElementCmp); ok {
					bench.Sink += len(e.Key)
				}
			}
		})
		b.Run("GetFunc/"+size, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				key := keys[i%n]
				if e, ok := tr.GetFunc(func(e *bench.Element) int { return strings.Compare(e.Key, key) }); ok {
					bench.Sink += len(e.Key)
				}
			}
		})
	}
}

// BenchmarkLookupBytes is etcd's lookup: degree 32, []byte keys.
func BenchmarkLookupBytes(b *testing.B) {
	for _, n := range bench.Sizes {
		keys := bench.Keys(n)
		items := bench.KeyIndexes(keys)
		probes := make([][]byte, n)
		for i, k := range keys {
			probes[i] = []byte(k)
		}
		tr := btree.New(32, bench.KeyIndexLess)
		for _, k := range items {
			tr.ReplaceOrInsert(k)
		}
		size := bench.SizeName(n)
		b.Run("Get-probe-alloc/"+size, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if k, ok := tr.Get(&bench.KeyIndex{Key: probes[i%n]}); ok {
					bench.Sink += len(k.Key)
				}
			}
		})
		b.Run("Get-probe-reused/"+size, func(b *testing.B) {
			b.ReportAllocs()
			probe := &bench.KeyIndex{}
			for i := 0; i < b.N; i++ {
				probe.Key = probes[i%n]
				if k, ok := tr.Get(probe); ok {
					bench.Sink += len(k.Key)
				}
			}
		})
		b.Run("Find/"+size, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if k, ok := btree.Find(tr, probes[i%n], bench.KeyIndexCmp); ok {
					bench.Sink += len(k.Key)
				}
			}
		})
		b.Run("FindBisect/"+size, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if k, ok := btree.FindBisect(tr, probes[i%n], bench.KeyIndexCmp); ok {
					bench.Sink += len(k.Key)
				}
			}
		})
		b.Run("GetFunc/"+size, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				key := probes[i%n]
				if k, ok := tr.GetFunc(func(k *bench.KeyIndex) int { return bytes.Compare(k.Key, key) }); ok {
					bench.Sink += len(k.Key)
				}
			}
		})
	}
}
