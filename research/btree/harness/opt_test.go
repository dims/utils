package bbench

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"bbench/btree"
	"bbench/btreeopt"
)

func ptrTreeOpt(degree int, ks []string) *btreeopt.BTree[*Element] {
	t := btreeopt.New(degree, func(a, b *Element) bool { return a.Key < b.Key })
	for _, k := range ks {
		t.ReplaceOrInsert(&Element{Key: k, Labels: map[string]string{"app": "x"}})
	}
	return t
}

func BenchmarkOptGet(b *testing.B) {
	ks := registryKeys()
	base, opt := ptrTree(16, ks), ptrTreeOpt(16, ks)
	b.Run("base", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if e, ok := base.Get(&Element{Key: ks[i%n]}); ok {
				sink += len(e.Key)
			}
		}
	})
	b.Run("opt", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if e, ok := opt.Get(&Element{Key: ks[i%n]}); ok {
				sink += len(e.Key)
			}
		}
	})
}

func BenchmarkOptRange(b *testing.B) {
	ks := registryKeys()
	base, opt := ptrTree(16, ks), ptrTreeOpt(16, ks)
	nsPrefix := func(i int) string { return fmt.Sprintf("/registry/pods/namespace-%03d/", i%500) }
	// Namespace list by prefix check in the callback, as the watch cache does.
	b.Run("prefix200/base", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			p, c := nsPrefix(i), 0
			base.AscendGreaterOrEqual(&Element{Key: p}, func(e *Element) bool {
				if !strings.HasPrefix(e.Key, p) {
					return false
				}
				c++
				return true
			})
			sink += c
		}
	})
	b.Run("prefix200/opt", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			p, c := nsPrefix(i), 0
			opt.AscendGreaterOrEqual(&Element{Key: p}, func(e *Element) bool {
				if !strings.HasPrefix(e.Key, p) {
					return false
				}
				c++
				return true
			})
			sink += c
		}
	})
	// Bounded range with an end key, as the etcd cache does: 200 items.
	b.Run("range200/base", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			p, c := nsPrefix(i), 0
			base.AscendRange(&Element{Key: p}, &Element{Key: p + "\xff"}, func(e *Element) bool { c++; return true })
			sink += c
		}
	})
	b.Run("range200/opt", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			p, c := nsPrefix(i), 0
			opt.AscendRange(&Element{Key: p}, &Element{Key: p + "\xff"}, func(e *Element) bool { c++; return true })
			sink += c
		}
	})
	// Long scan: 20k items from a start key, stopped by count, as a paginated list is.
	b.Run("scan20k/base", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			c := 0
			base.AscendGreaterOrEqual(&Element{Key: nsPrefix(i)}, func(e *Element) bool { c++; return c < 20000 })
			sink += c
		}
	})
	b.Run("scan20k/opt", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			c := 0
			opt.AscendGreaterOrEqual(&Element{Key: nsPrefix(i)}, func(e *Element) bool { c++; return c < 20000 })
			sink += c
		}
	})
}

// Building a 100k tree: the watch cache Replace and the etcd cache Restore
// insert one by one from a list that is already in key order.
func BenchmarkOptBuild(b *testing.B) {
	ks := registryKeys()
	sorted := slices.Clone(ks)
	slices.Sort(sorted)
	elems := make([]*Element, len(sorted))
	for i, k := range sorted {
		elems[i] = &Element{Key: k}
	}
	less := func(a, b *Element) bool { return a.Key < b.Key }
	b.Run("insert-sorted/base", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			t := btree.New(16, less)
			for _, e := range elems {
				t.ReplaceOrInsert(e)
			}
			sink += t.Len()
		}
	})
	b.Run("insert-random/base", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sink += ptrTree(16, ks).Len()
		}
	})
	b.Run("NewFromSorted/opt", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			sink += btreeopt.NewFromSorted(16, less, elems).Len()
		}
	})
}
