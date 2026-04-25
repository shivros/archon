## 1. Client Adapter Surface

- [x] 1.1 In `cmd/archon/client_adapter.go`, extend the interface with `ListApprovals(ctx context.Context, sessionID string) ([]*types.Approval, error)` and `ApproveSession(ctx context.Context, sessionID string, req clientpkg.ApproveSessionRequest) error`.
- [x] 1.2 Implement both methods on `controlClientAdapter`, delegating to the existing `client.Client` methods.
- [x] 1.3 Update test fakes (e.g. `fakeCommandClient`) to satisfy the new interface methods with configurable stubs.

## 2. `archon approvals` Command

- [x] 2.1 Create `cmd/archon/command_approvals.go` with an `ApprovalsCommand` struct following the style of `PSCommand` / `TailCommand`.
- [x] 2.2 Parse the positional session id; missing id emits a usage error and exits non-zero.
- [x] 2.3 Add a `--json` boolean flag.
- [x] 2.4 Call `ListApprovals(ctx, sessionID)`; on error print a single-line stderr message and exit non-zero.
- [x] 2.5 Default output: print a tab-separated human table with header `REQUEST_ID\tMETHOD\tCREATED`, one row per approval. Format `created_at` as local time with seconds precision.
- [x] 2.6 `--json` output: marshal the returned `[]*types.Approval` slice with `json.MarshalIndent` (two-space indent), write to stdout with a trailing `\n`. Empty slice emits `[]\n`.

## 3. `archon approve` Command

- [x] 3.1 Create `cmd/archon/command_approve.go` with an `ApproveCommand` struct.
- [x] 3.2 Parse the positional session id; missing id emits a usage error and exits non-zero.
- [x] 3.3 Define flags: `--request-id int` (required), `--decision string` (required), `--response string` (repeatable), `--accept-settings string` (optional JSON object).
- [x] 3.4 Validate required flags locally; missing required flags emit single-line stderr errors and exit non-zero without contacting the daemon.
- [x] 3.5 When `--accept-settings` is provided, decode its value into `map[string]any`; decode failure emits a single-line stderr error and exits non-zero without contacting the daemon.
- [x] 3.6 Build `ApproveSessionRequest{RequestID, Decision, Responses, AcceptSettings}`, call `client.ApproveSession`, exit `0` on success; on daemon-side error print a single-line stderr message with the daemon's error text and exit non-zero.
- [x] 3.7 Ensure `responses` is omitted from the request body when `--response` is never set; include it as an ordered slice when set.

## 4. Command Registration

- [x] 4.1 Register `approvals` and `approve` in the command dispatcher (alongside `ps`, `start`, `tail`, etc.).
- [x] 4.2 Update top-level help (printed by `archon --help` and locked by tests in `main_test.go`) to include:
  - `approvals  list pending approvals for a session`
  - `approve    respond to a pending approval`
- [x] 4.3 Add a representative example to the `Examples:` section showing `archon approvals <id>` followed by `archon approve <id> --request-id N --decision allow_once`.

## 5. Tests

- [x] 5.1 `approvals` — empty list, default table output: header-only, exit 0.
- [x] 5.2 `approvals` — non-empty list, default table output: columns populated, exit 0.
- [x] 5.3 `approvals` — non-empty list, `--json` output: valid JSON, `params` preserved as raw JSON.
- [x] 5.4 `approvals` — empty list, `--json` output: exactly `[]\n`.
- [x] 5.5 `approvals` — daemon error: single-line stderr, non-zero exit.
- [x] 5.6 `approve` — missing session id / `--request-id` / `--decision`: each a usage error, no adapter call.
- [x] 5.7 `approve` — happy path: adapter called with exact request, exit 0.
- [x] 5.8 `approve` — `--response alpha --response beta` produces `responses: ["alpha","beta"]` in order.
- [x] 5.9 `approve` — `--accept-settings '{"a":1}'` decodes correctly and forwards the map.
- [x] 5.10 `approve` — malformed `--accept-settings`: single-line stderr, no adapter call.
- [x] 5.11 `approve` — daemon error: single-line stderr with the daemon's message, non-zero exit.

## 6. Documentation

- [x] 6.1 Add `approvals` and `approve` rows to the CLI commands table in `README.md` with concise descriptions.
- [x] 6.2 Add a worked example to the README: "list pending approvals, then respond" showing both commands in sequence with `jq` composing the two.

## 7. Verification

- [x] 7.1 `go build ./...` — no regressions.
- [x] 7.2 `go test ./cmd/archon/...` — new tests green, existing tests unchanged.
- [x] 7.3 Manual smoke: start a Codex session that triggers an approval (e.g., a protected command), confirm `archon approvals <id>` shows the pending request, run `archon approve <id> --request-id N --decision allow_once`, and verify the session resumes.
