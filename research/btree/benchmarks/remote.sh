#!/opt/homebrew/bin/bash
# Mirrors the unmodified (base) and final (new) btree.go with the repo's
# external benchmark file into two scratch modules, ships them to
# ubuntu-box, runs the benchmarks there, and brings the results back.
set -uo pipefail
E=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp
W=/Users/dsrinivas/go/src/k8s.io/utils/.claude/worktrees/btree-benchmarks
B=$W/third_party/forked/golang/btree
for v in base new; do
  mkdir -p "$E/$v/ext"
  rm -f "$E/$v/btree_bench_test.go"
  gsed -e "s|\"k8s.io/utils/third_party/forked/golang/btree\"|btree \"exp$v\"|" "$B/btree_bench_test.go" > "$E/$v/ext/bench_test.go"
  printf 'module exp%s\n\ngo 1.26\n' "$v" > "$E/$v/go.mod"
done
cp "$B/btree.go" "$E/new/btree.go"
cmp -s "$E/base/btree.go" "$E/new/btree.go" && { echo "base and new btree.go are identical, aborting"; exit 1; }
(cd "$E/base" && go vet ./ext/) && (cd "$E/new" && go vet ./ext/) && echo "local vet ok"
rsync -a --delete "$E/base" "$E/new" ubuntu-box:btree-exp/ && echo "rsync ok"
ssh ubuntu-box 'export PATH=$PATH:/usr/local/go/bin; cd btree-exp && for v in base new; do (cd $v && go test -run "^$" -bench BenchmarkStringKey -benchmem -count=8 ./ext/ > ../$v.txt 2>&1); done; tail -1 base.txt new.txt'
for v in base new; do
  ssh ubuntu-box "cat btree-exp/$v.txt" | grep -v '^pkg:' > "$E/ubuntu_$v.txt"
done
echo "=== ubuntu-box: base vs new ==="
benchstat "$E/ubuntu_base.txt" "$E/ubuntu_new.txt" 2>&1 | grep -E 'StringKey|geomean|cpu:' | head -20
