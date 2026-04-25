## Context

`archon tail` in `cmd/archon/command_tail.go` today does two things: parse `--lines N`, then call `client.TailItems(ctx, id, lines)` and `json.NewEncoder(stdout).Encode(resp.Items)` on the returned slice. That's it. On the wire it hits `GET /v1/sessions/{id}/tail?lines=N`, a one-shot JSON snapshot.

Parallel to that, `internal/client/sse.go` already implements a streaming client against `GET /v1/sessions/{id}/tail?follow=1&stream=<name>` — that is the endpoint used by the TUI for its live event feed. The daemon side is in `internal/daemon/api_streams_handlers.go`. Both the daemon and the client are production-tested today; the capability simply has no CLI surface.

So this design is almost entirely about: (1) the flag shape, (2) the NDJSON contract, (3) signal / termination handling, (4) wiring the existing SSE client into the CLI's command adapter.

## Goals / Non-Goals

**Goals:**
- Give CLI callers — especially LLM agents — a way to observe session events in real time as they arrive.
- Keep the CLI a thin wrapper: reuse `internal/client/sse.go` rather than re-implement SSE framing in `cmd/archon`.
- Preserve today's snapshot behavior exactly when `--follow` is absent.
- Make the streaming output trivial to pipe into `jq`, `grep`, `tee` without any framing surprises (NDJSON, flush after every line, exit cleanly on SIGINT).

**Non-Goals:**
- Reconnect / resume on mid-stream disconnect. V1 exits non-zero and lets the caller retry.
- Filtering events at the CLI layer — the client is the thin wire surface; filtering belongs to the consumer's `jq` pipeline (or a future dedicated proposal).
- Pretty-printing in follow mode. NDJSON must be single-line per event.
- Adding `--follow` to `transcript` / `debug` stream endpoints — separate proposal.

## Decisions

### 1. NDJSON, not a streaming JSON array or SSE passthrough
Output in follow mode is newline-delimited JSON: one complete JSON object per line, `\n` at end-of-line, stdout flushed after every line.

**Why:** NDJSON is the standard Unix pipe contract for event streams. It composes with `jq -c`, `awk`, `grep`, and every log-shipping tool. A streaming JSON array would require consumers to implement incremental JSON parsing; raw SSE would expose the `data:` prefix and blank-line framing that nobody downstream wants to strip.

**Alternative considered:** Emit raw SSE. Rejected — it leaks protocol detail to the consumer and breaks `jq` pipelines.

### 2. Reuse `internal/client/sse.go`; expose via the command adapter
Add a new method on the `sessionClientFactory` adapter interface (e.g., `StreamTail(ctx, id, stream string) (<-chan types.TailItem, func(), error)`) that delegates to the existing SSE helper in the client package. The CLI ranges the channel, encodes each item as a single JSON line to stdout.

**Why:** The SSE client already handles framing, reconnect-on-scan-error logging, and unmarshalling. Duplicating that in `cmd/archon` is a classic "two places to fix the same bug" trap.

**Alternative considered:** Dial the daemon directly from `command_tail.go`. Rejected — the `client` package is the one that owns the daemon wire contract and should stay the single source of truth.

### 3. `--stream` defaults to `combined`, only meaningful with `--follow`
Accepted values: `stdout`, `stderr`, `combined`. Default: `combined`. Without `--follow` the flag is ignored (silently, since the snapshot endpoint does not split by stream).

**Why:** `combined` is the most useful default for an LLM watching a session — it gets everything in arrival order. Explicit named values avoid ambiguity and match what the daemon endpoint already accepts. Silent-ignore without `--follow` keeps the existing CLI usage unchanged.

**Alternative considered:** Default to `stdout`. Rejected — agents watching for tool-call results and error output equally need to observe both, and asking them to specify a flag they don't yet understand is unfriendly.

### 4. SIGINT / SIGTERM cleanly cancels and exits 0
Install a `signal.Notify` handler on `SIGINT` and `SIGTERM`. On signal, cancel the stream's context, drain the remaining in-flight events already received, and return nil from `Run`. The process exits `0`.

**Why:** `Ctrl+C` is the universal "I'm done watching" gesture. Exiting non-zero on it would make scripts that intentionally `tail --follow | head -n 10` look like they failed.

**Alternative considered:** Exit non-zero on signal. Rejected — mismatches `tail -f` behavior on Linux and `kubectl logs -f`.

### 5. Daemon-side disconnect is a non-zero exit
If the SSE read loop ends because of a network error, scan error, or the daemon shuts down the connection for any reason that is not a clean EOF tied to session termination, the CLI writes a single one-line error to stderr and exits with a non-zero status. A clean session-ended close exits `0`.

**Why:** Callers need to be able to distinguish "I watched until the session ended" from "my stream broke mid-turn." Exit code is the primary signal.

### 6. `--lines` + `--follow` is allowed but changes semantics
If both are set, the CLI first streams the backfill (the last N items the daemon has buffered) in NDJSON, then transitions seamlessly to live events. This matches `tail -n N -f`.

**Why:** An agent reconnecting mid-turn needs the last few seconds of context to know what state it is rejoining. This is cheap if the daemon endpoint supports it in one request; if not, the CLI can make a snapshot request first, print those items as NDJSON, then open the stream. The implementation should check which the daemon supports in a single query-parameter form and prefer that; otherwise the two-step path.

**Alternative considered:** Disallow the combination. Rejected — it is the obvious question "how do I see what I missed?" and answering "you can't" is a usability dead end.

## Risks / Trade-offs

- **Dropped events if stdout is blocked** → A consumer pipe that stalls (e.g., a slow `jq` filter) will backpressure into the SSE reader and cause the daemon to close the connection. Mitigation: document in `--help` that the consumer is expected to keep up, and surface the daemon-side close as a non-zero exit so the caller can decide whether to retry.
- **Signal handler leak if `Run` is called in-process by tests** → Mitigation: install the signal handler scoped to the command instance, and remove it on return; tests should already isolate this via the existing command-test harness.
- **`--stream combined` vs. the daemon's actual default** → Mitigation: confirm during implementation that the daemon accepts `stream=combined` or the empty value and choose whichever is the daemon's canonical "everything" selector.
- **Contract drift if the daemon later adds a new `stream` value** → Mitigation: accept the flag value as an opaque string on the CLI side and let the daemon reject unknown values, rather than encoding the enum in the CLI; the CLI documents the three known values but does not hard-reject others.
