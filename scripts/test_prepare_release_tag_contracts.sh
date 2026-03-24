#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
cd "${repo_root}"
BASH_BIN="$(command -v bash)"

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
	if ! grep -Fq -- "${needle}" "${file}"; then
		echo "assertion failed: expected '${file}' to contain '${needle}'" >&2
		exit 1
	fi
}

assert_file_not_contains() {
	local file="$1"
	local needle="$2"
	if grep -Fq -- "${needle}" "${file}"; then
		echo "assertion failed: expected '${file}' not to contain '${needle}'" >&2
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

create_git_stub() {
	local stub_dir="$1"
	cat >"${stub_dir}/git" <<'EOF'
#!/usr/bin/bash
set -euo pipefail
printf 'git:%s\n' "$*" >> "${GIT_STUB_LOG:?}"

if [[ "$#" -ge 2 && "$1" == "rev-parse" && "$2" == "--is-inside-work-tree" ]]; then
	echo "${GIT_STUB_INSIDE_WORKTREE:-true}"
	exit 0
fi

if [[ "$#" -ge 2 && "$1" == "rev-parse" && "$2" == "--short" ]]; then
	echo "${GIT_STUB_HEAD_SHORT:-deadbeef}"
	exit 0
fi

if [[ "$#" -ge 4 && "$1" == "rev-parse" && "$2" == "-q" && "$3" == "--verify" ]]; then
	if [[ "${GIT_STUB_LOCAL_TAG_EXISTS:-false}" == "true" ]]; then
		exit 0
	fi
	exit 1
fi

if [[ "$#" -ge 2 && "$1" == "diff" && "$2" == "--quiet" ]]; then
	if [[ "${GIT_STUB_DIRTY_WORKTREE:-false}" == "true" ]]; then
		exit 1
	fi
	exit 0
fi

if [[ "$#" -ge 3 && "$1" == "diff" && "$2" == "--cached" && "$3" == "--quiet" ]]; then
	if [[ "${GIT_STUB_DIRTY_INDEX:-false}" == "true" ]]; then
		exit 1
	fi
	exit 0
fi

if [[ "$#" -ge 3 && "$1" == "ls-files" && "$2" == "--others" && "$3" == "--exclude-standard" ]]; then
	if [[ "${GIT_STUB_UNTRACKED:-false}" == "true" ]]; then
		echo "temp-untracked.txt"
	fi
	exit 0
fi

if [[ "$#" -ge 4 && "$1" == "ls-remote" && "$2" == "--tags" ]]; then
	if [[ "${GIT_STUB_REMOTE_TAG_EXISTS:-false}" == "true" ]]; then
		echo "abc123	$4"
	fi
	exit 0
fi

if [[ "$#" -ge 2 && "$1" == "tag" && "$2" == "-a" ]]; then
	exit 0
fi

if [[ "$#" -ge 1 && "$1" == "push" ]]; then
	exit 0
fi

echo "unexpected git command: $*" >&2
exit 1
EOF
	chmod +x "${stub_dir}/git"
}

run_with_git_stub() {
	local log_file="$1"
	local stub_dir="$2"
	shift 2
	PATH="${stub_dir}:${PATH}" GIT_STUB_LOG="${log_file}" "$@"
}

test_help_and_unknown_argument_validation() {
	assert_file_contains <("${BASH_BIN}" scripts/prepare_release_tag.sh --help) "Usage:"
	assert_exit_failure "unknown argument validation" "${BASH_BIN}" scripts/prepare_release_tag.sh --nope
}

test_invalid_tag_and_push_without_create_fail() {
	assert_exit_failure "invalid tag format should fail" "${BASH_BIN}" scripts/prepare_release_tag.sh --tag "1.2.3"
	assert_exit_failure "--push without --create should fail" "${BASH_BIN}" scripts/prepare_release_tag.sh --tag "v1.2.3" --push
	assert_exit_failure "--check-only with --create should fail" "${BASH_BIN}" scripts/prepare_release_tag.sh --tag "v1.2.3" --check-only --create
}

test_dirty_tree_and_existing_tags_fail() {
	local stub_dir log_file
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	create_git_stub "${stub_dir}"

	assert_exit_failure "dirty worktree should fail" env \
		PATH="${stub_dir}:${PATH}" \
		GIT_STUB_LOG="${log_file}" \
		GIT_STUB_DIRTY_WORKTREE="true" \
		"${BASH_BIN}" scripts/prepare_release_tag.sh --tag "v1.2.3"

	assert_exit_failure "dirty index should fail" env \
		PATH="${stub_dir}:${PATH}" \
		GIT_STUB_LOG="${log_file}" \
		GIT_STUB_DIRTY_INDEX="true" \
		"${BASH_BIN}" scripts/prepare_release_tag.sh --tag "v1.2.3"

	assert_exit_failure "untracked files should fail" env \
		PATH="${stub_dir}:${PATH}" \
		GIT_STUB_LOG="${log_file}" \
		GIT_STUB_UNTRACKED="true" \
		"${BASH_BIN}" scripts/prepare_release_tag.sh --tag "v1.2.3"

	assert_exit_failure "local existing tag should fail" env \
		PATH="${stub_dir}:${PATH}" \
		GIT_STUB_LOG="${log_file}" \
		GIT_STUB_LOCAL_TAG_EXISTS="true" \
		"${BASH_BIN}" scripts/prepare_release_tag.sh --tag "v1.2.3"

	assert_exit_failure "remote existing tag should fail" env \
		PATH="${stub_dir}:${PATH}" \
		GIT_STUB_LOG="${log_file}" \
		GIT_STUB_REMOTE_TAG_EXISTS="true" \
		"${BASH_BIN}" scripts/prepare_release_tag.sh --tag "v1.2.3"
}

test_dry_run_create_and_push_outputs_next_steps() {
	local stub_dir log_file output
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	create_git_stub "${stub_dir}"

	output="$(
		run_with_git_stub "${log_file}" "${stub_dir}" \
			"${BASH_BIN}" scripts/prepare_release_tag.sh \
				--tag "v1.2.3-rc1" \
				--create \
				--push \
				--dry-run
	)"

	assert_file_contains <(printf '%s\n' "${output}") "release_tag=v1.2.3-rc1"
	assert_file_contains <(printf '%s\n' "${output}") "dry_run: git tag -a v1.2.3-rc1 -m Release v1.2.3-rc1"
	assert_file_contains <(printf '%s\n' "${output}") "dry_run: git push origin v1.2.3-rc1"
	assert_file_contains <(printf '%s\n' "${output}") "next_step: Run workflow 'Release (Manual)' with tag='v1.2.3-rc1'."
}

test_create_and_push_execute_commands() {
	local stub_dir log_file output
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	create_git_stub "${stub_dir}"

	output="$(
		run_with_git_stub "${log_file}" "${stub_dir}" \
			"${BASH_BIN}" scripts/prepare_release_tag.sh \
				--tag "v1.2.3" \
				--create \
				--push \
				--remote "origin"
	)"

	assert_file_contains "${log_file}" "git:tag -a v1.2.3 -m Release v1.2.3"
	assert_file_contains "${log_file}" "git:push origin v1.2.3"
	assert_file_contains <(printf '%s\n' "${output}") "commit=deadbeef"
}

test_skip_remote_check_allows_conflicting_remote_tag() {
	local stub_dir log_file output
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	create_git_stub "${stub_dir}"

	output="$(
		PATH="${stub_dir}:${PATH}" \
		GIT_STUB_LOG="${log_file}" \
		GIT_STUB_REMOTE_TAG_EXISTS="true" \
			"${BASH_BIN}" scripts/prepare_release_tag.sh --tag "v1.2.3" --skip-remote-check
	)"

	assert_file_contains <(printf '%s\n' "${output}") "skip_remote_check=true"
}

test_check_only_mode_does_not_run_mutations() {
	local stub_dir log_file output
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	create_git_stub "${stub_dir}"

	output="$(
		PATH="${stub_dir}:${PATH}" \
		GIT_STUB_LOG="${log_file}" \
			"${BASH_BIN}" scripts/prepare_release_tag.sh --tag "v1.2.3" --check-only
	)"

	assert_file_contains <(printf '%s\n' "${output}") "check_only=true"
	assert_file_not_contains "${log_file}" "git:tag -a"
	assert_file_not_contains "${log_file}" "git:push "
}

test_default_mode_infers_check_only() {
	local stub_dir log_file output
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	create_git_stub "${stub_dir}"

	output="$(
		PATH="${stub_dir}:${PATH}" \
		GIT_STUB_LOG="${log_file}" \
			"${BASH_BIN}" scripts/prepare_release_tag.sh --tag "v1.2.3"
	)"

	assert_file_contains <(printf '%s\n' "${output}") "check_only=true"
	assert_file_not_contains "${log_file}" "git:tag -a"
	assert_file_not_contains "${log_file}" "git:push "
}

test_help_and_unknown_argument_validation
test_invalid_tag_and_push_without_create_fail
test_dirty_tree_and_existing_tags_fail
test_dry_run_create_and_push_outputs_next_steps
test_create_and_push_execute_commands
test_skip_remote_check_allows_conflicting_remote_tag
test_check_only_mode_does_not_run_mutations
test_default_mode_infers_check_only

echo "prepare_release_tag_contract_tests: ok"
