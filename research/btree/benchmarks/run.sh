#!/opt/homebrew/bin/bash
# Same benchmark file on the pre-patch (base) and patched (new) sources,
# under GOMAXPROCS=1 and the default, plus the compiler's inlining decisions
# for find and sort.Search in each.
set -uo pipefail
E=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp
for v in base new; do
  printf 'module exp%s\n\ngo 1.26\n' "$v" > "$E/$v/go.mod"
done
echo "=== inlining: base ==="
(cd "$E/base" && go build -gcflags='-m' . 2>&1 | grep -E 'find|Search' | head -8 || true)
echo "=== inlining: new ==="
(cd "$E/new" && go build -gcflags='-m' . 2>&1 | grep -E 'find|Search' | head -8 || true)
for cpu in 1 default; do
  for v in base new; do
    if [ "$cpu" = 1 ]; then flag="-cpu=1"; else flag=""; fi
    (cd "$E/$v" && go test -run '^$' -bench 'BenchmarkStringKeyGet' -benchmem -count=4 $flag . > "$E/get_${v}_cpu${cpu}.txt" 2>&1)
  done
  echo "=== Get, cpu=$cpu: base vs new ==="
  benchstat "$E/get_base_cpu${cpu}.txt" "$E/get_new_cpu${cpu}.txt" 2>&1 | grep -E 'StringKeyGet|geomean' | head -4
done
