#!/opt/homebrew/bin/bash
# Ships the etcd and Kubernetes trees, the branch, and the scripts to
# ubuntu-box, runs both benchmark suites there one after the other, and
# brings the results back.
set -uo pipefail
E=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp
SRC=/Users/dsrinivas/go/src
echo "=== rsync etcd, utils branch, scripts ==="
rsync -a --delete --exclude=.git "$SRC/github.com/etcd-io/etcd/" ubuntu-box:btree-exp/etcd/ && echo "etcd shipped"
rsync -a --delete --exclude=.git "$SRC/k8s.io/utils/.claude/worktrees/btree-benchmarks/" ubuntu-box:btree-exp/utils-branch/ && echo "utils branch shipped"
rsync -a "$E/remote/etcd_bench_remote.sh" "$E/remote/k8s_bench_remote.sh" "$E/patch_unsafevisit.py" ubuntu-box:btree-exp/ && echo "scripts shipped"
echo "=== rsync kubernetes (346 MB, no .git, no _output) ==="
rsync -a --delete --exclude=.git --exclude=_output "$SRC/k8s.io/kubernetes/" ubuntu-box:btree-exp/kubernetes/ && echo "kubernetes shipped"
echo "=== run etcd suite on ubuntu-box ==="
ssh ubuntu-box 'bash btree-exp/etcd_bench_remote.sh' 2>&1 | tail -12
echo "=== run kubernetes suite on ubuntu-box ==="
ssh ubuntu-box 'bash btree-exp/k8s_bench_remote.sh' 2>&1 | tail -12
echo "=== fetch results ==="
mkdir -p "$E/ubuntu"
for f in etcd_range_base etcd_range_fork etcd_range_fork_ar k8s_getlist_before k8s_getlist_after k8s_storelist_before k8s_storelist_after; do
  ssh ubuntu-box "cat btree-exp/$f.txt" > "$E/ubuntu/$f.txt" 2>/dev/null
done
echo "=== ubuntu-box etcd: base vs fork vs fork+AscendRange ==="
benchstat "$E/ubuntu/etcd_range_base.txt" "$E/ubuntu/etcd_range_fork.txt" "$E/ubuntu/etcd_range_fork_ar.txt" 2>&1 | grep -E 'Range|geomean' | head -5
echo "=== ubuntu-box k8s GetList ==="
benchstat "$E/ubuntu/k8s_getlist_before.txt" "$E/ubuntu/k8s_getlist_after.txt" 2>&1 | grep -E 'GetList|geomean' | head -7
echo "=== ubuntu-box k8s StoreList ==="
benchstat "$E/ubuntu/k8s_storelist_before.txt" "$E/ubuntu/k8s_storelist_after.txt" 2>&1 | grep -E 'StoreList|geomean' | head -6
