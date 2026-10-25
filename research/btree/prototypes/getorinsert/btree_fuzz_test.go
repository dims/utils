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

package btree

import (
	"slices"
	"testing"
)

// fuzzModel is the reference an ordered set must agree with: a sorted slice
// of unique keys.
type fuzzModel struct{ keys []int }

func (m *fuzzModel) insert(k int) (had bool) {
	i, found := slices.BinarySearch(m.keys, k)
	if !found {
		m.keys = slices.Insert(m.keys, i, k)
	}
	return found
}

func (m *fuzzModel) delete(k int) (had bool) {
	i, found := slices.BinarySearch(m.keys, k)
	if found {
		m.keys = slices.Delete(m.keys, i, i+1)
	}
	return found
}

func (m *fuzzModel) has(k int) bool {
	_, found := slices.BinarySearch(m.keys, k)
	return found
}

// filter returns up to limit keys for which keep is true, in ascending or
// descending order.
func (m *fuzzModel) filter(keep func(int) bool, descending bool, limit int) []int {
	var out []int
	for i := range m.keys {
		if len(out) >= limit {
			break
		}
		k := m.keys[i]
		if descending {
			k = m.keys[len(m.keys)-1-i]
		}
		if keep(k) {
			out = append(out, k)
		}
	}
	return out
}

// tagged is an item whose identity is visible beside its key, so that the
// test can tell whether GetOrInsert kept the existing item or replaced it.
type tagged struct{ key, tag int }

// FuzzBTree runs random operations on a tree and on a sorted slice and stops
// at the first disagreement. Degrees 2 to 5 force frequent splits and
// merges, and keys 0 to 63 force collisions. Clones are kept and checked
// at the end, so that writes to the live tree never show in them. Items
// carry a tag, and the model tracks which tag the tree must hold, so that
// GetOrInsert is checked to keep the existing item and ReplaceOrInsert to
// replace it.
func FuzzBTree(f *testing.F) {
	f.Add([]byte{1, 0, 5, 0, 7, 3, 5, 2, 7, 8, 9, 1, 4})
	f.Add([]byte{0, 0, 1, 0, 2, 0, 3, 0, 4, 0, 5, 8, 0, 3, 1, 3, 2, 9, 2, 10, 2, 6, 0, 7, 0})
	f.Add([]byte{2, 12, 1, 12, 1, 0, 1, 12, 2, 12, 3, 3, 1, 12, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 {
			return
		}
		tr := New(2+int(data[0]%4), func(a, b tagged) bool { return a.key < b.key })
		m := &fuzzModel{}
		tags := map[int]int{}
		type snapshot struct {
			tree *BTree[tagged]
			keys []int
		}
		var snapshots []snapshot
		// keysOf returns the keys of items and checks their tags against
		// the model.
		keysOf := func(items []tagged) []int {
			out := make([]int, len(items))
			for i, it := range items {
				out[i] = it.key
				if tags[it.key] != it.tag {
					t.Fatalf("item %d has tag %d, want %d", it.key, it.tag, tags[it.key])
				}
			}
			return out
		}
		collect := func(limit int, iterate func(ItemIterator[tagged])) []tagged {
			var out []tagged
			iterate(func(k tagged) bool {
				out = append(out, k)
				return len(out) < limit
			})
			return out
		}
		check := func(what string, got, want []int) {
			t.Helper()
			if !slices.Equal(got, want) {
				t.Fatalf("%s: got %v want %v", what, got, want)
			}
		}
		for i := 1; i+1 < len(data); i += 2 {
			op, k := data[i]%13, int(data[i+1]%64)
			k2, limit := (k*7+3)%64, 1+int(data[i]%5)
			switch op {
			case 0, 1, 2:
				if _, had := tr.ReplaceOrInsert(tagged{k, i}); had != m.insert(k) {
					t.Fatalf("ReplaceOrInsert(%d): had %v", k, had)
				}
				tags[k] = i
			case 3, 4:
				if _, had := tr.Delete(tagged{key: k}); had != m.delete(k) {
					t.Fatalf("Delete(%d): had %v", k, had)
				}
			case 5:
				v, ok := tr.Get(tagged{key: k})
				if ok != m.has(k) || (ok && (v.key != k || v.tag != tags[k])) {
					t.Fatalf("Get(%d): %v %v", k, v, ok)
				}
			case 6:
				v, ok := tr.DeleteMin()
				if ok != (len(m.keys) > 0) || (ok && v.key != m.keys[0]) {
					t.Fatalf("DeleteMin: %v %v, model %v", v, ok, m.keys)
				}
				if ok {
					m.delete(v.key)
				}
			case 7:
				v, ok := tr.DeleteMax()
				if ok != (len(m.keys) > 0) || (ok && v.key != m.keys[len(m.keys)-1]) {
					t.Fatalf("DeleteMax: %v %v, model %v", v, ok, m.keys)
				}
				if ok {
					m.delete(v.key)
				}
			case 8:
				snapshots = append(snapshots, snapshot{tr.Clone(), slices.Clone(m.keys)})
				if len(snapshots) > 4 {
					snapshots = snapshots[1:]
				}
			case 9:
				check("AscendRange", keysOf(collect(limit, func(f ItemIterator[tagged]) { tr.AscendRange(tagged{key: k}, tagged{key: k2}, f) })),
					m.filter(func(x int) bool { return x >= k && x < k2 }, false, limit))
				check("AscendGreaterOrEqual", keysOf(collect(limit, func(f ItemIterator[tagged]) { tr.AscendGreaterOrEqual(tagged{key: k}, f) })),
					m.filter(func(x int) bool { return x >= k }, false, limit))
			case 10:
				check("DescendRange", keysOf(collect(limit, func(f ItemIterator[tagged]) { tr.DescendRange(tagged{key: k}, tagged{key: k2}, f) })),
					m.filter(func(x int) bool { return x <= k && x > k2 }, true, limit))
				check("DescendLessOrEqual", keysOf(collect(limit, func(f ItemIterator[tagged]) { tr.DescendLessOrEqual(tagged{key: k}, f) })),
					m.filter(func(x int) bool { return x <= k }, true, limit))
			case 11:
				lo, okLo := tr.Min()
				hi, okHi := tr.Max()
				if okLo != (len(m.keys) > 0) || okHi != okLo || (okLo && (lo.key != m.keys[0] || hi.key != m.keys[len(m.keys)-1])) {
					t.Fatalf("Min/Max: %v %v %v %v, model %v", lo, okLo, hi, okHi, m.keys)
				}
			case 12:
				had := m.insert(k)
				v, inserted := tr.GetOrInsert(tagged{k, i})
				if inserted != !had || v.key != k {
					t.Fatalf("GetOrInsert(%d): %v %v, had %v", k, v, inserted, had)
				}
				if inserted {
					tags[k] = i
				} else if v.tag != tags[k] {
					t.Fatalf("GetOrInsert(%d): returned tag %d, want existing %d", k, v.tag, tags[k])
				}
			}
			if tr.Len() != len(m.keys) {
				t.Fatalf("Len: %d want %d", tr.Len(), len(m.keys))
			}
			if i%8 == 1 {
				check("Ascend", keysOf(collect(len(m.keys)+1, tr.Ascend)), m.keys)
			}
		}
		check("Ascend", keysOf(collect(len(m.keys)+1, tr.Ascend)), m.keys)
		for _, s := range snapshots {
			got := collect(len(s.keys)+1, s.tree.Ascend)
			keys := make([]int, len(got))
			for i, it := range got {
				keys[i] = it.key
			}
			check("snapshot Ascend", keys, s.keys)
			if s.tree.Len() != len(s.keys) {
				t.Fatalf("snapshot Len: %d want %d", s.tree.Len(), len(s.keys))
			}
		}
	})
}
