#!/usr/bin/env bash
# Flags non-test, non-generated Go files that exceed MAX_LINES. Applies
# repo-wide — this template starts clean, so there is nothing to grandfather.

set -euo pipefail

MAX_LINES=1500
BACKEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../backend" && pwd)"

status=0

while IFS= read -r -d '' file; do
  lines=$(wc -l < "$file")
  if [[ $lines -gt $MAX_LINES ]]; then
    echo "FAIL — $file: $lines lines (limit $MAX_LINES)"
    status=1
  fi
done < <(find "$BACKEND_DIR" -name '*.go' ! -name '*_test.go' ! -path '*/gen/*' -print0)

if [[ $status -eq 0 ]]; then
  echo "OK — all backend Go files within $MAX_LINES lines"
fi

exit "$status"
