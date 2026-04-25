## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon ui` command that validates daemon readiness before launching the TUI
Archon's CLI SHALL provide a `ui` command that configures UI logging, verifies that a compatible daemon is available, and then launches the terminal UI.

#### Scenario: Default invocation performs a daemon-version check
- **WHEN** a caller runs `archon ui`
- **THEN** the CLI MUST configure UI logging before launch
- **AND** the CLI MUST verify daemon compatibility against the CLI version before launching the UI
- **AND** the CLI MUST launch the UI only after that compatibility check succeeds

#### Scenario: `--restart-daemon` requests daemon restart during version verification
- **WHEN** a caller runs `archon ui --restart-daemon`
- **THEN** the CLI MUST pass restart intent into the daemon-version compatibility check before launching the UI

### Requirement: The `archon ui` command SHALL support `--ignore-daemon-mismatch`
When `--ignore-daemon-mismatch` is set, `archon ui` SHALL bypass version-compatibility enforcement but SHALL still require a reachable daemon before launching the UI.

#### Scenario: Ignore-mismatch bypasses version enforcement
- **WHEN** a caller runs `archon ui --ignore-daemon-mismatch`
- **THEN** the CLI MUST check only that the daemon is reachable
- **AND** the CLI MUST NOT perform the version-compatibility gate
- **AND** the CLI MUST launch the UI if the daemon is reachable

### Requirement: Errors from `archon ui` SHALL surface as single-line stderr messages with non-zero exit codes
Any daemon-readiness or UI-launch failure SHALL produce a single-line stderr message and a non-zero exit.

#### Scenario: Daemon check or UI-launch failure surfaces cleanly
- **WHEN** daemon readiness verification or UI launch fails
- **THEN** the command MUST print a single-line error on stderr
- **AND** the command MUST exit with a non-zero status
