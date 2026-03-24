#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'USAGE'
Usage:
  scripts/check_docs_build_release_contract.sh \
    [--readme <path>] \
    [--runbook <path>]
USAGE
}

readme_file="README.md"
runbook_file="docs/maintainer-build-release-runbook.md"

while [[ $# -gt 0 ]]; do
	case "$1" in
	--readme)
		readme_file="$2"
		shift 2
		;;
	--runbook)
		runbook_file="$2"
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

if [[ ! -f "${readme_file}" ]]; then
	echo "docs contract check failed: missing README file ${readme_file}" >&2
	exit 1
fi

if [[ ! -f "${runbook_file}" ]]; then
	echo "docs contract check failed: missing runbook file ${runbook_file}" >&2
	exit 1
fi

require_line_in_file() {
	local file="$1"
	local pattern="$2"
	if ! grep -Fq -- "${pattern}" "${file}"; then
		echo "docs contract check failed: expected '${pattern}' in ${file}" >&2
		exit 1
	fi
}

require_line_in_file "${readme_file}" "### Build & Release"
require_line_in_file "${readme_file}" "docs/maintainer-build-release-runbook.md"
require_line_in_file "${readme_file}" "Maintainer operational guidance"

require_line_in_file "${runbook_file}" "# Maintainer Build and Release Runbook"
require_line_in_file "${runbook_file}" "authoritative maintainer procedure"
require_line_in_file "${runbook_file}" "## Supported Release Targets"
require_line_in_file "${runbook_file}" "## CI Validation Scope"
require_line_in_file "${runbook_file}" "## Manual Artifact Build"
require_line_in_file "${runbook_file}" "## Manual GitHub Release Publish"
require_line_in_file "${runbook_file}" "## Optional Tag Preparation Helper"
require_line_in_file "${runbook_file}" "## Maintainer Flow (Commit to Release)"

require_line_in_file "${runbook_file}" 'Workflow: `CI`'
require_line_in_file "${runbook_file}" 'Workflow: `Build Artifacts (Manual)`'
require_line_in_file "${runbook_file}" 'Workflow: `Release (Manual)`'
require_line_in_file "${runbook_file}" '.github/workflows/ci.yml'
require_line_in_file "${runbook_file}" '.github/workflows/build-artifacts.yml'
require_line_in_file "${runbook_file}" '.github/workflows/release.yml'
require_line_in_file "${runbook_file}" '`workflow_dispatch` only'

require_line_in_file "${runbook_file}" '`ref`'
require_line_in_file "${runbook_file}" '`version`'
require_line_in_file "${runbook_file}" '`artifact_suffix`'
require_line_in_file "${runbook_file}" '`tag`'
require_line_in_file "${runbook_file}" '`draft`'
require_line_in_file "${runbook_file}" '`prerelease`'
require_line_in_file "${runbook_file}" '`release_notes`'

require_line_in_file "${runbook_file}" '`linux/amd64`'
require_line_in_file "${runbook_file}" '`linux/arm64`'
require_line_in_file "${runbook_file}" '`darwin/amd64`'
require_line_in_file "${runbook_file}" '`darwin/arm64`'
require_line_in_file "${runbook_file}" '`windows/amd64`'
require_line_in_file "${runbook_file}" '`windows/arm64`'
require_line_in_file "${runbook_file}" '`scripts/release_targets.sh`'

require_line_in_file "${runbook_file}" '`scripts/prepare_release_tag.sh`'
require_line_in_file "${runbook_file}" '`--check-only`'
require_line_in_file "${runbook_file}" '`--create`'
require_line_in_file "${runbook_file}" '`--push`'
require_line_in_file "${runbook_file}" '`--dry-run`'
require_line_in_file "${runbook_file}" "Releases are manual only."
require_line_in_file "${runbook_file}" "No automatic release publishing on push or tag creation."

echo "check_docs_build_release_contract: ok"
