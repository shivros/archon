#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Usage:
  scripts/build_artifact.sh \
    --goos <os> \
    --goarch <arch> \
    --version <version> \
    --commit <commit> \
    --build-date <rfc3339> \
    [--suffix <suffix>] \
    [--output-dir <dir>]
EOF
}

GOOS_VALUE=""
GOARCH_VALUE=""
VERSION_VALUE=""
COMMIT_VALUE=""
BUILD_DATE_VALUE=""
SUFFIX_VALUE=""
OUTPUT_DIR="dist/artifacts"

BINARY_NAME=""
ARCHIVE_EXT=""
BASE_NAME=""
ARCHIVE_PATH=""
CHECKSUM_PATH=""
TMP_DIR=""

parse_args() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
		--goos)
			GOOS_VALUE="$2"
			shift 2
			;;
		--goarch)
			GOARCH_VALUE="$2"
			shift 2
			;;
		--version)
			VERSION_VALUE="$2"
			shift 2
			;;
		--commit)
			COMMIT_VALUE="$2"
			shift 2
			;;
		--build-date)
			BUILD_DATE_VALUE="$2"
			shift 2
			;;
		--suffix)
			SUFFIX_VALUE="$2"
			shift 2
			;;
		--output-dir)
			OUTPUT_DIR="$2"
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

	if [[ -z "${GOOS_VALUE}" || -z "${GOARCH_VALUE}" || -z "${VERSION_VALUE}" || -z "${COMMIT_VALUE}" || -z "${BUILD_DATE_VALUE}" ]]; then
		echo "missing required arguments" >&2
		usage >&2
		exit 1
	fi
}

resolve_target_layout() {
	if [[ "${GOOS_VALUE}" == "windows" ]]; then
		BINARY_NAME="archon.exe"
		ARCHIVE_EXT="zip"
	else
		BINARY_NAME="archon"
		ARCHIVE_EXT="tar.gz"
	fi

	BASE_NAME="archon_${VERSION_VALUE}${SUFFIX_VALUE}_${GOOS_VALUE}_${GOARCH_VALUE}"
	ARCHIVE_PATH="${OUTPUT_DIR}/${BASE_NAME}.${ARCHIVE_EXT}"
	CHECKSUM_PATH="${ARCHIVE_PATH}.sha256"
}

build_binary() {
	mkdir -p "${OUTPUT_DIR}"
	TMP_DIR="$(mktemp -d)"
	ldflags="-X main.appVersion=${VERSION_VALUE} -X main.appCommit=${COMMIT_VALUE} -X main.appBuildDate=${BUILD_DATE_VALUE}"
	CGO_ENABLED=0 GOOS="${GOOS_VALUE}" GOARCH="${GOARCH_VALUE}" go build -trimpath -ldflags "${ldflags}" -o "${TMP_DIR}/${BINARY_NAME}" ./cmd/archon
}

package_binary() {
	if [[ "${GOOS_VALUE}" == "windows" ]]; then
		python - "$ARCHIVE_PATH" "${TMP_DIR}/${BINARY_NAME}" "${BINARY_NAME}" <<'PY'
import pathlib
import sys
import zipfile

archive_path = pathlib.Path(sys.argv[1])
binary_path = pathlib.Path(sys.argv[2])
archive_name = sys.argv[3]

with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED) as zf:
    zf.write(binary_path, archive_name)
PY
	else
		tar -C "${TMP_DIR}" -czf "${ARCHIVE_PATH}" "${BINARY_NAME}"
	fi
}

write_checksum() {
	if command -v sha256sum >/dev/null 2>&1; then
		(
			cd "${OUTPUT_DIR}"
			sha256sum "$(basename "${ARCHIVE_PATH}")" >"$(basename "${CHECKSUM_PATH}")"
		)
	elif command -v shasum >/dev/null 2>&1; then
		(
			cd "${OUTPUT_DIR}"
			shasum -a 256 "$(basename "${ARCHIVE_PATH}")" >"$(basename "${CHECKSUM_PATH}")"
		)
	else
		echo "missing checksum tool: sha256sum or shasum is required" >&2
		exit 1
	fi
}

emit_output_line() {
	local key="$1"
	local value="$2"
	if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
		echo "${key}=${value}" >>"${GITHUB_OUTPUT}"
	fi
	echo "${key}=${value}"
}

emit_outputs() {
	emit_output_line "artifact_name" "${BASE_NAME}"
	emit_output_line "artifact_archive" "${ARCHIVE_PATH}"
	emit_output_line "artifact_checksum" "${CHECKSUM_PATH}"
}

cleanup() {
	if [[ -n "${TMP_DIR}" && -d "${TMP_DIR}" ]]; then
		rm -rf "${TMP_DIR}"
	fi
}

main() {
	parse_args "$@"
	resolve_target_layout
	build_binary
	package_binary
	write_checksum
	emit_outputs
}

trap cleanup EXIT
main "$@"
