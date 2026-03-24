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
	assert_file_contains <(bash scripts/validate_release_inputs.sh --help) "Usage:"
	assert_exit_failure "unknown argument validation" bash scripts/validate_release_inputs.sh --nope
}

test_valid_tag_and_suffix_normalization() {
	local output_file output
	output_file="$(mktemp)"
	tmp_paths+=("${output_file}")

	output="$(
		GITHUB_OUTPUT="${output_file}" \
			bash scripts/validate_release_inputs.sh \
				--tag " refs/tags/v1.2.3 " \
				--suffix " -rc1 "
	)"

	assert_file_contains <(printf '%s\n' "${output}") "tag=v1.2.3"
	assert_file_contains <(printf '%s\n' "${output}") "version=v1.2.3"
	assert_file_contains <(printf '%s\n' "${output}") "suffix=-rc1"
	assert_file_contains "${output_file}" "tag=v1.2.3"
	assert_file_contains "${output_file}" "version=v1.2.3"
	assert_file_contains "${output_file}" "suffix=-rc1"
}

test_valid_tag_without_suffix() {
	local output
	output="$(bash scripts/validate_release_inputs.sh --tag "v2.0.0")"
	assert_file_contains <(printf '%s\n' "${output}") "tag=v2.0.0"
	assert_file_contains <(printf '%s\n' "${output}") "version=v2.0.0"
	assert_file_contains <(printf '%s\n' "${output}") "suffix="
}

test_invalid_inputs_fail() {
	assert_exit_failure "missing tag validation" bash scripts/validate_release_inputs.sh --suffix rc1
	assert_exit_failure "invalid tag characters" bash scripts/validate_release_inputs.sh --tag "v1.2.3/alpha"
	assert_exit_failure "invalid suffix characters" bash scripts/validate_release_inputs.sh --tag "v1.2.3" --suffix "rc/1"
}

test_help_and_unknown_argument_validation
test_valid_tag_and_suffix_normalization
test_valid_tag_without_suffix
test_invalid_inputs_fail

echo "validate_release_inputs_contract_tests: ok"
