## Context

The daemon exposes two endpoints for session approvals:
- `GET /v1/sessions/{id}/approvals` returns `{ approvals: []types.Approval }`, where each approval has `session_id`, `request_id` (int), `method` (provider-specific string like `exec_command` / `session/request_permission`), `params` (`json.RawMessage`), `created_at` (RFC3339). Client wrapper: `Client.ListApprovals`.
- `POST /v1/sessions/{id}/approval` accepts `ApproveSessionRequest { request_id, decision, responses?, accept_settings? }` and returns no body on success. Client wrapper: `Client.ApproveSession`.

The TUI uses these through a dedicated approval flow. No CLI command exists for either today.

Approval decisions are not a fixed set: Codex uses `allow_once`, `allow_always`, `deny` among others; Hermes/ACP uses `allow_once`, `allow_always`, `deny`, `cancelled`. New providers can add new values. The CLI has no business validating the decision string — the daemon and the provider own that.

## Goals / Non-Goals

**Goals:**
- Let CLI callers discover pending approvals and respond to a specific one.
- Keep both commands single-purpose and composable: `approvals | jq` to select a request id, then `approve --request-id ... --decision ...` to respond.
- Avoid any hidden "approve the only/latest pending" shortcut — an LLM agent that doesn't name the request id is a bug, not a feature.
- Default output is human-friendly; `--json` is the machine contract.

**Non-Goals:**
- Streaming approval events. If an agent wants live notifications, it should subscribe to the separate metadata/transcript stream (tracked in a later proposal).
- Client-side validation of decision strings — providers own the list, and it drifts.
- A combined "list + respond" interactive command — tools should do one thing; the user (or their LLM) composes.

## Decisions

### 1. Two commands, not one subcommand group
Introduce `archon approvals <id>` (list) and `archon approve <id>` (respond) as two top-level commands, not a `archon approvals list` / `archon approvals respond` subcommand group.

**Why:** Matches the existing top-level command style (`ps`, `start`, `kill`, `tail`). Introducing a nested subcommand structure for one capability would be inconsistent with the rest of the CLI. If more approval-adjacent commands accumulate later (dismiss, bulk approve, etc.), migrating to a group at that point is a straightforward rename with aliases.

**Alternative considered:** `archon approvals list|approve` subcommand group. Rejected for now — adds complexity that is not paid back until there are at least three verbs.

### 2. `approvals` default output is a human table; `--json` emits the array
Human table columns: `REQUEST_ID`, `METHOD`, `CREATED` (the latter formatted in local time with seconds precision). `--json` emits `[]types.Approval` directly (with `params` as `json.RawMessage` — passed through verbatim).

**Why:** Consistent with `ps` (default human table, `--json` opt-in). `method` is the most useful human-facing column because it tells the user what kind of approval it is. `params` is often large and provider-specific — including it in the default table would be noise; `--json` exposes it verbatim for machine consumers.

**Alternative considered:** Default to JSON. Rejected for the same reason we didn't do it for `ps` — humans running `archon approvals foo` from the shell expect something readable.

### 3. `approve` requires explicit `--request-id`, no "latest" shortcut
The command has no default/most-recent behavior. If `--request-id` is missing, the command exits with a usage error.

**Why:** Approvals are security-sensitive by definition. An accidental approval of the wrong request because "the latest one" moved between `approvals` and `approve` is exactly the failure mode we're trying to avoid. Making the caller name the id is cheap friction with outsized correctness benefit.

### 4. `--response` is repeatable, becomes the `responses` array
`--response alpha --response beta --response gamma` produces `responses: ["alpha", "beta", "gamma"]`. Order is preserved.

**Why:** Matches the daemon DTO shape (`[]string`). Repeatable string flags are the idiomatic Go-CLI pattern (already used elsewhere in `archon start --tag a --tag b`).

### 5. `--accept-settings` takes a JSON value; parsed locally and forwarded
Accepts a JSON object as a string. Decoded into `map[string]any` and placed on `AcceptSettings`. Parse errors surface before the daemon round-trip.

**Why:** `accept_settings` is advanced/provider-specific. Making it a raw JSON value keeps the CLI flexible without inventing a per-field flag shape that will inevitably drift from providers. Local parsing ensures bad JSON fails fast.

**Alternative considered:** A `--accept-settings @file.json` form similar to `send --input-items`. Reasonable, and can be added later. For v1, keep the flag value inline — approval accept-settings payloads are typically small.

### 6. Empty list is exit `0`, not exit `1`
If `archon approvals <id>` finds zero pending, it exits `0` with the table header only (or `[]` in JSON mode). Same for a session that has no approvals at all.

**Why:** An empty result is not an error. Scripts that want to treat "no pending approvals" as a signal can check output length; treating it as exit `1` would force every caller to special-case the happy path.

### 7. Reuse the adapter interface; don't bypass it
Add `ListApprovals(ctx, id)` and `ApproveSession(ctx, id, req)` to `cmd/archon/client_adapter.go`'s interface and have `controlClientAdapter` delegate to the existing `client.Client` methods. Tests use fakes as with every other command.

## Risks / Trade-offs

- **Decision-string drift** → New providers can add new decision values the CLI has never heard of. Mitigation: CLI accepts any string; daemon rejects unknowns. Caller gets a clear daemon-side error.
- **`params` JSON can be large** → The list may include big diff previews or command strings. Mitigation: human table omits `params`; `--json` includes it; no truncation on the CLI layer.
- **Request-id collisions across sessions** → `request_id` is unique per session, not globally. The CLI takes the session id as a positional argument on both commands so there is no cross-session confusion, but this must be documented clearly in the help text and README.
- **Approval expiry between list and respond** → The daemon may have already resolved the approval by the time `approve` is called. Mitigation: the daemon returns an error with a meaningful message; the CLI forwards it verbatim. No client-side caching or retry.
