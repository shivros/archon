#!/usr/bin/env bash
set -euo pipefail

workflow_file=".github/workflows/build-artifacts.yml"

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

require_line "goos: linux"
require_line "goarch: amd64"
require_line "goarch: arm64"
require_line "goos: darwin"
require_line "goos: windows"
require_line "id: build_artifact"
require_line "run: |"
require_line "scripts/build_artifact.sh"
require_line '${{ steps.build_artifact.outputs.artifact_name }}'
require_line '${{ steps.build_artifact.outputs.artifact_archive }}'
require_line '${{ steps.build_artifact.outputs.artifact_checksum }}'

echo "check_build_artifacts_workflow_contract: ok"
