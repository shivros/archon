#!/usr/bin/env bash
set -euo pipefail

unformatted="$(find . -name '*.go' -not -path './.git/*' -print0 | xargs -0 gofmt -l)"
if [[ -n "${unformatted}" ]]; then
	echo "These files are not gofmt-formatted:"
	echo "${unformatted}"
	exit 1
fi

scripts/check_duplicate_logic.sh
scripts/check_deprecated_workflow_fields.sh
