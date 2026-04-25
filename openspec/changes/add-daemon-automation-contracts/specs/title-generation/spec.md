## ADDED Requirements

### Requirement: Title generation SHALL be an opt-in daemon automation feature
Archon SHALL treat asynchronous title generation as disabled unless the configured provider resolves to a supported title-generation backend. In the current product surface, setting `[title_generation].provider = "openrouter"` enables the feature; unsupported or empty provider values disable it.

#### Scenario: Empty or unsupported provider disables the feature
- **WHEN** the configured title-generation provider is empty or unsupported
- **THEN** Archon MUST treat title generation as disabled
- **AND** session or workflow creation MUST continue without waiting on or failing for title generation

### Requirement: Session and workflow creation SHALL enqueue best-effort title-generation work when the feature is enabled
When title generation is enabled, creating a session or creating a guided workflow run SHALL enqueue asynchronous title-generation work tied to the new object's current fallback title and prompt context.

#### Scenario: Session creation enqueues title generation
- **WHEN** a new session is created while title generation is enabled
- **THEN** Archon MUST enqueue a session title-generation request for that session id
- **AND** the request MUST carry a non-empty expected fallback title
- **AND** the request MUST carry prompt context derived from the creation input

#### Scenario: Workflow creation enqueues title generation
- **WHEN** a new guided workflow run is created while title generation is enabled
- **THEN** Archon MUST enqueue a workflow title-generation request for that run id
- **AND** the request MUST carry a non-empty expected current title
- **AND** the request MUST carry the workflow's user prompt context

### Requirement: Generated titles SHALL be applied with compare-and-set safety
Title generation SHALL update session and workflow titles only when the current title still matches the expected pre-generation title and the title remains eligible for AI updates. Archon SHALL NOT overwrite user-locked or manually changed titles.

#### Scenario: Locked session titles are not overwritten
- **WHEN** a queued session title-generation result arrives for a session whose title has been locked by the user
- **THEN** Archon MUST skip the generated title update

#### Scenario: Changed current title prevents stale overwrite
- **WHEN** a queued session or workflow title-generation result arrives after the visible title has changed away from the expected title
- **THEN** Archon MUST skip the generated title update

#### Scenario: Successful generated update does not lock the title
- **WHEN** a generated title update is applied successfully
- **THEN** Archon MUST update the visible title
- **AND** a generated session-title update MUST leave the title unlocked for future user-initiated renames

### Requirement: Title generation SHALL remain best-effort and MUST NOT fail the originating user action
Generation failures, empty generated titles, closed queues, or dropped background work SHALL not fail session creation or workflow creation. Successful generated updates SHALL publish the corresponding metadata update so UIs can refresh.

#### Scenario: Generator failure does not fail the original action
- **WHEN** title generation fails after a session or workflow has already been created
- **THEN** the creation action MUST remain successful
- **AND** Archon MUST NOT roll back the created session or workflow

#### Scenario: Successful generated update publishes metadata
- **WHEN** Archon applies a generated session or workflow title update
- **THEN** Archon MUST publish a metadata update event for that object so subscribers can observe the new title
