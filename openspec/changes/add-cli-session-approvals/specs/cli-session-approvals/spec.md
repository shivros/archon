## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon approvals` command that lists pending approvals for a session
Archon's CLI SHALL provide an `approvals` command that takes a session id as its first positional argument and lists all pending approvals for that session by invoking the daemon's `GET /v1/sessions/{id}/approvals` endpoint through the existing client-library `ListApprovals` helper.

#### Scenario: Missing session id fails with a usage error
- **WHEN** a caller runs `archon approvals` with no session id
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a usage error on stderr

#### Scenario: Non-empty list renders as a human table by default
- **WHEN** a caller runs `archon approvals <session-id>` against a daemon that returns one or more approvals
- **THEN** the command MUST print a human-readable table on stdout
- **AND** the table MUST begin with the header row `REQUEST_ID\tMETHOD\tCREATED`
- **AND** each subsequent row MUST correspond to one approval in the response
- **AND** the command MUST exit with status code `0`

#### Scenario: Empty list renders as the header row only and exits `0`
- **WHEN** a caller runs `archon approvals <session-id>` against a daemon that returns zero approvals
- **THEN** the command MUST print exactly the header row and no data rows
- **AND** the command MUST exit with status code `0`

### Requirement: The `archon approvals` command SHALL support a `--json` output flag
When invoked with `--json`, `archon approvals` SHALL emit the full daemon response as JSON — a top-level array whose elements are the `types.Approval` objects verbatim, with the `params` field preserved as raw JSON.

#### Scenario: `--json` emits the full approvals array
- **WHEN** a caller runs `archon approvals <session-id> --json` and the daemon returns one or more approvals
- **THEN** the command MUST print a single JSON array on stdout
- **AND** each element MUST include at minimum `session_id`, `request_id`, `method`, `created_at`
- **AND** `params` MUST be preserved verbatim as raw JSON (not re-encoded or stringified)
- **AND** the output MUST end with a single trailing newline

#### Scenario: `--json` with zero approvals emits `[]`
- **WHEN** a caller runs `archon approvals <session-id> --json` and the daemon returns zero approvals
- **THEN** the command MUST emit `[]` followed by a single trailing newline
- **AND** the command MUST exit with status code `0`

### Requirement: The CLI SHALL expose an `archon approve` command that responds to a specific approval
Archon's CLI SHALL provide an `approve` command that takes a session id as its first positional argument and posts a response to a specific approval by invoking the daemon's `POST /v1/sessions/{id}/approval` endpoint through the existing client-library `ApproveSession` helper.

#### Scenario: Missing session id fails with a usage error
- **WHEN** a caller runs `archon approve` with no session id
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a usage error on stderr
- **AND** the command MUST NOT contact the daemon

#### Scenario: Missing `--request-id` fails with a usage error
- **WHEN** a caller runs `archon approve <session-id>` without `--request-id`
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a usage error on stderr identifying the missing flag
- **AND** the command MUST NOT contact the daemon

#### Scenario: Missing `--decision` fails with a usage error
- **WHEN** a caller runs `archon approve <session-id> --request-id 1` without `--decision`
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a usage error on stderr identifying the missing flag
- **AND** the command MUST NOT contact the daemon

#### Scenario: Successful approval exits zero
- **WHEN** a caller runs `archon approve <session-id> --request-id 7 --decision allow_once` against a daemon that accepts the request
- **THEN** the command MUST post to `/v1/sessions/<session-id>/approval` with `request_id: 7, decision: "allow_once"`
- **AND** the command MUST exit with status code `0`
- **AND** the command MUST NOT print to stderr

### Requirement: The `archon approve` command SHALL accept repeated `--response` values and forward them as the `responses` array
Each occurrence of `--response <string>` on the command line SHALL append to an ordered list that is placed on the daemon request's `responses` field. When `--response` is absent, the `responses` field SHALL be omitted from the request body.

#### Scenario: Multiple `--response` flags preserve order
- **WHEN** a caller runs `archon approve <id> --request-id 1 --decision allow --response alpha --response beta --response gamma`
- **THEN** the daemon request MUST include `responses: ["alpha", "beta", "gamma"]` in that order

#### Scenario: No `--response` flags omits the field
- **WHEN** a caller runs `archon approve <id> --request-id 1 --decision allow`
- **THEN** the daemon request MUST NOT include a `responses` field (or MUST include an empty/omitted value equivalent to the daemon DTO's default)

### Requirement: The `archon approve` command SHALL accept `--accept-settings` as an inline JSON object
The `--accept-settings <json>` flag SHALL accept a JSON object as its value. The CLI SHALL decode the value locally before contacting the daemon; a decode failure SHALL surface as a single-line stderr error with a non-zero exit, and MUST NOT contact the daemon.

#### Scenario: Valid accept-settings JSON is forwarded
- **WHEN** a caller runs `archon approve <id> --request-id 1 --decision allow --accept-settings '{"remember":true}'`
- **THEN** the daemon request MUST include `accept_settings: {"remember": true}`

#### Scenario: Malformed accept-settings JSON fails before the round-trip
- **WHEN** a caller runs `archon approve <id> --request-id 1 --decision allow --accept-settings 'not-json'`
- **THEN** the command MUST print a single-line error on stderr identifying the decode failure
- **AND** the command MUST exit with a non-zero status
- **AND** the command MUST NOT contact the daemon

### Requirement: The approvals-related commands SHALL surface errors as single-line stderr messages with non-zero exit codes
Both `archon approvals` and `archon approve` SHALL surface any failure — daemon unreachable, session not found, daemon-side validation failure — as a single-line stderr message and a non-zero exit status. Neither command SHALL print a stack trace.

#### Scenario: Daemon-side error on list surfaces as a single-line stderr message
- **WHEN** `archon approvals <id>` is run against a daemon that returns an error
- **THEN** the command MUST print a single-line error on stderr
- **AND** the command MUST exit with a non-zero status

#### Scenario: Daemon-side error on approve surfaces as a single-line stderr message
- **WHEN** `archon approve <id> --request-id N --decision ...` is rejected by the daemon (e.g. already resolved, unknown decision, session gone)
- **THEN** the command MUST print a single-line error on stderr that includes the daemon's error message
- **AND** the command MUST exit with a non-zero status
