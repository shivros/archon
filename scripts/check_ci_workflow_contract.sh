#!/usr/bin/env bash
set -euo pipefail

workflow_file=".github/workflows/ci.yml"

if [[ ! -f "${workflow_file}" ]]; then
	echo "workflow contract check failed: missing ${workflow_file}" >&2
	exit 1
fi

require_line() {
	local pattern="$1"
	if ! grep -Fq "${pattern}" "${workflow_file}"; then
		echo "workflow contract check failed: expected '${pattern}' in ${workflow_file}" >&2
		exit 1
	fi
}

require_line "pull_request:"
require_line "push:"
require_line "workflow_dispatch:"
require_line "ubuntu-latest"
require_line "macos-latest"
require_line "windows-latest"
require_line "run: scripts/ci_baseline.sh"
require_line "run: scripts/ci_quality.sh"
require_line "run: scripts/ci_race.sh"

echo "check_ci_workflow_contract: ok"
