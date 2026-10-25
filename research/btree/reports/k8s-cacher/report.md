# Watch cache use of the forked B-tree

Setup: kubernetes master 765dd1d1458, Go 1.27.1, Apple M4 Pro. Vendored fork = k8s.io/utils master (no iteration speedup). Measured with `BenchmarkCacher_GetList*` (MockStorage, no etcd or HTTP) and two temporary benchmarks (deleted). Estimates are marked.

## 1. Call inventory

| Path | Tree calls | Lock |
|---|---|---|
| Write event | watch_cache.go:249 → watch_cache_storage.go:178-191: ReplaceOrInsert btree.go:74 or Delete :54, Clone :43, snapshotter.Add → ReplaceOrInsert on a second tree snapshotter.go:154 | write |
| Bookmark | watch_cache.go:306 → Clone watch_cache_storage.go:201 (same-RV case skipped, watch_cache.go:296-304) | write |
| Compaction | watch_cache.go:265, compactor.go:75, cacher.go:1242 → snapshotter.go:185-194: DescendLessOrEqual (probe :200), Min, DeleteMin | write |
| Replace (resync) | watch_cache.go:636 → btree.go:62-68 Clear, ReplaceOrInsert per element; Clone; snapshotter.Replace | write |
| GET, single-key LIST | watch_cache.go:576-587 → GetByKey btree.go:79-80 | RV=0 lock-free; RV>0 RLock for freshness |
| LIST latest RV | watch_cache.go:503 RangePrefix; cacher.go:824 Count → countPrefix btree.go:117-128; cacher.go:831/848 All → rangePrefix btree.go:103-114 | snapshot under RLock (:508-519), walk lock-free |
| LIST exact RV, continue | watch_cache.go:478 → snapshotter.getSnapshot :200 (probe), wrapper :130; then as above | RLock, walk lock-free |
| LIST indexed | watch_cache.go:530 → indexer.go:53-56 bucket copy, no tree | RLock |
| WATCH init | cacher.go:601 → watch_cache.go:705-707 takes LatestSnapshot; watch_cache_interval.go:321/333 walks RangePrefix or GetByKey | RLock, walk lock-free |
| CanServeExactRV | watch_cache_storage.go:66-70 → Min snapshotter.go:108 | RLock |

## 2. Measured tree cost

| Operation | ns/op | B/op | allocs |
|---|---|---|---|
| GetByKey, 100k keys | 137 | 48 | 1 |
| GetByKey, reused probe | 121 | 0 | 0 |
| countPrefix 100 / 10k / 100k items | 749 / 56k / 625k | 48 | 1 |
| rangePrefix walk 100 / 10k / 100k | 874 / 68k / 700k | 136 | 4 |
| ReplaceOrInsert, no Clone | 158 | 0 | 0 |
| ReplaceOrInsert then Clone | 794 | ~2,100 | 14 |
| UpdateStore, snapshotter on, compacting | 917 | 2,193 | 17 |
| UpdateResourceVersion (Clone only) | 50 | 80 | 4 |
| Replace, 100k sorted elements | 11.3 ms | 3.1 MB | 20.8k |
| snapshotter.GetSnapshot | 71 | 32 | 2 |

## 3. Avoidable work

A. Double walk for unfiltered lists. cacher.go:824 Count walks the whole remaining prefix range; :831 All walks it again. Measured: 56 µs of a 594 µs 10k-item list (9%); 0.75 µs of an 8.9 µs 100-item namespace list (8%). Worst: paginated exact-RV lists, where Count walks everything after the continue key just for remainingItemCount (continue.go:114-116): 56 µs of a 92 µs 500-item page at 10k objects (61%); 625 µs per page at 100k (estimate). Fixes: consumer-side, if continueKey is empty and Min and Max both carry the prefix, Count = Len in O(log n); fork-side, subtree counts give O(log n) Count(ge, lt) for all lists. A one-pass []*Element collect measured 134 vs 131 µs plus 310 KB garbage: no gain.

B. Probe allocation. GetByKey, countPrefix, rangePrefix and OrderedListPrefix each heap-allocate a 48 B Element (btree.go:79, 89, 108, 121 "escapes to heap"); snapshotter.go:200 and :130 add 32 B. Per GET: 1 of 17 allocs, 48 of 904 B, ~15 ns.

C. Write path. ReplaceOrInsert alone is 158 ns, zero allocs. The Clone after each write forces a root-to-leaf copy on the next write: 2.1 KB and 14 allocs per event, 17 with the snapshotter. Inherent to persistent snapshots. mutableFor (btree.go:211-230) copies slices at old capacity (up to 504 B per node); copying at length cuts write garbage ~30% (estimate). The bookmark Clone (50 ns, 80 B) needs no change.

D. Replace inserts one by one (btree.go:62-68): 11.3 ms, 3.1 MB, 20.8k allocs per 100k objects under the write lock (watch_cache.go:617-640); all readers stall during init and relist. A bulk builder for sorted input was prototyped at 0.7 ms per 100k; it must check order and fall back.

E. Snapshotter second tree: ReplaceOrInsert per event plus DescendLessOrEqual, Min and DeleteMin per compaction, 1-3 allocs and ~150 ns per event. The cyclic-buffer TODO (snapshotter.go:64) removes it. btreeStore.OrderedListPrefix (btree.go:84) has no production caller.

## 4. Tree share on KEP-6404 hot paths

| Path | Total (measured) | Tree share |
|---|---|---|
| GET from snapshot, RV=0 | 637 ns, 904 B, 17 allocs | 130 ns, 48 B, 1 alloc (20%, 5%, 6%); rest: tracing span, prepareKey, reflect copy |
| GuaranteedUpdate retry from cache (PR 143131: cacher.get, DeepCopyObject, etcd txn) | DeepCopy of example.Pod adds 220 ns, 848 B, 3 allocs (real Pods far more); etcd txn ~1 ms (estimate) | two GetByKey: <0.3 µs, 96 B, 2 allocs (<1%) |
| Direct lister ListPods, labels.Everything, 100 pods | 8.9 µs, 59 KB, 43 allocs, plus ConvertToVersion copies in direct/client.go | 1.6 µs (18%), 2 allocs; 0.75 µs avoidable |
| Direct lister, label selector, 10 of 100 | 5.0 µs, 8.1 KB, 51 allocs | 0.9 µs (18%), 1 alloc |
| Paginated 500 of 10k, exact RV | 92 µs, 256 KB, 51 allocs | 60 µs (65%); 56 µs avoidable |

The tree adds 1-2 allocations per read; object copies and conversion dominate. Tree-side gains are list CPU time and write-lock hold time.

## 5. Ranking by gain over risk

Consumer side (kubernetes):
1. Min/Max prefix shortcut in countPrefix: 56 µs per 10k-item list, 625 µs per 100k. Risk very low.
2. Probe pool for GetByKey and prefix walks: 1 alloc, 48 B per read. Risk low.
3. Cyclic-buffer snapshotter: 1-3 allocs, ~150 ns per write event. Risk medium.
4. Delete dead btreeStore.OrderedListPrefix. Risk none.

Fork side (k8s.io/utils):
1. Merge the iteration speedup: 13-72% faster scans on 18-65% of list cost. Risk low (fuzz-tested).
2. Bulk build from sorted input: 11.3 ms to ~0.7 ms per 100k under the write lock. Risk low to medium.
3. Subtree counts for O(log n) Count: removes the paginated count walk (56 µs per page at 10k, 625 µs at 100k). Risk medium (touches split and merge).
4. Allocation-free lookup API: 1 alloc per Get. Risk low.
5. Length-based copies in mutableFor: ~30% less write garbage (estimate). Risk low to medium.
