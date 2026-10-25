#!/opt/homebrew/bin/bash
# The three variants back to back on this machine.
set -uo pipefail
E=/private/tmp/claude-502/-Users-dsrinivas-go-src-k8s-io-utils/d8991197-f1db-40de-9971-e3e62f1d2824/scratchpad/exp
for v in base iter new; do
  (cd "$E/$v" && go test -run '^$' -bench 'BenchmarkStringKey' -benchmem -count=6 ./ext/ 2>&1 | grep -v '^pkg:' > "$E/mac_attrib_$v.txt")
done
echo "=== M4 Pro: base vs iter-only vs iter+search ==="
benchstat "$E/mac_attrib_base.txt" "$E/mac_attrib_iter.txt" "$E/mac_attrib_new.txt" 2>&1 | grep -E 'StringKey|geomean|cpu:' | head -9
