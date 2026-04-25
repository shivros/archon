## ADDED Requirements

### Requirement: Session sidebar rows SHALL render provider badges with stable defaults
Archon SHALL render a provider badge for session rows in the sidebar. Known providers SHALL use Archon-defined default badge prefixes and colors, while providers without a named default SHALL use a derived fallback badge.

#### Scenario: Known providers use named defaults
- **WHEN** the sidebar renders a session for a known provider such as `codex`, `claude`, or `kilocode`
- **THEN** Archon MUST render that row with the provider's default badge prefix
- **AND** Archon MUST render that row with the provider's default badge color

#### Scenario: Unknown providers use a derived fallback badge
- **WHEN** the sidebar renders a session for a provider that has no named default badge
- **THEN** Archon MUST derive a fallback three-character badge prefix from the normalized provider name
- **AND** Archon MUST use the default fallback badge color when no provider-specific default color exists

### Requirement: `provider_badges` overrides from app state SHALL apply per normalized provider key
Archon SHALL allow users to override badge prefix and color per provider by storing `provider_badges` in `~/.archon/state.json`. Override lookup SHALL normalize provider keys before applying them.

#### Scenario: Override keys are normalized before lookup
- **WHEN** `state.json` stores a provider badge override under a key such as `" CoDeX "`
- **THEN** Archon MUST normalize that key to the canonical provider name before lookup
- **AND** the override MUST apply to `codex` sidebar rows

#### Scenario: Blank or invalid override keys are ignored
- **WHEN** `provider_badges` contains a blank key or a key that normalizes to empty
- **THEN** Archon MUST ignore that entry
- **AND** it MUST NOT break badge resolution for other providers

### Requirement: Provider badge overrides SHALL be selective rather than all-or-nothing
An override may replace the badge prefix, the badge color, or both. Blank override fields SHALL not erase the corresponding default value.

#### Scenario: Prefix and color can both be overridden
- **WHEN** `state.json` contains `provider_badges.codex = { "prefix": "[GPT]", "color": "231" }`
- **THEN** Archon MUST render codex session rows with prefix `[GPT]`
- **AND** Archon MUST render codex session rows with color `231`

#### Scenario: Blank override fields do not erase defaults
- **WHEN** an override omits or leaves blank either `prefix` or `color`
- **THEN** Archon MUST keep using the default value for that field
- **AND** it MUST apply only the non-empty override fields

### Requirement: Provider badge overrides SHALL persist through app-state storage
Provider badge overrides are part of Archon's app state and SHALL round-trip through `~/.archon/state.json`.

#### Scenario: Saved provider badge overrides load back unchanged
- **WHEN** Archon saves app state containing `provider_badges`
- **THEN** a subsequent app-state load MUST restore those provider badge overrides
- **AND** the restored overrides MUST preserve the configured `prefix` and `color` values
