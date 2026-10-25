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

// FuzzNewFromSorted is FuzzBTree on a tree that starts from NewFromSorted
// with up to 255 keys in 0..255, and then takes the same random operations
// over keys 0..255. The shape invariants are checked after every operation.
func FuzzNewFromSorted(f *testing.F) {
	f.Add([]byte{1, 40, 3, 0, 5, 0, 7, 3, 5, 2, 7, 8, 9, 1, 4})
	f.Add([]byte{0, 255, 17, 0, 1, 0, 2, 3, 3, 3, 4, 6, 5, 8, 0, 3, 1, 3, 2, 9, 2, 10, 2, 6, 0, 7, 0})
	f.Add([]byte{3, 200, 5, 3, 1, 3, 2, 6, 0, 7, 0, 4, 100, 4, 101, 4, 102, 4, 103, 4, 104, 9, 90})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 {
			return
		}
		m := &fuzzModel{}
		for j := 0; j < int(data[1]); j++ {
			m.insert((int(data[2])*3 + j*7) % 256)
		}
		tr := NewFromSorted(2+int(data[0]%4), func(a, b int) bool { return a < b }, slices.Clone(m.keys))
		checkInvariants(t, tr)
		type snapshot struct {
			tree *BTree[int]
			keys []int
		}
		var snapshots []snapshot
		check := func(what string, got, want []int) {
			t.Helper()
			if !slices.Equal(got, want) {
				t.Fatalf("%s: got %v want %v", what, got, want)
			}
		}
		check("Ascend", collectItems(len(m.keys)+1, tr.Ascend), m.keys)
		for i := 3; i+1 < len(data); i += 2 {
			op, k := data[i]%12, int(data[i+1])
			k2, limit := (k*7+3)%256, 1+int(data[i]%5)
			switch op {
			case 0, 1, 2:
				if _, had := tr.ReplaceOrInsert(k); had != m.insert(k) {
					t.Fatalf("ReplaceOrInsert(%d): had %v", k, had)
				}
			case 3, 4:
				if _, had := tr.Delete(k); had != m.delete(k) {
					t.Fatalf("Delete(%d): had %v", k, had)
				}
			case 5:
				v, ok := tr.Get(k)
				if ok != m.has(k) || (ok && v != k) {
					t.Fatalf("Get(%d): %d %v", k, v, ok)
				}
			case 6:
				v, ok := tr.DeleteMin()
				if ok != (len(m.keys) > 0) || (ok && v != m.keys[0]) {
					t.Fatalf("DeleteMin: %d %v, model %v", v, ok, m.keys)
				}
				if ok {
					m.delete(v)
				}
			case 7:
				v, ok := tr.DeleteMax()
				if ok != (len(m.keys) > 0) || (ok && v != m.keys[len(m.keys)-1]) {
					t.Fatalf("DeleteMax: %d %v, model %v", v, ok, m.keys)
				}
				if ok {
					m.delete(v)
				}
			case 8:
				snapshots = append(snapshots, snapshot{tr.Clone(), slices.Clone(m.keys)})
				if len(snapshots) > 4 {
					snapshots = snapshots[1:]
				}
			case 9:
				check("AscendRange", collectItems(limit, func(f ItemIterator[int]) { tr.AscendRange(k, k2, f) }),
					m.filter(func(x int) bool { return x >= k && x < k2 }, false, limit))
				check("AscendLessThan", collectItems(limit, func(f ItemIterator[int]) { tr.AscendLessThan(k, f) }),
					m.filter(func(x int) bool { return x < k }, false, limit))
				check("AscendGreaterOrEqual", collectItems(limit, func(f ItemIterator[int]) { tr.AscendGreaterOrEqual(k, f) }),
					m.filter(func(x int) bool { return x >= k }, false, limit))
			case 10:
				check("DescendRange", collectItems(limit, func(f ItemIterator[int]) { tr.DescendRange(k, k2, f) }),
					m.filter(func(x int) bool { return x <= k && x > k2 }, true, limit))
				check("DescendLessOrEqual", collectItems(limit, func(f ItemIterator[int]) { tr.DescendLessOrEqual(k, f) }),
					m.filter(func(x int) bool { return x <= k }, true, limit))
				check("DescendGreaterThan", collectItems(limit, func(f ItemIterator[int]) { tr.DescendGreaterThan(k, f) }),
					m.filter(func(x int) bool { return x > k }, true, limit))
			case 11:
				lo, okLo := tr.Min()
				hi, okHi := tr.Max()
				if okLo != (len(m.keys) > 0) || okHi != okLo || (okLo && (lo != m.keys[0] || hi != m.keys[len(m.keys)-1])) {
					t.Fatalf("Min/Max: %d %v %d %v, model %v", lo, okLo, hi, okHi, m.keys)
				}
			}
			if tr.Len() != len(m.keys) {
				t.Fatalf("Len: %d want %d", tr.Len(), len(m.keys))
			}
			checkInvariants(t, tr)
			if i%8 == 1 {
				check("Ascend", collectItems(len(m.keys)+1, tr.Ascend), m.keys)
			}
		}
		check("Ascend", collectItems(len(m.keys)+1, tr.Ascend), m.keys)
		check("Descend", collectItems(len(m.keys)+1, tr.Descend), m.filter(func(int) bool { return true }, true, len(m.keys)+1))
		for _, s := range snapshots {
			check("snapshot Ascend", collectItems(len(s.keys)+1, s.tree.Ascend), s.keys)
			if s.tree.Len() != len(s.keys) {
				t.Fatalf("snapshot Len: %d want %d", s.tree.Len(), len(s.keys))
			}
		}
	})
}
