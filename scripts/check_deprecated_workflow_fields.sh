#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "check_deprecated_workflow_fields: scanning Go sources"

SCAN_ROOTS_RAW="${ARCHON_DEPRECATED_FIELD_SCAN_ROOTS:-cmd internal}"
read -r -a SCAN_ROOTS <<<"$SCAN_ROOTS_RAW"
if [ "${#SCAN_ROOTS[@]}" -eq 0 ]; then
  SCAN_ROOTS=(cmd internal)
fi

found=0
if command -v rg >/dev/null 2>&1; then
  if rg -n --glob '*.go' --glob '!**/*_test.go' '(SelectedPolicySensitivity|selected_policy_sensitivity)' "${SCAN_ROOTS[@]}"; then
    found=1
  fi
else
  while IFS= read -r -d '' file; do
    case "$file" in
      *_test.go) continue ;;
    esac
    if grep -qn 'SelectedPolicySensitivity\|selected_policy_sensitivity' "$file"; then
      grep -n 'SelectedPolicySensitivity\|selected_policy_sensitivity' "$file"
      found=1
    fi
  done < <(find "${SCAN_ROOTS[@]}" -name '*.go' -print0 2>/dev/null)
fi

if [ "$found" -ne 0 ]; then
  echo "check_deprecated_workflow_fields: found reintroduced deprecated field selected_policy_sensitivity" >&2
  exit 1
fi

echo "check_deprecated_workflow_fields: ok"
