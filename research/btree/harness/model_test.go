package bbench

import "slices"

// model is the reference an ordered set must agree with: a sorted slice.
type model struct{ keys []int }

func (m *model) insert(k int) (had bool) {
	i, found := slices.BinarySearch(m.keys, k)
	if !found {
		m.keys = slices.Insert(m.keys, i, k)
	}
	return found
}

func (m *model) delete(k int) (had bool) {
	i, found := slices.BinarySearch(m.keys, k)
	if found {
		m.keys = slices.Delete(m.keys, i, i+1)
	}
	return found
}

func (m *model) has(k int) bool {
	_, found := slices.BinarySearch(m.keys, k)
	return found
}

func (m *model) filter(keep func(int) bool, desc bool, limit int) []int {
	var out []int
	if desc {
		for i := len(m.keys) - 1; i >= 0 && len(out) < limit; i-- {
			if keep(m.keys[i]) {
				out = append(out, m.keys[i])
			}
		}
	} else {
		for _, k := range m.keys {
			if len(out) >= limit {
				break
			}
			if keep(k) {
				out = append(out, k)
			}
		}
	}
	return out
}
