## Why

When a session is in the middle of a long or runaway turn, the TUI lets the user hit a key to send an interrupt — the provider stops generating, any in-flight tool call is cancelled, and the session becomes ready for the next turn. The CLI has no equivalent. An LLM agent driving Archon that notices a turn has gone off-track (e.g., via `tail --follow`) has no way to stop it short of killing the whole session with `archon kill`, which is destructive and loses the session context. The daemon already exposes `POST /v1/sessions/{id}/interrupt` and `internal/client/client.go`'s `InterruptSession` wraps it — what is missing is the CLI surface.

## What Changes

- Add an `archon interrupt <session-id>` command that calls `POST /v1/sessions/{id}/interrupt` via the existing `Client.InterruptSession` helper.
- On success, exit `0` with no stdout output (matching the pattern of `archon kill`, which prints nothing on success).
- On failure (daemon unreachable, session not found, session not in an interruptible state), print a single-line stderr error and exit non-zero.
- No flags beyond the positional session id; `--json` is unnecessary because there is no response body to emit.

## Capabilities

### New Capabilities
- `cli-session-interrupt`: Defines the CLI contract for interrupting a running turn — the `archon interrupt <id>` command, its zero-body success semantics, and its single-line stderr error behavior.

### Modified Capabilities

## Impact

- **Affected code:**
  - `cmd/archon/command_interrupt.go` — new file defining `InterruptCommand`, modeled on `KillCommand`.
  - `cmd/archon/commands.go` or `main.go` — register the command.
  - `cmd/archon/client_adapter.go` — add `InterruptSession(ctx, id) error` to the adapter interface.
  - `cmd/archon/commands_test.go` — coverage for happy path, missing id, daemon error.
  - Top-level help + `README.md` — add the command.
- **Affected behavior:** Additive only. No existing commands change.
- **Dependencies:** None. Daemon endpoint and client-library method already exist.
- **Out of scope for this change:**
  - Waiting for the interrupt to take effect (observable via `tail --follow` — consumers compose).
  - Bulk / multi-session interrupt.
  - Distinguishing "interrupt sent but session had nothing to interrupt" from an actual error — the daemon already handles this gracefully (no-op) and the CLI exits `0` in both cases.
