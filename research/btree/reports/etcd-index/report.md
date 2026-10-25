# etcd treeIndex and forked btree: audit

Setup: Apple M4 Pro, Go 1.27.1, temporary benchmark file in `server/storage/mvcc` (deleted). Keys `/registry/pods/ns-%04d/pod-%06d`, 100k and 1M keys, random insert order, one revision each. The modified fork ran through a scratch `go.work` replace (pinned module copy plus the branch's `btree.go`). Medians of 5 runs. Between-process noise at 1M is about 15 percent, so compare rows inside one column.

## 1. Call sites

| Path | Caller | Index code | btree calls | Lock |
|---|---|---|---|---|
| Range: values, keys-only, count-only | kvstore_txn.go:108, :91, :85 | index.go:112-190, unsafeVisit :95-107 | AscendGreaterOrEqual, Less per item | RLock, whole walk |
| Put on apply | kvstore_txn.go:230 Get, :260 Put | index.go:54-80 | Get; Get then ReplaceOrInsert | RLock; Lock |
| DeleteRange | kvstore_txn.go:298, :324 | index.go:192-203 | walk, then Get per key | RLock; Lock |
| Compact | kvstore_compaction.go:35 | index.go:205-227 | Clone, Ascend, Delete | Lock per key |
| Keep (hash) | kvstore.go:186 | index.go:230-239 | Ascend | RLock, whole walk |
| Restore | kvstore.go:456, 485 | index.go:88-93, 262-266 | Get, then ReplaceOrInsert | RLock; Lock |
| Equal | tests only (index_test.go:904) | index.go:241-260 | Len, Ascend, Get | none |

## 2. Measurements, 1M keys (100k in parentheses)

| Benchmark | pinned fork | modified fork |
|---|---|---|
| CountRevisions, full range | 128 ms (1.96 ms) | 135 ms (1.82 ms) |
| Walk only, current pattern | 35 ms (0.81) | 43 ms (0.74) |
| Walk only, AscendRange | 41 ms (0.85) | 5.8 ms (0.31) |
| keyIndex.get on every key, no tree | 38 ms (0.94) | |
| Fast get from ki.modified, no tree | 22 ms (0.50) | |
| Revisions, limit 500, with / without total | 111 ms / 8.2 µs | 82 ms |
| Get hit, 1 alloc of 64 B | 1.29 µs (436 ns) | |
| Get hit, key-beside-pointer item, 0 allocs | 856 ns (332 ns) | |
| Count live flags in item tree | 10.8 ms (0.70) | 12.6 ms (0.30) |
| Compact, 2 revs/key: total / clone walk / batched lock | 312 / 7.8 / 271 ms | |
| restoreIntoIndex: current / map then sorted | 2.06 s / 0.94 s | |
| New key: one descent / Get then insert | 1.62 / 1.78 µs (1.23 / 1.73) | |

Btree share: the walk is 27 to 41 percent of count-only and keys-only ranges today. With AscendRange on the modified fork it drops to 4 percent at 1M and 17 percent at 100k. The rest is pointer chasing into keyIndex: `ki.get` takes three dependent cache misses (struct, generations, revs). The `Less` in the callback costs 15 to 40 ms per 1M keys because it dereferences the keyIndex, not because bytes.Compare is slow.

## 3. Avoidable work

a. End check in unsafeVisit (index.go:99). Call `AscendRange` when end is non-empty. Measured with the modified fork: walk 35 to 43 ms becomes 5.8 ms per 1M keys, on all Range paths. On the pinned fork AscendRange is slower (41 vs 35 ms), so land the fork change first. Risk: low.

b. keyIndex.get at the current revision (key_index.go:149-167). When `atRev >= ki.modified.Main`, return `ki.modified` with the last generation's `created` and `ver`, or not found when that generation is empty. Measured: 38 to 22 ms per 1M. Risk: low; add a tombstone case to TestKeyIndexGet.

c. Put. tw.put calls Get (kvstore_txn.go:230), then index.Put calls Get again and ReplaceOrInsert (index.go:59-62): two descents per update, three per create, each with a 64 B probe alloc. Make index.Put return the previous created revision and version: saves 1.3 µs and one alloc per Put at 1M. A fork `GetOrInsert` saves 0.16 to 0.5 µs more per create. Risk: low; the interface change touches the test fake.

d. Compact. The btree is 2.5 percent (clone walk 7.8 of 312 ms). Per-key locking costs about 40 ms per 1M; 64 keys per lock saves 13 percent. keyIndex.compact and the `available` map dominate. Risk: low.

e. Restore. kiCache holds 10,000 keys (kvstore.go:440-452), so in a large store nearly every key misses and does Get plus ReplaceOrInsert under separate locks, in revision order. A full `map[string]*keyIndex` then sorted inserts halves index build time (2.06 to 0.94 s per 1M); sorted inserts into an empty tree are 9x cheaper than random (0.14 vs 1.34 s). Cost: about 50 MB transient at 1M keys. Risk: low to medium.

f. Locks (estimated). Every Range path holds RLock for the whole walk. A LIST with limit over 1M keys holds it 110 ms and blocks Put on the apply path; RWMutex writer preference then blocks new readers too.

## 4. Deeper options

Key-beside-pointer item, `struct{key []byte; ki *keyIndex}` by value. index.go changes: tree type, less func, stack probes, Get returns item.ki, Compact's Delete and Equal pass items. Measured: Get minus 34 percent at 1M, zero allocs; end checks stop dereferencing keyIndex. Memory: 24 bytes more per key, about 12 percent of index memory. Compact's clone path copies 2 KB nodes instead of 0.5 KB; negligible. A `live bool` in the item (last generation non-empty) makes count-only at the current revision a pure node walk: 10.8 ms instead of 128 ms per 1M, measured. Liveness changes only on create, tombstone, and re-create (one extra ReplaceOrInsert each). The index must track its maximum revision and use the per-key path when `atRev < maxRev` (a reader racing a writer, or an explicit old revision).

Order-statistic count, fork-side. Subtree sums make the LIST total O(log n): about 3 µs instead of 110 ms, and no long RLock. This is the #18184 goal without its Put regression: updates do no tree write, creates add integer increments on one path. What breaks a plain `Len` count: tombstoned keys stay in the tree until compaction, so sum a per-item weight; counts at a past revision need the walk; copy-on-write means sums must be copied in `mutableFor` (btree.go:211) and fixed in split, merge, and remove (btree.go:241-465). Needs a generic weight hook and `CountRange(ge, lt)`, about 150 lines. Risk: medium to high; it diverges from upstream and touches rebalancing. The #18184 benchmarks (IndexCountRevisions, IndexRange, IndexRevisions) are not in the tree; add them first.

## 5. Ranking, gain divided by risk

etcd-side: (1) fast get at the current revision; (2) AscendRange in unsafeVisit, after the fork change lands; (3) fold the Put-path Get into index.Put; (4) item layout with live flag; (5) restore through a map and sorted inserts; (6) batched Compact locks.

Fork-side: (1) the iteration branch, 6x faster walks at 1M with AscendRange; (2) GetOrInsert; (3) weighted order-statistic counts, after the item layout exists.
