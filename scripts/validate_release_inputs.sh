#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'USAGE'
Usage:
  scripts/validate_release_inputs.sh \
    --tag <tag> \
    [--suffix <suffix>]
USAGE
}

TAG_VALUE=""
SUFFIX_VALUE=""

emit_output_line() {
	local key="$1"
	local value="$2"
	if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
		echo "${key}=${value}" >>"${GITHUB_OUTPUT}"
	fi
	echo "${key}=${value}"
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--tag)
		TAG_VALUE="$2"
		shift 2
		;;
	--suffix)
		SUFFIX_VALUE="$2"
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

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

TAG_VALUE="$(printf '%s' "${TAG_VALUE}" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"
TAG_VALUE="${TAG_VALUE#refs/tags/}"

if [[ -z "${TAG_VALUE}" ]]; then
	echo "missing required argument: --tag" >&2
	usage >&2
	exit 1
fi

emit_output_line "tag" "${TAG_VALUE}"
"${repo_root}/scripts/validate_artifact_inputs.sh" \
	--version "${TAG_VALUE}" \
	--suffix "${SUFFIX_VALUE}"
