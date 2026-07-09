#!/usr/bin/env bash
# Enforces a cyclomatic complexity ceiling for the whole backend module. Unlike
# a retrofitted codebase, this template starts clean, so the check applies
# repo-wide from commit one — no per-package opt-in, no suppression file.
#
# gocyclo runs via `go run <pkg>@<version>` (same pinned-version, no-install
# convention as backend/makefile's sqlc/oapi targets), so there is no separate
# setup step to forget and nothing to silently skip: the first invocation
# fetches the pinned version into the module cache, every later one reuses it.

set -euo pipefail

MAX=15
GOCYCLO_VERSION="v0.6.0"
BACKEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../backend" && pwd)"

set +e
violations=$(go run "github.com/fzipp/gocyclo/cmd/gocyclo@${GOCYCLO_VERSION}" \
  -over "$MAX" -ignore '_test\.go|/gen/' "$BACKEND_DIR")
status=$?
set -e

# gocyclo exits non-zero both when it finds violations (expected — the
# `-over` output *is* the violation list, captured above) and when it fails to
# run at all (module fetch failure, bad Go toolchain, etc — reported on
# stderr, left uncaptured so it's visible directly). Only the second case has
# no violation output to show, so that combination is what actually means
# something is broken; report it as a hard failure rather than treating empty
# output as "no violations found".
if [[ -z "$violations" && $status -ne 0 ]]; then
  echo "FAIL — could not run gocyclo (exit $status), see error output above" >&2
  exit 1
fi

if [[ -n "$violations" ]]; then
  echo "FAIL — functions exceed cyclomatic complexity $MAX:"
  echo "$violations"
  exit 1
fi

echo "OK — all backend functions within complexity $MAX"
