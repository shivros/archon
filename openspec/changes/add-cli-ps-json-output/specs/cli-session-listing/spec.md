## ADDED Requirements

### Requirement: The `archon ps` command SHALL continue to emit a tab-separated human table by default
When `archon ps` is invoked without any output-format flags, the command SHALL produce the existing tab-separated table with columns `ID`, `STATUS`, `PROVIDER`, `PID`, `TITLE` on stdout. This requirement exists to pin the default behavior so that adding JSON output does not accidentally change it.

#### Scenario: Default invocation emits the human table
- **WHEN** a caller runs `archon ps` with no flags and the daemon returns one or more sessions
- **THEN** the command MUST emit the five-column tab-separated table starting with the header row `ID\tSTATUS\tPROVIDER\tPID\tTITLE`
- **AND** each subsequent row MUST correspond to one session in the daemon's response

#### Scenario: Default invocation with zero sessions emits the header row only
- **WHEN** a caller runs `archon ps` with no flags and the daemon returns zero sessions
- **THEN** the command MUST emit exactly the header row and no data rows
- **AND** the command MUST exit with status code `0`

### Requirement: The `archon ps` command SHALL support a `--json` flag that emits the daemon's session DTO as JSON
When invoked with `--json`, `archon ps` SHALL emit a single JSON array on stdout whose elements are the `types.Session` objects returned by the daemon's `GET /v1/sessions` endpoint. The CLI SHALL NOT invent a CLI-local session DTO; the JSON schema IS the daemon's serialization.

#### Scenario: `--json` emits the daemon's session array
- **WHEN** a caller runs `archon ps --json` and the daemon returns one or more sessions
- **THEN** the command MUST emit a single JSON array on stdout whose elements serialize each session's full `types.Session` JSON fields
- **AND** the output MUST be pretty-printed with two-space indentation
- **AND** the output MUST end with a single trailing newline
- **AND** the command MUST exit with status code `0`

#### Scenario: `--json` with zero sessions emits an empty array
- **WHEN** a caller runs `archon ps --json` and the daemon returns zero sessions
- **THEN** the command MUST emit `[]` followed by a single trailing newline
- **AND** the command MUST exit with status code `0`
- **AND** the command MUST NOT emit `null` or an empty string

#### Scenario: `--json` output includes the fields programmatic consumers rely on
- **WHEN** a caller runs `archon ps --json` and the daemon returns at least one session
- **THEN** each element in the emitted array MUST include the JSON fields corresponding to `types.Session`'s `id`, `status`, `provider`, `pid`, and `title`
- **AND** any additional `types.Session` fields with JSON tags MUST be emitted as well, reflecting the daemon's current DTO

#### Scenario: `--json` does not change the human table output path
- **WHEN** a caller runs `archon ps` without `--json` in the same environment where `archon ps --json` would return data
- **THEN** the command MUST emit the tab-separated human table and MUST NOT emit any JSON

### Requirement: The JSON output of `archon ps --json` SHALL be treated as part of the CLI contract
The JSON shape emitted by `archon ps --json` is a load-bearing contract for programmatic consumers. Any change to the daemon's `types.Session` JSON serialization that would remove or rename fields emitted by this command SHALL be made as a conscious CLI-contract decision, not as an incidental consequence of a daemon refactor.

#### Scenario: A test locks the top-level shape and documented fields
- **WHEN** the CLI test suite runs against a representative session list returned by the daemon
- **THEN** there MUST be a test that asserts the output is a JSON array
- **AND** the test MUST assert the presence of the documented fields `id`, `status`, `provider`, `pid`, and `title` on each element
- **AND** a daemon-side change that would remove or rename any of those fields MUST cause the test to fail

### Requirement: The `archon ps --json` output SHALL reflect the same session selection as the default output
The `--json` flag SHALL change only the output format, not which sessions the daemon is asked to return. Any existing or future `ps` flags that affect session selection (for example flags controlling whether dismissed or workflow-owned sessions are included) SHALL apply identically in JSON mode and in default mode.

#### Scenario: Selection-affecting flags apply in JSON mode
- **WHEN** a caller runs `archon ps` with a selection-affecting flag and compares against `archon ps --json` with the same flag
- **THEN** the set of session IDs in the JSON output MUST equal the set of session IDs in the tab-separated output
- **AND** neither mode MUST include sessions the other mode excludes
