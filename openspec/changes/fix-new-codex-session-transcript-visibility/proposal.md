## Why

Starting a brand-new Codex session currently has a brittle bootstrap path: the UI often creates the session, but the transcript pane does not immediately show the new session's user message or subsequent assistant/progress output. In practice, the content often appears only after a manual reselection or another refresh-triggering action, which breaks the expected "start session and land in it" flow.

This needs to be fixed now because the bug affects the first moments of every new Codex conversation, makes Archon feel unreliable, and undermines the transcript-first UI model the app is built around.

## What Changes

- Make new Codex session startup immediately bind the visible transcript pane to the newly created session without waiting for a manual reselection.
- Ensure the first submitted user message for a brand-new session is visible right away while history snapshot and stream bootstrap catch up.
- Make transcript snapshot, follow-stream attachment, and sidebar/session selection reconciliation robust to the race between `startSessionMsg`, `sessionsWithMetaMsg`, and transcript bootstrap responses.
- Add regression coverage for the "new session appears only after switching away and back" failure mode.

## Capabilities

### New Capabilities
- `session-bootstrap-visibility`: Guarantees that starting a new session immediately shows the new session transcript and keeps it visible as bootstrap data arrives.

### Modified Capabilities

## Impact

- Affected code: `internal/app/model_reducers.go`, `internal/app/model_update_messages.go`, session bootstrap/selection helpers, transcript projection/bootstrap services, and related UI tests under `internal/app/*test.go`.
- Affected behavior: new-session compose flow, sidebar reselection behavior, transcript snapshot/follow handoff, and optimistic/initial transcript rendering for new Codex sessions.
- Dependencies: no new external dependencies; uses existing transcript snapshot, stream, and sidebar state machinery.
