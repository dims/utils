# B-tree API candidates: measurements and recommendations

Setup: Apple M4 Pro, Go 1.27.1. Everything is in `/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/agents/fork-api/`. External test packages, consumer shapes: string key at degree 16, []byte key at degree 32. Medians of 2 to 3 runs (noise 3% at 100k, 10% at 1M). Compared variants ran in separate processes.

## 1. `NewFromSorted(degree, less, sorted []T)` - keep

`bulk/bulk.go`, 55 code lines; `btree.go` unchanged. It checks strict ascent in one pass (else it inserts one by one), then builds without comparisons: nodes 74% full, all leaves at one depth.

| degree 16 | insert 100k | bulk 100k | insert 1M | bulk 1M |
|---|---|---|---|---|
| build | 12.0 ms | 0.84 ms (0.49 check + 0.30 build) | 145 ms | 8.6 ms (5.2 + 2.6) |
| live tree | 2.2 MB | 1.1 MB | 22.4 MB | 11.5 MB |
| Get | 368 ns | 360 ns | 1.44 us | 1.44 us |
| scan 20k | 48.6 us | 51.2 us | 49.1 us | 50.5 us |

The bulk build allocates 2.6x fewer bytes. Degree 32: 10.9 vs 0.73 ms, 130 vs 7.6 ms. `FuzzNewFromSorted` (175k execs, shape invariants verified after every operation) and a delete-and-reinsert test at degrees 2 to 32 pass.

Risks: the order check costs more than the build; exact-capacity node slices reallocate once on the first insert; the fallback changes the cost class silently.

## 2. Probe-free lookup - defer

`find/find.go`: `Find[T, K any](t *BTree[T], key K, cmp func(K, T) int) (T, bool)` and `(*BTree[T]).GetFunc(cmp func(T) int)`, about 35 code lines.

| | Get, probe alloc | Find | GetFunc |
|---|---|---|---|
| string 100k | 356 ns, 48 B | 334 ns | 337 ns, 0 B |
| string 1M | 1.56 us | 1.55 us | 1.57 us |
| []byte 100k | 350 ns, 64 B | 326 ns | 320 ns, 0 B |

Get with a reused probe costs 335 ns, so the whole gain is the allocation: 6% at 100k, nothing measurable at 1M. A hand-rolled three-way search (`FindBisect`, measured only) gains 10% more here, but that loop gave nothing on x86. Risk: a second ordering contract (`cmp` must agree with `less`) that fails silently.

## 3. `GetOrInsert(item T) (T, bool)` - keep, for etcd

`getorinsert/btree.go`: +27/-6 lines. `insert` takes a `replace` flag; both methods share one descent. []byte keys, degree 32.

| | Get then ReplaceOrInsert | GetOrInsert |
|---|---|---|
| new key, 100k / 1M | 773 ns / 2.28 us | 546 ns (-29%) / 2.12 us (-7%) |
| existing key, 100k / 1M | 348 ns / 1.55 us | 353 ns / 1.55 us |
| existing key just after Clone, 100k / 1M | 359 ns / 1.57 us (Get) | 959 ns, 9 allocs / 2.28 us |

Fuzz: 284k execs, verifying that it keeps the existing item. Risk: on a freshly cloned tree a found `GetOrInsert` copies the path and splits full nodes. The watch cache (clone per write) must not use it as a Get; etcd's treeIndex (clone at compaction) is the target.

## 4. Subtree counts and `CountRange(ge, lt)` - keep

`counts/btree.go`: +45/-4 lines over every structural path, plus `count.go`, 24 code lines: `CountRange` = rank(lt) - rank(ge), O(degree x log n). `node` grows from 56 to 64 bytes inside the same size class: no memory cost.

| degree 16 | baseline | counts |
|---|---|---|
| replace existing, 100k | 342 ns | 345 ns (+1%) |
| delete + insert, 100k | 624 ns | 632 ns (+1.3%) |
| Clone + delete + insert, 100k | 1208 ns, 1761 B, 12 allocs | 1227 ns, same (+1.6%) |
| count 200-item namespace, 100k / 1M | walk 1.23 us / 6.06 us | 0.41 us / 0.76 us |
| count whole tree, 100k / 1M | walk 284 us / 3.27 ms | 0.19 us / 0.24 us |

At 1M the write workloads are within noise. Fuzz: 254k execs with every node's count verified after every operation and `CountRange` checked against the model. The overhead is acceptable for the watch cache's write-then-Clone pattern: about 20 ns on a 1.2 us operation dominated by copying 4 nodes (1.7 KB, 12 allocs).

## 5. Clone allocation trimming - drop

`clone/btree.go`: +7/-3 lines. Both contexts in one `[2]copyOnWriteContext`: 3 allocations become 2.

| degree 16 | baseline | contexts array |
|---|---|---|
| Clone alone, kept, 100k | 27.3 ns, 3 allocs | 22.5 ns, 2 allocs |
| Clone + delete + insert, 100k | 1207 ns | 1223 ns |
| insert + Clone kept, 100k / 1M | 925 ns / 2.0 us | 923 ns / 2.0 us |

The 5 ns saved are under 1% of a write-then-Clone. A first attempt put the tree struct and both contexts in one block: 8 to 15% slower, and 344 MB kept alive after 200k writes against 1.6 MB, because the old tree's context sat beside the new tree's root, so every replaced node stayed reachable. `CloneOneBlock` and `TestCloneRetention` keep the proof: never put a context in the same allocation as a root.

## Summary

Keep 1 (14 to 17x faster build, half the memory), 3 (-29% for new keys) and 4 (1 to 2% write overhead). Defer 2. Drop 5.
