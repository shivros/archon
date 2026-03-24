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

make_tools_dir() {
	local tools_dir
	tools_dir="$(mktemp -d)"
	tmp_paths+=("${tools_dir}")

	local required_bins=("mkdir" "mktemp" "rm" "basename" "touch")
	for bin in "${required_bins[@]}"; do
		local resolved
		resolved="$(command -v "${bin}")"
		ln -s "${resolved}" "${tools_dir}/${bin}"
	done

	echo "${tools_dir}"
}

create_go_stub() {
	local tools_dir="$1"
	local log_file="$2"
cat >"${tools_dir}/go" <<'EOF'
#!/usr/bin/bash
set -euo pipefail
printf 'go:%s\n' "$*" >> "${GO_STUB_LOG:?}"
printf 'go_env:%s|%s|%s\n' "${GOOS:-}" "${GOARCH:-}" "${CGO_ENABLED:-}" >> "${GO_STUB_LOG:?}"
out_path=""
for ((i=1; i<=$#; i++)); do
	if [[ "${!i}" == "-o" ]]; then
		next=$((i + 1))
		out_path="${!next}"
		break
	fi
done
if [[ -n "${out_path}" ]]; then
	touch "${out_path}"
fi
exit 0
EOF
	chmod +x "${tools_dir}/go"
}

create_tar_stub() {
	local tools_dir="$1"
	local log_file="$2"
cat >"${tools_dir}/tar" <<'EOF'
#!/usr/bin/bash
set -euo pipefail
printf 'tar:%s\n' "$*" >> "${GO_STUB_LOG:?}"
archive=""
for ((i=1; i<=$#; i++)); do
	if [[ "${!i}" == "-czf" ]]; then
		next=$((i + 1))
		archive="${!next}"
		break
	fi
done
if [[ -n "${archive}" ]]; then
	touch "${archive}"
fi
exit 0
EOF
	chmod +x "${tools_dir}/tar"
}

create_python_stub() {
	local tools_dir="$1"
	local log_file="$2"
cat >"${tools_dir}/python" <<'EOF'
#!/usr/bin/bash
set -euo pipefail
printf 'python:%s\n' "$*" >> "${GO_STUB_LOG:?}"
# Invocation shape: python - <archive_path> <binary_path> <archive_name>
archive_path="${2:-}"
if [[ -n "${archive_path}" ]]; then
	touch "${archive_path}"
fi
exit 0
EOF
	chmod +x "${tools_dir}/python"
}

create_sha256sum_stub() {
	local tools_dir="$1"
cat >"${tools_dir}/sha256sum" <<'EOF'
#!/usr/bin/bash
set -euo pipefail
printf 'sha256sum:%s\n' "$*" >> "${GO_STUB_LOG:?}"
printf 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  %s\n' "$1"
exit 0
EOF
	chmod +x "${tools_dir}/sha256sum"
}

create_shasum_stub() {
	local tools_dir="$1"
cat >"${tools_dir}/shasum" <<'EOF'
#!/usr/bin/bash
set -euo pipefail
printf 'shasum:%s\n' "$*" >> "${GO_STUB_LOG:?}"
file=""
for arg in "$@"; do
	file="$arg"
done
printf 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  %s\n' "${file}"
exit 0
EOF
	chmod +x "${tools_dir}/shasum"
}

test_help_and_argument_validation() {
	assert_file_contains <("${BASH_BIN}" scripts/build_artifact.sh --help) "Usage:"

	assert_exit_failure "unknown argument validation" "${BASH_BIN}" scripts/build_artifact.sh --nope
	assert_exit_failure "missing required argument validation" "${BASH_BIN}" scripts/build_artifact.sh --goos linux --goarch amd64
}

test_linux_outputs_and_github_output() {
	local tools_dir log_file output_dir github_output output
	tools_dir="$(make_tools_dir)"
	log_file="$(mktemp)"
	output_dir="$(mktemp -d)"
	github_output="$(mktemp)"
	tmp_paths+=("${log_file}" "${output_dir}" "${github_output}")

	create_go_stub "${tools_dir}" "${log_file}"
	create_tar_stub "${tools_dir}" "${log_file}"
	create_sha256sum_stub "${tools_dir}"

	output="$(
		PATH="${tools_dir}" \
		GO_STUB_LOG="${log_file}" \
		GITHUB_OUTPUT="${github_output}" \
		"${BASH_BIN}" scripts/build_artifact.sh \
			--goos linux \
			--goarch amd64 \
			--version v-test \
			--suffix -sfx \
			--commit deadbeef \
			--build-date 2026-03-24T00:00:00Z \
			--output-dir "${output_dir}"
	)"

	assert_file_contains "${log_file}" "go_env:linux|amd64|0"
	assert_file_contains "${log_file}" "tar:-C"
	assert_file_contains "${log_file}" "sha256sum:archon_v-test-sfx_linux_amd64.tar.gz"
	assert_file_contains "${github_output}" "artifact_name=archon_v-test-sfx_linux_amd64"
	assert_file_contains "${github_output}" "artifact_archive=${output_dir}/archon_v-test-sfx_linux_amd64.tar.gz"
	assert_file_contains "${github_output}" "artifact_checksum=${output_dir}/archon_v-test-sfx_linux_amd64.tar.gz.sha256"
	assert_file_contains <(printf '%s\n' "${output}") "artifact_name=archon_v-test-sfx_linux_amd64"
	test -f "${output_dir}/archon_v-test-sfx_linux_amd64.tar.gz"
	test -f "${output_dir}/archon_v-test-sfx_linux_amd64.tar.gz.sha256"
}

test_windows_uses_python_and_shasum_fallback() {
	local tools_dir log_file output_dir github_output
	tools_dir="$(make_tools_dir)"
	log_file="$(mktemp)"
	output_dir="$(mktemp -d)"
	github_output="$(mktemp)"
	tmp_paths+=("${log_file}" "${output_dir}" "${github_output}")

	create_go_stub "${tools_dir}" "${log_file}"
	create_python_stub "${tools_dir}" "${log_file}"
	create_shasum_stub "${tools_dir}"

	# No sha256sum in PATH so shasum fallback must be used.
	PATH="${tools_dir}" \
	GO_STUB_LOG="${log_file}" \
	GITHUB_OUTPUT="${github_output}" \
	"${BASH_BIN}" scripts/build_artifact.sh \
		--goos windows \
		--goarch arm64 \
		--version v-test \
		--commit deadbeef \
		--build-date 2026-03-24T00:00:00Z \
		--output-dir "${output_dir}" >/dev/null

	assert_file_contains "${log_file}" "go_env:windows|arm64|0"
	assert_file_contains "${log_file}" "python:- ${output_dir}/archon_v-test_windows_arm64.zip"
	assert_file_contains "${log_file}" "shasum:-a 256 archon_v-test_windows_arm64.zip"
	test -f "${output_dir}/archon_v-test_windows_arm64.zip"
	test -f "${output_dir}/archon_v-test_windows_arm64.zip.sha256"
}

test_missing_checksum_tool_fails() {
	local tools_dir log_file output_dir err_file
	tools_dir="$(make_tools_dir)"
	log_file="$(mktemp)"
	output_dir="$(mktemp -d)"
	err_file="$(mktemp)"
	tmp_paths+=("${log_file}" "${output_dir}" "${err_file}")

	create_go_stub "${tools_dir}" "${log_file}"
	create_tar_stub "${tools_dir}" "${log_file}"

	if PATH="${tools_dir}" GO_STUB_LOG="${log_file}" "${BASH_BIN}" scripts/build_artifact.sh \
		--goos linux \
		--goarch amd64 \
		--version v-test \
		--commit deadbeef \
		--build-date 2026-03-24T00:00:00Z \
		--output-dir "${output_dir}" >/dev/null 2>"${err_file}"; then
		echo "assertion failed: missing checksum tool branch should fail" >&2
		exit 1
	fi

	assert_file_contains "${err_file}" "missing checksum tool: sha256sum or shasum is required"
}

test_help_and_argument_validation
test_linux_outputs_and_github_output
test_windows_uses_python_and_shasum_fallback
test_missing_checksum_tool_fails

echo "build_artifact_contract_tests: ok"
