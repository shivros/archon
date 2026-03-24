#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
cd "${repo_root}"

tmp_paths=()
cleanup() {
	for path in "${tmp_paths[@]:-}"; do
		rm -rf "${path}"
	done
}
trap cleanup EXIT

assert_exit_failure() {
	local description="$1"
	shift
	if "$@" >/dev/null 2>&1; then
		echo "assertion failed: ${description} should fail" >&2
		exit 1
	fi
}

test_release_contract_checker_with_fixtures() {
	local fixture_bad_trigger
	fixture_bad_trigger="$(mktemp)"
	tmp_paths+=("${fixture_bad_trigger}")
	cp .github/workflows/release.yml "${fixture_bad_trigger}"

	python - "${fixture_bad_trigger}" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
text = path.read_text()
text = text.replace("workflow_dispatch:", "workflow_dispatch_removed:", 1)
path.write_text(text)
PY

	assert_exit_failure "release checker should fail without workflow_dispatch" \
		bash scripts/check_release_workflow_contract.sh --workflow-file "${fixture_bad_trigger}"
}

test_build_artifacts_contract_checker_with_fixtures() {
	local fixture_bad_validator
	fixture_bad_validator="$(mktemp)"
	tmp_paths+=("${fixture_bad_validator}")
	cp .github/workflows/build-artifacts.yml "${fixture_bad_validator}"

	python - "${fixture_bad_validator}" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
text = path.read_text()
text = text.replace("scripts/validate_artifact_inputs.sh", "scripts/legacy_validator.sh", 1)
path.write_text(text)
PY

	assert_exit_failure "build-artifacts checker should fail when shared validator is missing" \
		bash scripts/check_build_artifacts_workflow_contract.sh --workflow-file "${fixture_bad_validator}"
}

test_release_contract_checker_with_fixtures
test_build_artifacts_contract_checker_with_fixtures

echo "workflow_contract_checker_tests: ok"
