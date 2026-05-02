## 1. GoReleaser Configuration

> **Replaced by custom build scripts.** Instead of GoReleaser, the project uses a set of shell scripts
> (`scripts/build_artifact.sh`, `scripts/release_targets.sh`, `scripts/publish_github_release.sh`)
> orchestrated by GitHub Actions workflows. This gives more control over artifact naming, checksum
> generation, and release publishing without introducing a GoReleaser dependency.

- [x] 1.1 Cross-platform build configuration for all 6 targets (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64) — implemented in `scripts/release_targets.sh` and `scripts/build_artifact.sh`.
- [x] 1.2 Build safety check (go mod tidy) — covered by `scripts/ci_baseline.sh` in CI.
- [x] 1.3 Single build from `cmd/archon/`, output binary `archon`, pure Go (CGO_ENABLED=0) — implemented in `scripts/build_artifact.sh`.
- [x] 1.4 GOOS/GOARCH matrix for all 6 platforms — implemented in `scripts/release_targets.sh`.
- [x] 1.5 Archives: `.tar.gz` for unix, `.zip` for windows. Name template: `archon_<version>_<os>_<arch>.<ext>` — implemented in `scripts/build_artifact.sh`.
- [x] 1.6 Checksum section: per-archive `.sha256` files + aggregate `SHA256SUMS.txt` — implemented in build-artifacts and release workflows.
- [x] 1.7 Release as draft (manual publish) — implemented in `.github/workflows/release.yml` (draft=true default).
- [x] 1.8 Changelog: auto-generated via GitHub Release creation — implemented in `scripts/publish_github_release.sh`.
- [x] 1.9 Validation: CI workflow validates build scripts and contract tests — implemented in `.github/workflows/ci.yml`.

## 2. GitHub Actions Workflow

- [x] 2.1 Release workflow created (`.github/workflows/release.yml`) — manual trigger with tag input.
- [x] 2.2 Trigger on manual dispatch with tag input (not auto-tag-triggered — intentional manual-only release).
- [x] 2.3 Job: checkout, setup Go (from go.mod), build artifacts with ldflags — implemented in release workflow.
- [x] 2.4 Release created as draft by default — implemented with `--draft` flag.
- [x] 2.5 CI workflow (`.github/workflows/ci.yml`) runs build, vet, test on push and PR — implemented with 3-OS matrix.

## 3. Install Script

- [x] 3.1 Create `install.sh` at repo root (executable).
- [x] 3.2 Detect OS via `uname -s` (Linux, Darwin, Windows/MSYS/Cygwin).
- [x] 3.3 Detect arch via `uname -m` (x86_64→amd64, aarch64/arm64→arm64).
- [x] 3.4 Fetch latest release tag from GitHub API.
- [x] 3.5 Download the appropriate archive (`.tar.gz` or `.zip`).
- [x] 3.6 Verify SHA256 checksum against `SHA256SUMS.txt`.
- [x] 3.7 Extract binary to `$HOME/.local/bin/archon` (or `/usr/local/bin/archon` if run with sudo).
- [x] 3.8 Print success message with installed version and path.
- [x] 3.9 Handle errors gracefully: missing tools (curl/tar), network failures, checksum mismatches, permission issues.

## 4. Makefile Targets

- [x] 4.1 Tag preparation helper: `scripts/prepare_release_tag.sh` validates, creates, and pushes tags. Makefile `release` target not needed — tag prep is a separate script.
- [x] 4.2 GoReleaser validation not applicable (not using GoReleaser).
- [x] 4.3 Install target exists in Makefile. Shell completions not yet applicable.
- [x] 4.4 Existing `build`, `test`, `install`, `uninstall` targets still work.

## 5. README Documentation

- [x] 5.1 Add "Installation" section to README with three methods: install script, download from releases, `go install`.
- [x] 5.2 Include the one-liner install command.
- [x] 5.3 Add supported platforms and link to GitHub Releases.
- [x] 5.4 `archon version` already prints version, commit, and build date.

## 6. Version Embedding

- [x] 6.1 `version` variables in `cmd/archon/build_metadata.go` read from ldflags at build time.
- [x] 6.2 Ldflags template: `-X main.appVersion=... -X main.appCommit=... -X main.appBuildDate=...` in Makefile and `scripts/build_artifact.sh`.
- [x] 6.3 `archon version` (and `archon --version`) prints version, commit, and build date.
- [x] 6.4 Runtime VCS fallback: falls back to Go build-info VCS revision when ldflags not set.
