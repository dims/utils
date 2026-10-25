# Research material for the forked B-tree speedup

This branch holds the material behind `btree-fork-speedup.md`, the write-up on the forked B-tree in `third_party/forked/golang/btree`, its consumers in Kubernetes and etcd, and the iteration change on the `btree-benchmarks` branch of this fork. The write-up is copied here as `btree-fork-speedup.md`. Nothing on this branch is meant to be merged.

## Contents

- `reports/`: the five audit reports and their raw benchmark outputs.
  - `k8s-cacher/`: how the apiserver watch cache uses the tree, call sites by request path, measured costs, and the tree's share on the paths KEP-6404 makes hot.
  - `etcd-index/`: the same for etcd's treeIndex, with measurements of count-only and keys-only ranges on the pinned and the modified fork.
  - `etcd-cache/`: the same for etcd's experimental client cache.
  - `marek-program/`: a map of Marek Siarkowicz's performance program and where the tree sits on each item, with the GitHub material it was built from under `raw/`.
- `prototypes/`: scratch copies of the fork with the candidate additions, each in its own package with tests and a fuzz target: `bulk/` for `NewFromSorted`, `counts/` for subtree counts and `CountRange`, `getorinsert/` for `GetOrInsert`, `find/` for the probe-free lookup, `clone/` for the Clone allocation experiment, `baseline/` for the unmodified copy, and the measurements under `results/`.
- `benchmarks/`: the scripts and raw outputs of every run in the write-up. `remote/` holds the scripts that ran on the amd64 machine, `ubuntu/` their results, `exp/`-style files the arm64 results. `patch_unsafevisit.py` is the temporary etcd change used for the `AscendRange` measurements, `patch_iter_only.py` the iteration change as applied to the fork.
- `harness/`: the microbenchmark module used for the primitive-level numbers, with the original fork in `btree/`, the modified copy in `btreeopt/`, the search-loop-only copy in `btreefind/`, the differential fuzz targets, and the consumer-shaped benchmarks.

## Reading the scripts

The scripts carry absolute paths from the machine they ran on, under `/Users/dsrinivas/...` and `/private/tmp/claude-502/...` for the arm64 runs and `/home/dsrinivas/btree-exp/...` for the amd64 runs. Change the path variables at the top of each script to run them elsewhere. The method is described in the write-up under "How to reproduce these runs".

Benchmark outputs are plain `go test -bench` output and can be compared with `benchstat`.
