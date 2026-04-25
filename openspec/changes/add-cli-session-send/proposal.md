## Why

Archon's CLI has no way to send a message to an existing session — the only way to interact with a running session today is through the TUI's compose box. An LLM agent driving Archon through the CLI can therefore start sessions (`archon start`) and observe them (`archon tail`) but cannot actually drive a turn. That is the single biggest capability gap: without it, the CLI is a read-only observer. The daemon already exposes `POST /v1/sessions/{id}/send` (used by the TUI), and `internal/client/client.go`'s `SendMessage` already wraps it — what's missing is a CLI surface.

## What Changes

- Add a new `archon send <session-id>` command that posts to `POST /v1/sessions/{id}/send` and prints the resulting turn id on success.
- Accept message content in three mutually exclusive forms:
  - Positional text: `archon send <session-id> "message body here"` — the most common invocation.
  - `--text <string>`: equivalent to positional text; useful when the message begins with a `-` or the caller wants to be explicit.
  - `--input-items <path-or-dash>`: a path to a JSON file containing the `input` array (or `-` to read from stdin). This supports richer, provider-specific input-item payloads (tool responses, multimodal content, etc.) that cannot be expressed as plain text.
- Default output is the turn id on a single line (mirroring how `archon start` emits the new session id), exit `0` on success.
- Add a `--json` flag that emits the full response JSON (`{ ok, turn_id }`) instead of just the turn id.
- When the send fails (daemon error, session not found, session not sendable), exit non-zero and print a single-line error to stderr.

## Capabilities

### New Capabilities
- `cli-session-send`: Defines the CLI contract for sending a message to an existing session — input-form accepted (positional / `--text` / `--input-items`), default output (turn id), optional JSON output, and error-exit behavior.

### Modified Capabilities

## Impact

- **Affected code:**
  - `cmd/archon/command_send.go` — new file defining the `send` command.
  - `cmd/archon/commands.go` (or wherever commands are registered) — register `send` in the dispatcher.
  - `cmd/archon/client_adapter.go` — expose `SendMessage(ctx, sessionID, SendSessionRequest) (*SendSessionResponse, error)` on the adapter interface if not already present, delegating to the existing `client.SendMessage`.
  - `cmd/archon/commands_test.go` — add test coverage for the three input forms, default vs `--json` output, error paths, and stdin reading via `--input-items -`.
  - `cmd/archon/main.go` (or the top-level help in `main_test.go`) — add `send` to the printed command list.
  - `README.md` — document the new command with the canonical single-line invocation.
- **Affected behavior:** Purely additive. No existing command changes.
- **Dependencies:** None. Daemon endpoint and client-library method already exist.
- **Out of scope for this change:**
  - Interactive multi-turn chat against a single session (that is the TUI's job).
  - Reading-from-stdin of raw text (`archon send <id> -` for text) — can be added later if users ask, but for v1 only `--input-items -` supports stdin to keep the flag surface tight and the text-input path unambiguous.
  - Streaming the follow-up turn output in-band with the send — users combine `archon send` with `archon tail --follow` explicitly.
  - Provider-specific validation of the `input` items — the CLI passes the decoded array through as-is; the daemon owns validation.
