## 1. Client Adapter Surface

- [x] 1.1 In `cmd/archon/client_adapter.go`, extend the interface with `GetSession(ctx context.Context, sessionID string) (*types.Session, error)`.
- [x] 1.2 Implement it on `controlClientAdapter`, delegating to `client.Client.GetSession`.
- [x] 1.3 Update test fakes (e.g. `fakeCommandClient`) to satisfy the new interface method.

## 2. Command Implementation

- [x] 2.1 Create `cmd/archon/command_session.go` with a `SessionCommand` struct following the style of `PSCommand`.
- [x] 2.2 Define the flag set with `--format` string defaulting to `json` (valid values: `json`, `human`).
- [x] 2.3 Parse the positional session id; missing id emits a usage error on stderr and exits non-zero.
- [x] 2.4 Call `client.GetSession(ctx, id)`; on error print a single-line stderr message and exit non-zero.
- [x] 2.5 In `json` mode (default): marshal the returned `*types.Session` with `json.MarshalIndent` (two-space indent), write to stdout with a trailing `\n`.
- [x] 2.6 In `human` mode: print `id`, `status`, `provider`, `title`, `pid`, `workspace_id`, `worktree_id`, `created_at`, `updated_at` as `KEY: value` lines (skip fields that are empty / zero).
- [x] 2.7 Reject unknown `--format` values with a single-line stderr error and exit non-zero without contacting the daemon.

## 3. Command Registration

- [x] 3.1 Register `session` in the command dispatcher.
- [x] 3.2 Update top-level help (and any test that locks its contents) to include `session  show a single session's state`.
- [x] 3.3 Add a representative `archon session <id>` line to the help `Examples:` section.

## 4. Tests

- [x] 4.1 Happy path JSON: call returns a session, assert output is valid JSON that decodes back to the same fields.
- [x] 4.2 Happy path human: assert each expected `KEY: value` line is present and nothing else is.
- [x] 4.3 Unknown `--format`: usage error, no adapter call.
- [x] 4.4 Missing session id: usage error, no adapter call.
- [x] 4.5 Daemon 404 / error: single-line stderr, non-zero exit.
- [x] 4.6 Assert that `--format json` and no `--format` flag produce byte-for-byte identical output.

## 5. Documentation

- [x] 5.1 Add `session` to the CLI commands table in `README.md`.
- [x] 5.2 Mention that output matches `types.Session`'s JSON serialization — same contract as `ps --json`.

## 6. Verification

- [x] 6.1 `go build ./...` — no regressions.
- [x] 6.2 `go test ./cmd/archon/...` — all green.
- [x] 6.3 Manual smoke: run `archon session <id> | jq .status`, then `archon session <id> --format human`, confirm both produce sensible output.
