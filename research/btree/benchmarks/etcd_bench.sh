#!/opt/homebrew/bin/bash
# etcd's own BenchmarkRange (txn level, withTotalCount=true) in three states:
#   base      pinned k8s.io/utils, etcd as is
#   fork      branch btree.go through a scratch go.work, etcd as is
#   fork+ar   branch btree.go, and unsafeVisit using AscendRange (temporary)
# index.go is restored from a saved copy at the end, whatever happens.
set -uo pipefail
E=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp
ETCD=/Users/dsrinivas/go/src/github.com/etcd-io/etcd
IDX=$ETCD/server/storage/mvcc/index.go
GW=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/gowork-branch/go.work
cp "$IDX" "$E/index.go.orig" || exit 1
restore() { cp "$E/index.go.orig" "$IDX"; cmp -s "$IDX" "$E/index.go.orig" && echo "index.go restored" || echo "INDEX.GO NOT RESTORED"; }
trap restore EXIT
BENCH='^BenchmarkRange$/keys_10000/(no_sort_no_limit|no_sort_limit_10|key_ascend_limit_10)$'
cd "$ETCD/server" || exit 1
echo "=== base ==="
go test -run '^$' -bench "$BENCH" -benchmem -benchtime=1s -count=4 ./etcdserver/txn/ 2>&1 | grep -E '^Benchmark|^ok|FAIL|panic' > "$E/etcd_range_base.txt"; tail -1 "$E/etcd_range_base.txt"
echo "=== fork only ==="
GOWORK=$GW go test -run '^$' -bench "$BENCH" -benchmem -benchtime=1s -count=4 ./etcdserver/txn/ 2>&1 | grep -E '^Benchmark|^ok|FAIL|panic' > "$E/etcd_range_fork.txt"; tail -1 "$E/etcd_range_fork.txt"
python3 -I "$E/patch_unsafevisit.py" "$IDX" || exit 1
echo "=== fork + AscendRange ==="
GOWORK=$GW go test -run '^$' -bench "$BENCH" -benchmem -benchtime=1s -count=4 ./etcdserver/txn/ 2>&1 | grep -E '^Benchmark|^ok|FAIL|panic' > "$E/etcd_range_fork_ar.txt"; tail -1 "$E/etcd_range_fork_ar.txt"
echo "=== mvcc tests with the patched unsafeVisit ==="
GOWORK=$GW go test -count=1 -run 'TestIndex|TestStoreRange|TestKV' ./storage/mvcc/ 2>&1 | tail -1
restore
trap - EXIT
echo "=== benchstat: base vs fork vs fork+AscendRange ==="
benchstat "$E/etcd_range_base.txt" "$E/etcd_range_fork.txt" "$E/etcd_range_fork_ar.txt" 2>&1 | grep -E 'Range|geomean' | head -12
