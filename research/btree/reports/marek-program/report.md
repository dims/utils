# Marek Siarkowicz's performance program and the forked B-tree

Sources: KEP-6404, the issues and PRs linked below, and the local checkouts. Statements marked "inference" are mine.

## 1. The program

**Goal (KEP-6404, #142223).** Allocations cap kube-apiserver write throughput. A trivial PATCH of a 10 KB pod allocates over 1 MB. The Go runtime stalls near 6 GB/s of allocation. Bursts double allocations (watch breaks, admission relists, TXN-conflict GETs from etcd) and create "sticky degradation". The program cut the allocation factor from 100-180x to 50x and reached 4k QPS on 30 KB pods. Marek: "Around that point we stopped seeing improvements from reducing allocations. Next stop: watch cache." Watch termination blocks measurement, so KEP-6268 (stall-resume) must land first.

**Three tracks.**
1. Write path allocations (#142228): managedFields and SSA; 70% of remaining allocations are in `PatchResource`, 30% in watch propagation.
2. Degraded-state fallbacks: serve every GET and TXN-conflict retry from the watch cache (`ConsistentGetFromCache`, alpha v1.38), direct listers for admission (#142304), stall-resume (#142676).
3. etcd storage capacity (#22332): 30 GB datasets, bbolt vs pebble/badger. This track is about the on-disk engine, not the in-memory index.

## 2. Work-item map

| Item | Mechanism | Status | Tree on path? | Tree op | Tree cost matters? |
|---|---|---|---|---|---|
| [#143129](https://github.com/kubernetes/kubernetes/pull/143129) consistent GET | RV="" fetches the current RV from etcd, waits, then `snap.GetByKey` | prototype | yes | Get on latest snapshot | No: 0.5 µs and one 48 B probe against 132-176 KiB per GET (#142872) |
| [#143131](https://github.com/kubernetes/kubernetes/pull/143131) TXN from cache | `Update` loop: `cacher.get(key,"0")`, DeepCopy, etcd txn without `GetOnFailure`; on conflict `cacher.get(key, rv)` and retry | prototype | yes | Get per attempt | No. DeepCopy, re-encode and the etcd round trip dominate |
| [#143134](https://github.com/kubernetes/kubernetes/pull/143134) minheap | `rvWaiter` heap of waiters keyed by RV replaces `sync.Cond` broadcast; waiting leaves the lock | prototype, on hold | no | none | n/a |
| [#143153](https://github.com/kubernetes/kubernetes/pull/143153) PrevKV | `WatchFromStorageWithoutPrevKV`: etcd watch without `WithPrevKV`; deletes arrive as a name/namespace/RV stub; store `Delete` returns the old object | prototype | yes | return value of ReplaceOrInsert and Delete (from #142300) | Return value is required; cost is not |
| [#143167](https://github.com/kubernetes/kubernetes/pull/143167), [#143276](https://github.com/kubernetes/kubernetes/pull/143276) | wait for freshness outside the lock; one `getLatestSnapshot` path for list and get | approved | indirect | LatestSnapshot pointer | no |
| [#142519](https://github.com/kubernetes/kubernetes/pull/142519) WatchList snapshot only | initial events always from `newCacheIntervalFromLazySnapshot(LatestSnapshot())` | draft | yes | AscendGreaterOrEqual | Small; encode per object dominates |
| [#142676](https://github.com/kubernetes/kubernetes/pull/142676) stall-resume (KEP-6268) | slow watchers park and catch up from the event ring buffer | draft | no | none | n/a |
| [#142304](https://github.com/kubernetes/kubernetes/pull/142304) direct lister | admission Lister reads the cacher; wojtek-t and liggitt object to a second read path | open, contested | yes | Get, AscendGreaterOrEqual | No; conversion and copies dominate |
| [#141728](https://github.com/kubernetes/kubernetes/pull/141728) filters down the stack | `ProcessPrefix(ListRequest)` with `ComputeTotalCount`: keeps walking after the limit to count | draft PoC | yes | one AscendGreaterOrEqual per LIST | **Yes**: the count walk is O(N) per page (inference) |
| #143353, #143212, #143164, #143107, [#140808](https://github.com/kubernetes/kubernetes/pull/140808) | watcher init refactors; etcd gRPC connection pool | open | no | none | n/a |
| etcd [#22332](https://github.com/etcd-io/etcd/issues/22332), [#22333](https://github.com/etcd-io/etcd/pull/22333), [#22339](https://github.com/etcd-io/etcd/pull/22339) | bbolt benchmarks (write, read, watch, size) and MVCC correctness harness; pebble and badger prototypes | open | no (bbolt) | read benchmark targets `RangeLimit100` and `CountOnly` hit treeIndex | Only for `CountOnly` and keys-only |
| etcd [#18184](https://github.com/etcd-io/etcd/pull/18184) | per-revision immutable trees for cheap counts; "adding fast counting should not be hard" | stale since 2024 | yes | Clone per put, Get, Range | Puts went from 1 to 7-16 allocs; that blocked it |
| etcd [#21485](https://github.com/etcd-io/etcd/pull/21485) | robustness test for the client cache (#21428 consistent reads) | open | yes | Clone per revision, AscendGreaterOrEqual | Experimental library; kubernetes does not import it |
| etcd [#19693](https://github.com/etcd-io/etcd/pull/19693) | WAL bootstrap from backend position | open | no | none | n/a |

**Allocation budget (sources).** PATCH costs about 1.1 MiB and 13-16k allocs per request; GET rv=0 costs 50-89 KiB. KEP-4988 measured the B-tree at 0.2% of allocations and 1.3% of memory. Inference: the 1.8 KB and 2 µs per write-plus-Clone is below 0.2% of a write. No KEP-6404 item is gated on per-call tree cost. The watch cache also clones on every bookmark (`UpdateResourceVersion`).

## 3. Gaps where a tree feature helps

1. **Range count in O(log N).** Three live paths count by walking every key. (a) `cacher.go` calls `resp.Count()` before `resp.All()` for every predicate-free LIST, so a limit=500 page over 150k pods walks all 150k elements twice. (b) #141728 `ComputeTotalCount` has the same shape. (c) The normal etcd Range RPC passes `withTotalCount=true` (`v3_server.go:149`), so every kube-apiserver LIST with limit from etcd visits every key in the range; only RangeStream opts out (#21618). This is the motivation of #18184 and is the only tree cost that grows with collection size instead of page size (inference). etcd caveat: tombstoned `keyIndex` entries stay until compaction and historical counts check each key, so a tree count covers latest-revision reads only unless tombstones are tracked.
2. **Probe-free lookups.** Every `Get` and scan allocates a probe: `&Element{Key}` (48 B) in the cacher, two `&keyIndex{key}` per etcd visit, one `string(key)` copy in the client cache. Bytes are negligible against 50 KiB per GET. Allocation count is what the APF cost model (#142803) will price and what index-only etcd requests (keys-only, count-only, `GetCurrentResourceVersion`) pay.
3. **Bulk build.** `Replace` clears and re-inserts one element at a time; etcd `restoreIntoIndex` inserts in revision order (not key order); the client cache `Restore` inserts per kv. Relist is the degradation path the KEP names, but decode of N objects dominates (inference).
4. **PrevKV.** Already served by the return values of ReplaceOrInsert and Delete. No tree change is needed.
5. **Minheap.** Not tree-related.

## 4. Open questions

- The offline numbers for #142304 and the write-throughput profile share of the tree.
- Whether etcd maintainers still want cheap counts now that RangeStream skips them, and whether they accept tombstone-aware counting.
- Whether kube-apiserver will adopt the etcd client cache (KEP-4743) and so inherit its Clone-per-revision design.
- Which of the stacked prototypes (#143129, #143131, #143134, #143153) target v1.38.
- The state of the Go runtime 6 GB/s report.
