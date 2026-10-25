#!/opt/homebrew/bin/bash
# Runs Kubernetes' own cacher benchmarks against the vendored fork (before)
# and against the branch's btree.go copied into vendor (after), then
# restores vendor and compares. The vendored copy is byte-identical to the
# fork's master, so restoring from the fork's master checkout is exact.
set -uo pipefail
E=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp
K=/Users/dsrinivas/go/src/k8s.io/kubernetes
V=$K/vendor/k8s.io/utils/third_party/forked/golang/btree/btree.go
ORIG=/Users/dsrinivas/go/src/k8s.io/utils/third_party/forked/golang/btree/btree.go
NEW=/Users/dsrinivas/go/src/k8s.io/utils/.claude/worktrees/btree-benchmarks/third_party/forked/golang/btree/btree.go
cmp -s "$V" "$ORIG" || { echo "vendor copy differs from fork master, aborting"; exit 1; }
restore() { cp "$ORIG" "$V"; cmp -s "$V" "$ORIG" && echo "vendor restored" || echo "VENDOR NOT RESTORED"; }
trap restore EXIT
PKG=./staging/src/k8s.io/apiserver/pkg/storage/cacher/
GETLIST='^BenchmarkCacher_GetList$'
STORELIST='^BenchmarkStoreList$/Namespaces=50/Pods=150000/Nodes=5000/Indexed=false/RV=(Exact|)$/Scope=(Cluster|Namespace)$/Paginate=1000$'
cd "$K" || exit 1
echo "=== before: StoreList (embedded etcd setup, slow) ==="
go test -run '^$' -bench "$STORELIST" -benchmem -benchtime=2s -count=4 -timeout 40m $PKG 2>&1 | grep -E '^Benchmark|^ok|FAIL|panic' > "$E/k8s_storelist_before.txt"; tail -1 "$E/k8s_storelist_before.txt"
cp "$NEW" "$V" && echo "vendor now has the branch btree.go"
echo "=== after: StoreList ==="
go test -run '^$' -bench "$STORELIST" -benchmem -benchtime=2s -count=4 -timeout 40m $PKG 2>&1 | grep -E '^Benchmark|^ok|FAIL|panic' > "$E/k8s_storelist_after.txt"; tail -1 "$E/k8s_storelist_after.txt"
restore
trap - EXIT
echo "=== benchstat GetList ==="
benchstat "$E/k8s_getlist_before.txt" "$E/k8s_getlist_after.txt" 2>&1 | grep -E 'GetList|geomean' | head -14
echo "=== benchstat StoreList ==="
benchstat "$E/k8s_storelist_before.txt" "$E/k8s_storelist_after.txt" 2>&1 | grep -E 'StoreList|geomean' | head -20
