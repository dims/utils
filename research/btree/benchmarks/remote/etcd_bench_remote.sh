#!/bin/bash
# On ubuntu-box: etcd's BenchmarkRange (txn level, withTotalCount=true) in
# three states: pinned fork, branch fork via a go.work replace, branch fork
# plus AscendRange in unsafeVisit (temporary, restored at the end).
set -uo pipefail
export PATH=$PATH:/usr/local/go/bin
R=$HOME/btree-exp
ETCD=$R/etcd
IDX=$ETCD/server/storage/mvcc/index.go
GW=$R/gowork-branch/go.work
mkdir -p "$R/gowork-branch"
{
  echo "go 1.27"
  echo
  echo "use ("
  for m in . api cache client/pkg client/v3 etcdctl etcdutl pkg server tests tests/antithesis/pkg tools/mod tools/rw-heatmaps tools/testgrid-analysis; do
    echo "	$ETCD/$m"
  done
  echo ")"
  echo
  echo "replace k8s.io/utils => $R/utils-branch"
} > "$GW"
cp "$IDX" "$R/index.go.orig" || exit 1
restore() { cp "$R/index.go.orig" "$IDX"; cmp -s "$IDX" "$R/index.go.orig" && echo "index.go restored" || echo "INDEX.GO NOT RESTORED"; }
trap restore EXIT
BENCH='^BenchmarkRange$/keys_10000/(no_sort_no_limit|no_sort_limit_10|key_ascend_limit_10)$'
cd "$ETCD/server" || exit 1
echo "=== etcd base ==="
go test -run '^$' -bench "$BENCH" -benchmem -benchtime=1s -count=4 ./etcdserver/txn/ 2>&1 | grep -E '^Benchmark|^ok|FAIL|panic' > "$R/etcd_range_base.txt"; tail -1 "$R/etcd_range_base.txt"
echo "=== etcd fork only ==="
GOWORK=$GW go test -run '^$' -bench "$BENCH" -benchmem -benchtime=1s -count=4 ./etcdserver/txn/ 2>&1 | grep -E '^Benchmark|^ok|FAIL|panic' > "$R/etcd_range_fork.txt"; tail -1 "$R/etcd_range_fork.txt"
python3 -I "$R/patch_unsafevisit.py" "$IDX" || exit 1
echo "=== etcd fork + AscendRange ==="
GOWORK=$GW go test -run '^$' -bench "$BENCH" -benchmem -benchtime=1s -count=4 ./etcdserver/txn/ 2>&1 | grep -E '^Benchmark|^ok|FAIL|panic' > "$R/etcd_range_fork_ar.txt"; tail -1 "$R/etcd_range_fork_ar.txt"
echo "=== mvcc tests with patched unsafeVisit ==="
GOWORK=$GW go test -count=1 -run 'TestIndex|TestStoreRange|TestKV' ./storage/mvcc/ 2>&1 | tail -1
restore
trap - EXIT
