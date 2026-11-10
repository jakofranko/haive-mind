#!/usr/bin/env bash
# Show red: run this working tree's tests for one package against the code at
# an earlier ref, to confirm they fail without the fix. Repeats the run with
# the race detector and prints how often each test passed and failed, so a
# race regression's fail rate is visible.
#
#   red.sh <ref> <pkg-dir> <run-regex> [count] [test-file...]
#
#   ref        the code before the fix, e.g. main or HEAD~1
#   pkg-dir    package directory, e.g. coordinator
#   run-regex  passed to go test -run
#   count      runs per test (default 10)
#   test-file  the test files to take from this tree (default: every
#              *_test.go in pkg-dir). Taken files replace that package's tests
#              at ref, so they must compile against the old code; name only
#              the files the regression test needs when others use new API.
#
# Exits 0 when a test went red, 1 when every run passed, 2 when the tests did
# not build at ref. The checkout at ref is a temporary git worktree, so this
# working tree is never touched.
set -euo pipefail

if [ $# -lt 3 ]; then
  sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
fi
ref=$1 pkg=${2%/} run=$3 count=${4:-10}
shift $(($# < 4 ? $# : 4))
root=$(git rev-parse --show-toplevel)
cd "$root"

if [ $# -gt 0 ]; then
  files=("$@")
else
  files=()
  for f in "$pkg"/*_test.go; do files+=("$(basename "$f")"); done
fi

tree=$(mktemp -d)
cleanup() { git worktree remove --force "$tree" >/dev/null 2>&1 || rm -rf "$tree"; }
trap cleanup EXIT
git worktree add --quiet --detach "$tree" "$ref"

rm -f "$tree/$pkg"/*_test.go
for f in "${files[@]}"; do
  cp "$pkg/$f" "$tree/$pkg/$f"
done

echo "running ${files[*]} from this tree against $ref ($(git rev-parse --short "$ref")), $count times" >&2
if ! (cd "$tree" && go test -count=1 -run '^$' "./$pkg/") >"$tree/build.log" 2>&1; then
  echo "the tests do not build at $ref:" >&2
  cat "$tree/build.log" >&2
  exit 2
fi

out=$(cd "$tree" && go test -race -count="$count" -v -run "$run" "./$pkg/" 2>&1 || true)
tally=$(printf '%s\n' "$out" | awk '/^--- (PASS|FAIL):/ { sub(":", "", $2); n[$3 " " $2]++ } END { for (k in n) print k, n[k] "/'"$count"'" }' | sort)
if [ -z "$tally" ]; then
  echo "no test matched -run '$run'" >&2
  exit 2
fi
printf '%s\n' "$tally"

if printf '%s\n' "$tally" | grep -q ' FAIL '; then
  echo "red: at least one run failed at $ref" >&2
  exit 0
fi
echo "never red: every run passed at $ref" >&2
exit 1
