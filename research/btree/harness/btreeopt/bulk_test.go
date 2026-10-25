package btreeopt

import (
	"math/rand"
	"slices"
	"testing"
)

// checkInvariants walks the tree and checks node fill, shape, order and depth.
func checkInvariants(t *testing.T, tr *BTree[int]) []int {
	t.Helper()
	if tr.root == nil {
		if tr.length != 0 {
			t.Fatalf("nil root with length %d", tr.length)
		}
		return nil
	}
	var out []int
	leafDepth := -1
	var walk func(n *node[T0], depth int, isRoot bool)
	walk = func(n *node[int], depth int, isRoot bool) {
		if !isRoot && (len(n.items) < tr.minItems() || len(n.items) > tr.maxItems()) {
			t.Fatalf("node fill %d outside [%d,%d]", len(n.items), tr.minItems(), tr.maxItems())
		}
		if len(n.children) != 0 && len(n.children) != len(n.items)+1 {
			t.Fatalf("children %d items %d", len(n.children), len(n.items))
		}
		if len(n.children) == 0 {
			if leafDepth == -1 {
				leafDepth = depth
			} else if leafDepth != depth {
				t.Fatalf("leaf depth %d vs %d", depth, leafDepth)
			}
		}
		for i, it := range n.items {
			if len(n.children) > 0 {
				walk(n.children[i], depth+1, false)
			}
			out = append(out, it)
		}
		if len(n.children) > 0 {
			walk(n.children[len(n.children)-1], depth+1, false)
		}
	}
	walk(tr.root, 0, true)
	if !slices.IsSorted(out) || len(out) != tr.length {
		t.Fatalf("order/len broken: sorted=%v len=%d want %d", slices.IsSorted(out), len(out), tr.length)
	}
	return out
}

type T0 = int

func TestNewFromSortedThenMutate(t *testing.T) {
	less := func(a, b int) bool { return a < b }
	rng := rand.New(rand.NewSource(7))
	for _, degree := range []int{2, 3, 4, 8, 16, 32} {
		for _, n := range []int{0, 1, 2, 3, 30, 31, 32, 63, 64, 100, 999, 5000} {
			in := make([]int, n)
			for i := range in {
				in[i] = i * 2
			}
			tr := NewFromSorted(degree, less, in)
			got := checkInvariants(t, tr)
			if !slices.Equal(got, in) {
				t.Fatalf("degree %d n %d: content mismatch", degree, n)
			}
			model := slices.Clone(in)
			for op := 0; op < 300; op++ {
				k := rng.Intn(2*n + 3)
				if rng.Intn(2) == 0 {
					_, had := tr.ReplaceOrInsert(k)
					i, found := slices.BinarySearch(model, k)
					if had != found {
						t.Fatalf("insert %d: had %v want %v", k, had, found)
					}
					if !found {
						model = slices.Insert(model, i, k)
					}
				} else {
					_, had := tr.Delete(k)
					i, found := slices.BinarySearch(model, k)
					if had != found {
						t.Fatalf("delete %d: had %v want %v", k, had, found)
					}
					if found {
						model = slices.Delete(model, i, i+1)
					}
				}
				if n <= 1000 || op%50 == 0 {
					if got := checkInvariants(t, tr); !slices.Equal(got, model) {
						t.Fatalf("degree %d n %d op %d: content mismatch", degree, n, op)
					}
				}
			}
		}
	}
}
