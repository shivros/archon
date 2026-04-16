## ADDED Requirements

### Requirement: New session startup SHALL show the started session immediately
When Archon successfully starts a brand-new session, the UI SHALL bind the transcript pane to that started session immediately and SHALL not require the user to change selection or trigger a manual refresh before transcript content becomes visible.

#### Scenario: Started session becomes visible before sidebar refresh settles
- **WHEN** a new Codex session start request succeeds and the session list refresh has not yet fully reconciled sidebar selection
- **THEN** Archon MUST keep the transcript pane targeted at the newly started session
- **AND** Archon MUST continue rendering transcript updates for that session while sidebar/session metadata catches up

#### Scenario: Late session-list reconciliation does not require reselection
- **WHEN** the newly started session appears in a later `sessionsWithMeta` refresh
- **THEN** Archon MUST reconcile sidebar selection to that session automatically
- **AND** the transcript pane MUST remain visible without the user switching away and back

### Requirement: New session startup SHALL render the first prompt and hydration updates without a visibility gap
After a brand-new Codex session starts, Archon SHALL show the initial submitted user message immediately and SHALL reconcile later snapshot and stream bootstrap data without losing visibility of the active transcript.

#### Scenario: Initial user message appears immediately
- **WHEN** Archon receives a successful response for a new Codex session start
- **THEN** the transcript pane MUST show the user's first submitted message immediately
- **AND** the user MUST not have to wait for transcript snapshot history before seeing that prompt

#### Scenario: Snapshot and stream hydration replace bootstrap content cleanly
- **WHEN** transcript snapshot or stream events arrive for the newly started session after the initial local transcript is shown
- **THEN** Archon MUST reconcile the visible transcript with authoritative transcript data
- **AND** Archon MUST avoid requiring a manual reload or reselection to reveal assistant or progress output

#### Scenario: History-pending bootstrap still preserves visibility
- **WHEN** the first transcript snapshot for a newly started Codex session reports transcript history pending or otherwise requires follow-up bootstrap work
- **THEN** Archon MUST keep the new session transcript visible during the retry/follow process
- **AND** subsequent assistant or progress output for that session MUST appear without manual user intervention
