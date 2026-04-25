## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon start` command that creates a session
Archon's CLI SHALL provide a `start` command that creates a new session through the daemon's start-session API. The command SHALL require a provider name and SHALL print only the created session id on stdout when the daemon accepts the request.

#### Scenario: Missing provider fails before contacting the daemon
- **WHEN** a caller runs `archon start` without `--provider`
- **THEN** the command MUST exit with a non-zero status
- **AND** the command MUST print a single-line usage or validation error on stderr
- **AND** the command MUST NOT contact the daemon

#### Scenario: Successful start prints the created session id
- **WHEN** a caller runs `archon start --provider codex`
- **THEN** the CLI MUST ensure the daemon is available before issuing the start request
- **AND** the CLI MUST create the session through the daemon client
- **AND** the CLI MUST print the returned session id followed by a single trailing newline on stdout
- **AND** the command MUST exit with status code `0`

### Requirement: The `archon start` command SHALL forward session options and trailing arguments verbatim
The `start` command SHALL translate CLI flags into the daemon start request without reordering or rewriting user-supplied values. This includes the provider, optional working directory, optional command override, title, repeatable tags, repeatable environment variables, and any trailing positional arguments.

#### Scenario: Flags map directly onto the daemon request
- **WHEN** a caller runs `archon start --provider codex --cwd /tmp/project --cmd codex --title demo --tag one --tag two --env A=B --env C=D`
- **THEN** the daemon request MUST include `provider=codex`, `cwd=/tmp/project`, `cmd=codex`, and `title=demo`
- **AND** the daemon request MUST include tags `["one", "two"]` in that order
- **AND** the daemon request MUST include environment entries `["A=B", "C=D"]` in that order

#### Scenario: Trailing positional arguments are preserved in order
- **WHEN** a caller runs `archon start --provider codex arg1 arg2 arg3`
- **THEN** the daemon request MUST include the positional arguments as `["arg1", "arg2", "arg3"]`
- **AND** the CLI MUST NOT treat those trailing values as tags or environment variables

### Requirement: Errors from `archon start` SHALL surface as single-line stderr messages with non-zero exit codes
Any failure after argument parsing — daemon unavailable, daemon-side validation failure, or start-session rejection — SHALL produce a single-line stderr message and a non-zero exit. The command MUST NOT emit a partial session id on stdout.

#### Scenario: Daemon-side start failure surfaces cleanly
- **WHEN** the daemon rejects a start request
- **THEN** the command MUST print a single-line error on stderr
- **AND** the command MUST exit with a non-zero status
- **AND** stdout MUST NOT contain a session id
