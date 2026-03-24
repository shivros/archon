#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'USAGE'
Usage:
  scripts/check_release_workflow_contract.sh [--workflow-file <path>]
USAGE
}

workflow_file=".github/workflows/release.yml"

while [[ $# -gt 0 ]]; do
	case "$1" in
	--workflow-file)
		workflow_file="$2"
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "unknown argument: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if [[ ! -f "${workflow_file}" ]]; then
	echo "release workflow contract check failed: missing ${workflow_file}" >&2
	exit 1
fi

require_line() {
	local pattern="$1"
	if ! grep -Fq "${pattern}" "${workflow_file}"; then
		echo "release workflow contract check failed: expected '${pattern}' in ${workflow_file}" >&2
		exit 1
	fi
}

forbid_line() {
	local pattern="$1"
	if grep -Fq "${pattern}" "${workflow_file}"; then
		echo "release workflow contract check failed: '${pattern}' must not appear in ${workflow_file}" >&2
		exit 1
	fi
}

require_line "workflow_dispatch:"
forbid_line "pull_request:"
forbid_line "push:"

require_line "tag:"
require_line "draft:"
require_line "prerelease:"
require_line "scripts/validate_release_inputs.sh"
require_line "Define Release Matrix"
require_line "scripts/release_targets.sh --output-key matrix"
require_line "fromJson(needs.define-matrix.outputs.matrix)"
require_line "scripts/build_artifact.sh"
require_line "scripts/publish_github_release.sh"
require_line "SHA256SUMS.txt"
require_line "contents: write"

echo "check_release_workflow_contract: ok"
