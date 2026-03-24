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

assert_file_contains() {
	local file="$1"
	local needle="$2"
	if ! grep -Fq "${needle}" "${file}"; then
		echo "assertion failed: expected '${file}' to contain '${needle}'" >&2
		exit 1
	fi
}

assert_exit_failure() {
	local description="$1"
	shift
	if "$@" >/dev/null 2>&1; then
		echo "assertion failed: ${description} should fail" >&2
		exit 1
	fi
}

test_help_and_unknown_argument_validation() {
	assert_file_contains <(bash scripts/release_targets.sh --help) "Usage:"
	assert_exit_failure "unknown argument validation" bash scripts/release_targets.sh --nope
}

test_stdout_json_contains_required_targets() {
	local output
	output="$(bash scripts/release_targets.sh)"
	assert_file_contains <(printf '%s\n' "${output}") '"goos":"linux","goarch":"amd64"'
	assert_file_contains <(printf '%s\n' "${output}") '"goos":"linux","goarch":"arm64"'
	assert_file_contains <(printf '%s\n' "${output}") '"goos":"darwin","goarch":"amd64"'
	assert_file_contains <(printf '%s\n' "${output}") '"goos":"darwin","goarch":"arm64"'
	assert_file_contains <(printf '%s\n' "${output}") '"goos":"windows","goarch":"amd64"'
	assert_file_contains <(printf '%s\n' "${output}") '"goos":"windows","goarch":"arm64"'

	python - "${output}" <<'PY'
import json
import sys

payload = json.loads(sys.argv[1])
include = payload.get("include")
if not isinstance(include, list):
    raise SystemExit("missing include list")

expected = {
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("windows", "amd64"),
    ("windows", "arm64"),
}
actual = {(item.get("goos"), item.get("goarch")) for item in include}
if actual != expected:
    raise SystemExit(f"unexpected targets: {sorted(actual)}")
PY
}

test_github_output_support() {
	local output_file
	output_file="$(mktemp)"
	tmp_paths+=("${output_file}")

	GITHUB_OUTPUT="${output_file}" bash scripts/release_targets.sh --output-key matrix >/dev/null
	assert_file_contains "${output_file}" 'matrix={"include":['
}

test_output_key_requires_github_output() {
	assert_exit_failure "output key should require GITHUB_OUTPUT" bash scripts/release_targets.sh --output-key matrix
}

test_help_and_unknown_argument_validation
test_stdout_json_contains_required_targets
test_github_output_support
test_output_key_requires_github_output

echo "release_targets_contract_tests: ok"
