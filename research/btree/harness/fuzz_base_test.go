package bbench

import (
	"slices"
	"testing"

	"bbench/btree"
)

func collectBase(limit int, run func(func(int) bool)) []int {
	var out []int
	run(func(k int) bool {
		out = append(out, k)
		return len(out) < limit
	})
	return out
}

// FuzzBase runs random operations on the tree and on a sorted slice and
// stops at the first disagreement. Degrees 2 to 5 force frequent splits and
// merges; keys 0 to 63 force collisions. Clones are kept and checked later
// so that writes to the live tree never leak into them.
func FuzzBase(f *testing.F) {
	f.Add([]byte{1, 0, 5, 0, 7, 3, 5, 2, 7, 8, 9, 1, 4})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 {
			return
		}
		less := func(a, b int) bool { return a < b }
		tr := btree.New[int](2+int(data[0]%4), less)
		m := &model{}
		type snap struct {
			tree *btree.BTree[int]
			keys []int
		}
		var snaps []snap
		check := func(what string, got, want []int) {
			if !slices.Equal(got, want) {
				t.Fatalf("%s: got %v want %v", what, got, want)
			}
		}
		for i := 1; i+1 < len(data); i += 2 {
			op, k := data[i]%12, int(data[i+1]%64)
			k2, limit := (k*7+3)%64, 1+int(data[i]%5)
			switch op {
			case 0, 1, 2:
				_, had := tr.ReplaceOrInsert(k)
				if had != m.insert(k) {
					t.Fatalf("insert %d had %v", k, had)
				}
			case 3, 4:
				_, had := tr.Delete(k)
				if had != m.delete(k) {
					t.Fatalf("delete %d had %v", k, had)
				}
			case 5:
				v, ok := tr.Get(k)
				if ok != m.has(k) || (ok && v != k) {
					t.Fatalf("get %d ok %v v %d", k, ok, v)
				}
			case 6:
				v, ok := tr.DeleteMin()
				if ok != (len(m.keys) > 0) || (ok && v != m.keys[0]) {
					t.Fatalf("deletemin %v %d", ok, v)
				}
				if ok {
					m.delete(v)
				}
			case 7:
				v, ok := tr.DeleteMax()
				if ok != (len(m.keys) > 0) || (ok && v != m.keys[len(m.keys)-1]) {
					t.Fatalf("deletemax %v %d", ok, v)
				}
				if ok {
					m.delete(v)
				}
			case 8:
				snaps = append(snaps, snap{tr.Clone(), slices.Clone(m.keys)})
				if len(snaps) > 4 {
					snaps = snaps[1:]
				}
			case 9:
				check("AscendRange", collectBase(limit, func(f func(int) bool) { tr.AscendRange(k, k2, f) }),
					m.filter(func(x int) bool { return x >= k && x < k2 }, false, limit))
				check("AscendLessThan", collectBase(limit, func(f func(int) bool) { tr.AscendLessThan(k, f) }),
					m.filter(func(x int) bool { return x < k }, false, limit))
				check("AscendGreaterOrEqual", collectBase(limit, func(f func(int) bool) { tr.AscendGreaterOrEqual(k, f) }),
					m.filter(func(x int) bool { return x >= k }, false, limit))
			case 10:
				check("DescendRange", collectBase(limit, func(f func(int) bool) { tr.DescendRange(k, k2, f) }),
					m.filter(func(x int) bool { return x <= k && x > k2 }, true, limit))
				check("DescendLessOrEqual", collectBase(limit, func(f func(int) bool) { tr.DescendLessOrEqual(k, f) }),
					m.filter(func(x int) bool { return x <= k }, true, limit))
				check("DescendGreaterThan", collectBase(limit, func(f func(int) bool) { tr.DescendGreaterThan(k, f) }),
					m.filter(func(x int) bool { return x > k }, true, limit))
			case 11:
				mn, ok1 := tr.Min()
				mx, ok2 := tr.Max()
				if ok1 != (len(m.keys) > 0) || ok2 != ok1 || (ok1 && (mn != m.keys[0] || mx != m.keys[len(m.keys)-1])) {
					t.Fatalf("min/max %v %d %v %d", ok1, mn, ok2, mx)
				}
			}
			if tr.Len() != len(m.keys) {
				t.Fatalf("len %d want %d", tr.Len(), len(m.keys))
			}
			if i%8 == 1 {
				check("Ascend", collectBase(1<<30, func(f func(int) bool) { tr.Ascend(f) }), m.keys)
			}
		}
		check("Ascend", collectBase(1<<30, func(f func(int) bool) { tr.Ascend(f) }), m.keys)
		check("Descend", collectBase(1<<30, func(f func(int) bool) { tr.Descend(f) }), m.filter(func(int) bool { return true }, true, 1<<30))
		for _, s := range snaps {
			check("snapshot Ascend", collectBase(1<<30, func(f func(int) bool) { s.tree.Ascend(f) }), s.keys)
			if s.tree.Len() != len(s.keys) {
				t.Fatalf("snapshot len %d want %d", s.tree.Len(), len(s.keys))
			}
		}
	})
}
