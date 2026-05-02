## ADDED Requirements

### Requirement: GoReleaser SHALL build Archon for all target platforms on tag push
When a version tag matching `v*` is pushed to the repository, GoReleaser SHALL compile the `cmd/archon/` binary for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, and windows/arm64 with `CGO_ENABLED=0`.

#### Scenario: Tag push triggers cross-platform build
- **WHEN** a tag matching `v*` is pushed to the repository
- **THEN** the GitHub Actions workflow SHALL run GoReleaser
- **AND** GoReleaser SHALL produce one binary per target platform (6 binaries total)
- **AND** each binary SHALL be statically compiled with no CGO dependencies

### Requirement: Release artifacts SHALL be archived and checksummed
GoReleaser SHALL produce platform-appropriate archives (`.tar.gz` for linux/darwin, `.zip` for windows) each containing the binary, README, and LICENSE. A `checksums.txt` file with SHA256 hashes SHALL be included in the release.

#### Scenario: Linux archive contains binary and docs
- **WHEN** a release is built
- **THEN** the linux/amd64 archive SHALL be named `archon_vX.Y.Z_linux_amd64.tar.gz`
- **AND** it SHALL contain the `archon` binary, `README.md`, and `LICENSE`

#### Scenario: Checksums file covers all archives
- **WHEN** a release is built
- **THEN** `checksums.txt` SHALL contain one SHA256 line per archive (6 lines total)

### Requirement: Releases SHALL be created as drafts
GoReleaser SHALL create GitHub Releases in draft state. A human SHALL review and publish the release manually.

#### Scenario: Release requires manual publish
- **WHEN** GoReleaser finishes building
- **THEN** the GitHub Release SHALL be in draft state
- **AND** the release SHALL NOT be visible to users until manually published

### Requirement: The install script SHALL detect OS/arch and install the correct binary
`install.sh` SHALL detect the current OS and architecture, fetch the latest release from GitHub, verify its checksum, and install the binary.

#### Scenario: Linux amd64 install
- **WHEN** `install.sh` is run on a linux/amd64 machine
- **THEN** it SHALL download `archon_vX.Y.Z_linux_amd64.tar.gz` from the latest GitHub Release
- **AND** verify the SHA256 checksum against `checksums.txt`
- **AND** extract the `archon` binary to `$HOME/.local/bin/archon`

#### Scenario: macOS arm64 install
- **WHEN** `install.sh` is run on a darwin/arm64 machine
- **THEN** it SHALL download `archon_vX.Y.Z_darwin_arm64.tar.gz`
- **AND** install to the same path

#### Scenario: Checksum mismatch aborts install
- **WHEN** the downloaded archive checksum does not match `checksums.txt`
- **THEN** the script SHALL exit non-zero with an error message
- **AND** the extracted binary SHALL be deleted

#### Scenario: Install with sudo
- **WHEN** `install.sh` is run with root privileges
- **THEN** the binary SHALL be installed to `/usr/local/bin/archon`

### Requirement: `archon --version` SHALL print the build version, commit, and date
The binary SHALL accept a `--version` flag (and/or `version` subcommand) that prints the version string, git commit hash, and build date. These values SHALL be injected via Go ldflags at build time.

#### Scenario: Version output
- **WHEN** a user runs `archon --version`
- **THEN** the output SHALL include the version (e.g. `v0.1.0`), commit hash, and build date
- **AND** exit with status code `0`

### Requirement: CI SHALL run on every push to main and on PRs
A GitHub Actions CI workflow SHALL run `go build`, `go vet`, and `go test ./...` on every push to main and on every pull request. This is independent of the release workflow.

#### Scenario: PR triggers CI
- **WHEN** a pull request is opened against main
- **THEN** the CI workflow SHALL run build, vet, and test
- **AND** block merge if any step fails
