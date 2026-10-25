#!/opt/homebrew/bin/bash
# Three variants back to back on ubuntu-box: base (unmodified), iter
# (iteration change only), new (iteration and search-loop changes).
set -uo pipefail
E=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp
W=/Users/dsrinivas/go/src/k8s.io/utils/.claude/worktrees/btree-benchmarks
B=$W/third_party/forked/golang/btree
mkdir -p "$E/iter/ext"
cp "$E/base/btree.go" "$E/iter/btree.go"
python3 -I "$E/patch_iter_only.py" "$E/iter/btree.go" || exit 1
gsed -e 's|"k8s.io/utils/third_party/forked/golang/btree"|btree "expiter"|' "$B/btree_bench_test.go" > "$E/iter/ext/bench_test.go"
printf 'module expiter\n\ngo 1.26\n' > "$E/iter/go.mod"
(cd "$E/iter" && go vet ./ext/ && go test -count=1 ./... >/dev/null && echo "iter variant vets") || exit 1
rsync -a --delete "$E/base" "$E/iter" "$E/new" ubuntu-box:btree-exp/ && echo "rsync ok"
ssh ubuntu-box 'export PATH=$PATH:/usr/local/go/bin; cd btree-exp && for v in base iter new; do (cd $v && go test -run "^$" -bench BenchmarkStringKey -benchmem -count=8 ./ext/ > ../attrib_$v.txt 2>&1); done; echo remote done'
for v in base iter new; do
  ssh ubuntu-box "cat btree-exp/attrib_$v.txt" | grep -v '^pkg:' > "$E/ubuntu_attrib_$v.txt"
done
echo "=== ubuntu-box: base vs iter-only vs iter+search ==="
benchstat "$E/ubuntu_attrib_base.txt" "$E/ubuntu_attrib_iter.txt" "$E/ubuntu_attrib_new.txt" 2>&1 | grep -E 'StringKey|geomean|cpu:' | head -9
