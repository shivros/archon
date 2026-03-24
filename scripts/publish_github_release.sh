#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'USAGE'
Usage:
  scripts/publish_github_release.sh \
    --tag <tag> \
    --assets-dir <directory> \
    [--draft <true|false>] \
    [--prerelease <true|false>] \
    [--notes <text>]
USAGE
}

TAG_VALUE=""
ASSETS_DIR_VALUE=""
DRAFT_VALUE="true"
PRERELEASE_VALUE="false"
NOTES_VALUE=""
NOTES_FILE=""

emit_output_line() {
	local key="$1"
	local value="$2"
	if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
		echo "${key}=${value}" >>"${GITHUB_OUTPUT}"
	fi
	echo "${key}=${value}"
}

cleanup() {
	if [[ -n "${NOTES_FILE}" && -f "${NOTES_FILE}" ]]; then
		rm -f "${NOTES_FILE}"
	fi
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--tag)
		TAG_VALUE="$2"
		shift 2
		;;
	--assets-dir)
		ASSETS_DIR_VALUE="$2"
		shift 2
		;;
	--draft)
		DRAFT_VALUE="$2"
		shift 2
		;;
	--prerelease)
		PRERELEASE_VALUE="$2"
		shift 2
		;;
	--notes)
		NOTES_VALUE="$2"
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

if [[ -z "${TAG_VALUE}" || -z "${ASSETS_DIR_VALUE}" ]]; then
	echo "missing required arguments: --tag and --assets-dir" >&2
	usage >&2
	exit 1
fi

if [[ "${DRAFT_VALUE}" != "true" && "${DRAFT_VALUE}" != "false" ]]; then
	echo "invalid --draft value '${DRAFT_VALUE}': expected true or false" >&2
	exit 1
fi

if [[ "${PRERELEASE_VALUE}" != "true" && "${PRERELEASE_VALUE}" != "false" ]]; then
	echo "invalid --prerelease value '${PRERELEASE_VALUE}': expected true or false" >&2
	exit 1
fi

if [[ ! -d "${ASSETS_DIR_VALUE}" ]]; then
	echo "assets directory not found: ${ASSETS_DIR_VALUE}" >&2
	exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
	echo "missing required dependency: gh" >&2
	exit 1
fi

mapfile -t assets < <(find "${ASSETS_DIR_VALUE}" -type f \( -name '*.tar.gz' -o -name '*.zip' -o -name '*.sha256' -o -name 'SHA256SUMS.txt' \) | sort)
if [[ "${#assets[@]}" -eq 0 ]]; then
	echo "no release assets were found in ${ASSETS_DIR_VALUE}" >&2
	exit 1
fi

if [[ -n "${NOTES_VALUE}" ]]; then
	NOTES_FILE="$(mktemp)"
	printf '%s\n' "${NOTES_VALUE}" >"${NOTES_FILE}"
fi

trap cleanup EXIT

if gh release view "${TAG_VALUE}" >/dev/null 2>&1; then
	gh release upload "${TAG_VALUE}" "${assets[@]}" --clobber

	edit_args=(gh release edit "${TAG_VALUE}" --title "${TAG_VALUE}" --draft="${DRAFT_VALUE}" --prerelease="${PRERELEASE_VALUE}")
	if [[ -n "${NOTES_FILE}" ]]; then
		edit_args+=(--notes-file "${NOTES_FILE}")
	fi
	"${edit_args[@]}"
else
	create_args=(gh release create "${TAG_VALUE}" --verify-tag --target "refs/tags/${TAG_VALUE}" --title "${TAG_VALUE}")
	if [[ "${DRAFT_VALUE}" == "true" ]]; then
		create_args+=(--draft)
	fi
	if [[ "${PRERELEASE_VALUE}" == "true" ]]; then
		create_args+=(--prerelease)
	fi
	if [[ -n "${NOTES_FILE}" ]]; then
		create_args+=(--notes-file "${NOTES_FILE}")
	else
		create_args+=(--generate-notes)
	fi
	create_args+=("${assets[@]}")
	"${create_args[@]}"
fi

release_url="$(gh release view "${TAG_VALUE}" --json url --jq '.url')"
emit_output_line "release_url" "${release_url}"
