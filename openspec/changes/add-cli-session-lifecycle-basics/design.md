## Context

The CLI already exposes the session lifecycle commands through thin wrappers around the daemon client adapter. Their behavior is simple and mostly stable, but that stability currently lives only in code and tests. This change needs to make those implicit contracts explicit without widening the feature surface.

## Goals / Non-Goals

**Goals:**
- Specify the existing contracts for `start`, `kill`, and snapshot `tail`.
- Keep the requirements aligned with the current command implementations and tests.
- Separate snapshot `tail` behavior from the already-proposed follow-mode contract.

**Non-Goals:**
- Redesigning the command UX.
- Adding new flags or changing output formats.
- Covering other CLI commands in the same change.

## Decisions

- Document each command as its own capability so future changes can evolve `start`, `kill`, and `tail` independently.
- Treat current stdout output as load-bearing:
  - `start` prints only the created session id.
  - `kill` prints `ok`.
  - snapshot `tail` prints the daemon's buffered tail items as JSON.
- Capture the current `--lines` default for snapshot `tail` in the spec so command refactors cannot silently change pagination depth.
- Keep error handling at the CLI-contract level rather than binding the spec to a particular implementation helper. The command may change internally, but users should still see single-line stderr failures and non-zero exits.

## Risks / Trade-offs

- **Locking current output may constrain future UX tweaks** -> Future UX changes can still happen, but they will require an intentional OpenSpec delta instead of drifting accidentally.
- **Snapshot `tail` behavior may differ slightly across daemon providers when the item list is empty** -> The implementation should normalize empty results to an empty JSON array so the CLI contract remains provider-agnostic.
- **The commands depend on daemon DTOs and transport helpers outside `cmd/archon`** -> The specs focus on user-visible behavior, leaving room to refactor the client adapter later.
