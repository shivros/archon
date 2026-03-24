#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'USAGE'
Usage:
  scripts/check_build_artifacts_workflow_contract.sh [--workflow-file <path>]
USAGE
}

workflow_file=".github/workflows/build-artifacts.yml"

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
	echo "build-artifacts workflow contract check failed: missing ${workflow_file}" >&2
	exit 1
fi

require_line() {
	local pattern="$1"
	if ! grep -Fq "${pattern}" "${workflow_file}"; then
		echo "build-artifacts workflow contract check failed: expected '${pattern}' in ${workflow_file}" >&2
		exit 1
	fi
}

forbid_line() {
	local pattern="$1"
	if grep -Fq "${pattern}" "${workflow_file}"; then
		echo "build-artifacts workflow contract check failed: '${pattern}' must not appear in ${workflow_file}" >&2
		exit 1
	fi
}

require_line "workflow_dispatch:"
forbid_line "pull_request:"
forbid_line "push:"

require_line "Define Build Matrix"
require_line "scripts/release_targets.sh --output-key matrix"
require_line "fromJson(needs.define-matrix.outputs.matrix)"
require_line "scripts/validate_artifact_inputs.sh"
require_line "id: build_artifact"
require_line "run: |"
require_line "scripts/build_artifact.sh"
require_line '${{ steps.build_artifact.outputs.artifact_name }}'
require_line '${{ steps.build_artifact.outputs.artifact_archive }}'
require_line '${{ steps.build_artifact.outputs.artifact_checksum }}'

echo "check_build_artifacts_workflow_contract: ok"
