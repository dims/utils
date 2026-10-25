package bbench

import (
	"bytes"
	"testing"

	"bbench/btreeopt"
)

// The key-beside-pointer item on top of the search fix.
func BenchmarkStack(b *testing.B) {
	ks := registryKeys()
	t := btreeopt.New(16, func(a, c valItem) bool { return a.key < c.key })
	for _, k := range ks {
		t.ReplaceOrInsert(valItem{key: k, e: &Element{Key: k}})
	}
	b.Run("get-100k/opt+val-item", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if e, ok := t.Get(valItem{key: ks[i%n]}); ok {
				sink += len(e.key)
			}
		}
	})
	bk := bigKeys()
	t2 := btreeopt.New(32, func(a, c etcdVal) bool { return bytes.Compare(a.key, c.key) < 0 })
	for _, k := range bk {
		t2.ReplaceOrInsert(etcdVal{key: k, ki: &keyIndex{key: k}})
	}
	b.Run("get-1M/opt+val-item", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if e, ok := t2.Get(etcdVal{key: bk[i%big]}); ok {
				sink += len(e.key)
			}
		}
	})
}
