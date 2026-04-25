## Context

The daemon already accepts `POST /v1/sessions/{id}/send` with a body of `{ text?: string, input?: array }` and returns `{ ok: bool, turn_id?: string }`. `internal/client/client.go`'s `SendMessage` wraps this with a 5-minute extended timeout (via `doJSONWithTimeout`) because the daemon may block briefly while the provider accepts the turn. `internal/daemon/api.go` owns the daemon-side DTOs; the shape is mirrored on the client side in `internal/client/dto.go`.

The TUI uses this same endpoint via its compose box. The CLI has no equivalent surface.

## Goals / Non-Goals

**Goals:**
- Expose the existing `POST /v1/sessions/{id}/send` capability through the CLI so programmatic callers can drive turns.
- Accept both simple text (the common case) and structured `input` items (the richer case) without forcing either caller to construct what they don't need.
- Keep the default output minimal and scriptable (single line: the turn id), with a `--json` escape hatch.
- Match the ergonomics of sibling commands (`archon start` prints the new session id; `archon send` prints the new turn id).

**Non-Goals:**
- Replacing the TUI's compose experience.
- Streaming the resulting turn's output as part of the send command (compose with `archon tail --follow`).
- Validating provider-specific `input` item shapes.
- Multi-turn interactive prompting.

## Decisions

### 1. Three input forms, mutually exclusive
The command accepts content via:
- positional arg: `archon send <id> "hello"`
- flag: `archon send <id> --text "hello"`
- file/stdin: `archon send <id> --input-items items.json` or `archon send <id> --input-items -`

If more than one is provided, the command exits non-zero with a clear error. If none is provided, the command exits non-zero with a usage error.

**Why:** Each form solves a distinct real problem. Positional is the 90% case for LLM agents typing a prompt. `--text` is the escape hatch for messages that start with `-` or need flag-like characters. `--input-items` is the only way to send provider-specific structured content (tool outputs, multimodal parts). Mutual exclusion is stricter than merging because merging creates a silent ambiguity ("which won?") that is much worse than a clear error.

**Alternative considered:** Allow `--text` plus `--input-items` to be merged into the same request (text becomes the first item). Rejected — the daemon treats `text` and `input` as peers, and the merge semantics would be CLI-specific invention. Callers that need both can pass both via a single `--input-items` JSON document.

### 2. Default output is the turn id on one line; `--json` for the full response
`archon send <id> "hello"` prints `trn_abc123\n` and exits `0`. `archon send <id> "hello" --json` prints `{"ok":true,"turn_id":"trn_abc123"}\n`.

**Why:** Sibling consistency (`archon start` prints the session id). For scripts that only need the id to pipe into a later command, the default form is the cleanest shape. The JSON form is there for consumers that want the `ok` flag or any future fields.

**Alternative considered:** Default to JSON. Rejected — harder to pipe into `archon tail`/`archon approvals`/etc. without a `jq` intermediary. JSON is opt-in for exactly the callers who want it.

### 3. `--input-items -` reads from stdin; otherwise a path
The `--input-items` value is interpreted as `os.Stdin` when it equals `-`, and as a file path otherwise. The CLI reads the full content, decodes it as JSON into `[]map[string]any`, and puts it on `SendSessionRequest.Input`. A decode failure produces a single-line error to stderr with the JSON error offset, and the command exits non-zero without hitting the daemon.

**Why:** `-` as stdin is a near-universal Unix idiom (`cat -`, `git apply -`). Pre-validating the JSON avoids wasting a daemon round-trip on obviously malformed input and gives the caller the parse error at the right layer.

### 4. Error path is one line on stderr, non-zero exit
Any error — daemon unreachable, session not found, daemon-side validation failure, JSON decode failure on `--input-items` — produces a single-line human-readable error on stderr and a non-zero exit. The command does NOT print a stack trace.

**Why:** Scripts need a clean signal. Multi-line errors complicate log collection and grep-ability.

### 5. Extend the adapter interface, don't bypass it
Add `SendMessage(ctx, sessionID, req) (*SendSessionResponse, error)` to the `cmd/archon` client adapter interface and have `controlClientAdapter` delegate to the existing `client.Client.SendMessage`. Tests use a fake that satisfies the same interface.

**Why:** Keeps the CLI's daemon-contact surface uniform and testable. Every other command already goes through the adapter; carving a direct path for `send` would be an invitation to drift.

## Risks / Trade-offs

- **5-minute client timeout for a one-shot send** → The existing `SendMessage` uses a 5-minute timeout because the daemon may block on provider readiness. For CLI callers that want to fail fast, this is longer than they expect. Mitigation: keep the existing timeout for v1 and revisit if reports come in; document it in `archon send -h`.
- **Silent success when `turn_id` is empty** → The daemon may respond `{ ok: true }` without a turn id in some race conditions. Mitigation: if default output is requested and `turn_id` is empty, print nothing (not an error) and rely on exit `0` + `--json` for callers that want to assert on the response shape explicitly.
- **`--input-items` lets callers send any JSON the daemon accepts** → That includes shapes the provider may reject asymmetrically. Mitigation: document that `--input-items` is a passthrough and validation is the daemon's responsibility; surface the daemon's error message verbatim on failure.
