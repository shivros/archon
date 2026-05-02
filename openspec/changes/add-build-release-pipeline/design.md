## Context

Archon is a Go CLI with a single binary built from `cmd/archon/`. Current build is manual (`go build -o /tmp/archon ./cmd/archon/`). The Makefile has `build`, `install`, and `uninstall` targets. No CI/CD, no release automation, no cross-compilation.

Target platforms where developers commonly code:
- linux/amd64 (workstations, cloud VMs, WSL)
- linux/arm64 (ARM cloud instances, Raspberry Pi dev)
- darwin/amd64 (older Intel Macs)
- darwin/arm64 (Apple Silicon Macs)
- windows/amd64 (common Windows dev)
- windows/arm64 (emerging ARM Windows devices, headless boxes)

The project uses Go modules (`go.mod`) and has no CGO dependencies — pure Go — so cross-compilation is straightforward.

## Goals / Non-Goals

**Goals:**
- Tagged releases (`v0.1.0`, `v0.2.0`, etc.) produce GitHub Release artifacts automatically.
- Users can install Archon with `curl -fsSL https://archon.diy/install.sh | bash` or equivalent.
- Release artifacts include compressed binaries (`.tar.gz` for unix, `.zip` for windows) and checksums.
- The `make release` target handles tagging and pushing.
- README documents all install methods.

**Non-Goals:**
- Homebrew tap (tracked as a separate future change).
- Code signing / Apple notarization / Windows Authenticode.
- Docker images or container distribution.
- Auto-release on merge to main (manual tag-triggered only for now).
- Nightly/dev builds.

## Decisions

### 1. GoReleaser for build automation
Use [GoReleaser](https://goreleaser.com/) as the release automation tool. It's the de facto standard for Go projects, handles cross-compilation natively, produces checksums, and integrates with GitHub Releases.

**Why:** No point reinventing this. GoReleaser handles all the platform matrix, archive formats, checksums, changelogs, and GitHub Release creation out of the box.

**Alternative considered:** Raw `go build` with `GOOS`/`GOARCH` in a GitHub Actions matrix. More control but more maintenance for no real benefit at this stage.

### 2. GitHub Actions for CI
Use GitHub Actions as the CI runner. Workflow triggers on pushes of tags matching `v*`.

**Why:** Native integration with GitHub Releases. Free for public repos. No additional infrastructure.

### 3. Install script at `install.sh` in repo root
A portable shell script that detects OS/arch, downloads the latest release binary from GitHub, and installs it to `/usr/local/bin/archon` (or `$HOME/.local/bin/archon` if no write permission).

**Why:** Lowest-friction install method. Works on Linux, macOS, and WSL. Users don't need Go installed.

**Alternative considered:** `go install github.com/...` — requires Go toolchain on the target machine, which defeats the purpose for non-Go developers.

### 4. Semantic versioning starting at v0.1.0
Alpha releases use `v0.x.y`. No `v1.0.0` until the API surface is stable.

**Why:** Communicates clearly that the project is alpha / breaking changes are expected.

### 5. Archive format: tar.gz for unix, zip for windows
GoReleaser default behavior — `.tar.gz` for linux/darwin, `.zip` for windows.

**Why:** Matches platform conventions. Users expect `.zip` on Windows and `.tar.gz` everywhere else.

### 6. No CGO, no libc dependencies
Archon is pure Go with no CGO. This means static binaries for linux, no runtime dependencies. Keep it that way — don't introduce CGO dependencies without updating this change.

**Why:** Static binaries are the easiest to distribute and the most portable. A single file, no dependency hell.
