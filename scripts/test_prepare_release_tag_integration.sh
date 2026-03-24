#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
script_path="${repo_root}/scripts/prepare_release_tag.sh"
bash_bin="$(command -v bash)"

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

init_test_repo_with_remote() {
	local workdir remote template_dir
	workdir="$(mktemp -d)"
	remote="$(mktemp -d)"
	template_dir="$(mktemp -d)"
	tmp_paths+=("${workdir}" "${remote}" "${template_dir}")

	git -C "${workdir}" init --template "${template_dir}" >/dev/null
	git -C "${workdir}" config user.name "Archon Test"
	git -C "${workdir}" config user.email "archon-test@example.com"
	printf 'seed\n' >"${workdir}/README.seed"
	git -C "${workdir}" add README.seed
	git -C "${workdir}" commit -m "seed" >/dev/null

	git -C "${remote}" init --bare --template "${template_dir}" >/dev/null
	git -C "${workdir}" remote add origin "${remote}"

	printf '%s\n%s\n' "${workdir}" "${remote}"
}

test_default_mode_is_check_only_and_non_mutating() {
	local paths workdir output
	mapfile -t paths < <(init_test_repo_with_remote)
	workdir="${paths[0]}"

	output="$(cd "${workdir}" && "${bash_bin}" "${script_path}" --tag v1.2.3)"
	assert_file_contains <(printf '%s\n' "${output}") "check_only=true"

	if git -C "${workdir}" rev-parse -q --verify "refs/tags/v1.2.3" >/dev/null 2>&1; then
		echo "assertion failed: default mode should not create local tag" >&2
		exit 1
	fi
}

test_create_mode_creates_annotated_tag() {
	local paths workdir
	mapfile -t paths < <(init_test_repo_with_remote)
	workdir="${paths[0]}"

	(cd "${workdir}" && "${bash_bin}" "${script_path}" --tag v1.2.3 --create >/dev/null)

	if [[ "$(git -C "${workdir}" cat-file -t "refs/tags/v1.2.3")" != "tag" ]]; then
		echo "assertion failed: expected annotated tag v1.2.3" >&2
		exit 1
	fi
}

test_create_push_mode_pushes_tag_to_remote() {
	local paths workdir remote
	mapfile -t paths < <(init_test_repo_with_remote)
	workdir="${paths[0]}"
	remote="${paths[1]}"

	(cd "${workdir}" && "${bash_bin}" "${script_path}" --tag v1.2.4 --create --push >/dev/null)

	if [[ -z "$(git -C "${workdir}" ls-remote --tags origin "refs/tags/v1.2.4")" ]]; then
		echo "assertion failed: expected tag v1.2.4 to be pushed to remote" >&2
		exit 1
	fi

	if [[ -z "$(git -C "${remote}" show-ref --tags "refs/tags/v1.2.4" 2>/dev/null || true)" ]]; then
		echo "assertion failed: expected bare remote to contain tag v1.2.4" >&2
		exit 1
	fi
}

test_outside_git_worktree_fails() {
	local outside_dir
	outside_dir="$(mktemp -d)"
	tmp_paths+=("${outside_dir}")

	assert_exit_failure "outside worktree should fail" env \
		HOME="${outside_dir}" \
		"${bash_bin}" -lc "cd '${outside_dir}' && '${script_path}' --tag v1.2.3"
}

test_default_mode_is_check_only_and_non_mutating
test_create_mode_creates_annotated_tag
test_create_push_mode_pushes_tag_to_remote
test_outside_git_worktree_fails

echo "prepare_release_tag_integration_tests: ok"
