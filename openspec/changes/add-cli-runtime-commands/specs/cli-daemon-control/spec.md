## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon daemon` command for daemon process control
Archon's CLI SHALL provide a `daemon` command that starts the local daemon in the foreground by default and exposes flags for background execution, stop-only behavior, and forced restart.

#### Scenario: Default invocation starts the daemon in the foreground
- **WHEN** a caller runs `archon daemon` with no flags
- **THEN** the CLI MUST invoke the daemon runner in foreground mode

#### Scenario: `--background` requests background daemon execution
- **WHEN** a caller runs `archon daemon --background`
- **THEN** the CLI MUST invoke the daemon runner in background mode

#### Scenario: `--force` stops any running daemon before starting
- **WHEN** a caller runs `archon daemon --force`
- **THEN** the CLI MUST attempt to stop any currently running daemon first
- **AND** after the stop attempt succeeds, the CLI MUST start the daemon

#### Scenario: `--kill` stops the daemon and exits without starting
- **WHEN** a caller runs `archon daemon --kill`
- **THEN** the CLI MUST attempt to stop any running daemon
- **AND** the CLI MUST exit after the stop path completes
- **AND** the CLI MUST NOT start a new daemon process during that invocation

### Requirement: `archon daemon --kill` SHALL be safe when no daemon is running
The stop-only form of the daemon command SHALL treat an already-stopped or unreachable local daemon as a successful no-op rather than a hard failure.

#### Scenario: No running daemon still exits successfully
- **WHEN** a caller runs `archon daemon --kill` and no local daemon is running
- **THEN** the command MUST exit with status code `0`
- **AND** the command MUST NOT require a second invocation to recover

### Requirement: Errors from `archon daemon` SHALL surface as single-line stderr messages with non-zero exit codes
Any failure during stop or start that is not a safe no-op SHALL produce a single-line stderr message and a non-zero exit.

#### Scenario: Foreground or forced start failure surfaces cleanly
- **WHEN** the daemon runner or stop step returns an error
- **THEN** the command MUST print a single-line error on stderr
- **AND** the command MUST exit with a non-zero status
