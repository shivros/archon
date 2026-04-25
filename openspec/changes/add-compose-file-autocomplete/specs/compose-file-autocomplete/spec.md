## ADDED Requirements

### Requirement: Compose SHALL support `@`-triggered file autocomplete through a provider-agnostic UI contract
When the active compose session uses a provider with file-search support, typing an `@`-style file fragment SHALL open the compose file-autocomplete flow and display normalized file candidates through one shared picker experience.

#### Scenario: Supported provider opens the file picker
- **WHEN** the active compose session uses a supported provider and the user types a file-search fragment beginning with `@`
- **THEN** Archon MUST start a provider-backed file search for that fragment
- **AND** Archon MUST present the results through the compose autocomplete picker

### Requirement: Unsupported providers SHALL degrade gracefully to plain text
Providers that do not advertise compose file-search support SHALL NOT open the file picker. For those providers, `@` input SHALL remain ordinary compose text.

#### Scenario: Unsupported provider leaves `@` as normal text
- **WHEN** the active compose session uses a provider without file-search support and the user types `@main`
- **THEN** Archon MUST NOT open the compose autocomplete picker
- **AND** the typed `@main` text MUST remain in the compose buffer as ordinary text

### Requirement: Selecting a compose file-search result SHALL insert a textual mention
V1 compose autocomplete SHALL insert a plain-text mention of the selected file path into the compose input. Archon SHALL NOT convert the selection into a structured provider payload.

#### Scenario: Selection inserts `@path/to/file`
- **WHEN** the user selects a file-search candidate from the compose picker
- **THEN** Archon MUST replace the active `@` fragment with a textual file mention
- **AND** the inserted text MUST contain the selected path in `@path/to/file` form
- **AND** the compose input MUST remain an ordinary text prompt after insertion

### Requirement: Compose autocomplete results SHALL be normalized before they reach the UI layer
Provider-specific search behavior may differ internally, but the compose UI SHALL consume one normalized candidate shape so the picker experience stays provider-agnostic.

#### Scenario: Different backends still produce one picker contract
- **WHEN** file-search results arrive from different supported providers
- **THEN** the compose UI MUST receive normalized file-search candidates rather than provider-specific result payloads
- **AND** the picker MUST NOT need provider-specific rendering rules to show those candidates
