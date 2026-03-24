#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'USAGE'
Usage:
  scripts/release_targets.sh [--output-key <key>]
USAGE
}

OUTPUT_KEY=""

while [[ $# -gt 0 ]]; do
	case "$1" in
	--output-key)
		OUTPUT_KEY="$2"
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

targets_json='{"include":[{"goos":"linux","goarch":"amd64"},{"goos":"linux","goarch":"arm64"},{"goos":"darwin","goarch":"amd64"},{"goos":"darwin","goarch":"arm64"},{"goos":"windows","goarch":"amd64"},{"goos":"windows","goarch":"arm64"}]}'

if [[ -n "${OUTPUT_KEY}" ]]; then
	if [[ -z "${GITHUB_OUTPUT:-}" ]]; then
		echo "--output-key requires GITHUB_OUTPUT to be set" >&2
		exit 1
	fi
	echo "${OUTPUT_KEY}=${targets_json}" >>"${GITHUB_OUTPUT}"
fi

echo "${targets_json}"
