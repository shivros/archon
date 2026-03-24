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

test_ci_quality_fails_on_unformatted_go() {
	local temp_go
	temp_go="$(mktemp "${repo_root}/tmp_ci_unformatted_XXXX.go")"
	tmp_paths+=("${temp_go}")
	cat >"${temp_go}" <<'EOF'
package main
func   tmpCIContract( ){}
EOF

	if bash scripts/ci_quality.sh >/dev/null 2>&1; then
		echo "expected ci_quality.sh to fail when an unformatted Go file exists" >&2
		exit 1
	fi
	rm -f "${temp_go}"
}

test_ci_baseline_defaults_and_override() {
	local stub_dir log_file go_stub
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	go_stub="${stub_dir}/go"

	cat >"${go_stub}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'cmd:%s\n' "$*" >> "${GO_STUB_LOG:?}"
printf 'env:%s|%s|%s|%s|%s\n' \
	"${ARCHON_UI_INTEGRATION:-}" \
	"${ARCHON_CLAUDE_INTEGRATION:-}" \
	"${ARCHON_CODEX_INTEGRATION:-}" \
	"${ARCHON_OPENCODE_INTEGRATION:-}" \
	"${ARCHON_KILOCODE_INTEGRATION:-}" >> "${GO_STUB_LOG:?}"
exit 0
EOF
	chmod +x "${go_stub}"

	PATH="${stub_dir}:${PATH}" GO_STUB_LOG="${log_file}" bash scripts/ci_baseline.sh
	assert_file_contains "${log_file}" "cmd:test -timeout 6m ./..."
	assert_file_contains "${log_file}" "cmd:build ./cmd/archon"
	assert_file_contains "${log_file}" "env:disabled|disabled|disabled|disabled|disabled"

	: >"${log_file}"
	PATH="${stub_dir}:${PATH}" GO_STUB_LOG="${log_file}" ARCHON_UI_INTEGRATION="custom-ui" bash scripts/ci_baseline.sh
	assert_file_contains "${log_file}" "env:custom-ui|disabled|disabled|disabled|disabled"
}

test_ci_race_defaults() {
	local stub_dir log_file go_stub
	stub_dir="$(mktemp -d)"
	log_file="$(mktemp)"
	tmp_paths+=("${stub_dir}" "${log_file}")
	go_stub="${stub_dir}/go"

	cat >"${go_stub}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'cmd:%s\n' "$*" >> "${GO_STUB_LOG:?}"
printf 'env:%s|%s|%s|%s|%s\n' \
	"${ARCHON_UI_INTEGRATION:-}" \
	"${ARCHON_CLAUDE_INTEGRATION:-}" \
	"${ARCHON_CODEX_INTEGRATION:-}" \
	"${ARCHON_OPENCODE_INTEGRATION:-}" \
	"${ARCHON_KILOCODE_INTEGRATION:-}" >> "${GO_STUB_LOG:?}"
exit 0
EOF
	chmod +x "${go_stub}"

	PATH="${stub_dir}:${PATH}" GO_STUB_LOG="${log_file}" bash scripts/ci_race.sh
	assert_file_contains "${log_file}" "cmd:test -race ./internal/app ./internal/daemon"
	assert_file_contains "${log_file}" "env:disabled|disabled|disabled|disabled|disabled"
}

test_ci_quality_fails_on_unformatted_go
test_ci_baseline_defaults_and_override
test_ci_race_defaults

echo "ci_contract_tests: ok"
