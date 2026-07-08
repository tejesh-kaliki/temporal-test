#!/usr/bin/env bash
# Enforces a cyclomatic complexity ceiling for the whole backend module. Unlike
# a retrofitted codebase, this template starts clean, so the check applies
# repo-wide from commit one — no per-package opt-in, no suppression file.
#
# Requires: gocyclo  (go install github.com/fzipp/gocyclo/cmd/gocyclo@latest)

set -euo pipefail

MAX=15
BACKEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../backend" && pwd)"

if ! command -v gocyclo &>/dev/null; then
  echo "SKIP: gocyclo not found (install with: go install github.com/fzipp/gocyclo/cmd/gocyclo@latest)"
  exit 0
fi

violations=$(gocyclo -over "$MAX" -ignore '_test\.go|/gen/' "$BACKEND_DIR" || true)
if [[ -n "$violations" ]]; then
  echo "FAIL — functions exceed cyclomatic complexity $MAX:"
  echo "$violations"
  exit 1
fi

echo "OK — all backend functions within complexity $MAX"
