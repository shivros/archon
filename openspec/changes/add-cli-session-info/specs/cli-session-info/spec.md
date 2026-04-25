## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon session` command that returns a single session's state
Archon's CLI SHALL provide a `session` command that takes a session id as its first positional argument and calls the daemon's `GET /v1/sessions/{id}` endpoint through the existing client-library `GetSession` helper, emitting the returned `types.Session` document to stdout.

#### Scenario: Missing session id fails with a usage error
- **WHEN** a caller runs `archon session` with no positional argument
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a usage error on stderr
- **AND** the command MUST NOT contact the daemon

#### Scenario: Successful invocation prints the session JSON and exits zero
- **WHEN** a caller runs `archon session <session-id>` against a live daemon with a valid session id
- **THEN** the command MUST print a single JSON document on stdout containing the `types.Session` fields as serialized by the daemon
- **AND** the document MUST be pretty-printed with two-space indentation
- **AND** the document MUST end with a single trailing newline
- **AND** the command MUST exit with status code `0`

#### Scenario: Unknown session id surfaces as a single-line stderr error and non-zero exit
- **WHEN** a caller runs `archon session <unknown-id>` against a live daemon
- **THEN** the command MUST print a single-line error on stderr
- **AND** the command MUST exit with a non-zero status
- **AND** the command MUST NOT print a JSON document on stdout

### Requirement: The `archon session` JSON output SHALL match the daemon's `types.Session` serialization
The CLI SHALL NOT invent a CLI-local session DTO. The emitted JSON IS the daemon's `types.Session` JSON serialization, including every field with a JSON tag.

#### Scenario: Output includes the core documented fields
- **WHEN** a caller runs `archon session <session-id>` with a valid session
- **THEN** the output JSON MUST include at minimum the fields corresponding to `types.Session`'s `id`, `status`, `provider`, `pid`, and `title`
- **AND** any additional `types.Session` fields with JSON tags MUST be emitted as well

### Requirement: The `archon session` command SHALL support a `--format human` output mode
When invoked with `--format human`, the command SHALL print a compact field-per-line summary of the session instead of the default JSON document. JSON remains the default output mode.

#### Scenario: `--format human` prints field-per-line output
- **WHEN** a caller runs `archon session <session-id> --format human`
- **THEN** the command MUST print a series of `KEY: value` lines on stdout, one per field
- **AND** the output MUST include at minimum the fields `id`, `status`, `provider`, `title`, `pid`, and the created/updated timestamps
- **AND** the command MUST exit with status code `0`

#### Scenario: `--format json` is an explicit alias for the default
- **WHEN** a caller runs `archon session <session-id> --format json`
- **THEN** the command MUST produce exactly the same output as `archon session <session-id>` without the flag

### Requirement: Errors in `archon session` SHALL surface as single-line stderr messages with non-zero exit codes
Any failure — daemon unreachable, daemon-side 404, network error — SHALL produce a single-line stderr message and a non-zero exit. The command MUST NOT print a stack trace.

#### Scenario: Daemon-side error surfaces as a single-line stderr message
- **WHEN** the daemon returns an error response for the session query
- **THEN** the command MUST print a single-line error on stderr including the daemon's error message
- **AND** the command MUST exit with a non-zero status
