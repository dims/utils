# etcd client cache and the forked btree: audit

Scope: etcd `cache/` module on k8s.io/utils v0.0.0-20260108192941; the `btree-benchmarks` branch measured in situ through a scratch `GOWORK` replace. Temporary in-package benchmark (deleted): 100k and 1M sorted 38 B keys, degree 32, ring 2048, arm64. Design issue etcd-io/etcd#19371 (GSoC 2025 watch cache with Kubernetes parity; benchmarks are an open milestone). Open PR etcd-io/etcd#21604 adds benchmarks up to 10k keys.

## 1. Call sites

| Call | Site | Lock |
|---|---|---|
| New | store.go:42; store.go:109 | none; write |
| ReplaceOrInsert | store.go:111 (Restore loop); store.go:161 | write |
| Delete | store.go:157, probe `&kvItem{}` | write |
| Clone | snapshot.go:30 from store.go:115 and :166, one per revision batch | write |
| Get | snapshot.go:37 | none: store.go:76 drops the RLock before Range (store.go:71) |
| AscendGreaterOrEqual | snapshot.go:41, WithFromKey only (`isPrefixScan` is a misnomer) | none |
| AscendRange | snapshot.go:46, explicit ranges and WithPrefix (cache.go:205 passes a real end key) | none |

Unused: Len, Has, Min, Max, Descend*, Clear. Reads are lock-free on immutable clones; a long LIST never blocks Apply.

## 2. Measured, released fork

| Operation | Result |
|---|---|
| store.Get one key, 100k / 1M | 0.50-0.75 µs / 1.2-1.6 µs; 3 allocs, 80 B (probe 24 B, key copy 48 B, result slice). Ring lookup 7.7 ns. Reused probe 0.56-0.63 µs: the probe allocation is invisible in latency. |
| Range, 100k results | 1.05 ms, 32 allocs, 4.50 MB garbage for 0.80 MB payload (5.6x; 3.8x at 10k). Iteration alone 0.63 ms; count-then-fill 1.35 ms, 1 alloc. |
| Apply one put per revision, 100k / 1M | 1.21 µs, 2.9 KB, 16 allocs / 1.93 µs, 3.5 KB |
| Retained per snapshot, ring full, after GC | 3.04 KB (100k), 3.59 KB (1M); 0.27 KB is the new KeyValue (120 B), item, key and Clone bookkeeping, the rest the copied root-to-leaf path. No growth after 2048 more revisions. Ring total 6.2 / 7.4 MB plus old values (not measured). |
| Restore, sorted input one by one, write lock held | 11.9 ms, 210k allocs, 10 MB (100k); 137 ms, 2.1M allocs, 100 MB (1M). Readers block on every reconnect (cache.go:339-346). |

Estimate: sorted inserts leave nodes 31 of 63 full and half the leaves keep cap-64 arrays, so a path copy is 2.2 KB at depth 4 and 3.5 KB at depth 5; measured 2.8 and 3.3 KB.

By-value `kvItem{key string; kv *KeyValue}` experiment: Get unchanged (647-718 against 656 ns), AscendRange 10 percent slower, Clone plus insert 2.19 µs and 6.0 KB against 1.41 µs and 2.8 KB. Not recommended. An `unsafe.String` view of `kv.Key` in the current layout saves 48 B per item (48 MB at 1M).

## 3. Branch iteration speedup, in situ

| Range (AscendRange path) | Released | Branch | Change |
|---|---|---|---|
| 1k results | 8.9 µs | 5.2 µs | -42% |
| 10k | 95 µs | 56 µs | -41% |
| 100k | 1.05 ms | 0.68 ms | -35% |
| Iteration only, 100k | 0.63 ms | 0.27 ms | -57% |
| FromKey full scan | 0.84 ms | 0.65 ms | -23% |
| store.Get, 3 runs | 0.70-0.75 µs | 0.63-0.65 µs | noise |

## 4. Ranking by gain over risk

Cache package:
1. Build the Restore tree before `s.mu.Lock()` (store.go:106-115), swap under the lock. Removes a 12 to 137 ms reader stall per reconnect. Minimal risk.
2. Presize the Range result with `tree.Len()` when the request covers the shard prefix (cache.go:479-480): a 100k LIST drops to about 0.72 ms, 0.8 MB, 1 alloc (estimated: count-then-fill minus iteration). Keep append for sub-ranges.
3. Bump k8s.io/utils when the branch lands: 35 to 42 percent lower LIST latency, no code change.
4. `unsafe.String` keys for items and probes: 48 B less per key, one alloc less per Get, 100k allocs less per Restore. Depends on KeyValue.Key staying immutable. Medium risk.
5. Limit needs no fork change (return false from the callback; cache.go:456 rejects it); whole-shard CountOnly is Len(). Document that Range returns pointers shared with every snapshot (snapshot.go:38-50). Do not adopt the by-value item type.

Fork:
1. Iteration speedup branch: measured above, fuzz-tested, no API change.
2. Bulk constructor from sorted input: Restore 10 to 17x faster (prototype 0.7 against 19 ms per 100k; about 10 ms at 1M, extrapolated), but full leaves raise the per-revision path copy about 15 percent (estimated). Medium risk; cache item 1 removes the visible stall alone.
3. Skip the FreeList mutex when its capacity is 0 (btree.go:81-93): 11 lock pairs per Apply, at most 15 percent of 1.2 µs (estimated). Low risk.
4. Not needed: probe-free lookup (under 20 ns), range Count (needs subtree counts), Ascend with limit (the callback already stops).
