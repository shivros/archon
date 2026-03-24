# Maintainer Build and Release Runbook

This runbook is the authoritative maintainer procedure for the current manual build/release flow.

Release policy:

- Releases are manual only.
- No automatic release publishing on push or tag creation.

## Supported Release Targets

The supported maintainer release matrix is:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`
- `windows/arm64`

These targets are source-of-truth in `scripts/release_targets.sh` and reused by manual artifact and release workflows.

Other `GOOS/GOARCH` combinations may be buildable locally with `go build`, but they are not part of the supported maintainer release workflow.

## Local Build Commands

Build to `dist/archon`:

```bash
make build
```

Verify build metadata output:

```bash
./dist/archon version
```

Build with explicit metadata:

```bash
make build \
  VERSION=v0.1.0 \
  COMMIT=$(git rev-parse --short HEAD) \
  BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
```

Legacy root-binary path (if needed):

```bash
make build-legacy
```

## Version and Build Metadata Contract

Build metadata fields:

- `version`
- `commit`
- `build_date`

These are injected from ldflags:

- `main.appVersion`
- `main.appCommit`
- `main.appBuildDate`

Defaults for local builds (via `Makefile`):

- `VERSION=dev`
- `COMMIT=<git short sha | none>`
- `BUILD_DATE=<current UTC RFC3339>`

## CI Validation Scope

Workflow: `CI` (`.github/workflows/ci.yml`)

What CI validates:

- `ubuntu-latest`, `macos-latest`, `windows-latest` baseline tests/build (`scripts/ci_baseline.sh`)
- Linux quality checks (`scripts/ci_quality.sh`)
- Workflow/script contract checks and tests for CI/build/release automation
- Workflow lint (`actionlint`)
- Linux race checks on core packages (`scripts/ci_race.sh`)

What CI does not do:

- It does not publish releases.
- It does not require secrets for normal validation.
- Provider/UI integration suites remain opt-in via environment defaults in scripts.

## Manual Artifact Build

Workflow: `Build Artifacts (Manual)` (`.github/workflows/build-artifacts.yml`)

Trigger:

- `workflow_dispatch` only

Inputs:

- `ref` (branch, tag, or commit SHA to build)
- `version` (embedded version label and artifact naming value)
- `artifact_suffix` (optional suffix, example `rc1`)

Outputs:

- Per-target archive plus per-target `.sha256`
- Aggregate `SHA256SUMS.txt`

Archive naming:

- `archon_<version><suffix>_<goos>_<goarch>.tar.gz` for linux/darwin
- `archon_<version><suffix>_<goos>_<goarch>.zip` for windows

Where artifacts appear:

- As downloadable artifacts on the workflow run page in GitHub Actions.

## Manual GitHub Release Publish

Workflow: `Release (Manual)` (`.github/workflows/release.yml`)

Trigger:

- `workflow_dispatch` only

Inputs:

- `tag` (required existing git tag, example `v1.2.3`)
- `artifact_suffix` (optional suffix, example `rc1`)
- `draft` (`true` or `false`)
- `prerelease` (`true` or `false`)
- `release_notes` (optional explicit notes; empty uses generated notes on create)

Behavior:

- Validates inputs.
- Resolves the tagged commit and build date.
- Builds the supported target matrix with release metadata.
- Creates or updates the GitHub Release for the input tag.
- Uploads archives, per-target checksums, and aggregate `SHA256SUMS.txt`.

Where artifacts appear:

- Attached to the GitHub Release.
- Also available as workflow run artifacts.

## Maintainer Flow (Commit to Release)

1. Merge the desired changes into the branch/tag source.
2. Optionally run `Build Artifacts (Manual)` for a pre-release artifact check.
3. Create and push a release tag:

```bash
git tag vX.Y.Z
git push origin vX.Y.Z
```

4. Run `Release (Manual)` with `tag=vX.Y.Z` and desired `draft/prerelease` inputs.
5. Verify attached archives and checksums.
6. If draft mode was used, publish the draft release when ready.
