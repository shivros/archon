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

assert_file_contains() {
	local file="$1"
	local needle="$2"
	if ! grep -Fq -- "${needle}" "${file}"; then
		echo "assertion failed: expected '${file}' to contain '${needle}'" >&2
		exit 1
	fi
}

test_help_and_unknown_argument_paths() {
	assert_file_contains <(bash scripts/check_docs_build_release_contract.sh --help) "Usage:"
	assert_exit_failure "unknown argument should fail" bash scripts/check_docs_build_release_contract.sh --nope
}

test_current_docs_contract_passes() {
	bash scripts/check_docs_build_release_contract.sh >/dev/null
}

test_missing_readme_link_fails() {
	local readme_fixture runbook_fixture
	readme_fixture="$(mktemp)"
	runbook_fixture="$(mktemp)"
	tmp_paths+=("${readme_fixture}" "${runbook_fixture}")
	cp README.md "${readme_fixture}"
	cp docs/maintainer-build-release-runbook.md "${runbook_fixture}"

	python - "${readme_fixture}" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
text = path.read_text()
text = text.replace(
    "[docs/maintainer-build-release-runbook.md](docs/maintainer-build-release-runbook.md)",
    "[docs/missing-runbook.md](docs/missing-runbook.md)",
    1,
)
path.write_text(text)
PY

	assert_exit_failure "missing README runbook link should fail" \
		bash scripts/check_docs_build_release_contract.sh --readme "${readme_fixture}" --runbook "${runbook_fixture}"
}

test_missing_release_workflow_name_fails() {
	local readme_fixture runbook_fixture
	readme_fixture="$(mktemp)"
	runbook_fixture="$(mktemp)"
	tmp_paths+=("${readme_fixture}" "${runbook_fixture}")
	cp README.md "${readme_fixture}"
	cp docs/maintainer-build-release-runbook.md "${runbook_fixture}"

	python - "${runbook_fixture}" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
text = path.read_text()
text = text.replace("Workflow: `Release (Manual)` (`.github/workflows/release.yml`)", "Workflow: `Release`", 1)
path.write_text(text)
PY

	assert_exit_failure "missing release workflow contract text should fail" \
		bash scripts/check_docs_build_release_contract.sh --readme "${readme_fixture}" --runbook "${runbook_fixture}"
}

test_missing_supported_target_fails() {
	local readme_fixture runbook_fixture
	readme_fixture="$(mktemp)"
	runbook_fixture="$(mktemp)"
	tmp_paths+=("${readme_fixture}" "${runbook_fixture}")
	cp README.md "${readme_fixture}"
	cp docs/maintainer-build-release-runbook.md "${runbook_fixture}"

	python - "${runbook_fixture}" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
text = path.read_text()
text = text.replace("- `windows/arm64`\n", "", 1)
path.write_text(text)
PY

	assert_exit_failure "missing supported target text should fail" \
		bash scripts/check_docs_build_release_contract.sh --readme "${readme_fixture}" --runbook "${runbook_fixture}"
}

test_missing_tag_helper_contract_fails() {
	local readme_fixture runbook_fixture
	readme_fixture="$(mktemp)"
	runbook_fixture="$(mktemp)"
	tmp_paths+=("${readme_fixture}" "${runbook_fixture}")
	cp README.md "${readme_fixture}"
	cp docs/maintainer-build-release-runbook.md "${runbook_fixture}"

	python - "${runbook_fixture}" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
text = path.read_text()
text = text.replace("Helper script: `scripts/prepare_release_tag.sh`\n", "", 1)
path.write_text(text)
PY

	assert_exit_failure "missing tag helper contract text should fail" \
		bash scripts/check_docs_build_release_contract.sh --readme "${readme_fixture}" --runbook "${runbook_fixture}"
}

test_non_contract_prose_changes_still_pass() {
	local readme_fixture runbook_fixture
	readme_fixture="$(mktemp)"
	runbook_fixture="$(mktemp)"
	tmp_paths+=("${readme_fixture}" "${runbook_fixture}")
	cp README.md "${readme_fixture}"
	cp docs/maintainer-build-release-runbook.md "${runbook_fixture}"

	python - "${runbook_fixture}" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
text = path.read_text()
text = text.replace(
    "prevents accidental local/remote tag collisions",
    "helps reduce accidental local or remote tag conflicts",
    1,
)
path.write_text(text)
PY

	bash scripts/check_docs_build_release_contract.sh --readme "${readme_fixture}" --runbook "${runbook_fixture}" >/dev/null
}

test_missing_files_fail() {
	assert_exit_failure "missing readme file should fail" \
		bash scripts/check_docs_build_release_contract.sh --readme /tmp/does-not-exist-readme.md
	assert_exit_failure "missing runbook file should fail" \
		bash scripts/check_docs_build_release_contract.sh --runbook /tmp/does-not-exist-runbook.md
}

test_help_and_unknown_argument_paths
test_current_docs_contract_passes
test_missing_readme_link_fails
test_missing_release_workflow_name_fails
test_missing_supported_target_fails
test_missing_tag_helper_contract_fails
test_non_contract_prose_changes_still_pass
test_missing_files_fail

echo "docs_build_release_contract_tests: ok"
