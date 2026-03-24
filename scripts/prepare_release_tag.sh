#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'USAGE'
Usage:
  scripts/prepare_release_tag.sh \
    --tag <tag> \
    [--create] \
    [--push] \
    [--remote <name>] \
    [--skip-remote-check] \
    [--check-only] \
    [--dry-run]

Examples:
  scripts/prepare_release_tag.sh --tag v1.2.3
  scripts/prepare_release_tag.sh --tag v1.2.3-rc1 --create
  scripts/prepare_release_tag.sh --tag v1.2.3 --create --push
USAGE
}

TAG_VALUE=""
REMOTE_VALUE="origin"
CREATE_TAG="false"
PUSH_TAG="false"
SKIP_REMOTE_CHECK="false"
CHECK_ONLY="false"
DRY_RUN="false"
CURRENT_COMMIT=""

run_cmd() {
	if [[ "${DRY_RUN}" == "true" ]]; then
		echo "dry_run: $*"
		return 0
	fi
	"$@"
}

parse_args() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
		--tag)
			TAG_VALUE="$2"
			shift 2
			;;
		--remote)
			REMOTE_VALUE="$2"
			shift 2
			;;
		--create)
			CREATE_TAG="true"
			shift
			;;
		--push)
			PUSH_TAG="true"
			shift
			;;
		--skip-remote-check)
			SKIP_REMOTE_CHECK="true"
			shift
			;;
		--check-only)
			CHECK_ONLY="true"
			shift
			;;
		--dry-run)
			DRY_RUN="true"
			shift
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
}

validate_tag_format() {
	TAG_VALUE="$(printf '%s' "${TAG_VALUE}" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"
	if [[ -z "${TAG_VALUE}" ]]; then
		echo "missing required argument: --tag" >&2
		usage >&2
		exit 1
	fi

	# Enforce a semver-like tag that starts with v.
	# Examples: v1.2.3, v1.2.3-rc1, v1.2.3+build.7
	if [[ ! "${TAG_VALUE}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
		echo "invalid --tag '${TAG_VALUE}': expected semver-like tag (for example v1.2.3 or v1.2.3-rc1)" >&2
		exit 1
	fi
}

validate_mode_flags() {
	if [[ "${PUSH_TAG}" == "true" && "${CREATE_TAG}" != "true" ]]; then
		echo "--push requires --create so this helper can manage the full tag lifecycle." >&2
		exit 1
	fi

	if [[ "${CHECK_ONLY}" == "true" && ("${CREATE_TAG}" == "true" || "${PUSH_TAG}" == "true") ]]; then
		echo "--check-only cannot be combined with --create or --push." >&2
		exit 1
	fi

	if [[ "${CHECK_ONLY}" != "true" && "${CREATE_TAG}" != "true" && "${PUSH_TAG}" != "true" ]]; then
		CHECK_ONLY="true"
	fi
}

validate_repo_state() {
	if [[ "$(git rev-parse --is-inside-work-tree 2>/dev/null || true)" != "true" ]]; then
		echo "not inside a git worktree" >&2
		exit 1
	fi

	if ! git diff --quiet; then
		echo "working tree has unstaged changes; commit or stash them before tagging" >&2
		exit 1
	fi

	if ! git diff --cached --quiet; then
		echo "working tree has staged but uncommitted changes; commit before tagging" >&2
		exit 1
	fi

	if [[ -n "$(git ls-files --others --exclude-standard)" ]]; then
		echo "working tree has untracked files; clean or commit them before tagging" >&2
		exit 1
	fi
}

validate_tag_collisions() {
	if git rev-parse -q --verify "refs/tags/${TAG_VALUE}" >/dev/null 2>&1; then
		echo "tag already exists locally: ${TAG_VALUE}" >&2
		exit 1
	fi

	if [[ "${SKIP_REMOTE_CHECK}" != "true" ]]; then
		local remote_tags
		remote_tags="$(git ls-remote --tags "${REMOTE_VALUE}" "refs/tags/${TAG_VALUE}" || true)"
		if [[ -n "${remote_tags}" ]]; then
			echo "tag already exists on remote '${REMOTE_VALUE}': ${TAG_VALUE}" >&2
			exit 1
		fi
	fi
}

collect_context() {
	CURRENT_COMMIT="$(git rev-parse --short HEAD)"
}

print_summary() {
	echo "release_tag=${TAG_VALUE}"
	echo "remote=${REMOTE_VALUE}"
	echo "commit=${CURRENT_COMMIT}"
	echo "check_only=${CHECK_ONLY}"
	echo "create=${CREATE_TAG}"
	echo "push=${PUSH_TAG}"
	echo "skip_remote_check=${SKIP_REMOTE_CHECK}"
}

apply_tag_changes() {
	if [[ "${CHECK_ONLY}" == "true" ]]; then
		return 0
	fi

	if [[ "${CREATE_TAG}" == "true" ]]; then
		run_cmd git tag -a "${TAG_VALUE}" -m "Release ${TAG_VALUE}"
	fi

	if [[ "${PUSH_TAG}" == "true" ]]; then
		run_cmd git push "${REMOTE_VALUE}" "${TAG_VALUE}"
	fi
}

print_next_step() {
	echo "next_step: Run workflow 'Release (Manual)' with tag='${TAG_VALUE}'."
}

main() {
	parse_args "$@"
	validate_tag_format
	validate_mode_flags
	validate_repo_state
	validate_tag_collisions
	collect_context
	print_summary
	apply_tag_changes
	print_next_step
}

main "$@"
