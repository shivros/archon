## Why

Archon is approaching a usable alpha with 13 completed OpenSpec changes, but there is no automated build or release pipeline. Installing Archon requires cloning the repo, having Go installed, and running `go build` manually. This doesn't scale — especially as the tool needs to run on multiple machines (Linux workstation, MacBook) and eventually be shared with others.

A proper CI/CD pipeline for cross-platform builds and GitHub Releases will:
- Make Archon installable with a single `curl | tar` on Linux and macOS
- Produce signed, checksummed release artifacts for all common dev platforms
- Enable versioned releases with semantic versioning (starting at v0.1.0)
- Lay the groundwork for a future Homebrew tap

## What Changes

- Add a GoReleaser configuration (`.goreleaser.yml`) that builds Archon for all target platforms: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64.
- Add a GitHub Actions workflow (`.github/workflows/release.yml`) triggered on version tags (`v*`) that runs GoReleaser and publishes artifacts to GitHub Releases.
- Add a `Makefile` target `release` that creates a git tag and pushes it to trigger the workflow.
- Add an install script (`install.sh`) for one-line installs via `curl | bash` — downloads the latest release binary for the current OS/arch.
- Update `README.md` with install instructions for all methods (release binary, install script, go install).

## Capabilities

### New Capabilities
- `build-release-pipeline`: Defines the GoReleaser config, GitHub Actions workflow, and install script for cross-platform builds and GitHub Releases.

### Modified Capabilities

## Impact

- **Affected code:**
  - `.goreleaser.yml` — new file: GoReleaser configuration
  - `.github/workflows/release.yml` — new file: GitHub Actions release workflow
  - `install.sh` — new file: one-line install script
  - `Makefile` — add `release` target, update `install` target
  - `README.md` — add install/release documentation section
- **Affected behavior:** Additive only. No existing commands or behavior change. Release is manual (`make release` or tag push).
- **Dependencies:** GoReleaser (runs in CI, not a local dependency). No new Go dependencies.
- **Out of scope for this change:**
  - Homebrew tap (future change)
  - Code signing / notarization (future change)
  - Auto-release on merge to main (manual tag-triggered only)
  - Docker images (future change)
