package bbench

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"bbench/btree"
)

// Element mirrors the apiserver watch cache store element.
type Element struct {
	Key    string
	Object any
	Labels map[string]string
	Fields map[string]string
}

// keyIndex mirrors etcd's treeIndex item: a []byte key behind a pointer.
type keyIndex struct {
	key         []byte
	modified    [2]int64
	generations []int
}

// valItem is the alternative bbtree suggests for Go: the key beside the
// pointer, inside the node, so a probe reads one less dependent pointer.
type valItem struct {
	key string
	e   *Element
}

const n = 100_000

func registryKeys() []string {
	ks := make([]string, n)
	for i := range ks {
		ks[i] = fmt.Sprintf("/registry/pods/namespace-%03d/deployment-name-replicaset-%06d", i%500, i)
	}
	rand.New(rand.NewSource(1)).Shuffle(len(ks), func(i, j int) { ks[i], ks[j] = ks[j], ks[i] })
	return ks
}

func ptrTree(degree int, ks []string) *btree.BTree[*Element] {
	t := btree.New(degree, func(a, b *Element) bool { return a.Key < b.Key })
	for _, k := range ks {
		t.ReplaceOrInsert(&Element{Key: k, Labels: map[string]string{"app": "x"}})
	}
	return t
}

func valTree(degree int, ks []string) *btree.BTree[valItem] {
	t := btree.New(degree, func(a, b valItem) bool { return a.key < b.key })
	for _, k := range ks {
		t.ReplaceOrInsert(valItem{key: k, e: &Element{Key: k}})
	}
	return t
}

func etcdTree(degree int, ks []string) *btree.BTree[*keyIndex] {
	t := btree.New(degree, func(a, b *keyIndex) bool { return bytes.Compare(a.key, b.key) < 0 })
	for _, k := range ks {
		t.ReplaceOrInsert(&keyIndex{key: []byte(k), generations: []int{1}})
	}
	return t
}

var sink int

// Point lookups, the shape of store.GetByKey and treeIndex.Get.
func BenchmarkGet(b *testing.B) {
	ks := registryKeys()
	for _, d := range []int{16, 32, 64} {
		b.Run(fmt.Sprintf("ptr-string/degree=%d", d), func(b *testing.B) {
			t := ptrTree(d, ks)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if e, ok := t.Get(&Element{Key: ks[i%n]}); ok {
					sink += len(e.Key)
				}
			}
		})
	}
	b.Run("ptr-bytes-etcd/degree=32", func(b *testing.B) {
		t := etcdTree(32, ks)
		probes := make([][]byte, n)
		for i, k := range ks {
			probes[i] = []byte(k)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if e, ok := t.Get(&keyIndex{key: probes[i%n]}); ok {
				sink += len(e.key)
			}
		}
	})
	b.Run("val-string/degree=16", func(b *testing.B) {
		t := valTree(16, ks)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if e, ok := t.Get(valItem{key: ks[i%n]}); ok {
				sink += len(e.key)
			}
		}
	})
}

// The watch cache write path: ReplaceOrInsert of an existing key, then
// Clone, every event (UpdateStore). Each write after a Clone copies the
// root-to-leaf path.
func BenchmarkUpdate(b *testing.B) {
	ks := registryKeys()
	b.Run("replace-only/degree=16", func(b *testing.B) {
		t := ptrTree(16, ks)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			t.ReplaceOrInsert(&Element{Key: ks[i%n]})
		}
	})
	b.Run("replace+clone/degree=16", func(b *testing.B) {
		t := ptrTree(16, ks)
		var latest *btree.BTree[*Element]
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			t.ReplaceOrInsert(&Element{Key: ks[i%n]})
			latest = t.Clone()
		}
		sink += latest.Len()
	})
	b.Run("replace+clone-retain1000/degree=16", func(b *testing.B) {
		t := ptrTree(16, ks)
		ring := make([]*btree.BTree[*Element], 1000)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			t.ReplaceOrInsert(&Element{Key: ks[i%n]})
			ring[i%len(ring)] = t.Clone()
		}
		sink += ring[0].Len()
	})
	b.Run("add+delete/degree=16", func(b *testing.B) {
		t := ptrTree(16, ks)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			k := fmt.Sprintf("/registry/pods/namespace-%03d/extra-%09d", i%500, i)
			t.ReplaceOrInsert(&Element{Key: k})
			t.Delete(&Element{Key: k})
		}
	})
}

// Namespace list: AscendGreaterOrEqual from the namespace prefix until the
// prefix stops matching, about 200 items (store.OrderedListPrefix).
func BenchmarkRangePrefix(b *testing.B) {
	ks := registryKeys()
	t := ptrTree(16, ks)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		prefix := fmt.Sprintf("/registry/pods/namespace-%03d/", i%500)
		count := 0
		t.AscendGreaterOrEqual(&Element{Key: prefix}, func(e *Element) bool {
			if !strings.HasPrefix(e.Key, prefix) {
				return false
			}
			count++
			return true
		})
		sink += count
	}
}

// Full ascend, the shape of treeIndex.Keep/Compact and etcd cache Restore.
func BenchmarkAscendAll(b *testing.B) {
	ks := registryKeys()
	t := etcdTree(32, ks)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		count := 0
		t.Ascend(func(e *keyIndex) bool { count++; return true })
		sink += count
	}
}
