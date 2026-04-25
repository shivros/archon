## Why

Archon sessions often pause mid-turn to request approval — for a dangerous shell command, a sensitive file write, a tool that exceeds access boundaries. The TUI surfaces these as interactive prompts and lets the user approve or decline with a keystroke. The CLI has nothing: no way to see pending approvals, no way to answer one. An LLM agent driving Archon therefore cannot progress past the first gated step of a Codex or Hermes session. The daemon already exposes both endpoints (`GET /v1/sessions/{id}/approvals`, `POST /v1/sessions/{id}/approval`) and the client library wraps them (`ListApprovals`, `ApproveSession`) — what's missing is the CLI surface.

## What Changes

- Add an `archon approvals <session-id>` command that lists pending approvals for a session by calling `GET /v1/sessions/{id}/approvals`. Default output is a human-readable table (columns `REQUEST_ID`, `METHOD`, `CREATED`); a `--json` flag emits the full `[]types.Approval` array.
- Add an `archon approve <session-id>` command that responds to a specific approval by calling `POST /v1/sessions/{id}/approval`. Required flags:
  - `--request-id <int>`: the approval's request id (from `archon approvals`).
  - `--decision <value>`: the approval decision string (e.g., `allow`, `allow_once`, `allow_always`, `deny`, depending on provider).
  - `--response <string>` (repeatable): zero or more string responses, accumulated into the `responses` array.
  - `--accept-settings <json>`: optional JSON object for `accept_settings` (advanced; provider-specific).
- The `approve` command exits `0` on a successful daemon acknowledgement, non-zero on any failure, with a single-line stderr error message.
- Both commands treat an empty result as a normal success (no pending approvals → exit `0` with an empty table or `[]` depending on `--json`).

## Capabilities

### New Capabilities
- `cli-session-approvals`: Defines the CLI contract for listing pending approvals and responding to a specific approval — covers both `archon approvals` and `archon approve`, their flags, output shapes, and exit-code behavior.

### Modified Capabilities

## Impact

- **Affected code:**
  - `cmd/archon/command_approvals.go` — new file: `ApprovalsCommand` (list).
  - `cmd/archon/command_approve.go` — new file: `ApproveCommand` (respond).
  - `cmd/archon/commands.go` or `main.go` — register both commands in the dispatcher.
  - `cmd/archon/client_adapter.go` — add `ListApprovals` and `ApproveSession` to the adapter interface (delegating to existing client methods).
  - `cmd/archon/commands_test.go` — coverage for empty/non-empty lists, `--json` vs. table, each decision variant, validation of `--accept-settings` JSON, error paths.
  - `cmd/archon/main.go` / help text — add both commands to the top-level help.
  - `README.md` — document the pair in the CLI reference with a canonical "approve a pending request" example.
- **Affected behavior:** Additive only. No existing commands change.
- **Dependencies:** None. Daemon endpoints and client-library methods already exist.
- **Out of scope for this change:**
  - Streaming approval events (tracked under the forthcoming metadata/stream or transcript/stream work).
  - A `--watch` mode on `approvals` — can be added later if demanded; v1 is point-in-time snapshot.
  - Decision-string validation on the CLI side — decisions are provider-specific; the CLI passes the value through and lets the daemon reject unknowns.
  - An "approve the latest / only pending" shortcut — require an explicit `--request-id` in v1 to avoid accidental auto-approvals.
