#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "check_duplicate_logic: scanning Go sources"

if rg -n -U --glob '*.go' 'if err != nil \{\n\s*if err != nil' cmd internal; then
  echo "check_duplicate_logic: found nested duplicate err guards" >&2
  exit 1
fi

if rg -n -U -P --glob '*.go' 'if [^{\n]+\{\n\s*return ([^\n]+)\n\s*\}\n\s*return \1\s*$' cmd internal; then
  echo "check_duplicate_logic: found redundant branches returning the same value" >&2
  exit 1
fi

echo "check_duplicate_logic: ok"
