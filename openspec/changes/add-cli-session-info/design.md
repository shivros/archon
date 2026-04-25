## Context

`GET /v1/sessions/{id}` on the daemon returns a `types.Session` JSON document. `internal/client/client.go:659` wraps this with `Client.GetSession(ctx, id)`. The TUI uses this via the session manager for detail rendering. The CLI has no wrapper today.

`types.Session` carries considerably more structure than the five-column `ps` table exposes: workspace/worktree ids, status, provider, pid, title, meta, tags, created/updated timestamps, flags. For programmatic consumers that already know which session they care about, the right UX is one round-trip that returns everything.

## Goals / Non-Goals

**Goals:**
- A single command that returns the full `types.Session` JSON for one session id.
- Thin wrapper over the existing daemon endpoint and client helper; no new DTO, no field whitelisting.
- JSON by default — this command's primary audience is automation.
- Optional human-readable mode for interactive inspection.

**Non-Goals:**
- A CLI-owned session DTO shape. (Same reasoning as `ps --json`.)
- Field-projection flags. Consumers compose with `jq`.
- Mutation — covered by separate proposals (`rename`, `dismiss`, `undismiss`).
- Bulk/multi-id fetch. If a caller wants details for several sessions, they have `ps --json` (already covered separately) which includes every field of every session.

## Decisions

### 1. Command name: `session`, positional id
`archon session <id>`. Not `archon get-session`, not `archon sessions show`.

**Why:** The top-level verb space is short and already mixes verbs (`start`, `kill`) with nouns (`ps`, `daemon`, `ui`, `config`). Using a noun here mirrors `config` — "give me the state of this thing." Requiring a positional id makes accidental no-arg invocations obvious.

**Alternative considered:** `archon get <id>` (shorter). Rejected — `get` is overloaded enough in CLIs that it reads as a starting position for a subcommand group we don't have.

### 2. Default output is pretty-printed JSON
A single JSON document, two-space indent, trailing newline. The same `types.Session` shape emitted per-element by `ps --json`.

**Why:** The primary audience is automation. JSON is what automation reads. Pretty-printing aids humans who happen to run it interactively; consumers can `| jq -c` if they need compact.

**Alternative considered:** Default to a human field-per-line view. Rejected — inverts the intended use case and forces automation into a flag it doesn't need.

### 3. `--format human` for a compact field-per-line view
A secondary output mode: each notable field (id, status, provider, title, workspace_id, worktree_id, pid, created_at, updated_at) printed as `KEY: value` on its own line.

**Why:** Humans asking "what does session X look like?" deserve something readable without `jq`. A field-per-line format is easier on the eye than JSON and straightforward to implement with `fmt.Fprintf`.

**Alternative considered:** Reuse the `ps` table for a single row. Rejected — a single row of a multi-column table without the multi-row context is worse than both JSON and field-per-line.

### 4. Error path: single-line stderr, non-zero exit
Missing id → usage error, exit non-zero. Session not found → daemon 404 → single-line stderr with the daemon's message, exit non-zero. Network / daemon error → single-line stderr, exit non-zero.

**Why:** Consistent with the rest of this CLI push: scripts want a clear signal.

### 5. Reuse the adapter interface
Add `GetSession(ctx, id) (*types.Session, error)` on the adapter; `controlClientAdapter` delegates to `client.Client.GetSession`. Tests use fakes.

## Risks / Trade-offs

- **Exposes the full `types.Session` shape** → Same risk as `ps --json`. Mitigation: the ps-json-output proposal already treats `types.Session` as the CLI contract; this command inherits that contract without extending it.
- **`--format human` drift** → If `types.Session` gains new fields, the human view will not show them unless updated. Mitigation: document that `--format human` is a best-effort summary and direct automation at JSON.
