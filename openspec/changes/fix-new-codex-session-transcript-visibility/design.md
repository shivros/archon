## Context

Archon's session UI is transcript-first, but the new-session path currently spans several asynchronous systems:

- `submitComposeInput` starts a workspace session and clears the previous transcript view.
- `startSessionMsg` swaps compose state to the returned session ID, starts session bootstrap, and schedules a background `fetchSessionsCmd(false)`.
- transcript visibility is later hydrated by snapshot and follow-stream responses, while sidebar selection is reconciled by `sessionsWithMetaMsg`.

This means a newly created Codex session can exist in a temporary split-brain state: the compose surface knows the new session ID, but sidebar selection, transcript cache, and bootstrap hydration do not always converge quickly enough to keep the new transcript visible. The current flow also does not immediately show the initial user message for a new session the way existing-session sends do.

## Goals / Non-Goals

**Goals:**
- Make a successful new Codex session start immediately visible in the transcript pane.
- Show the first submitted user message immediately, without waiting for snapshot history to land.
- Preserve visibility while session list refresh, transcript snapshot, and transcript stream events arrive in any order.
- Reuse existing transcript cache, optimistic overlay, and bootstrap coordination patterns where possible.

**Non-Goals:**
- Redesign sidebar navigation or general session-loading behavior for all providers.
- Change daemon transcript contracts or add new API endpoints.
- Introduce new persistence layers or external dependencies.

## Decisions

### 1. Treat new-session start as a first-class transcript bootstrap state
The UI should keep an explicit "just-started session is active" state from `startSessionMsg` until snapshot/stream hydration settles. That state must be sufficient to render and update the transcript even before the sidebar refresh has reselected the session.

Why:
- The current flow already knows the canonical session ID as soon as `startSessionMsg` succeeds.
- Waiting for a later sidebar/session refresh is what creates the visible gap.

Alternative considered:
- Rely only on `sessionsWithMetaMsg` to reselect the session before rendering transcript data.
- Rejected because the refresh can lag or omit the session transiently, recreating the current UX failure.

### 2. Seed the new session with immediate visible content using existing optimistic transcript patterns
When a new Codex session is created, the UI should project the initial user prompt into the new session's visible transcript and cache immediately, then let snapshot/stream hydration replace or merge it authoritatively.

Why:
- Existing-session sends already use optimistic local transcript rendering successfully.
- The user expectation is to see the prompt they just sent, not a placeholder-only pane.

Alternative considered:
- Keep the current "Starting new session..." placeholder until snapshot history appears.
- Rejected because it still leaves the user blind during the exact window this change is trying to fix.

### 3. Make hydration order-independent
Snapshot responses, history-pending retries, stream attachment, and late sidebar selection must all target the same active started session and must not require a manual switch away/back to become visible.

Why:
- The bug is fundamentally a race between async events, not a single missing refresh call.
- Order-independent hydration lets the UI tolerate delayed `sessionsWithMetaMsg`, empty initial snapshots, and stream-first updates.

Alternative considered:
- Force a hard reload/reselection after every new session start.
- Rejected because it adds redundant work, risks flicker, and encodes the bug as policy rather than fixing the state model.

### 4. Cover the regression with lifecycle-focused tests
Add tests around:
- new-session start before sidebar refresh contains the session
- snapshot/stream hydration while the session is only compose-active
- late session-list reconciliation selecting the same already-visible session without clobbering transcript state

Why:
- The failure is timing-sensitive and easy to reintroduce during future transcript/bootstrap work.

## Risks / Trade-offs

- [Risk] Optimistic first-message rendering could duplicate the user turn when snapshot history arrives.
  Mitigation: Reuse the existing optimistic overlay / authoritative snapshot reconciliation path instead of introducing a separate render-only path.

- [Risk] Holding a started-session bootstrap state too long could mask a genuine selection change.
  Mitigation: Scope it to the active new-session flow and clear it once sidebar selection and transcript hydration have converged or the user explicitly navigates elsewhere.

- [Risk] Fixing only Codex-specific symptoms could leave the state model fragile for other providers.
  Mitigation: Keep the state machine generic, while testing the Codex path that currently exhibits the bug.

## Migration Plan

- Implement behind the existing UI/session bootstrap codepaths with no data migration.
- Verify with targeted unit tests and existing app tests for selection/transcript bootstrap behavior.
- Rollback is low-risk: the change is confined to client-side session bootstrap and can be reverted without daemon or storage changes.

## Open Questions

- None blocking. The main implementation choice is whether to represent the initial user prompt as a new-session-specific bootstrap buffer or to adapt the existing pending-send/optimistic overlay model for the session-start flow.
