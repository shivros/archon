#!/usr/bin/env bash
# Archon install script — downloads the latest release binary from GitHub.
# Usage: curl -fsSL https://raw.githubusercontent.com/shivros/archon/main/install.sh | bash
set -euo pipefail

REPO="shivros/archon"
GITHUB_API="https://api.github.com"

# --- helpers ---

info()  { printf '\033[1;34minfo:\033[0m %s\n' "$*" >&2; }
warn()  { printf '\033[1;33mwarn:\033[0m %s\n' "$*" >&2; }
err()   { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; }
die()   { err "$@"; exit 1; }

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "required command not found: $1"
  fi
}

# --- OS/arch detection ---

detect_os() {
  local uname_out
  uname_out="$(uname -s 2>/dev/null || true)"
  case "${uname_out}" in
    Linux*)  echo "linux" ;;
    Darwin*) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *)       die "unsupported OS: ${uname_out}" ;;
  esac
}

detect_arch() {
  local uname_m
  uname_m="$(uname -m 2>/dev/null || true)"
  case "${uname_m}" in
    x86_64|amd64)   echo "amd64" ;;
    aarch64|arm64)  echo "arm64" ;;
    *)               die "unsupported architecture: ${uname_m}" ;;
  esac
}

# --- install dir resolution ---

resolve_bindir() {
  if [[ "$(id -u)" -eq 0 ]]; then
    echo "/usr/local/bin"
  else
    echo "${HOME}/.local/bin"
  fi
}

# --- GitHub latest release ---

get_latest_tag() {
  local tag
  tag="$(curl -fsSL "${GITHUB_API}/repos/${REPO}/releases/latest" 2>/dev/null \
    | grep '"tag_name"' \
    | head -1 \
    | sed -E 's/.*"tag_name"\s*:\s*"([^"]+)".*/\1/')"
  if [[ -z "${tag}" ]]; then
    die "could not determine latest release tag"
  fi
  echo "${tag}"
}

# --- download and verify ---

download_and_install() {
  local os="$1" arch="$2" tag="$3" bindir="$4"

  local version="${tag#v}"
  local ext="tar.gz"
  local binary="archon"
  if [[ "${os}" == "windows" ]]; then
    ext="zip"
    binary="archon.exe"
  fi

  local archive_name="archon_${version}_${os}_${arch}.${ext}"
  local download_url="https://github.com/${REPO}/releases/download/${tag}/${archive_name}"
  local checksums_url="https://github.com/${REPO}/releases/download/${tag}/SHA256SUMS.txt"

  local tmpdir
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' EXIT

  info "downloading ${archive_name} ..."
  curl -fsSL -o "${tmpdir}/${archive_name}" "${download_url}" \
    || die "download failed: ${download_url}"

  info "downloading checksums ..."
  curl -fsSL -o "${tmpdir}/SHA256SUMS.txt" "${checksums_url}" \
    || die "checksum download failed: ${checksums_url}"

  # Verify checksum
  local expected
  expected="$(grep "  ${archive_name}$" "${tmpdir}/SHA256SUMS.txt" | awk '{print $1}')"
  if [[ -z "${expected}" ]]; then
    die "archive not found in checksums file"
  fi

  local actual
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "${tmpdir}/${archive_name}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "${tmpdir}/${archive_name}" | awk '{print $1}')"
  else
    die "no checksum tool found (need sha256sum or shasum)"
  fi

  if [[ "${actual}" != "${expected}" ]]; then
    rm -f "${tmpdir}/${archive_name}"
    die "checksum mismatch: expected ${expected}, got ${actual}"
  fi
  info "checksum verified"

  # Extract
  info "extracting ..."
  if [[ "${ext}" == "zip" ]]; then
    need_cmd unzip
    (cd "${tmpdir}" && unzip -o "${archive_name}" "${binary}")
  else
    (cd "${tmpdir}" && tar xzf "${archive_name}" "${binary}")
  fi

  # Install
  mkdir -p "${bindir}"
  install -m 0755 "${tmpdir}/${binary}" "${bindir}/archon"

  info "installed archon ${tag} to ${bindir}/archon"
}

# --- main ---

main() {
  need_cmd curl
  need_cmd uname
  need_cmd install

  local os arch bindir tag
  os="$(detect_os)"
  arch="$(detect_arch)"
  bindir="$(resolve_bindir)"
  tag="$(get_latest_tag)"

  info "installing archon ${tag} for ${os}/${arch}"

  download_and_install "${os}" "${arch}" "${tag}" "${bindir}"

  # Verify
  if command -v "${bindir}/archon" >/dev/null 2>&1; then
    info "$("${bindir}/archon" version | head -1)"
  fi

  if [[ ":${PATH}:" != *":${bindir}:"* ]]; then
    warn "${bindir} is not in your PATH"
    warn "Add it with: export PATH=\"${bindir}:\$PATH\""
  fi
}

main "$@"
