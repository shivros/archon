## Context

`archon ps` is implemented in `cmd/archon/command_ps.go`. It calls `client.ListSessions(ctx)` (which wraps the daemon's `GET /v1/sessions`, returning `[]*types.Session`) and hands the slice to `printSessions` in `cmd/archon/command_common.go`. `printSessions` uses `text/tabwriter` to produce a fixed five-column table (`ID`, `STATUS`, `PROVIDER`, `PID`, `TITLE`) and nothing more — the richer session fields the daemon already serializes (workspace id, worktree id, meta, tags, timestamps, etc.) are dropped on the floor today.

Downstream, LLM-driven automation needs the full DTO and a stable contract: the shape of `types.Session` (or a deliberately-versioned subset of it) becomes the CLI's machine-readable contract the moment this flag ships.

## Goals / Non-Goals

**Goals:**
- Expose the full `types.Session` JSON serialization via `archon ps --json`.
- Keep the existing human table output untouched so no human user is surprised.
- Lock the JSON shape in a test so that incidental daemon-side field changes surface as intentional decisions.

**Non-Goals:**
- Redesigning the default human table.
- Inventing a CLI-owned DTO distinct from the daemon DTO (the whole point of the CLI being a thin wrapper is that it doesn't own a shape of its own).
- Filtering/querying flags for `ps` — a separate proposal.
- Extending `--json` to other CLI commands — each is its own proposal.

## Decisions

### 1. Emit the daemon's `types.Session` as-is, not a CLI-owned DTO
`archon ps --json` will marshal the same `[]*types.Session` slice the command already receives from `client.ListSessions`. No field renames, no field whitelisting, no wrapper envelope.

**Why:** The CLI's job is to be a thin, faithful surface over the daemon. Introducing a second DTO forces future daemon changes to be mirrored twice and makes drift inevitable. Consumers that want a stricter contract can use `jq` to project.

**Alternative considered:** Define a CLI-local `CLISession` struct that exposes a hand-picked field set. Rejected — it doubles maintenance cost and creates a translation layer whose only value is theoretical API stability, which `types.Session` already controls via its JSON tags.

### 2. One JSON document per invocation, pretty-printed, not NDJSON
Output is a single JSON array `[ {...}, {...} ]`, pretty-printed with two-space indent and trailing newline.

**Why:** `ps` is a point-in-time snapshot, not a stream. A single document composes cleanly with `jq` and does not require consumers to implement line-buffered streaming parsers. Pretty-printing is friendly for interactive debugging and does not meaningfully bloat output for typical session counts (≤ a few hundred).

**Alternative considered:** NDJSON. Rejected for `ps` — there is no streaming semantic here. If a future `archon tail --follow` (separate proposal) needs NDJSON, that's where it belongs.

### 3. Empty result is `[]`, not `null` or absent
If the daemon returns zero sessions, `archon ps --json` prints `[]\n` and exits 0.

**Why:** Stable shape regardless of contents; `jq`/`len` work without null-guarding.

### 4. Lock the JSON shape with a test, but do not version it
A test asserts the top-level array shape and the presence of a documented subset of fields (`id`, `status`, `provider`, `pid`, `title`). New fields on `types.Session` continue to flow through automatically; field removals or renames break the test and require a conscious update.

**Why:** Guards against the two failure modes that matter: accidental removal of a field automations rely on, and accidental change of the top-level shape. Does not over-index on a versioned envelope that nobody has asked for.

## Risks / Trade-offs

- **Exposing internal DTO fields** → Anything on `types.Session` with a JSON tag becomes part of the CLI's effective contract. Mitigation: flag this in the test comment and the README snippet so that future changes to `types.Session` get considered as CLI-facing.
- **No filtering means large outputs** → On machines with many dismissed/completed sessions, the JSON array can be large. Mitigation: acceptable for v1; filter flags are tracked as a separate proposal.
- **Default output unchanged** → Scripts today that parse the human table will keep doing so; the goal is that new automation uses `--json`. We are explicitly not deprecating the human output in this change.
