## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon config` command for read-only configuration inspection
Archon's CLI SHALL provide a `config` command that renders configuration to stdout without creating, modifying, or rewriting user config files. By default, the command SHALL print the effective configuration as JSON.

#### Scenario: Default invocation prints effective configuration as JSON
- **WHEN** a caller runs `archon config`
- **THEN** the CLI MUST print a valid JSON document to stdout
- **AND** the document MUST include the effective configuration values that result from the current user files plus built-in defaults

#### Scenario: The command is read-only
- **WHEN** a caller runs `archon config` with any supported flags
- **THEN** the CLI MUST NOT create or modify `config.toml`, `ui.toml`, `keybindings.json`, or `workflow_templates.json`

### Requirement: The `archon config` command SHALL support `--format json|toml`
`archon config` SHALL accept `--format json` and `--format toml`, defaulting to `json`. Unsupported output formats SHALL fail before any output is emitted.

#### Scenario: `--format toml` prints TOML
- **WHEN** a caller runs `archon config --format toml`
- **THEN** the CLI MUST print TOML to stdout
- **AND** the output MUST reflect the same resolved configuration data as the JSON form for the selected scope

#### Scenario: Invalid format is rejected
- **WHEN** a caller runs `archon config --format yaml`
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a single-line validation error on stderr

### Requirement: The `archon config` command SHALL support `--default` to inspect built-in defaults without depending on user files
When `--default` is set, `archon config` SHALL render built-in defaults rather than effective user configuration. Malformed user files SHALL NOT prevent `--default` output from succeeding.

#### Scenario: `--default` ignores malformed user files
- **WHEN** the user's config files are malformed and a caller runs `archon config --default`
- **THEN** the command MUST still succeed
- **AND** the output MUST reflect built-in defaults rather than failing on those malformed files

### Requirement: The `archon config` command SHALL support scoped projections
`archon config` SHALL accept repeatable `--scope` flags for `core`, `ui`, `keybindings`, `workflow_templates`, and `all`. The selected scopes SHALL control both which files are loaded and the top-level payload shape.

#### Scenario: `--scope core` skips UI parsing and renders only core configuration
- **WHEN** UI config files are malformed and a caller runs `archon config --scope core`
- **THEN** the command MUST still succeed
- **AND** the output MUST include only the core configuration projection

#### Scenario: `--scope keybindings` renders the keybinding map directly
- **WHEN** a caller runs `archon config --scope keybindings`
- **THEN** the output MUST be the keybinding map itself at the top level
- **AND** the output MUST NOT wrap that map in a nested `keybindings` object

#### Scenario: `--scope workflow_templates` renders the templates array directly
- **WHEN** a caller runs `archon config --scope workflow_templates`
- **THEN** the output MUST expose the workflow template document directly at the top level
- **AND** the output MUST NOT wrap it in a nested `workflow_templates` object

#### Scenario: Invalid scope is rejected
- **WHEN** a caller runs `archon config --scope notes`
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a single-line validation error on stderr
