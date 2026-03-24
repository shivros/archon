#!/usr/bin/env bash
set -euo pipefail

: "${ARCHON_UI_INTEGRATION:=disabled}"
: "${ARCHON_CLAUDE_INTEGRATION:=disabled}"
: "${ARCHON_CODEX_INTEGRATION:=disabled}"
: "${ARCHON_OPENCODE_INTEGRATION:=disabled}"
: "${ARCHON_KILOCODE_INTEGRATION:=disabled}"

export ARCHON_UI_INTEGRATION
export ARCHON_CLAUDE_INTEGRATION
export ARCHON_CODEX_INTEGRATION
export ARCHON_OPENCODE_INTEGRATION
export ARCHON_KILOCODE_INTEGRATION

go test -race ./internal/app ./internal/daemon
