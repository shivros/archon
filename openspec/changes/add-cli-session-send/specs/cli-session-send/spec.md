## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon send` command that posts a message to an existing session
Archon's CLI SHALL provide a `send` command that takes a session id as its first positional argument and sends a message to that session by invoking the daemon's `POST /v1/sessions/{id}/send` endpoint through the existing client-library `SendMessage` helper.

#### Scenario: Missing session id fails with a usage error
- **WHEN** a caller runs `archon send` with no session id
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a usage error on stderr

#### Scenario: Successful send returns the turn id
- **WHEN** a caller runs `archon send <session-id> "hello"` against a live daemon and a valid session
- **THEN** the command MUST post to `/v1/sessions/<session-id>/send` with the message
- **AND** the command MUST print the returned `turn_id` followed by a single `\n` to stdout
- **AND** the command MUST exit with status code `0`

### Requirement: The `archon send` command SHALL accept text input in three mutually exclusive forms
The `send` command SHALL accept message content as a positional argument, a `--text` flag, or a `--input-items` flag. At most one of these forms SHALL be supplied per invocation; providing more than one, or none, SHALL be a usage error.

#### Scenario: Positional text is the common-case invocation
- **WHEN** a caller runs `archon send <session-id> "hello"`
- **THEN** the command MUST populate the daemon request body's `text` field with `hello`
- **AND** the command MUST NOT populate the `input` field

#### Scenario: `--text` behaves identically to a positional argument
- **WHEN** a caller runs `archon send <session-id> --text "hello"`
- **THEN** the command MUST populate the daemon request body's `text` field with `hello`
- **AND** the command MUST NOT populate the `input` field

#### Scenario: `--input-items` accepts a JSON file path
- **WHEN** a caller runs `archon send <session-id> --input-items items.json` where `items.json` contains a JSON array of input-item objects
- **THEN** the command MUST read the file, decode it as `[]map[string]any`, and populate the daemon request body's `input` field with the decoded value
- **AND** the command MUST NOT populate the `text` field

#### Scenario: `--input-items -` reads from stdin
- **WHEN** a caller runs `archon send <session-id> --input-items -` and pipes a JSON array to stdin
- **THEN** the command MUST read from stdin, decode it as `[]map[string]any`, and populate the daemon request body's `input` field

#### Scenario: Conflicting input forms are a usage error
- **WHEN** a caller runs `archon send <session-id> "hello" --text "hi"` or combines any two of positional, `--text`, and `--input-items`
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a single-line error on stderr naming the conflict
- **AND** the command MUST NOT contact the daemon

#### Scenario: Missing input is a usage error
- **WHEN** a caller runs `archon send <session-id>` with no positional text, no `--text`, and no `--input-items`
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a usage error on stderr
- **AND** the command MUST NOT contact the daemon

#### Scenario: Malformed `--input-items` JSON is reported before contacting the daemon
- **WHEN** a caller runs `archon send <session-id> --input-items bad.json` where the file is not a valid JSON array
- **THEN** the command MUST decode locally and print a single-line error on stderr identifying the decode failure
- **AND** the command MUST NOT contact the daemon
- **AND** the command MUST exit with a non-zero status

### Requirement: The `archon send` command SHALL support a `--json` output flag
By default, `archon send` SHALL print only the returned `turn_id` on a single line. When `--json` is set, the command SHALL instead print the full daemon response JSON on a single line.

#### Scenario: Default output is the turn id only
- **WHEN** a caller runs `archon send <session-id> "hello"` and the daemon returns `{ok: true, turn_id: "trn_abc"}`
- **THEN** the command MUST print `trn_abc\n` on stdout
- **AND** the command MUST NOT print any JSON on stdout

#### Scenario: `--json` output includes the full response
- **WHEN** a caller runs `archon send <session-id> "hello" --json` and the daemon returns `{ok: true, turn_id: "trn_abc"}`
- **THEN** the command MUST print a single-line JSON document containing at least the `ok` and `turn_id` fields to stdout
- **AND** the command MUST end the line with a single `\n`

#### Scenario: Empty turn id in default mode prints nothing on stdout
- **WHEN** the daemon returns `{ok: true}` with no `turn_id`
- **THEN** in default (non-JSON) mode, the command MUST NOT print any line on stdout
- **AND** the command MUST still exit with status code `0`

### Requirement: The `archon send` command SHALL surface errors as single-line stderr messages with non-zero exit codes
Any failure — daemon unreachable, session not found, daemon-side validation failure, JSON decode failure — SHALL produce a single-line error on stderr and a non-zero exit status. The command MUST NOT print a stack trace.

#### Scenario: Daemon-side error surfaces as a single-line stderr message
- **WHEN** the daemon returns an error response to the send
- **THEN** the command MUST print a single-line error on stderr that includes the daemon's error message
- **AND** the command MUST exit with a non-zero status

#### Scenario: Daemon unreachable surfaces as a single-line stderr message
- **WHEN** the daemon is not reachable
- **THEN** the command MUST print a single-line error on stderr identifying the connection failure
- **AND** the command MUST exit with a non-zero status
