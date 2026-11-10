#!/usr/bin/env bash
# Local check: formatting, vet and tests.
#   check.sh         fast: every test except the end-to-end ones (which build
#                    and run the real binary)
#   check.sh --full  exactly what CI runs: every test, e2e included, with the
#                    race detector and no test cache
# The cache must stay off for e2e: it builds the binary with `go build`, which
# the test cache cannot see, so a cached e2e pass can predate the code.
set -euo pipefail
cd "$(dirname "$0")/.."

# Timing-based tests fail on a loaded machine; say so before they do.
if command -v tasklist >/dev/null 2>&1; then
  busy=$(tasklist 2>/dev/null | grep -ci 'opencode' || true)
else
  busy=$(pgrep -ci opencode || true)
fi
if [ "${busy:-0}" -gt 0 ]; then
  echo "note: $busy opencode process(es) are running; timing-based tests may be flaky" >&2
fi

test -z "$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
go vet ./...
if [ "${1:-}" = "--full" ]; then
  go test -race -count=1 ./...
else
  go test -short -count=1 ./...
fi
