## Context

`POST /v1/sessions/{id}/interrupt` on the daemon stops the in-flight turn for the given session and returns `200 OK` with no body. `Client.InterruptSession(ctx, id)` in `internal/client/client.go:738` wraps this as `func(ctx, id) error`. The TUI already invokes this path for its `ui.interruptSession` keybinding. No CLI surface exists.

`archon kill <id>` is the closest sibling: single positional, prints a terse success message, exits non-zero on daemon-side failure. `interrupt` is functionally identical in shape, just a different verb on a different endpoint and without the teardown semantics.

## Goals / Non-Goals

**Goals:**
- One-line command that stops a running turn.
- Mirror `archon kill`'s command shape so the CLI stays consistent.
- Non-zero exit only when something actually went wrong.

**Non-Goals:**
- Blocking until the interrupt is acknowledged by the provider. The daemon endpoint is fire-and-forget; mirroring that keeps the CLI's contract simple. Callers that need to observe the effect use `archon tail --follow`.
- Any success output. Silent success is the Unix idiom and matches `kill`.

## Decisions

### 1. Command shape: `archon interrupt <id>`, no flags
Single positional, no flags.

**Why:** No ambiguity, no alternatives to surface. Any future flag (e.g., a `--wait` mode) can be added later; v1 is the minimal useful surface.

### 2. Silent success, single-line error
On success, print nothing and exit `0`. On failure, print a single-line error to stderr and exit non-zero.

**Why:** Matches `kill`'s (current) behavior of printing "ok" only when it does — and even that is noise we avoid here because there is genuinely no user-facing result. An LLM that wants to verify the interrupt took effect watches the tail stream.

**Alternative considered:** Print "ok" on success. Rejected — noise for automation, adds nothing for humans (exit 0 is the signal).

### 3. "Nothing to interrupt" is not an error
If the daemon returns success when there is no in-flight turn, the CLI exits `0`. The daemon is the authority on interruptibility.

**Why:** The caller should not have to know whether the session is currently turning. An idempotent interrupt is safer than one that errors when you interrupt "too late."

## Risks / Trade-offs

- **Race between `interrupt` and the turn completing on its own** → Both outcomes are fine: either the interrupt cancels the turn, or the turn finishes and the interrupt is a no-op. Exit `0` in both cases. No mitigation needed.
- **No confirmation the interrupt took effect** → Users who need confirmation combine with `archon tail --follow`. Documenting this compose-with-tail pattern in the README is sufficient for v1.
