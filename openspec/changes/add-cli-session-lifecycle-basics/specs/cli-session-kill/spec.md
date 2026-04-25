## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon kill` command that terminates a session
Archon's CLI SHALL provide a `kill` command that takes a session id as its first positional argument and requests termination of that session through the daemon's existing kill-session API.

#### Scenario: Missing session id fails before contacting the daemon
- **WHEN** a caller runs `archon kill` with no positional session id
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a single-line usage or validation error on stderr
- **AND** the command MUST NOT contact the daemon

#### Scenario: Successful kill prints `ok`
- **WHEN** a caller runs `archon kill <session-id>` and the daemon accepts the kill request
- **THEN** the CLI MUST ensure the daemon is available before issuing the request
- **AND** the CLI MUST send the session id to the daemon kill endpoint
- **AND** the CLI MUST print `ok` followed by a single trailing newline on stdout
- **AND** the command MUST exit with status code `0`

### Requirement: Errors from `archon kill` SHALL surface as single-line stderr messages with non-zero exit codes
Any failure after argument parsing — daemon unavailable, unknown session id, or daemon-side kill rejection — SHALL produce a single-line stderr message and a non-zero exit. The command MUST NOT print `ok` on failure.

#### Scenario: Kill failure surfaces cleanly
- **WHEN** the daemon rejects the kill request
- **THEN** the command MUST print a single-line error on stderr
- **AND** the command MUST exit with a non-zero status
- **AND** stdout MUST NOT contain `ok`
