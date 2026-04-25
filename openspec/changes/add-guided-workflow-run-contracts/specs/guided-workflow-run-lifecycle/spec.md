## ADDED Requirements

### Requirement: The daemon SHALL create, list, and fetch guided workflow runs while preserving authored prompt context
Archon SHALL expose guided workflow run endpoints for creation, listing, and fetching. A created run SHALL preserve `user_prompt` when supplied, and it SHALL expose a user-facing `display_user_prompt` suitable for UI surfaces.

#### Scenario: Create/list/get preserve explicit user prompt
- **WHEN** a caller creates a workflow run with a non-empty `user_prompt`
- **THEN** the create response MUST include that `user_prompt`
- **AND** the create response MUST include the same value as `display_user_prompt`
- **AND** subsequent list and get responses for that run MUST preserve those fields

#### Scenario: Display prompt resolves from linked session metadata when needed
- **WHEN** a workflow run has no stored `user_prompt` but is linked to session metadata with an initial input
- **THEN** `GET /v1/workflow-runs/{id}` MUST expose that session initial input as `display_user_prompt`
- **AND** workflow-run list responses MUST expose the same resolved display prompt

### Requirement: Guided workflow runs SHALL support lifecycle actions through daemon endpoints
Archon SHALL expose lifecycle endpoints for start, pause, resume, stop, rename, dismiss, undismiss, and decision handling. Lifecycle actions SHALL transition the run into the matching externally visible state.

#### Scenario: Start, pause, and resume update run status
- **WHEN** a caller starts, pauses, and resumes a workflow run through the daemon endpoints
- **THEN** the run MUST transition to `running` after start
- **AND** the run MUST transition to `paused` after pause
- **AND** the run MUST transition back to `running` or a later terminal state after resume

#### Scenario: Stop marks the run stopped and records completion details
- **WHEN** a caller performs `POST /v1/workflow-runs/{id}/stop`
- **THEN** the run MUST transition to `stopped`
- **AND** the run MUST record a completion timestamp
- **AND** the run's visible error/detail text MUST indicate that the run was stopped

#### Scenario: Rename updates the workflow title
- **WHEN** a caller performs `POST /v1/workflow-runs/{id}/rename` with a new name
- **THEN** the run MUST expose that new name as its visible workflow title

#### Scenario: Decision actions update run progression
- **WHEN** a caller posts a workflow decision action of `pause_run` or `request_revision`
- **THEN** the run MUST remain or become `paused`
- **AND** when a caller posts `approve_continue`
- **THEN** the run MUST resume normal progression toward `running` or a later terminal state

### Requirement: Workflow-run dependencies SHALL queue downstream runs until their prerequisites are satisfied
Guided workflow runs MAY depend on other workflow runs. When a dependent run is started before its dependencies are satisfied, Archon SHALL queue it instead of running it immediately.

#### Scenario: Dependent run starts as queued
- **WHEN** a workflow run declares `depends_on_run_ids` and the caller starts it before the upstream run has satisfied its dependency conditions
- **THEN** the started run MUST transition to `queued`
- **AND** the run MUST NOT begin active execution until its dependencies become satisfied

### Requirement: Dismissed workflow runs SHALL be hidden by default and SHALL synchronize visibility to workflow-owned sessions
Dismissed workflow runs SHALL be excluded from the default run list, but callers SHALL be able to include them explicitly. Dismissing or undismissing a run SHALL synchronize the same visibility intent to workflow-owned or workflow-linked sessions that still belong to that run.

#### Scenario: Dismissed runs are hidden unless explicitly included
- **WHEN** a caller dismisses a workflow run
- **THEN** the default `GET /v1/workflow-runs` response MUST exclude that run
- **AND** `GET /v1/workflow-runs?include_dismissed=1` MUST include that run

#### Scenario: Dismiss and undismiss cascade to linked workflow sessions
- **WHEN** a caller dismisses or undismisses a workflow run that still owns or links sessions
- **THEN** Archon MUST apply the same dismissed visibility state to those linked sessions
- **AND** Archon MUST restore those linked sessions when the run is undismissed

### Requirement: Guided workflow run history SHALL survive daemon restarts without silently resuming interrupted active runs
Workflow run snapshots and timelines SHALL persist across daemon restarts. Runs that were already terminal or safely restorable SHALL remain visible after restart, but runs that were actively executing when the daemon stopped SHALL surface as interrupted failures rather than silently continuing.

#### Scenario: Persisted run and timeline survive restart
- **WHEN** a guided workflow run and its timeline have been persisted before daemon shutdown
- **THEN** the run and timeline MUST be available after daemon restart

#### Scenario: Running run becomes an interrupted failure after restart
- **WHEN** the daemon restarts while a workflow run was in a running in-flight state
- **THEN** Archon MUST surface that run as failed after restart
- **AND** the run's visible error/detail text MUST indicate interruption by daemon restart
- **AND** Archon MUST NOT silently resume that in-flight execution

### Requirement: Stopping a workflow run SHALL attempt best-effort interruption of owned sessions
When a workflow run is stopped, Archon SHALL best-effort interrupt any owned or linked sessions that are actively participating in that run. A failure to interrupt those sessions SHALL NOT prevent the run from transitioning to `stopped`.

#### Scenario: Stop succeeds even if session interruption fails
- **WHEN** a workflow run stop request triggers a session interruption attempt and that interruption fails
- **THEN** the workflow run MUST still transition to `stopped`
- **AND** the stop request MUST still succeed from the caller's perspective
