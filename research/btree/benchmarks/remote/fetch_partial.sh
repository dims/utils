#!/opt/homebrew/bin/bash
# Fetches the finished ubuntu-box result files and compares the pairs that
# are complete.
set -uo pipefail
U=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp/ubuntu
mkdir -p "$U"
for f in etcd_range_base etcd_range_fork etcd_range_fork_ar k8s_getlist_before k8s_getlist_after k8s_storelist_before k8s_storelist_after; do
  ssh -o BatchMode=yes ubuntu-box "cat btree-exp/$f.txt 2>/dev/null" > "$U/$f.txt"
done
echo "=== ubuntu-box etcd BenchmarkRange: base vs fork vs fork+AscendRange ==="
benchstat "$U/etcd_range_base.txt" "$U/etcd_range_fork.txt" "$U/etcd_range_fork_ar.txt" 2>&1 | grep -E 'Range|geomean' | sed -E 's/ +/ /g' | head -5
echo "=== ubuntu-box k8s GetList ==="
benchstat "$U/k8s_getlist_before.txt" "$U/k8s_getlist_after.txt" 2>&1 | grep -E 'GetList|geomean' | sed -E 's/ +/ /g' | head -6
echo "=== ubuntu-box StoreList before, cluster rows ==="
grep Cluster "$U/k8s_storelist_before.txt" | sed -E 's/BenchmarkStoreList\/Namespaces=50\/Pods=150000\/Nodes=5000\/Indexed=false\///; s/ +/ /g' | awk '{print $1, $3, $4}'
if [ -s "$U/k8s_storelist_after.txt" ]; then
  echo "=== ubuntu-box StoreList before vs after ==="
  benchstat "$U/k8s_storelist_before.txt" "$U/k8s_storelist_after.txt" 2>&1 | grep -E 'StoreList|geomean' | sed -E 's/StoreList\/Namespaces=50\/Pods=150000\/Nodes=5000\/Indexed=false\///; s/ +/ /g' | head -12
else
  echo "StoreList after: not finished yet"
fi
