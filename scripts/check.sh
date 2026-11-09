#!/usr/bin/env bash
# Fast local check: formatting, vet, and every test except the end-to-end ones
# (which build and run the real binary). CI runs the full suite with -race.
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
go test -short -count=1 ./...
