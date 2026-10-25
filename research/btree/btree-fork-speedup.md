# Faster range iteration in the forked B-tree in k8s.io/utils

Date: 2026-10-24, updated 2026-10-25
Branch: [`btree-benchmarks`](https://github.com/dims/utils/tree/btree-benchmarks) on [github.com/dims/utils](https://github.com/dims/utils), four commits on top of [kubernetes/utils](https://github.com/kubernetes/utils) master at [ca04ba5](https://github.com/kubernetes/utils/commit/ca04ba5)
Code change: `third_party/forked/golang/btree/btree.go`, 10 lines added, 5 removed
Research material: branch [`btree-research`](https://github.com/dims/utils/tree/btree-research/research/btree) on github.com/dims/utils, with the audit reports, benchmark scripts, raw results from both machines, the harness and the prototypes
etcd change used in the measurements: branch [`btree-ascendrange`](https://github.com/dims/etcd/tree/btree-ascendrange) on github.com/dims/etcd, commit [44d03d5bc](https://github.com/dims/etcd/commit/44d03d5bc)

## Executive summary

The B-tree under `third_party/forked/golang/btree` in k8s.io/utils is a frozen copy of [google/btree](https://github.com/google/btree). Three systems depend on it: the kube-apiserver watch cache, etcd's in-memory key index, and etcd's experimental client cache. A review of it against [bbtree](https://github.com/XiangpengHao/bbtree), a database B-tree written in Rust, led to one ten-line change in how the tree iterates over a range, a set of benchmarks shaped like the consumers' traffic, and a differential fuzz test. The change is on the branch named above and is ready for a pull request.

The main findings are these.

- The tree is cheap on the paths people usually worry about. It is below 0.2 percent of the allocations of a write and 1 to 2 percent of an object list, so it does not matter for [KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput), whose problem is allocations.
- The tree is the main cost of one class of request, counting keys. A paginated LIST served from the watch cache counts the rest of the collection on every page, and every kube-apiserver LIST with a limit that reaches etcd makes etcd walk every key in the range under its index read lock, which blocks writes. That cost grows with collection size, and it is the problem Marek Siarkowicz's stalled etcd experiment [#18184](https://github.com/etcd-io/etcd/pull/18184) tried to solve.
- The iteration change makes range scans 13 to 72 percent faster on the primitive. On etcd's own `BenchmarkRange` it gives 5 to 7 percent on a limited range on both arm64 and amd64, and 14 percent once etcd's `unsafeVisit` uses `AscendRange`. On Kubernetes' own `StoreList` benchmark it gives 19 to 27 percent on paginated lists on an Apple M4 and nothing measurable on an Intel desktop, because a tree visit is two to three times cheaper per item on the Intel machine and page copying dominates there.
- The change that would move paginated lists on every CPU is an O(log n) range count in the tree, prototyped at 1 to 2 percent write overhead, plus a bulk constructor for relist and restore. Several consumer-side fixes need no tree change at all, such as building the etcd client cache's restore tree outside its lock.

The recommended order is at the end of this document: merge the branch, then a second fork PR with `CountRange`, `NewFromSorted` and `GetOrInsert`, then the etcd and Kubernetes consumer changes that use them.

## Summary

Kubernetes and etcd both use the B-tree in k8s.io/utils under `third_party/forked/golang/btree`. While comparing that code with [bbtree](https://github.com/XiangpengHao/bbtree), a B-tree written in Rust for database workloads, I found one clear inefficiency in the fork. During a range iteration, the tree ran a binary search for the start key in every node it visited, including every node after it had already passed the start. It also compared every item with the stop key, even in nodes that lie completely inside the range.

A ten-line change removes both. Range scans are now 13 to 72 percent faster, depending on the shape of the scan and the CPU, with no change in behavior and no change in allocations. Point lookups are unchanged.

The branch also adds two things the fork did not have. The first is a set of benchmarks that model how the watch cache and etcd use the tree. The second is a differential fuzz test that checks the tree against a sorted slice. Both were needed to make the change and to show that it is safe.

## Background

The fork is [google/btree](https://github.com/google/btree), which Google archived. Kubernetes copied the generic version into k8s.io/utils in January 2026, dropped the old interface-based API, and kept the algorithm as it was. The file is byte for byte the same as the last upstream release, v1.1.3, apart from the removed API.

The fork has three consumers.

| Consumer | Items | Degree | How it uses Clone |
|---|---|---|---|
| apiserver watch cache store | `*Element`, string key | 16 | Clones after every write to publish a lock-free snapshot and to serve lists at exact resource versions |
| etcd server treeIndex (3.7) | `*keyIndex`, byte-slice key | 32 | Clones at each compaction and walks the clone while deleting from the live tree |
| etcd client cache, experimental | `*kvItem`, string key | 32 | Clones after each revision batch and keeps 2048 snapshots |

All three store a pointer per item, and each comparison follows that pointer to a string such as `/registry/pods/<namespace>/<name>`. All three list ranges with `AscendGreaterOrEqual` or `AscendRange`, which is the path the change speeds up.

## What the tree costs the consumers today

Before changing anything, I measured the consumers' access shapes against the unmodified fork, with 100k registry-style keys, and compared the numbers with the consumers' own benchmarks. The harness for those measurements is under [`research/btree/harness`](https://github.com/dims/utils/tree/btree-research/research/btree/harness) on the `btree-research` branch.

| Operation | Cost in the fork | Share of the consumer's work |
|---|---|---|
| Point lookup, 100k keys | about 0.5 µs | about 1 percent of a watch cache write, which costs about 250 µs through the cacher |
| Write then Clone, the watch cache pattern | about 2 µs and 1.8 KB of garbage | the same 1 percent |
| Iteration | about 10 ns per item | about 2 percent of a listed object |
| etcd treeIndex Get, 1M keys, etcd's own benchmark | 1.8 µs | small next to the bbolt read and gRPC |

On these two paths, writes and object lists, the tree is a small share and the gains below are gains on the primitive. The tree is the main cost on a different set of paths, those that count keys: paginated lists served from a watch cache snapshot, and etcd ranges with a limit, which count every key in the range to report a total. The update at the end of this document has the measurements.

## The change

The iteration code in `iterate` carries a flag named `hit` that becomes true once the first item at or past the start has been reached. After that point, every item in every later node is past the start, so a search for the start key in those nodes always returns index zero. The old code ran the search anyway, once per node, with several string comparisons each time.

The stop check had a similar cost. The old code compared each item with the stop key. The new code compares the node's last item with the stop key once. If that item is below the stop, every item in the node is, and the per-item check is skipped. Nodes at the end of the range still check each item.

The ascending case now reads as follows. The descending case mirrors it.

```go
case ascend:
	// Past the start, every item is greater than it: nothing to find.
	if start.valid && !hit {
		index, _ = n.items.find(start.item, n.cow.less)
	}
	// If the last item is below the stop, every item is.
	allBelowStop := !stop.valid || len(n.items) == 0 || n.cow.less(n.items[len(n.items)-1], stop.item)
	for i := index; i < len(n.items); i++ {
		...
		if !allBelowStop && !n.cow.less(n.items[i], stop.item) {
			return hit, false
		}
```

Every public method keeps its contract. The fuzz test and the upstream test suite confirm that.

## Results

The benchmarks use 100k keys of the form `/registry/pods/namespace-NNN/deployment-name-replicaset-NNNNNN`, 500 namespaces of 200 keys, inserted in a fixed random order, with pointer items as the consumers store them. Each run compares the unmodified fork with the branch, back to back on the same machine. Allocations are identical before and after in every row.

Apple M4 Pro, macOS, Go 1.27.1, six runs each.

| Benchmark | Before | After | Change |
|---|---|---|---|
| Prefix list of one namespace, about 200 items | 2.94 µs | 2.13 µs | −28% |
| Bounded range with an end key, about 200 items | 3.15 µs | 1.43 µs | −55% |
| Ascending scan of 20k items | 213 µs | 61 µs | −72% |
| Descending scan of 20k items | 294 µs | 178 µs | −40% |
| Point lookup, degree 16 and 32 | 0.49 to 0.51 µs | 0.49 µs | no change |

Intel Core Ultra 9 285K, Ubuntu, Go 1.27.2, eight runs each.

| Benchmark | Before | After | Change |
|---|---|---|---|
| Prefix list of one namespace, about 200 items | 2.31 µs | 2.00 µs | −13% |
| Bounded range with an end key, about 200 items | 2.85 µs | 1.45 µs | −49% |
| Ascending scan of 20k items | 68 µs | 41 µs | −40% |
| Descending scan of 20k items | 260 µs | 214 µs | −18% |
| Point lookup, degree 16 and 32 | 0.38 to 0.40 µs | 0.38 to 0.40 µs | no change |

The gain is larger on the M4 because each saved comparison costs more there. The shape of the result is the same on both machines.

## A pitfall in how the fork is benchmarked

The first version of the benchmarks lived inside package `btree`, and it reported point lookups at 350 ns on the M4. The same benchmark compiled as an external test package reported 500 ns. The difference is in how the compiler handles the generic code across a package boundary. Inside the package, it inlines `sort.Search` and the closure the node search passes to it. Across the boundary, it does not, so each probe pays two indirect calls. Kubernetes and etcd call the tree from other packages, so the external number is the one they pay.

The benchmarks on the branch are therefore in `package btree_test`, and a comment at the top of the file explains why. Anyone who benchmarks the fork from inside the package will understate what the consumers pay by about 30 percent on lookups.

## What was tried and left out

A plain binary search loop in place of `sort.Search` removes those indirect calls. On the M4 it made point lookups 31 percent faster. On the Intel machine it made no measurable difference, so I dropped it from the branch to keep the diff small. It remains a candidate if someone measures a gain on arm64 servers such as Graviton or Ampere.

A bulk constructor that builds a tree from an already sorted list in one pass, with no comparisons, built 100k items in 0.7 ms against 18.8 ms for inserting them one by one, and used half the memory. The watch cache's Replace and the etcd cache's Restore both receive their input in key order. The constructor is a new API, so it belongs in its own change.

Storing the key beside the pointer inside the node, instead of only the pointer, made lookups 36 to 44 percent faster in a prototype, because each probe follows one pointer instead of two. That change belongs to the consumers, since it changes the item type they store, and it makes the watch cache's cloned path about twice as large per write.

## Verification

- The fork's own upstream test suite passes on the branch under the race detector, including its concurrent Clone test.
- The new fuzz test runs random operations on a tree with degree 2 to 5 and keys 0 to 63, against a sorted slice. It checks insert, delete, get, min and max removal, every Ascend and Descend variant with early stops, Len after every step, and up to four kept clones at the end. It ran about two minutes in total across the original code and the branch and found no disagreement.
- `go vet`, `gofmt -s` and `make verify-apidiff` are clean for the package.
- `make verify-fmt` and `make verify-lint` fail on master too, in `inotify`, `internal/third_party/forked/golang/net`, `io`, `lru` and `pointer`, because the local Go 1.27 tools are newer than the ones the repo was formatted with. The branch does not touch those files.

## How to reproduce

Run the benchmarks on master and on the branch, then compare.

```sh
git checkout master
go test -run '^$' -bench 'BenchmarkStringKey' -benchmem -count=8 ./third_party/forked/golang/btree/ > before.txt
git checkout btree-benchmarks
go test -run '^$' -bench 'BenchmarkStringKey' -benchmem -count=8 ./third_party/forked/golang/btree/ > after.txt
benchstat before.txt after.txt
```

The benchmark file exists only on the branch, so for the master run copy `btree_bench_test.go` into the package first, or cherry-pick the first and third commits.

Run the fuzz test for longer than the seed corpus.

```sh
go test -run '^$' -fuzz '^FuzzBTree$' -fuzztime=60s ./third_party/forked/golang/btree/
```

## The commits

| Commit | Content |
|---|---|
| [66b03dc](https://github.com/dims/utils/commit/66b03dc) | Benchmarks for string-keyed workloads |
| [ef1e95e](https://github.com/dims/utils/commit/ef1e95e) | Differential fuzz test against a sorted slice |
| [d88b6d3](https://github.com/dims/utils/commit/d88b6d3) | Benchmarks moved to an external test package |
| [094923d](https://github.com/dims/utils/commit/094923d) | Iteration stops re-searching nodes and tests the stop once per node |

## Suggested next steps, as of 2026-10-24

Superseded by the recommendations at the end of this document, which take the later measurements into account.

## Update, 2026-10-25: where the tree is the bottleneck, and what to do about it

Five follow-up investigations looked at each consumer's call sites, at Marek Siarkowicz's performance program ([KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput) and his open PRs), and at prototypes of further tree changes. The correction to the claim above, and the plan that follows from it, are below. All numbers are from an Apple M4 Pro unless stated. The five reports and their raw outputs are under [`research/btree/reports`](https://github.com/dims/utils/tree/btree-research/research/btree/reports) on the `btree-research` branch.

### The correction

The tree's per-call cost is below 0.2 percent of a write and about 20 percent of a GET served from the snapshot, which is 130 ns of 637 ns. Neither matters for [KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput), whose problem is allocations measured in hundreds of kilobytes per request.

The tree dominates one class of request, and that class grows with collection size rather than with page size.

| Path | Measured | Tree share |
|---|---|---|
| Watch cache, paginated LIST at an exact RV, 500 of 10k objects | 92 µs per page, of which 56 µs is the count walk for `remainingItemCount` | 65 percent |
| Watch cache, the same at 100k objects | about 625 µs per page for the count alone | grows with the collection |
| Watch cache, predicate-free LIST | walks the prefix twice, once to count and once to fill | 8 to 9 percent at 10k items |
| etcd, count-only or keys-only Range | walk is 27 to 41 percent of the request | the rest is `keyIndex.get` per key |
| etcd, LIST with a limit served from etcd | every kube-apiserver LIST with a limit asks for the total, so etcd walks every key in the range under the index read lock, about 110 ms per 1M keys | blocks the apply path |

The etcd case is the one Marek's stalled experiment [etcd-io/etcd#18184](https://github.com/etcd-io/etcd/pull/18184) tried to remove, and the read lock it holds is on the write path that [KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput) is about.

### What the branch already does for these paths

With the branch, etcd can replace its callback end-check in `unsafeVisit` with `AscendRange`. The walk over 1M keys drops from 35 ms to 5.8 ms, because the old callback dereferenced every `keyIndex` to compare it with the end key, and the branch tests the end once per node. On the current fork the same switch is slower, so the fork change must land first. The etcd client cache gets 35 to 42 percent lower list latency with no code change.

### Prototyped fork additions, measured on a scratch copy

| Candidate | Size | Effect | Verdict |
|---|---|---|---|
| `CountRange(ge, lt)` with a subtree count per node | +45 −4 lines, plus 24 | count of a 200-item namespace at 1M keys 6.06 µs to 0.76 µs; whole tree 3.27 ms to 0.24 µs; writes 1 to 2 percent slower; node stays in the same size class | keep |
| `NewFromSorted` bulk build | 55 lines | 100k build 12 ms to 0.84 ms, 1M 145 ms to 8.6 ms, half the live memory, same lookup speed | keep |
| `GetOrInsert` | +27 −6 lines | new key 29 percent faster at 100k; not for the watch cache, which clones after every write | keep, for etcd |
| probe-free lookup | 35 lines | 6 percent at 100k, nothing at 1M | defer |
| Clone allocation trimming | +7 −3 lines | 5 ns per Clone; a one-block variant kept 344 MB alive | drop |

All three kept candidates passed differential fuzzing with shape invariants checked after every operation.

### Consumer changes the data supports

Kubernetes watch cache: answer `resp.Count()` with `CountRange` or with `Len` when the tree holds one prefix, which removes 56 µs per page at 10k objects and 625 µs at 100k; build the Replace tree with `NewFromSorted`, which cuts a 100k relist under the write lock from 11 ms to under 1 ms; pool the probe element to save one allocation per GET.

etcd server: switch `unsafeVisit` to `AscendRange` once the fork lands; add a fast path in `keyIndex.get` for the current revision, 38 ms to 22 ms per 1M keys; fold the extra Get into `index.Put`, one descent and one allocation less per Put; build the restore index through a map in key order, 2.06 s to 0.94 s per 1M keys; later, a live flag per item and a weighted `CountRange` to make count-only ranges O(log n) without the long read lock.

etcd client cache: build the Restore tree before taking the write lock, which removes a 12 to 137 ms reader stall per reconnect; presize the Range result, which cuts 4.5 MB of garbage per 100k-key list to 0.8 MB.

## Update, 2026-10-25: existing consumer benchmarks, and what they show for this change

The question was whether benchmarks that already exist in Kubernetes and etcd, the ones Marek and the etcd authors used to show their own improvements, would show this change. The inventory is below, then the measured results on both machines.

### Inventory

| Benchmark | Where | What it measures | Who used it | Shows this change? |
|---|---|---|---|---|
| `BenchmarkStoreList` via `RunBenchmarkStoreList` | k8s `pkg/storage/cacher/cacher_test.go`, `pkg/storage/testing/store_benchmarks.go` | lists from the cacher against an embedded etcd with 150k pods: RV empty, exact and not-older-than; cluster, node and namespace scope; with and without a 1000-item page; reports time per list, lists per second and objects per second | Marek introduced it in [kubernetes#125884](https://github.com/kubernetes/kubernetes/pull/125884) and used it to justify the btree store in [kubernetes#126754](https://github.com/kubernetes/kubernetes/pull/126754), with a 25 percent gain in operations per second | yes, for paginated lists, see below |
| `BenchmarkStoreWriteThroughput` via `RunBenchmarkWriteThroughput` | same files | writes with background lists, watch lists and watchers; reports writes per second, list objects per second and watch latency | Marek, [kubernetes#139440](https://github.com/kubernetes/kubernetes/pull/139440) and follow-ups, for [KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput) | no, the tree is below one percent of a write |
| `BenchmarkCacher_GetList` | k8s `cacher_whitebox_test.go` | a field-selector list over 5000 pods from a mock store | long-standing cacher benchmark | no, predicate evaluation dominates |
| `BenchmarkRequestCost` | k8s `test/integration/apiserver/flowcontrol/cost/cost_test.go` | bytes allocated per request type against a real apiserver, exported to perfdash | Marek, [kubernetes#142872](https://github.com/kubernetes/kubernetes/pull/142872), [#142934](https://github.com/kubernetes/kubernetes/pull/142934), [#142945](https://github.com/kubernetes/kubernetes/pull/142945), for [KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput) | no, it measures allocations and the tree adds one 48 byte probe per read |
| `BenchmarkPatchPod` and friends | k8s `test/integration/apiserver/storage/pod_write_benchmark_test.go` | allocations of pod writes through the apiserver | [KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput) work | no |
| clusterloader2 write-throughput and list scenarios | kubernetes/perf-tests, PRs [#4253](https://github.com/kubernetes/perf-tests/pull/4253), [#4255](https://github.com/kubernetes/perf-tests/pull/4255), [#4378](https://github.com/kubernetes/perf-tests/pull/4378), [#4391](https://github.com/kubernetes/perf-tests/pull/4391), [#4508](https://github.com/kubernetes/perf-tests/pull/4508) | cluster-level throughput and latency with 10 KB pods | Marek, for [KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput); [KEP-2340](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/2340-Consistent-reads-from-cache), [KEP-4988](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/4988-snapshottable-api-server-cache) and [KEP-5116](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/5116-streaming-response-encoding) were validated with scalability jobs on testgrid | not at tree granularity |
| `BenchmarkRange` | etcd `server/etcdserver/txn/range_bench_test.go` | a Range through the txn layer with `withTotalCount=true`, 100 to 10k keys, with and without a limit of 10 and sorting | Jeff Tree, [etcd#21618](https://github.com/etcd-io/etcd/pull/21618) and [#21635](https://github.com/etcd-io/etcd/pull/21635), for the `WithTotalCount` optimization | yes, for the limit cases, see below |
| `BenchmarkRangeFastKeysOnlyWithLimit` | same file | keys-only range with a limit | nwnt, [etcd#21791](https://github.com/etcd-io/etcd/pull/21791) and [#21834](https://github.com/etcd-io/etcd/pull/21834) | would, once etcd uses `AscendRange` |
| `BenchmarkIndexGet`, `BenchmarkIndexPut`, `BenchmarkIndexCompact*` | etcd `server/storage/mvcc/index_bench_test.go` | single index operations | long-standing | no, lookups are unchanged by design |
| `BenchmarkStoreRangeKey1`, `BenchmarkStoreRangeKey100`, `BenchmarkStoreRestore*` | etcd `server/storage/mvcc/kvstore_bench_test.go` | ranges with values from bbolt; restore of the index | long-standing | no |
| `benchmark range --count-only --keys-only --paginate --stream` | etcd `tools/benchmark` | end-to-end ranges against a running cluster | the keys-only and RangeStream work | would, for count-only and keys-only |
| `Index*` benchmarks in [etcd#18184](https://github.com/etcd-io/etcd/pull/18184) | not merged | counts, ranges and revisions over the index | Marek's 2024 index experiment | would, but they are not in the tree |
| `BenchmarkStorageRead/Write/Watch/Size` in [etcd#22333](https://github.com/etcd-io/etcd/pull/22333) | not merged | bbolt against pebble and badger, with `RangeLimit100` and `CountOnly` cases | Marek, for issue [etcd#22332](https://github.com/etcd-io/etcd/issues/22332) | the count-only case would |

### Results on this machine, Apple M4 Pro, macOS, arm64, Go 1.27.1

etcd `BenchmarkRange`, 10k keys, four repetitions per state. "Fork" is the branch alone. "Fork + AscendRange" adds the one-function change in etcd's `unsafeVisit` that the branch makes worthwhile.

| Case | Pinned fork | Fork | Fork + AscendRange |
|---|---|---|---|
| no limit, all 10k values from bbolt | 3.64 ms | 3.62 ms | 3.68 ms |
| limit 10, total counted over 10k keys | 99.6 µs | 94.5 µs, −5% | 85.9 µs, −14% |
| limit 10 sorted by key | 101.8 µs | 94.9 µs, −7% | 87.1 µs, −14% |

Kubernetes `BenchmarkCacher_GetList`: no change in time or allocations in any of its five cases.

Kubernetes `BenchmarkStoreList`, 150k pods in 50 namespaces, no index, pages of 1000, six repetitions pooled from two processes per state. This benchmark varies 15 to 30 percent between repetitions because it runs an embedded etcd and four parallel clients, so only the rows with a p value below 0.05 are conclusions.

| Case | Before | After | Change |
|---|---|---|---|
| RV empty, cluster scope | 74.4 ms per page | 54.4 ms | −27%, p=0.002 |
| RV empty, namespace scope | 754 µs | 612 µs | −19%, p=0.041 |
| RV exact, cluster scope | 55.6 ms | 53.7 ms | within noise |
| RV exact, namespace scope | 766 µs | 680 µs | within noise |
| listed objects per second, RV empty, cluster scope | 2.02 M | 2.76 M | +37%, p=0.002 |

The cluster-scope page over 150k pods is the count walk case: each page counts the whole remaining collection before it fills 1000 items.

### Results on ubuntu-box, Intel Core Ultra 9 285K, Ubuntu, amd64, Go 1.27.2

Same trees, same scripts, same filters. The etcd rows pool two interleaved rounds of four repetitions for the first two columns; the third column is one round of four against its own baseline of 96.3 µs. The Kubernetes rows pool two separate processes per state, eight repetitions each.

etcd `BenchmarkRange`, 10k keys.

| Case | Pinned fork | Fork | Fork + AscendRange |
|---|---|---|---|
| no limit, all 10k values from bbolt | 5.52 ms | 5.51 ms, within noise | 5.54 ms, within noise |
| limit 10, total counted over 10k keys | 97.5 µs | 91.1 µs, −6.6%, p=0.001 | 82.2 µs, −14.6%, p=0.029 |
| limit 10 sorted by key | 100.0 µs | 92.7 µs, within noise | 85.7 µs, −14.4%, p=0.029 |

Kubernetes `BenchmarkCacher_GetList`: within noise in all five cases, geomean −2 percent.

Kubernetes `BenchmarkStoreList`, 150k pods, pages of 1000.

| Case | Before | After | Change |
|---|---|---|---|
| RV empty, cluster scope | 45.9 ms per traversal | 46.9 ms | within noise |
| RV empty, namespace scope | 488 µs | 476 µs | −2.4%, p=0.050 |
| RV exact, cluster scope | 41.2 ms | 40.8 ms | within noise |
| RV exact, namespace scope | 477 µs | 473 µs | within noise |

### Reading the two machines together

One `StoreList` operation is a full paginated traversal: the benchmark's `paginateList` follows continue tokens until the collection is exhausted, so a cluster-scope operation over 150k pods is 150 pages, and each page counts everything after its continue key before it fills 1000 items. That is about 11 million tree visits per operation, plus the copy of 150k pod structs into responses, which is the 180 MB allocated per operation that the benchmark reports.

On the M4 the tree visit is the larger of the two costs, so a faster walk shows as 19 to 27 percent. On the Intel machine the walk was already cheap per visit, 3 to 4 ns against 7 to 11 on the M4, and the allocation and copying of the pages dominate, so the same change disappears into noise. The etcd limit case moves on both machines because its per-request work is the count walk and little else.

The conclusion for the work plan does not change, and the amd64 numbers sharpen it. A faster walk helps etcd's count and keys-only paths on every CPU, and helps Kubernetes list pagination only on CPUs with slow pointer chasing. Removing the count walk, with an O(log n) range count in the tree, is what would move the paginated list path on amd64, because it removes the 11 million visits instead of making each one cheaper.

### How to reproduce these runs

The scripts and the raw outputs for both machines are under [`research/btree/benchmarks`](https://github.com/dims/utils/tree/btree-research/research/btree/benchmarks) on the `btree-research` branch, and the etcd change is commit [44d03d5bc](https://github.com/dims/etcd/commit/44d03d5bc) on branch [`btree-ascendrange`](https://github.com/dims/etcd/tree/btree-ascendrange) of github.com/dims/etcd. The method is: build the consumer's own benchmark against the vendored fork for the baseline, copy the branch's `btree.go` over the vendored copy or point a scratch `go.work` at the branch for the after run, restore, and compare with `benchstat`. For etcd the `AscendRange` state additionally replaces the body of `treeIndex.unsafeVisit` with `AscendRange(keyi, endi, f)` when an end key is given.

## Recommendations: what to do next

### Why this list, and why in this order

The measurements narrow the question of what the tree can do for its consumers to two mechanisms. The first is counting. On every path we measured, the tree's cost per request is small and fixed, except where a request counts keys, and there the cost grows with the size of the collection: a paginated LIST served from the watch cache counts the rest of the collection on every page, and a LIST with a limit that reaches etcd makes etcd walk every key in the range. The second is lock hold time. The watch cache rebuilds its tree under its write lock on every relist, etcd's index holds its read lock for the whole of a count walk and blocks the apply path, and the etcd client cache rebuilds under its write lock on every reconnect. Everything below either removes a count walk, shortens a lock hold, or makes one of those two changes possible.

The order is dictated by dependencies. The fork change has to land first, because etcd's `AscendRange` switch is slower on the current fork. The new fork APIs have to exist before the consumers can use them. etcd pins the same k8s.io/utils module as Kubernetes, so every fork change must be additive, and the consumer changes are listed by the project that owns them. Within each group, items are ordered by measured gain divided by risk.

### 1. Merge the branch into [kubernetes/utils](https://github.com/kubernetes/utils)

What: the four commits as they stand, benchmarks in an external test package, the fuzz test, and the iteration change.

Why: it is the smallest change in this document with a measured gain, it changes no behavior, and the fuzz test and the fork's upstream tests pass with the race detector. It is also a prerequisite: etcd's count-only and keys-only paths get 5 to 7 percent from it at once on both CPUs, 14 percent once etcd switches to `AscendRange`, and that switch is slower without it. The etcd client cache gets 35 to 42 percent lower list latency from the module bump alone. Risk is low.

### 2. A second fork PR with three additions

What: `CountRange(ge, lt)`, `NewFromSorted(degree, less, sorted)` and `GetOrInsert(item)`, each prototyped and fuzzed with the tree's shape invariants checked after every operation. The prototypes and their measurements are under [`research/btree/prototypes`](https://github.com/dims/utils/tree/btree-research/research/btree/prototypes).

Why `CountRange`: counting is the one tree cost that grows with the collection. A faster walk only helps where a visit is expensive, which is why the Kubernetes paginated list got 19 to 27 percent faster on the Apple M4 and nothing on the Intel machine. A count kept as a subtree size per node answers in O(log n) and removes the walk itself, about 11 million visits for a 150-page traversal of 150k pods, on every CPU. The prototype costs 1 to 2 percent on writes and nothing in memory, which matters because the watch cache clones after every write and could not afford a heavier write path. About 70 lines.

Why `NewFromSorted`: relist and restore are the recovery paths, and [KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput) names relist as the start of a sticky degradation. Today they insert an already sorted list one item at a time under a write lock, and sorted insertion also leaves every node half full. The bulk build is 14 to 17 times faster, uses half the memory, and checks its input so that unsorted input falls back to the old path. About 55 lines.

Why `GetOrInsert`: etcd's Put descends the index two or three times per write, with a probe allocation each time, on the serial apply path. One descent is the direct fix, measured at 29 percent for new keys. It must not be used as a lookup where a Clone follows every write, because a found key on a freshly cloned tree still copies the path, so it is documented as etcd's. About 30 lines.

Why additive: etcd and Kubernetes pin the same module, so new functions beside the old ones keep both compiling and let each adopt them on its own schedule.

### 3. etcd server, in `server/storage/mvcc`

What, after bumping k8s.io/utils to a version with item 1: `AscendRange` in `unsafeVisit`, a fast path in `keyIndex.get` for the current revision, one descent in `index.Put`, restore through a map in key order, batched locks in `Compact`, and later a live flag per item with a weighted count.

Why `AscendRange`: the callback end check dereferences every `keyIndex` to compare it with the end key, three dependent cache misses per key, and that is what made the walk cost 35 ms per 1M keys. The branch tests the end once per node, so the walk drops to 5.8 ms, and a limited range on etcd's own `BenchmarkRange` gets 14 percent faster on both CPUs. One function, commit [44d03d5bc](https://github.com/dims/etcd/commit/44d03d5bc) on github.com/dims/etcd.

Why the `keyIndex.get` fast path: counts and keys-only ranges call `get` for every key, and at the current revision the answer is already in `ki.modified`; the generation walk it does today is wasted work. Measured at 38 ms to 22 ms per 1M keys. Add a tombstone case to its tests.

Why one descent in Put: the apply path is serial under the index lock, so every microsecond there is write throughput. Folding the extra Get into `index.Put` saves a descent and an allocation per write; `GetOrInsert` saves another descent for new keys.

Why restore through a map: restore inserts in revision order through a 10k-entry cache that misses on every key of a large store, so each key costs a Get and an insert under separate locks. Sorted inserts are nine times cheaper than random ones. Index build per 1M keys drops from 2.06 s to 0.94 s for about 50 MB of transient memory, and that is startup time on large clusters.

Why batched compaction locks: per-key lock and unlock is about 40 ms per 1M keys of a compaction that runs under production load, and batching 64 keys per acquisition is a few lines. About 13 percent of compaction time, an estimate.

Why the live flag and weighted count last: a LIST with a limit over 1M keys holds the index read lock for about 110 ms today, which blocks writes. If each item carries whether its latest generation is live, a count at the current revision becomes a tree count in O(log n) and the lock is held for microseconds. It needs `CountRange`, a decision on the item layout, and a rule to fall back to the walk for counts at a past revision, so it comes after the rest.

### 4. Kubernetes watch cache, in `staging/src/k8s.io/apiserver/pkg/storage/cacher`

What: `resp.Count()` through `CountRange`, `Replace` through `NewFromSorted`, a pooled probe element, the cyclic-buffer snapshotter noted in `snapshotter.go`, and removal of `btreeStore.OrderedListPrefix`.

Why `CountRange` for `Count`: every predicate-free LIST counts the whole remaining prefix before it fills the page, so an unpaginated list walks twice and a paginated list counts quadratically. On the Intel machine nothing else moved Marek's `StoreList` benchmark, and this is the change that removes the walks rather than speeding them up. Until `CountRange` lands, `Len` already answers the count when the tree holds a single prefix and there is no continue key.

Why `NewFromSorted` for `Replace`: relist is the recovery path after a watch failure, readers stall for the whole rebuild under the write lock, and the input arrives from etcd already in key order. A 100k relist drops from 11 ms to under 1 ms.

Why the probe pool: [KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput) counts allocations per request and the APF cost model will price them. Each GET and each prefix walk allocates one 48 byte probe with pointer fields the garbage collector has to scan, and a pool removes it at no risk.

Why the snapshotter and the dead method: the maintainers already noted the cyclic buffer as a TODO; it removes a second tree and one to three allocations per write event. The unused list method is review burden with no caller.

### 5. etcd client cache, in `cache/`

What: build the `Restore` tree before taking the write lock, presize the `Range` result, and bump k8s.io/utils.

Why: on every reconnect readers stall for 12 to 137 ms while the tree is rebuilt under the lock, and the fix is local and needs no tree change. A 100k-key list allocates 4.5 MB of garbage for 0.8 MB of results because the result slice grows by append, and `tree.Len()` already gives the size when the request covers the prefix. The bump brings the iteration change, measured in situ at 35 to 42 percent lower list latency.

### 6. How to measure each step

Why both architectures: the tree's cost per visit differs two to three times between the Apple M4 and the Intel machine, and that difference reversed the Kubernetes list conclusion. A number from one machine is not a result.

Why external test packages for the fork: measured from inside the package, the compiler inlines `sort.Search` and its closure and hides about 30 percent of the lookup cost that cross-package callers pay.

Why pooled repetitions for `StoreList`: the embedded etcd and four parallel clients give 15 to 30 percent variance between repetitions, so single runs can show a gain or a loss that is not there. Pool at least six.

Why etcd's `BenchmarkRange` with a limit and the benchmarks from PR [#18184](https://github.com/etcd-io/etcd/pull/18184): the limit case carries `withTotalCount=true`, which is the count walk, and the `IndexCountRevisions`, `IndexRange` and `IndexRevisions` benchmarks are the index-only shapes the count work targets. They are not in the tree today.

### 7. Not recommended

- Replacing the fork with bbtree or a port of it, because all three consumers depend on the O(1) copy-on-write Clone that bbtree does not have.
- The hand-rolled binary search in `find`, because it gained 31 percent on lookups on Apple silicon and nothing on Intel; it stays on file for an arm64 server measurement.
- A probe-free lookup API, which gained 6 percent at 100k keys and nothing at 1M, and Clone allocation trimming, which saved 5 ns and in one variant kept 344 MB alive.
- Storing the key beside the pointer in the watch cache's item type. It makes lookups 36 to 44 percent faster but doubles the cloned path per write, and the watch cache clones on every write. It remains reasonable for etcd's index, which clones only at compaction, once the live-flag work settles the item layout.

### Ownership and sequencing

The fork changes go to kubernetes/utils first, as two pull requests. The etcd changes go to etcd-io/etcd after a module bump, with the `AscendRange` switch in the same change as the bump so that no commit is slower than its parent. The watch cache changes go to kubernetes/kubernetes under SIG API Machinery, where the paginated count and the relist build fit the [KEP-6404](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/6404-apiserver-write-throughput) work. The client cache changes go to the etcd `cache/` owners and do not depend on anything else.
