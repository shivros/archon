## Context

Title generation and notifications are both asynchronous background behaviors coordinated by the daemon. They influence the user experience after session/workflow creation and after important lifecycle events, but they must remain best-effort so they never block the primary action that triggered them.

## Goals / Non-Goals

**Goals:**
- Specify when title generation is enabled and what it is allowed to update.
- Specify the compare-and-set and best-effort semantics that keep title generation safe.
- Specify notification override precedence, dispatch methods, and script-command payload delivery.

**Non-Goals:**
- Designing new AI title prompts or new notification transports.
- Reworking the UI toast layer.
- Freezing implementation-specific queue sizes or goroutine topology beyond what users depend on.

## Decisions

- Keep title generation and notifications as separate capabilities because they solve different problems and evolve independently.
- Treat title generation as asynchronous and best-effort:
  - creation paths enqueue work
  - generation failures do not fail the user action
  - compare-and-set rules prevent overwriting manual or stale titles
- Treat notification settings as layered policy:
  - global defaults from core config
  - optional worktree overrides
  - optional session overrides with highest precedence
- Specify script-command delivery explicitly because it is the most automation-sensitive notification surface.

## Risks / Trade-offs

- **Background behavior can be hard to reason about** -> The spec focuses on externally visible guarantees: enqueueing, compare-and-set, and non-blocking failure semantics.
- **Notification resolution spans multiple stores** -> The precedence rules are captured directly to avoid accidental reorderings.
- **`auto` notification dispatch depends on host capabilities** -> The spec locks fallback order while still allowing individual sinks to fail gracefully.
