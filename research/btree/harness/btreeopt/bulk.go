package btreeopt

// NewFromSorted builds a tree from items already in ascending order with
// no comparisons, in O(n). Nodes are filled to about three quarters so
// that the first inserts into them do not split at once. The caller's
// slice is not retained.
func NewFromSorted[T any](degree int, less LessFunc[T], sorted []T) *BTree[T] {
	t := New(degree, less)
	if len(sorted) == 0 {
		return t
	}
	t.length = len(sorted)
	level := sorted
	var children []*node[T]
	for {
		nodes, seps := t.buildLevel(level, children)
		if len(nodes) == 1 {
			t.root = nodes[0]
			return t
		}
		level, children = seps, nodes
	}
}

// buildLevel packs keys, and the children between which they sit when
// children is non-nil, into nodes of one level. It returns the nodes and
// the separators left between them, for the level above.
func (t *BTree[T]) buildLevel(keys []T, children []*node[T]) ([]*node[T], []T) {
	maxItems, minItems := t.maxItems(), t.minItems()
	target := maxItems * 3 / 4
	if target < minItems {
		target = minItems
	}
	n := len(keys)
	// k nodes hold n-(k-1) keys; keep each between minItems and maxItems.
	k := (n + 1 + target) / (target + 1)
	if k < 1 {
		k = 1
	}
	for k > 1 && (n-(k-1))/k < minItems {
		k--
	}
	for (n-(k-1)+k-1)/k > maxItems {
		k++
	}
	base, extra := (n-(k-1))/k, (n-(k-1))%k
	nodes := make([]*node[T], 0, k)
	var seps []T
	if k > 1 {
		seps = make([]T, 0, k-1)
	}
	pos := 0
	for j := 0; j < k; j++ {
		cnt := base
		if j < extra {
			cnt++
		}
		nd := t.cow.newNode()
		nd.items = append(make(items[T], 0, maxItems), keys[pos:pos+cnt]...)
		if children != nil {
			nd.children = append(make(items[*node[T]], 0, maxItems+1), children[pos:pos+cnt+1]...)
		}
		pos += cnt
		nodes = append(nodes, nd)
		if j < k-1 {
			seps = append(seps, keys[pos])
			pos++
		}
	}
	return nodes, seps
}
