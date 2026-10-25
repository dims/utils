#!/bin/bash
# On ubuntu-box: Kubernetes' cacher benchmarks with the vendored fork
# (before) and with the branch's btree.go copied into vendor (after).
set -uo pipefail
export PATH=$PATH:/usr/local/go/bin
R=$HOME/btree-exp
K=$R/kubernetes
V=$K/vendor/k8s.io/utils/third_party/forked/golang/btree/btree.go
NEW=$R/utils-branch/third_party/forked/golang/btree/btree.go
cp "$V" "$R/btree.go.vendor.orig" || exit 1
restore() { cp "$R/btree.go.vendor.orig" "$V"; cmp -s "$V" "$R/btree.go.vendor.orig" && echo "vendor restored" || echo "VENDOR NOT RESTORED"; }
trap restore EXIT
PKG=./staging/src/k8s.io/apiserver/pkg/storage/cacher/
GETLIST='^BenchmarkCacher_GetList$'
STORELIST='^BenchmarkStoreList$/Namespaces=50/Pods=150000/Nodes=5000/Indexed=false/RV=(Exact|)$/Scope=(Cluster|Namespace)$/Paginate=1000$'
cd "$K" || exit 1
echo "=== k8s before: GetList ==="
go test -run '^$' -bench "$GETLIST" -benchmem -benchtime=1s -count=4 $PKG 2>&1 | grep -E '^Benchmark|^ok|FAIL' > "$R/k8s_getlist_before.txt"; tail -1 "$R/k8s_getlist_before.txt"
echo "=== k8s before: StoreList ==="
go test -run '^$' -bench "$STORELIST" -benchmem -benchtime=2s -count=4 -timeout 60m $PKG 2>&1 | grep -E '^Benchmark|^ok|FAIL|panic' > "$R/k8s_storelist_before.txt"; tail -1 "$R/k8s_storelist_before.txt"
cp "$NEW" "$V" && echo "vendor now has the branch btree.go"
echo "=== k8s after: GetList ==="
go test -run '^$' -bench "$GETLIST" -benchmem -benchtime=1s -count=4 $PKG 2>&1 | grep -E '^Benchmark|^ok|FAIL' > "$R/k8s_getlist_after.txt"; tail -1 "$R/k8s_getlist_after.txt"
echo "=== k8s after: StoreList ==="
go test -run '^$' -bench "$STORELIST" -benchmem -benchtime=2s -count=4 -timeout 60m $PKG 2>&1 | grep -E '^Benchmark|^ok|FAIL|panic' > "$R/k8s_storelist_after.txt"; tail -1 "$R/k8s_storelist_after.txt"
restore
trap - EXIT
