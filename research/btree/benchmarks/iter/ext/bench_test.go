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

// The benchmarks in this file model how the tree's consumers use it: the
// Kubernetes watch cache and etcd's key index. Both store pointers to
// records whose key is a string such as /registry/pods/<namespace>/<name>,
// so every comparison follows a pointer and compares strings that share a
// long prefix. The int benchmarks in btree_test.go measure the tree itself.
// These measure what a lookup, a list and a rebuild cost those consumers.
//
// They are in an external test package on purpose. The consumers call the
// tree from other packages, and the compiler instantiates and inlines the
// generic code differently across a package boundary: measured from inside
// the package, a lookup appears about 30% cheaper than it is for them.
package btree_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	btree "expiter"
)

// benchElement mirrors the watch cache's store element: a key, and pointers
// to the object and its index sets.
type benchElement struct {
	Key    string
	Object any
	Labels map[string]string
	Fields map[string]string
}

func benchLess(a, b *benchElement) bool { return a.Key < b.Key }

const (
	benchItems      = 100_000
	benchNamespaces = 500 // 200 items per namespace
	benchScanLen    = 20_000
)

// benchKeys returns benchItems registry-style keys in a fixed random order.
func benchKeys() []string {
	keys := make([]string, benchItems)
	for i := range keys {
		keys[i] = fmt.Sprintf("/registry/pods/namespace-%03d/deployment-name-replicaset-%06d", i%benchNamespaces, i)
	}
	rand.New(rand.NewSource(1)).Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	return keys
}

// benchNamespacePrefix is the key prefix of namespace n.
func benchNamespacePrefix(n int) string {
	return fmt.Sprintf("/registry/pods/namespace-%03d/", n)
}

func benchTree(degree int, keys []string) *btree.BTree[*benchElement] {
	tr := btree.New(degree, benchLess)
	for _, k := range keys {
		tr.ReplaceOrInsert(&benchElement{Key: k})
	}
	return tr
}

// benchSink keeps the compiler from removing the benchmarked work.
var benchSink int

// BenchmarkStringKeyGet is a point lookup with a probe element, as the watch
// cache's GetByKey and etcd's treeIndex.Get do it. Degree 16 is the watch
// cache's, 32 is etcd's.
func BenchmarkStringKeyGet(b *testing.B) {
	keys := benchKeys()
	for _, degree := range []int{16, 32} {
		tr := benchTree(degree, keys)
		b.Run(fmt.Sprintf("degree=%d", degree), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if e, ok := tr.Get(&benchElement{Key: keys[i%benchItems]}); ok {
					benchSink += len(e.Key)
				}
			}
		})
	}
}

// BenchmarkStringKeyAscendPrefix lists one namespace: it ascends from the
// prefix until a key no longer has it, as the watch cache's prefix listing
// does. About 200 items.
func BenchmarkStringKeyAscendPrefix(b *testing.B) {
	keys := benchKeys()
	tr := benchTree(16, keys)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prefix, count := benchNamespacePrefix(i%benchNamespaces), 0
		tr.AscendGreaterOrEqual(&benchElement{Key: prefix}, func(e *benchElement) bool {
			if !strings.HasPrefix(e.Key, prefix) {
				return false
			}
			count++
			return true
		})
		benchSink += count
	}
}

// BenchmarkStringKeyAscendRange lists one namespace by start and end key, as
// a range request with an end key does. About 200 items.
func BenchmarkStringKeyAscendRange(b *testing.B) {
	keys := benchKeys()
	tr := benchTree(16, keys)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prefix, count := benchNamespacePrefix(i%benchNamespaces), 0
		tr.AscendRange(&benchElement{Key: prefix}, &benchElement{Key: prefix + "\xff"}, func(*benchElement) bool {
			count++
			return true
		})
		benchSink += count
	}
}

// BenchmarkStringKeyAscendScan reads benchScanLen items upward from a start
// key and stops, as a paginated list does. The start leaves at least that
// many items above it.
func BenchmarkStringKeyAscendScan(b *testing.B) {
	keys := benchKeys()
	tr := benchTree(16, keys)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start, count := benchNamespacePrefix(i%(benchNamespaces-benchScanLen/200)), 0
		tr.AscendGreaterOrEqual(&benchElement{Key: start}, func(*benchElement) bool {
			count++
			return count < benchScanLen
		})
		benchSink += count
	}
}

// BenchmarkStringKeyDescendScan is BenchmarkStringKeyAscendScan in reverse.
func BenchmarkStringKeyDescendScan(b *testing.B) {
	keys := benchKeys()
	tr := benchTree(16, keys)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start, count := benchNamespacePrefix(benchScanLen/200+i%(benchNamespaces-benchScanLen/200))+"\xff", 0
		tr.DescendLessOrEqual(&benchElement{Key: start}, func(*benchElement) bool {
			count++
			return count < benchScanLen
		})
		benchSink += count
	}
}
