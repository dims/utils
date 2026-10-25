#!/opt/homebrew/bin/bash
# External-package variants of the benchmark file (as consumers call the
# tree), inlining decisions with a forced rebuild, and lookups across the
# package boundary for the pre-patch (base) and patched (new) sources.
set -uo pipefail
E=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp
for v in base new; do
  mkdir -p "$E/$v/ext"
  gsed -e 's/^package btree$/package ext/' \
       -e "s/^import (/import (\n\tbtree \"exp$v\"\n/" \
       -e 's/ New(/ btree.New(/g' \
       -e 's/\*BTree\[/*btree.BTree[/g' \
       "$E/$v/btree_bench_test.go" > "$E/$v/ext/bench_test.go"
  (cd "$E/$v" && gofmt -w ext/bench_test.go && go vet ./ext/ && echo "vet ok $v/ext")
done
echo "=== inlining, internal build, base (forced rebuild) ==="
(cd "$E/base" && go build -a -gcflags='-m' . 2>&1 | grep -E 'Search|find' | head -8)
echo "=== inlining, external test build, base ==="
(cd "$E/base" && go test -c -o /dev/null -a -gcflags='-m' ./ext/ 2>&1 | grep -E 'Search|find' | head -8)
echo "=== inlining, external test build, new ==="
(cd "$E/new" && go test -c -o /dev/null -a -gcflags='-m' ./ext/ 2>&1 | grep -E 'Search|find' | head -8)
for v in base new; do
  echo "=== Get across the package boundary: $v ==="
  (cd "$E/$v" && go test -run '^$' -bench 'BenchmarkStringKeyGet' -benchmem -count=4 ./ext/ 2>&1 | grep -E 'Benchmark')
done
