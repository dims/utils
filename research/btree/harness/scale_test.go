package bbench

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	"bbench/btree"
)

// etcd's scale: 1M keys, degree 32, []byte keys. Pointer item as etcd has
// it, against the key beside the pointer inside the node.
const big = 1_000_000

type etcdVal struct {
	key []byte
	ki  *keyIndex
}

func bigKeys() [][]byte {
	ks := make([][]byte, big)
	for i := range ks {
		ks[i] = []byte(fmt.Sprintf("/registry/pods/namespace-%04d/deployment-name-replicaset-%07d", i%2000, i))
	}
	rand.New(rand.NewSource(2)).Shuffle(len(ks), func(i, j int) { ks[i], ks[j] = ks[j], ks[i] })
	return ks
}

func BenchmarkScale1M(b *testing.B) {
	ks := bigKeys()
	b.Run("get/ptr-bytes/degree=32", func(b *testing.B) {
		t := btree.New(32, func(a, c *keyIndex) bool { return bytes.Compare(a.key, c.key) < 0 })
		for _, k := range ks {
			t.ReplaceOrInsert(&keyIndex{key: k, generations: []int{1}})
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if e, ok := t.Get(&keyIndex{key: ks[i%big]}); ok {
				sink += len(e.key)
			}
		}
	})
	b.Run("get/val-bytes/degree=32", func(b *testing.B) {
		t := btree.New(32, func(a, c etcdVal) bool { return bytes.Compare(a.key, c.key) < 0 })
		for _, k := range ks {
			t.ReplaceOrInsert(etcdVal{key: k, ki: &keyIndex{key: k, generations: []int{1}}})
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if e, ok := t.Get(etcdVal{key: ks[i%big]}); ok {
				sink += len(e.key)
			}
		}
	})
}
