package bbench

import (
	"fmt"
	"testing"

	"bbench/btree"
)

// What the key-beside-pointer item and a larger degree cost the write path
// that clones after every event, and what they buy lookups.
func BenchmarkTradeoff(b *testing.B) {
	ks := registryKeys()
	for _, d := range []int{16, 32} {
		b.Run(fmt.Sprintf("get/val-string/degree=%d", d), func(b *testing.B) {
			t := valTree(d, ks)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if e, ok := t.Get(valItem{key: ks[i%n]}); ok {
					sink += len(e.key)
				}
			}
		})
		b.Run(fmt.Sprintf("replace+clone/ptr/degree=%d", d), func(b *testing.B) {
			t := ptrTree(d, ks)
			var latest *btree.BTree[*Element]
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				t.ReplaceOrInsert(&Element{Key: ks[i%n]})
				latest = t.Clone()
			}
			sink += latest.Len()
		})
		b.Run(fmt.Sprintf("replace+clone/val/degree=%d", d), func(b *testing.B) {
			t := valTree(d, ks)
			var latest *btree.BTree[valItem]
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				k := ks[i%n]
				t.ReplaceOrInsert(valItem{key: k, e: &Element{Key: k}})
				latest = t.Clone()
			}
			sink += latest.Len()
		})
	}
}
