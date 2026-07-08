#!/usr/bin/env bash
# Runs every fitness check. Strict: any failure fails the run. There is no
# suppression file — this template starts clean, so nothing needs
# grandfathering in. Fix the violation or change the check, don't suppress it.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
status=0

while IFS= read -r -d '' script_path; do
  rel="${script_path#"$script_dir/"}"
  echo "==> $rel"

  case "$script_path" in
    *.py) python3 "$script_path" || status=1 ;;
    *.sh) bash "$script_path" || status=1 ;;
  esac

  echo
done < <(find "$script_dir" -type f \( -name 'check_*.py' -o -name 'check_*.sh' \) -print0 | sort -z)

exit "$status"
