## Why

`archon tail <session-id>` today calls `GET /v1/sessions/{id}/tail?lines=N` and returns a single JSON array — a static snapshot of the last N items. For LLM-driven automation this is a dead end: the agent cannot react to live events (tool calls, approvals, stop reasons) because it has no way to observe them arriving. The daemon already exposes a working Server-Sent-Events stream at `GET /v1/sessions/{id}/tail?follow=1&stream=<name>`, and `internal/client/sse.go` already wraps it; what's missing is the CLI flag that lets a caller opt into the streaming mode and emit each event to stdout as it arrives.

This is the second-most-important CLI addition for agent automation (after `ps --json`): without it, there is no event loop.

## What Changes

- Add a `--follow` (short `-f`) boolean flag to `archon tail` that switches output mode from "one-shot JSON array" to "newline-delimited JSON stream" (NDJSON), one item per line.
- Add a `--stream <name>` flag accepting `stdout`, `stderr`, or `combined`, defaulting to `combined`. The flag is only meaningful with `--follow`; without `--follow` it is ignored (to keep backwards compatibility with today's snapshot invocation).
- When `--follow` is set, the CLI opens the daemon's SSE stream via the existing `internal/client/sse.go` path, writes each decoded event to stdout as a compact (single-line) JSON document followed by `\n`, and flushes after each line so pipes see events in real time.
- The streaming mode terminates cleanly on (a) `SIGINT`/`SIGTERM` from the user, (b) daemon-side stream close (session exited), or (c) daemon shutdown. On termination the CLI exits `0` for user-initiated close and non-zero for network/daemon errors, writing a single error line to stderr.
- The default invocation (`archon tail <id>` with no `--follow`) behaves exactly as today — no breaking change for existing scripts.

## Capabilities

### New Capabilities
- `cli-session-tail`: Defines the CLI contract for observing session output — snapshot mode (today's behavior) plus a new follow mode that streams NDJSON events.

### Modified Capabilities

## Impact

- **Affected code:**
  - `cmd/archon/command_tail.go` — add `--follow` and `--stream` flags, branch into the SSE client path when `--follow` is set.
  - `cmd/archon/client_adapter.go` — expose the existing SSE streaming API to the command layer (add a method on the adapter interface if not already present).
  - `cmd/archon/commands_test.go` — add coverage for the snapshot path (unchanged), the follow path (events forwarded as NDJSON), termination on context cancel, and error on daemon-side stream error.
  - `README.md` — short note in CLI docs that `tail --follow` is the streaming contract for LLM consumers.
- **Affected behavior:** Additive only — existing `archon tail` invocations keep their current JSON-array snapshot output. The new behavior is gated behind `--follow`.
- **Dependencies:** None. The daemon endpoint and client SSE wrapper already exist; this change is the CLI surface over capability that is already implemented on the wire.
- **Out of scope for this change:**
  - Reconnect / resume semantics if the daemon drops the connection mid-stream — for v1 the CLI exits with a non-zero status and leaves the caller to re-invoke.
  - Filtering (by event type / role / toolCallId) — belongs to its own proposal.
  - Adding `--follow` to other streaming-capable endpoints (`transcript`, `debug`) — tracked as a separate proposal.
  - Pretty-printing in `--follow` mode — NDJSON stays compact so consumers can use `jq -c` without post-processing.
