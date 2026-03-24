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

make_assets_dir() {
	local assets_dir
	assets_dir="$(mktemp -d)"
	tmp_paths+=("${assets_dir}")
	touch "${assets_dir}/archon_v1.2.3_linux_amd64.tar.gz"
	touch "${assets_dir}/archon_v1.2.3_linux_amd64.tar.gz.sha256"
	touch "${assets_dir}/SHA256SUMS.txt"
	echo "${assets_dir}"
}

create_gh_stub() {
	local stub_dir="$1"
	cat >"${stub_dir}/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'gh:%s\n' "$*" >> "${GH_STUB_LOG:?}"

if [[ "$1" == "release" && "$2" == "view" ]]; then
	if [[ "$*" == *"--json url --jq .url"* ]]; then
		echo "${GH_STUB_RELEASE_URL:-https://example.invalid/release}"
		exit 0
	fi
	if [[ "${GH_STUB_VIEW_EXISTS:-false}" == "true" ]]; then
		echo "release exists"
		exit 0
	fi
	exit 1
fi

if [[ "$1" == "release" && "$2" == "create" ]]; then
	exit 0
fi

if [[ "$1" == "release" && "$2" == "upload" ]]; then
	exit 0
fi

if [[ "$1" == "release" && "$2" == "edit" ]]; then
	exit 0
fi

echo "unexpected gh command: $*" >&2
exit 1
EOF
	chmod +x "${stub_dir}/gh"
}

test_help_and_unknown_argument_validation() {
	assert_file_contains <(bash scripts/publish_github_release.sh --help) "Usage:"
	assert_exit_failure "unknown argument validation" bash scripts/publish_github_release.sh --nope
}

test_create_release_path() {
	local assets_dir stub_dir log_file output_file output
	assets_dir="$(make_assets_dir)"
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	output_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}" "${output_file}")
	create_gh_stub "${stub_dir}"

	output="$(
		PATH="${stub_dir}:${PATH}" \
		GH_STUB_LOG="${log_file}" \
		GH_STUB_VIEW_EXISTS="false" \
		GH_STUB_RELEASE_URL="https://example.invalid/v1.2.3" \
		GITHUB_OUTPUT="${output_file}" \
			bash scripts/publish_github_release.sh \
				--tag v1.2.3 \
				--assets-dir "${assets_dir}" \
				--draft true \
				--prerelease false
	)"

	assert_file_contains "${log_file}" "gh:release view v1.2.3"
	assert_file_contains "${log_file}" "gh:release create v1.2.3 --verify-tag --target refs/tags/v1.2.3 --title v1.2.3 --draft --generate-notes"
	assert_file_contains "${log_file}" "gh:release view v1.2.3 --json url --jq .url"
	assert_file_contains <(printf '%s\n' "${output}") "release_url=https://example.invalid/v1.2.3"
	assert_file_contains "${output_file}" "release_url=https://example.invalid/v1.2.3"
}

test_create_release_with_notes_uses_notes_file() {
	local assets_dir stub_dir log_file
	assets_dir="$(make_assets_dir)"
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	create_gh_stub "${stub_dir}"

	PATH="${stub_dir}:${PATH}" \
	GH_STUB_LOG="${log_file}" \
	GH_STUB_VIEW_EXISTS="false" \
	GH_STUB_RELEASE_URL="https://example.invalid/v1.2.5" \
		bash scripts/publish_github_release.sh \
			--tag v1.2.5 \
			--assets-dir "${assets_dir}" \
			--draft true \
			--prerelease false \
			--notes "explicit notes" >/dev/null

	assert_file_contains "${log_file}" "gh:release create v1.2.5 --verify-tag --target refs/tags/v1.2.5 --title v1.2.5 --draft"
	assert_file_contains "${log_file}" "--notes-file"
	assert_file_not_contains "${log_file}" "--generate-notes"
}

test_update_release_path() {
	local assets_dir stub_dir log_file
	assets_dir="$(make_assets_dir)"
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	create_gh_stub "${stub_dir}"

	PATH="${stub_dir}:${PATH}" \
	GH_STUB_LOG="${log_file}" \
	GH_STUB_VIEW_EXISTS="true" \
	GH_STUB_RELEASE_URL="https://example.invalid/v1.2.4" \
		bash scripts/publish_github_release.sh \
			--tag v1.2.4 \
			--assets-dir "${assets_dir}" \
			--draft false \
			--prerelease true \
			--notes "release notes" >/dev/null

	assert_file_contains "${log_file}" "gh:release upload v1.2.4"
	assert_file_contains "${log_file}" "gh:release edit v1.2.4 --title v1.2.4 --draft=false --prerelease=true"
	assert_file_contains "${log_file}" "--notes-file"
}

test_invalid_inputs_fail() {
	local assets_dir
	assets_dir="$(make_assets_dir)"

	assert_exit_failure "missing required arguments" bash scripts/publish_github_release.sh --tag v1.2.3
	assert_exit_failure "invalid draft boolean" bash scripts/publish_github_release.sh --tag v1.2.3 --assets-dir "${assets_dir}" --draft maybe
	assert_exit_failure "invalid prerelease boolean" bash scripts/publish_github_release.sh --tag v1.2.3 --assets-dir "${assets_dir}" --prerelease maybe
}

test_empty_assets_fail() {
	local empty_assets_dir stub_dir log_file
	empty_assets_dir="$(mktemp -d)"
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${empty_assets_dir}" "${stub_dir}" "${log_file}")
	create_gh_stub "${stub_dir}"

	assert_exit_failure "empty assets should fail" env \
		PATH="${stub_dir}:${PATH}" \
		GH_STUB_LOG="${log_file}" \
		GH_STUB_VIEW_EXISTS="false" \
		bash scripts/publish_github_release.sh \
			--tag v1.2.3 \
			--assets-dir "${empty_assets_dir}"
}

test_assets_dir_missing_fails() {
	local stub_dir log_file
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	create_gh_stub "${stub_dir}"

	assert_exit_failure "missing assets dir should fail" env \
		PATH="${stub_dir}:${PATH}" \
		GH_STUB_LOG="${log_file}" \
		GH_STUB_VIEW_EXISTS="false" \
		bash scripts/publish_github_release.sh \
			--tag v1.2.3 \
			--assets-dir /tmp/does-not-exist-archon-release-assets
}

test_missing_gh_dependency_fails() {
	local assets_dir empty_path
	assets_dir="$(make_assets_dir)"
	empty_path="$(mktemp -d)"
	tmp_paths+=("${empty_path}")

	assert_exit_failure "missing gh dependency should fail" env \
		PATH="${empty_path}" \
		"${BASH_BIN}" scripts/publish_github_release.sh \
			--tag v1.2.3 \
			--assets-dir "${assets_dir}"
}

test_help_and_unknown_argument_validation
test_create_release_path
test_create_release_with_notes_uses_notes_file
test_update_release_path
test_invalid_inputs_fail
test_empty_assets_fail
test_assets_dir_missing_fails
test_missing_gh_dependency_fails

echo "publish_github_release_contract_tests: ok"
