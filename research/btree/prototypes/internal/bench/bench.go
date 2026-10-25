// Package bench holds the consumer-shaped item types and key generators the
// prototype benchmarks share. The types mirror the watch cache's storeElement
// (string key) and etcd's keyIndex ([]byte key).
package bench

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"strings"
)

// Element mirrors the watch cache's store element.
type Element struct {
	Key    string
	Object any
	Labels map[string]string
	Fields map[string]string
}

func ElementLess(a, b *Element) bool { return a.Key < b.Key }

// ElementCmp compares a bare string key with an element (for Find).
func ElementCmp(key string, e *Element) int { return strings.Compare(key, e.Key) }

// KeyIndex mirrors etcd's keyIndex.
type KeyIndex struct {
	Key         []byte
	Modified    [2]int64
	Generations []int
}

func KeyIndexLess(a, b *KeyIndex) bool { return bytes.Compare(a.Key, b.Key) == -1 }

// KeyIndexCmp compares a bare []byte key with a keyIndex (for Find).
func KeyIndexCmp(key []byte, k *KeyIndex) int { return bytes.Compare(key, k.Key) }

// ItemsPerNamespace is how many keys share a namespace prefix.
const ItemsPerNamespace = 200

// Namespaces returns the number of namespaces for n keys.
func Namespaces(n int) int { return n / ItemsPerNamespace }

// Keys returns n registry-style keys in a fixed random order.
func Keys(n int) []string {
	keys := make([]string, n)
	ns := Namespaces(n)
	for i := range keys {
		keys[i] = fmt.Sprintf("/registry/pods/namespace-%05d/deployment-name-replicaset-%07d", i%ns, i)
	}
	rand.New(rand.NewSource(1)).Shuffle(len(keys), func(i, j int) { keys[i], keys[j] = keys[j], keys[i] })
	return keys
}

// SortedKeys returns the same keys as Keys(n), sorted.
func SortedKeys(n int) []string {
	keys := Keys(n)
	sort.Strings(keys)
	return keys
}

// NamespacePrefix is the key prefix of namespace ns.
func NamespacePrefix(ns int) string {
	return fmt.Sprintf("/registry/pods/namespace-%05d/", ns)
}

// Elements wraps keys in elements.
func Elements(keys []string) []*Element {
	out := make([]*Element, len(keys))
	for i, k := range keys {
		out[i] = &Element{Key: k}
	}
	return out
}

// KeyIndexes wraps keys in key indexes.
func KeyIndexes(keys []string) []*KeyIndex {
	out := make([]*KeyIndex, len(keys))
	for i, k := range keys {
		out[i] = &KeyIndex{Key: []byte(k)}
	}
	return out
}

// Sizes are the tree sizes every benchmark measures.
var Sizes = []int{100_000, 1_000_000}

// SizeName names a size for sub-benchmarks.
func SizeName(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%dM", n/1_000_000)
	}
	return fmt.Sprintf("%dk", n/1000)
}

// Sink keeps the compiler from removing benchmarked work.
var Sink int
