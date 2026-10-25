#!/opt/homebrew/bin/bash
# Mirrors the repo's external benchmark file into the scratch base module,
# which holds the unmodified btree.go, and records the external baseline.
set -uo pipefail
E=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp
W=/Users/dsrinivas/go/src/k8s.io/utils/.claude/worktrees/btree-benchmarks
rm -f "$E/base/ext/bench_test.go" "$E/base/btree_bench_test.go" "$E/new/ext/bench_test.go" "$E/new/btree_bench_test.go"
rmdir "$E/base/ext" "$E/new/ext" 2>/dev/null
mkdir -p "$E/base/ext"
gsed -e 's|"k8s.io/utils/third_party/forked/golang/btree"|btree "expbase"|' "$W/third_party/forked/golang/btree/btree_bench_test.go" > "$E/base/ext/bench_test.go"
cd "$E/base" && gofmt -l ext/ ; go vet ./ext/ && echo "vet ok" && go test -run '^$' -bench 'BenchmarkStringKey' -benchmem -count=8 ./ext/ > "$E/before_ext.txt" 2>&1; tail -1 "$E/before_ext.txt"
