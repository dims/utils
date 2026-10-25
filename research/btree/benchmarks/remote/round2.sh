#!/opt/homebrew/bin/bash
# Second round on ubuntu-box: an interleaved recheck of etcd's BenchmarkRange
# (base, fork, base, fork) and a second Kubernetes StoreList pair, then
# pooled comparisons with round one.
set -uo pipefail
E=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp
U=$E/ubuntu
for f in etcd_range_base etcd_range_fork etcd_range_fork_ar k8s_getlist_before k8s_getlist_after k8s_storelist_before k8s_storelist_after; do
  cp "$U/$f.txt" "$U/${f}_r1.txt"
done
rsync -a "$E/remote/etcd_recheck_remote.sh" ubuntu-box:btree-exp/ && echo "script shipped"
echo "=== etcd interleaved recheck on ubuntu-box ==="
ssh ubuntu-box 'bash btree-exp/etcd_recheck_remote.sh' 2>&1 | tail -8
echo "=== kubernetes second pair on ubuntu-box ==="
ssh ubuntu-box 'bash btree-exp/k8s_bench_remote.sh' 2>&1 | tail -10
for f in k8s_getlist_before k8s_getlist_after k8s_storelist_before k8s_storelist_after; do
  ssh ubuntu-box "cat btree-exp/$f.txt" > "$U/${f}_r2.txt"
done
for s in base fork; do
  for i in 1 2; do ssh ubuntu-box "cat btree-exp/etcd_recheck_${s}_$i.txt" > "$U/etcd_recheck_${s}_$i.txt"; done
done
echo "=== ubuntu-box etcd recheck, pooled n=8: base vs fork ==="
cat "$U/etcd_recheck_base_1.txt" "$U/etcd_recheck_base_2.txt" > "$U/etcd_recheck_base.txt"
cat "$U/etcd_recheck_fork_1.txt" "$U/etcd_recheck_fork_2.txt" > "$U/etcd_recheck_fork.txt"
benchstat "$U/etcd_recheck_base.txt" "$U/etcd_recheck_fork.txt" 2>&1 | grep -E 'Range|geomean' | sed -E 's/ +/ /g' | head -4
echo "=== ubuntu-box StoreList pooled n=8 ==="
cat "$U/k8s_storelist_before_r1.txt" "$U/k8s_storelist_before_r2.txt" | grep '^Benchmark' > "$U/k8s_storelist_before_all.txt"
cat "$U/k8s_storelist_after_r1.txt" "$U/k8s_storelist_after_r2.txt" | grep '^Benchmark' > "$U/k8s_storelist_after_all.txt"
benchstat "$U/k8s_storelist_before_all.txt" "$U/k8s_storelist_after_all.txt" 2>&1 | grep -E 'StoreList|geomean' | sed -E 's/StoreList\/Namespaces=50\/Pods=150000\/Nodes=5000\/Indexed=false\///; s/ +/ /g' | head -12
echo "=== ubuntu-box GetList pooled n=8 ==="
cat "$U/k8s_getlist_before_r1.txt" "$U/k8s_getlist_before_r2.txt" | grep '^Benchmark' > "$U/k8s_getlist_before_all.txt"
cat "$U/k8s_getlist_after_r1.txt" "$U/k8s_getlist_after_r2.txt" | grep '^Benchmark' > "$U/k8s_getlist_after_all.txt"
benchstat "$U/k8s_getlist_before_all.txt" "$U/k8s_getlist_after_all.txt" 2>&1 | grep -E 'GetList|geomean' | sed -E 's/ +/ /g' | head -7
