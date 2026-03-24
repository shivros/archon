#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'USAGE'
Usage:
  scripts/validate_artifact_inputs.sh \
    --version <version> \
    [--suffix <suffix>]
USAGE
}

VERSION_VALUE=""
SUFFIX_VALUE=""

trim_spaces() {
	printf '%s' "$1" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//'
}

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
	--version)
		VERSION_VALUE="$2"
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

VERSION_VALUE="$(trim_spaces "${VERSION_VALUE}")"
SUFFIX_VALUE="$(trim_spaces "${SUFFIX_VALUE}")"
SUFFIX_VALUE="${SUFFIX_VALUE#-}"

if [[ -z "${VERSION_VALUE}" ]]; then
	echo "missing required argument: --version" >&2
	usage >&2
	exit 1
fi

if [[ ! "${VERSION_VALUE}" =~ ^[0-9A-Za-z._-]+$ ]]; then
	echo "invalid version '${VERSION_VALUE}'. Allowed chars: [0-9A-Za-z._-]" >&2
	exit 1
fi

if [[ -n "${SUFFIX_VALUE}" && ! "${SUFFIX_VALUE}" =~ ^[0-9A-Za-z._-]+$ ]]; then
	echo "invalid suffix '${SUFFIX_VALUE}'. Allowed chars: [0-9A-Za-z._-]" >&2
	exit 1
fi

NORMALIZED_SUFFIX=""
if [[ -n "${SUFFIX_VALUE}" ]]; then
	NORMALIZED_SUFFIX="-${SUFFIX_VALUE}"
fi

emit_output_line "version" "${VERSION_VALUE}"
emit_output_line "suffix" "${NORMALIZED_SUFFIX}"
