## 1. Client Adapter Surface

- [x] 1.1 In `cmd/archon/client_adapter.go`, extend the interface with `InterruptSession(ctx context.Context, sessionID string) error`.
- [x] 1.2 Implement it on `controlClientAdapter`, delegating to `client.Client.InterruptSession`.
- [x] 1.3 Update test fakes (e.g. `fakeCommandClient`) to satisfy the new interface method.

## 2. Command Implementation

- [x] 2.1 Create `cmd/archon/command_interrupt.go` with an `InterruptCommand` struct following the style of `KillCommand`.
- [x] 2.2 Parse the positional session id; missing id emits a usage error on stderr and exits non-zero without contacting the daemon.
- [x] 2.3 Call `client.InterruptSession(ctx, id)`; on error print a single-line stderr message (including the daemon's message) and exit non-zero.
- [x] 2.4 On success, return nil from `Run`; write nothing to stdout or stderr.

## 3. Command Registration

- [x] 3.1 Register `interrupt` in the command dispatcher.
- [x] 3.2 Update top-level help to include `interrupt  stop the in-flight turn for a session`.
- [x] 3.3 Add a representative `archon interrupt <id>` line to the `Examples:` section.

## 4. Tests

- [x] 4.1 Happy path: adapter called exactly once with the session id, stdout empty, stderr empty, exit 0.
- [x] 4.2 Missing session id: usage error on stderr, no adapter call, non-zero exit.
- [x] 4.3 Daemon error: single-line stderr with the daemon's message, non-zero exit.
- [x] 4.4 Daemon success (no-op): exit 0 with no output (same as happy path).

## 5. Documentation

- [x] 5.1 Add `interrupt` to the CLI commands table in `README.md`.
- [x] 5.2 Add a short note that combining `archon interrupt <id>` with `archon tail --follow <id>` is the way to observe the interrupt taking effect.

## 6. Verification

- [x] 6.1 `go build ./...` — no regressions.
- [x] 6.2 `go test ./cmd/archon/...` — all green.
- [x] 6.3 Manual smoke: start a long-running turn, run `archon interrupt <id>`, confirm the turn ends as cancelled in the TUI or via `tail --follow`.
