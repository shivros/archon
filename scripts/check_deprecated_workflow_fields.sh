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

if rg -n --glob '*.go' --glob '!**/*_test.go' '(SelectedPolicySensitivity|selected_policy_sensitivity)' "${SCAN_ROOTS[@]}"; then
  echo "check_deprecated_workflow_fields: found reintroduced deprecated field selected_policy_sensitivity" >&2
  exit 1
fi

echo "check_deprecated_workflow_fields: ok"
