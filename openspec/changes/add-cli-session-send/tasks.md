## 1. Client Adapter Surface

- [x] 1.1 In `cmd/archon/client_adapter.go`, extend the client interface used by commands with `SendMessage(ctx context.Context, sessionID string, req clientpkg.SendSessionRequest) (*clientpkg.SendSessionResponse, error)` (using the existing package alias and types).
- [x] 1.2 Implement the new interface method on `controlClientAdapter` so it delegates to `client.Client.SendMessage` without duplicating timeout logic.
- [x] 1.3 Update the test fakes (e.g. `fakeCommandClient` in `cmd/archon/commands_test.go`) to satisfy the new interface method with a configurable stub.

## 2. Command Implementation

- [x] 2.1 Create `cmd/archon/command_send.go` with a `SendCommand` struct and constructor following the style of `TailCommand` / `PSCommand`.
- [x] 2.2 Define the `send` flag set with `--text`, `--input-items`, and `--json`; document each in the flag usage strings.
- [x] 2.3 Parse the positional session id as the first argument; return a usage error on stderr and exit non-zero if missing.
- [x] 2.4 Validate the input-form rule: exactly one of (positional text, `--text`, `--input-items`) must be present; otherwise emit a single-line usage error and exit non-zero.
- [x] 2.5 When `--input-items` is `-`, read stdin; otherwise read the file path. Decode into `[]map[string]any`; on decode error print a single-line stderr message and exit non-zero before contacting the daemon.
- [x] 2.6 Build the `SendSessionRequest` with either `Text` or `Input` populated (never both), call `client.SendMessage`, and handle the error path per the spec (single-line stderr, non-zero exit).
- [x] 2.7 On success, print `turn_id\n` to stdout by default, or the full response JSON (compact single-line, trailing `\n`) when `--json` is set. If the response omits `turn_id` in default mode, emit nothing on stdout but still exit `0`.

## 3. Command Registration

- [x] 3.1 Register `send` in the command dispatcher (wherever `ps`, `start`, `kill`, `tail` are registered in `cmd/archon/commands.go` or `main.go`).
- [x] 3.2 Update the top-level help text (printed by `archon --help` and tested in `main_test.go`) to include `send    send a message to a session`.
- [x] 3.3 Add a representative `archon send` line to the `Examples:` section of the top-level help.

## 4. Tests

- [x] 4.1 Happy-path test: positional text, assert the adapter's `SendMessage` was called with `Text` set and `Input` nil, and stdout is `turn_id\n`.
- [x] 4.2 `--text` variant: equivalent to positional text.
- [x] 4.3 `--input-items` from file: create a temp JSON file, assert it is decoded and passed as `Input`.
- [x] 4.4 `--input-items -`: pipe JSON into the command's stdin fake, assert decode and passthrough.
- [x] 4.5 Conflict cases: assert that providing two of the three input forms produces a non-zero exit and a single-line stderr message; assert no adapter call occurred.
- [x] 4.6 Missing input: no positional, no flags → usage error, no adapter call.
- [x] 4.7 Malformed `--input-items`: invalid JSON file → local parse error, no adapter call.
- [x] 4.8 `--json` output: assert the compact JSON line contains `ok` and `turn_id` and is newline-terminated.
- [x] 4.9 Daemon error surfaces as single-line stderr and non-zero exit.
- [x] 4.10 Empty `turn_id` in default mode prints nothing on stdout but exits `0`.

## 5. Documentation

- [x] 5.1 Add `send` to the `archon` commands table in `README.md`, with a one-line description and the canonical example `archon send <session-id> "hello"`.
- [x] 5.2 Document the `--input-items` JSON shape in the README (or link to the daemon DTO) so callers know what the `input` array expects.

## 6. Verification

- [x] 6.1 `go build ./...` — no regressions.
- [x] 6.2 `go test ./cmd/archon/...` — new tests green, existing tests unchanged.
- [x] 6.3 Manual smoke: start the daemon, create a codex session, run `archon send <id> "hello"`, confirm the turn id prints and the session receives the message (verified via `archon tail --follow` once that proposal lands, or via the TUI in the interim).
