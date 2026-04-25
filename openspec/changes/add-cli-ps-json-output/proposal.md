## Why

`archon ps` currently emits a tab-separated human table (columns `ID`, `STATUS`, `PROVIDER`, `PID`, `TITLE`). That's fine for humans, but it is the foundational state-query command that any programmatic caller — especially an LLM agent driving Archon via its CLI — has to run first to discover sessions. Column-parsing a tab-separated human format is fragile to release-to-release tweaks and does not expose the full session DTO (workspace/worktree ids, meta, timestamps, tags, etc.) that the daemon already returns. Every downstream automation task (send, approve, interrupt, tail) begins with "which sessions exist and what is their state?" — so this is the highest-leverage CLI change for enabling LLM-driven workflows.

## What Changes

- Add a `--json` flag to `archon ps` that emits the full JSON array the daemon returns from `GET /v1/sessions` to stdout, pretty-printed, as a single JSON document per invocation (not NDJSON).
- Keep the existing tab-separated human table as the default — no change for humans running `archon ps` today.
- Do not introduce a new DTO shape: the JSON output is the daemon's existing `types.Session` serialization, so the CLI stays a thin wrapper.
- Treat the JSON shape as part of the CLI contract: add a test that locks in the field set so future daemon changes that would alter it surface as an intentional decision, not an incidental break.
- Return a stable exit code of `0` on success even when the session list is empty, emitting `[]` in that case.

## Capabilities

### New Capabilities
- `cli-session-listing`: Defines the CLI contract for listing sessions — the default human table, the opt-in JSON output, and the rule that the JSON schema tracks the daemon DTO.

### Modified Capabilities

## Impact

- **Affected code:**
  - `cmd/archon/command_ps.go` — add the `--json` flag and branch output on it.
  - `cmd/archon/command_common.go` — add a JSON emitter next to `printSessions`, or inline in the command file.
  - `cmd/archon/commands_test.go` — add coverage asserting the JSON output shape and that the human table path is unchanged when the flag is absent.
  - `README.md` — short note in the existing CLI docs (optional but preferred) that `ps --json` is the machine-readable contract.
- **Affected behavior:** Additive only — existing invocations of `archon ps` keep their current output byte-for-byte. New behavior is gated behind `--json`.
- **Dependencies:** None. The daemon `GET /v1/sessions` endpoint and `client.ListSessions` already return the structured data — no daemon or client-library changes required.
- **Out of scope for this change:**
  - Changing the default (human table) output format.
  - Adding filter/query flags to `ps` (e.g., `--include-dismissed`, `--workspace`) — tracked separately.
  - Adding `--json` to other CLI commands (`session info`, `workspaces`, etc.) — each is its own proposal.
  - Introducing NDJSON or streaming output for `ps` — if that becomes valuable later it belongs in its own proposal, since it implies a different contract than "one JSON document per invocation".
