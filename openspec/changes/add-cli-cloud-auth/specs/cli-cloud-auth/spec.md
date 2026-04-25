## ADDED Requirements

### Requirement: The CLI SHALL expose an `archon login` command for the cloud device flow
Archon's CLI SHALL provide a `login` command that starts a cloud device-authorization flow through the daemon, prints fallback activation instructions to stdout, optionally opens a browser, and polls until the device flow resolves.

#### Scenario: Login always prints fallback activation instructions
- **WHEN** a caller runs `archon login`
- **THEN** the CLI MUST ensure the daemon is available before starting the cloud login
- **AND** the CLI MUST print the verification URL as `Visit: <url>`
- **AND** the CLI MUST print the user code as `Code: <code>`
- **AND** those fallback instructions MUST be printed even when the browser opener succeeds

#### Scenario: Successful login prints the linked identity when available
- **WHEN** the device flow reaches an `approved` status and the daemon returns linked user details
- **THEN** the CLI MUST exit with status code `0`
- **AND** the CLI MUST print a success line identifying the linked user email when the email is available

#### Scenario: `--no-browser` suppresses the browser opener
- **WHEN** a caller runs `archon login --no-browser`
- **THEN** the CLI MUST still print the fallback activation instructions
- **AND** the CLI MUST NOT attempt to open a browser

#### Scenario: Browser-open failure warns but does not abort login
- **WHEN** the browser opener returns an error after login setup succeeds
- **THEN** the CLI MUST print a warning on stderr identifying the browser-open failure
- **AND** the CLI MUST continue polling the device flow instead of failing immediately

#### Scenario: `slow_down` increases the polling delay
- **WHEN** the daemon returns a `slow_down` poll status
- **THEN** the CLI MUST increase the next poll delay relative to the current interval
- **AND** the CLI MUST continue polling until a terminal outcome is reached or a later error occurs

### Requirement: The CLI SHALL expose an `archon whoami` command for current cloud link status
Archon's CLI SHALL provide a `whoami` command that reports whether this Archon daemon is linked to Archon Cloud and, when linked, prints the current linked user and installation information in a human-readable form.

#### Scenario: Unlinked daemon prints `not logged in`
- **WHEN** a caller runs `archon whoami` and the daemon reports `linked=false`
- **THEN** the CLI MUST print `not logged in` followed by a trailing newline
- **AND** the command MUST exit with status code `0`

#### Scenario: Linked daemon prints user and installation details
- **WHEN** a caller runs `archon whoami` and the daemon reports a linked user and installation
- **THEN** the CLI MUST print available user display name and email fields on separate lines
- **AND** the CLI MUST print the installation name on a separate line when it is available
- **AND** the command MUST exit with status code `0`

### Requirement: The CLI SHALL expose an `archon logout` command that unlinks cloud auth and reports the result
Archon's CLI SHALL provide a `logout` command that asks the daemon to clear cloud credentials and SHALL print the daemon-provided result message on stdout.

#### Scenario: Full logout prints the success message
- **WHEN** a caller runs `archon logout` and the daemon fully revokes the remote token and clears local cloud credentials
- **THEN** the CLI MUST print the daemon's success message followed by a trailing newline
- **AND** the command MUST exit with status code `0`

#### Scenario: Partial logout still prints the daemon result
- **WHEN** the daemon clears local cloud credentials but reports that remote revoke failed
- **THEN** the CLI MUST print the daemon's partial-success message on stdout
- **AND** the command MUST still exit with status code `0`

### Requirement: Cloud-auth command failures SHALL surface as single-line stderr messages with non-zero exit codes
Any failure after argument parsing — daemon unavailable, poll failure, unexpected device-flow status, cloud-status lookup failure, or logout failure — SHALL produce a single-line stderr message and a non-zero exit.

#### Scenario: Login poll failure surfaces cleanly
- **WHEN** `archon login` encounters a polling error or cancellation
- **THEN** the command MUST print a single-line error on stderr
- **AND** the command MUST exit with a non-zero status

#### Scenario: WhoAmI or logout failure surfaces cleanly
- **WHEN** `archon whoami` or `archon logout` encounters a daemon-side error
- **THEN** the command MUST print a single-line error on stderr
- **AND** the command MUST exit with a non-zero status
