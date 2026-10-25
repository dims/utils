package bbench

import (
	"bytes"
	"testing"

	"bbench/btree"
	"bbench/btreefind"
	"bbench/btreeopt"
)

func BenchmarkAttribGet(b *testing.B) {
	ks := registryKeys()
	less := func(a, c *Element) bool { return a.Key < c.Key }
	base, find, opt := btree.New(16, less), btreefind.New(16, less), btreeopt.New(16, less)
	for _, k := range ks {
		e := &Element{Key: k}
		base.ReplaceOrInsert(e)
		find.ReplaceOrInsert(e)
		opt.ReplaceOrInsert(e)
	}
	b.Run("base", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if e, ok := base.Get(&Element{Key: ks[i%n]}); ok {
				sink += len(e.Key)
			}
		}
	})
	b.Run("find-only", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if e, ok := find.Get(&Element{Key: ks[i%n]}); ok {
				sink += len(e.Key)
			}
		}
	})
	b.Run("find+iterative-get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if e, ok := opt.Get(&Element{Key: ks[i%n]}); ok {
				sink += len(e.Key)
			}
		}
	})
}

// etcd's shape and scale: 1M byte-slice keys behind pointers, degree 32.
func BenchmarkAttribGet1M(b *testing.B) {
	ks := bigKeys()
	less := func(a, c *keyIndex) bool { return bytes.Compare(a.key, c.key) < 0 }
	base, opt := btree.New(32, less), btreeopt.New(32, less)
	for _, k := range ks {
		ki := &keyIndex{key: k, generations: []int{1}}
		base.ReplaceOrInsert(ki)
		opt.ReplaceOrInsert(ki)
	}
	b.Run("base", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if e, ok := base.Get(&keyIndex{key: ks[i%big]}); ok {
				sink += len(e.key)
			}
		}
	})
	b.Run("opt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if e, ok := opt.Get(&keyIndex{key: ks[i%big]}); ok {
				sink += len(e.key)
			}
		}
	})
}
