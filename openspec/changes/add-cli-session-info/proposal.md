## Why

`archon ps` lists every session; the TUI shows the full detail of a single session in its sidebar. The CLI has no way to ask "give me the details of session X" — the caller has to fetch the entire list and filter with `jq`. For an LLM agent that was just told "session trn_abc123 is yours," that is wasteful and fragile. The daemon already exposes `GET /v1/sessions/{id}` and the client library already wraps it (`Client.GetSession`, returning a `*types.Session`). What is missing is the CLI surface.

## What Changes

- Add an `archon session <session-id>` command that calls `GET /v1/sessions/{id}` via the existing `Client.GetSession` and emits the full `types.Session` JSON document to stdout.
- Output is JSON by default (single pretty-printed document), not a human table — a single record has no sensible tab-separated layout, and the primary audience for this command is programmatic.
- Add a `--format human` flag (optional) that prints a compact field-per-line view for interactive inspection.
- Non-existent session id produces a single-line stderr error and a non-zero exit.

## Capabilities

### New Capabilities
- `cli-session-info`: Defines the CLI contract for querying a single session's state — the `archon session <id>` command, its JSON-by-default output, and its error-exit behavior.

### Modified Capabilities

## Impact

- **Affected code:**
  - `cmd/archon/command_session.go` — new file defining `SessionCommand`.
  - `cmd/archon/commands.go` or `main.go` — register the command.
  - `cmd/archon/client_adapter.go` — add `GetSession(ctx, id) (*types.Session, error)` to the adapter interface.
  - `cmd/archon/commands_test.go` — coverage for happy path, missing id, daemon 404, `--format human`.
  - Top-level help + `README.md` — document the new command.
- **Affected behavior:** Additive only.
- **Dependencies:** None. Daemon endpoint and client method already exist.
- **Out of scope for this change:**
  - Alternative DTO shapes — exposes `types.Session` directly, matching the `ps --json` principle of "the daemon's DTO is the contract."
  - Sub-queries (e.g. `archon session <id> --field title`) — consumers use `jq` on the JSON.
  - Any mutation ability — this is read-only; rename/dismiss/undismiss are separate proposals.
