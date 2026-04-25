## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon interrupt` command that stops an in-flight turn
Archon's CLI SHALL provide an `interrupt` command that takes a session id as its first positional argument and calls the daemon's `POST /v1/sessions/{id}/interrupt` endpoint through the existing client-library `InterruptSession` helper.

#### Scenario: Missing session id fails with a usage error
- **WHEN** a caller runs `archon interrupt` with no positional argument
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a usage error on stderr
- **AND** the command MUST NOT contact the daemon

#### Scenario: Successful interrupt exits silently with status zero
- **WHEN** a caller runs `archon interrupt <session-id>` against a live daemon with a valid session
- **THEN** the command MUST post to `/v1/sessions/<session-id>/interrupt`
- **AND** the command MUST NOT print any output on stdout
- **AND** the command MUST NOT print any output on stderr
- **AND** the command MUST exit with status code `0`

#### Scenario: Interrupt when the session has no in-flight turn is still success
- **WHEN** a caller runs `archon interrupt <session-id>` against a session with no in-flight turn
- **AND** the daemon accepts the request as a no-op
- **THEN** the command MUST exit with status code `0`
- **AND** the command MUST NOT print any error on stderr

### Requirement: The `archon interrupt` command SHALL surface errors as single-line stderr messages with non-zero exit codes
Any failure — daemon unreachable, session not found, daemon-side rejection — SHALL produce a single-line stderr error and a non-zero exit. The command MUST NOT print a stack trace.

#### Scenario: Daemon-side error surfaces as a single-line stderr message
- **WHEN** the daemon returns an error response to the interrupt
- **THEN** the command MUST print a single-line error on stderr that includes the daemon's error message
- **AND** the command MUST exit with a non-zero status

#### Scenario: Daemon unreachable surfaces as a single-line stderr message
- **WHEN** the daemon is not reachable
- **THEN** the command MUST print a single-line error on stderr identifying the connection failure
- **AND** the command MUST exit with a non-zero status
