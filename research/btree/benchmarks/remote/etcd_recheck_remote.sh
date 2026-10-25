#!/bin/bash
# On ubuntu-box: interleaved base, fork, base, fork runs of etcd's
# BenchmarkRange, no index.go change, to separate process-to-process noise
# from the fork's effect.
set -uo pipefail
export PATH=$PATH:/usr/local/go/bin
R=$HOME/btree-exp
GW=$R/gowork-branch/go.work
BENCH='^BenchmarkRange$/keys_10000/(no_sort_no_limit|no_sort_limit_10)$'
cd "$R/etcd/server" || exit 1
for i in 1 2; do
  go test -run '^$' -bench "$BENCH" -benchmem -benchtime=1s -count=4 ./etcdserver/txn/ 2>&1 | grep -E '^Benchmark|^ok|FAIL' > "$R/etcd_recheck_base_$i.txt"; tail -1 "$R/etcd_recheck_base_$i.txt"
  GOWORK=$GW go test -run '^$' -bench "$BENCH" -benchmem -benchtime=1s -count=4 ./etcdserver/txn/ 2>&1 | grep -E '^Benchmark|^ok|FAIL' > "$R/etcd_recheck_fork_$i.txt"; tail -1 "$R/etcd_recheck_fork_$i.txt"
done
