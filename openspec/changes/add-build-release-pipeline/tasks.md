## 1. GoReleaser Configuration

- [ ] 1.1 Create `.goreleaser.yml` at repo root with `project_name: archon`.
- [ ] 1.2 Set `before` block to run `go mod tidy` as a safety check.
- [ ] 1.3 Configure `builds` section: single build from `cmd/archon/`, output binary `archon`, pure Go (no CGO: `env: [CGO_ENABLED=0]`).
- [ ] 1.4 Set GOOS/GOARCH matrix: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64.
- [ ] 1.5 Configure `archives` section: `.tar.gz` for unix (with binary + README + LICENSE), `.zip` for windows. Name template: `archon_{{ .Version }}_{{ .Os }}_{{ .Arch }}`.
- [ ] 1.6 Configure `checksum` section: `checksums.txt` with SHA256.
- [ ] 1.7 Configure `release` section: publish to GitHub Releases, mark as draft (manual publish).
- [ ] 1.8 Configure `changelog` section: use `git log` format, group by conventional commit prefix (feat/fix/chore/docs).
- [ ] 1.9 Add `.goreleaser.yml` validation to Makefile: `make check-goreleaser` runs `goreleaser check`.

## 2. GitHub Actions Workflow

- [ ] 2.1 Create `.github/workflows/release.yml`.
- [ ] 2.2 Trigger on push of tags matching `v*`.
- [ ] 2.3 Add job: checkout, setup Go (use version from `go.mod`), run GoReleaser with `GITHUB_TOKEN`.
- [ ] 2.4 Ensure the workflow creates a GitHub Release as draft (requires manual publish after review).
- [ ] 2.5 Add a separate CI workflow (`.github/workflows/ci.yml`) that runs `go build`, `go vet`, `go test` on push to main and on PRs — basic smoke gate.

## 3. Install Script

- [ ] 3.1 Create `install.sh` at repo root (executable).
- [ ] 3.2 Detect OS via `uname -s` (Linux, Darwin, Windows/MSYS/Cygwin).
- [ ] 3.3 Detect arch via `uname -m` (x86_64→amd64, aarch64/arm64→arm64).
- [ ] 3.4 Fetch latest release tag from GitHub API (`/repos/{owner}/{repo}/releases/latest`).
- [ ] 3.5 Download the appropriate archive (`.tar.gz` or `.zip`).
- [ ] 3.6 Verify SHA256 checksum against `checksums.txt`.
- [ ] 3.7 Extract binary to `$HOME/.local/bin/archon` (or `/usr/local/bin/archon` if run with sudo).
- [ ] 3.8 Print success message with installed version and path.
- [ ] 3.9 Handle errors gracefully: missing tools (curl/tar), network failures, checksum mismatches, permission issues.

## 4. Makefile Targets

- [ ] 4.1 Add `release` target: validate that working tree is clean, prompt for version tag (or accept as argument), create annotated tag `v<version>`, push tag to origin.
- [ ] 4.2 Add `check-goreleaser` target: run `goreleaser check` (no-op if goreleaser not installed, with a warning).
- [ ] 4.3 Update `install` target to also install the shell completions if they exist.
- [ ] 4.4 Ensure existing `build`, `test`, `install`, `uninstall` targets still work.

## 5. README Documentation

- [ ] 5.1 Add "Installation" section to README with three methods: install script, download from releases, `go install`.
- [ ] 5.2 Include the one-liner install command.
- [ ] 5.3 Add "Releases" section explaining versioning scheme and how to check `archon --version`.
- [ ] 5.4 Ensure `cmd/archon/main.go` exposes a `--version` flag (or version is printed by `archon version`) if not already present.

## 6. Version Embedding

- [ ] 6.1 Add a `version` package or variable in `cmd/archon/` that reads from `ldflags` at build time (`-X main.version=...`).
- [ ] 6.2 GoReleaser `ldflags` template: `-X main.version={{ .Version }} -X main.commit={{ .Commit }} -X main.date={{ .Date }}`.
- [ ] 6.3 Ensure `archon --version` (or `archon version`) prints the version, commit, and build date.
- [ ] 6.4 If a `version` command already exists, update it to use the ldflags-injected values instead of hardcoded "dev".
