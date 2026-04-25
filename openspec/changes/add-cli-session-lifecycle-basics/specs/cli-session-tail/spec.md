## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon tail` snapshot command for buffered session output
Archon's CLI SHALL provide a snapshot `tail` command that takes a session id as its first positional argument and fetches buffered tail items from the daemon. In snapshot mode, the command SHALL emit one JSON array representing the buffered items and SHALL terminate immediately after writing it.

#### Scenario: Missing session id fails before contacting the daemon
- **WHEN** a caller runs `archon tail` with no positional session id
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a single-line usage or validation error on stderr
- **AND** the command MUST NOT contact the daemon

#### Scenario: Successful snapshot emits one JSON array
- **WHEN** a caller runs `archon tail <session-id>`
- **THEN** the CLI MUST ensure the daemon is available before requesting buffered output
- **AND** the CLI MUST fetch the snapshot items through the daemon client
- **AND** the CLI MUST print a single JSON array followed by a trailing newline on stdout
- **AND** the command MUST exit with status code `0`

#### Scenario: Empty snapshot still emits an array
- **WHEN** a caller runs `archon tail <session-id>` and the daemon returns no buffered items
- **THEN** the CLI MUST emit an empty JSON array followed by a trailing newline
- **AND** the command MUST exit with status code `0`

### Requirement: The `archon tail` snapshot command SHALL support a `--lines` limit
In snapshot mode, `archon tail` SHALL accept a `--lines <count>` flag controlling how many buffered items the daemon is asked to return. When omitted, the CLI SHALL default to `200`.

#### Scenario: Explicit `--lines` value is forwarded
- **WHEN** a caller runs `archon tail <session-id> --lines 50`
- **THEN** the CLI MUST request exactly `50` lines from the daemon

#### Scenario: Omitted `--lines` uses the default
- **WHEN** a caller runs `archon tail <session-id>` without `--lines`
- **THEN** the CLI MUST request `200` lines from the daemon

### Requirement: Errors from snapshot `archon tail` SHALL surface as single-line stderr messages with non-zero exit codes
Any failure after argument parsing — daemon unavailable, unknown session id, or tail-fetch failure — SHALL produce a single-line stderr message and a non-zero exit. Snapshot mode MUST NOT emit partial JSON on failure.

#### Scenario: Tail failure surfaces cleanly
- **WHEN** the daemon returns an error for the tail snapshot request
- **THEN** the command MUST print a single-line error on stderr
- **AND** the command MUST exit with a non-zero status
- **AND** stdout MUST NOT contain a partial JSON document
