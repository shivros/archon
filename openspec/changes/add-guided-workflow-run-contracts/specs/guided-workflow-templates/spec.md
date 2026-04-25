## ADDED Requirements

### Requirement: Guided workflow templates SHALL come from either the user replacement file or the built-in defaults
Archon SHALL source guided workflow templates from `~/.archon/workflow_templates.json` when that file exists. When it does not exist, Archon SHALL use the built-in default workflow template catalog. The user file SHALL fully replace built-in defaults rather than merge with them.

#### Scenario: User template file replaces built-in defaults
- **WHEN** `~/.archon/workflow_templates.json` exists and defines a custom template set
- **THEN** Archon MUST use the user-defined templates for guided workflow listing
- **AND** Archon MUST NOT merge built-in templates into that result automatically

#### Scenario: Built-in defaults are used when no user template file exists
- **WHEN** `~/.archon/workflow_templates.json` does not exist
- **THEN** Archon MUST expose the built-in default workflow templates
- **AND** the default catalog MUST include the repo's built-in template set such as `solid_phase_delivery`

### Requirement: The daemon SHALL expose the resolved workflow template catalog through `GET /v1/workflow-templates`
Archon SHALL provide a daemon endpoint that returns the currently resolved workflow templates, regardless of whether they come from the user replacement file or from built-in defaults.

#### Scenario: Template endpoint returns the resolved catalog
- **WHEN** a caller performs `GET /v1/workflow-templates`
- **THEN** the daemon MUST return the resolved template list as JSON
- **AND** the returned templates MUST match the currently active source-of-truth catalog
