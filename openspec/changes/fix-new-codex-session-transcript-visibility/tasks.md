## 1. Reproduce And Lock The Regression

- [x] 1.1 Add a regression test for the new-session flow where `startSessionMsg` succeeds before the sidebar/session refresh fully reselects the new session.
- [x] 1.2 Add coverage for transcript bootstrap arriving in different orders (snapshot first, stream first, history-pending retry) while the new session is only compose-active.
- [x] 1.3 Add an assertion that the first user prompt is visible immediately for a newly started Codex session.

## 2. Fix New-Session Bootstrap Visibility

- [x] 2.1 Update the new-session start path to keep the just-started session as the active transcript target until sidebar reconciliation catches up.
- [x] 2.2 Seed the new session transcript with immediate visible bootstrap content using the existing optimistic/transcript-cache machinery.
- [x] 2.3 Reconcile snapshot, follow-stream, and session-list refresh handling so late async events hydrate the already-visible new session instead of requiring a manual reselection.

## 3. Verify And Harden

- [x] 3.1 Add or update tests to ensure late `sessionsWithMeta` reconciliation preserves the visible transcript and does not clobber newly started session content.
- [x] 3.2 Run targeted app tests covering session bootstrap, transcript snapshot/stream handling, and selection reload behavior.
- [x] 3.3 Perform a manual smoke check of the "start brand-new Codex session and watch the transcript appear immediately" flow.
