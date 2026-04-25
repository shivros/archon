## ADDED Requirements

### Requirement: The `archon tail` command SHALL preserve snapshot behavior when `--follow` is absent
When `archon tail <session-id>` is invoked without `--follow`, the command SHALL behave as it does today: call the daemon's snapshot tail endpoint, emit a single JSON array of items to stdout, and exit. This requirement is stated explicitly so that adding follow mode cannot inadvertently change the snapshot contract.

#### Scenario: Snapshot invocation emits a JSON array and exits
- **WHEN** a caller runs `archon tail <session-id>` without `--follow`
- **THEN** the command MUST request the snapshot tail endpoint with the effective `--lines` value
- **AND** the command MUST emit the returned items as a single JSON array on stdout
- **AND** the command MUST exit `0` after writing the array

### Requirement: The `archon tail` command SHALL support a `--follow` flag that streams events as NDJSON
When invoked with `--follow` (or `-f`), `archon tail` SHALL open the daemon's server-sent-events tail endpoint and emit each received event to stdout as a single compact JSON document followed by `\n`, flushing stdout after every line.

#### Scenario: Follow mode emits one JSON document per event, newline-terminated
- **WHEN** a caller runs `archon tail <session-id> --follow` and the daemon streams a sequence of tail events
- **THEN** the command MUST write each event as one line of JSON to stdout
- **AND** each line MUST end with a single `\n`
- **AND** the command MUST flush stdout after each line so downstream pipes observe events in real time
- **AND** the command MUST NOT emit a surrounding array or any SSE framing characters

#### Scenario: Follow mode delegates to the existing client-side SSE helper
- **WHEN** the CLI opens the stream in follow mode
- **THEN** the CLI MUST route the request through `internal/client/sse.go`'s existing streaming tail helper
- **AND** the CLI MUST NOT re-implement SSE parsing or framing in the `cmd/archon` layer

### Requirement: The `archon tail --follow` command SHALL accept a `--stream` selector
In follow mode, `archon tail` SHALL accept a `--stream <name>` flag selecting which stream the daemon should send. The CLI SHALL recognise at minimum the values `stdout`, `stderr`, and `combined`, defaulting to `combined` when the flag is omitted.

#### Scenario: Default stream selector is `combined`
- **WHEN** a caller runs `archon tail <session-id> --follow` with no `--stream` flag
- **THEN** the CLI MUST request the daemon's `combined` stream

#### Scenario: Explicit stream selector is honored
- **WHEN** a caller runs `archon tail <session-id> --follow --stream stderr`
- **THEN** the CLI MUST request the daemon's `stderr` stream
- **AND** the CLI MUST NOT emit events belonging to other streams

#### Scenario: `--stream` without `--follow` is ignored
- **WHEN** a caller runs `archon tail <session-id> --stream stdout` without `--follow`
- **THEN** the CLI MUST behave identically to a plain `archon tail <session-id>` invocation
- **AND** the CLI MUST NOT fail the invocation

### Requirement: The `archon tail --follow` command SHALL exit cleanly on SIGINT and SIGTERM
In follow mode, `archon tail` SHALL install a signal handler for `SIGINT` and `SIGTERM`. On receipt, the CLI SHALL cancel the stream's context, return from the command, and exit with status `0`.

#### Scenario: Ctrl+C terminates follow mode cleanly
- **WHEN** a caller running `archon tail <session-id> --follow` sends `SIGINT`
- **THEN** the CLI MUST cancel the streaming request
- **AND** the CLI MUST exit with status code `0`
- **AND** the CLI MUST NOT print a stack trace or signal error on stderr

#### Scenario: SIGTERM terminates follow mode cleanly
- **WHEN** a caller running `archon tail <session-id> --follow` receives `SIGTERM`
- **THEN** the CLI MUST cancel the streaming request and exit with status code `0`

### Requirement: The `archon tail --follow` command SHALL distinguish clean session end from stream errors in its exit status
When the daemon closes the stream because the session exited normally, the CLI SHALL exit `0`. When the stream closes for any other reason — network error, scan error, daemon shutdown — the CLI SHALL write a single-line human-readable error to stderr and exit with a non-zero status.

#### Scenario: Session exit produces a zero exit
- **WHEN** the daemon closes the stream with a session-ended signal
- **THEN** the CLI MUST exit with status code `0`
- **AND** the CLI MUST NOT print an error to stderr

#### Scenario: Stream error produces a non-zero exit
- **WHEN** the stream terminates because of a network error or daemon-side error
- **THEN** the CLI MUST write a single-line error to stderr identifying the condition
- **AND** the CLI MUST exit with a non-zero status code

### Requirement: The `archon tail` command SHALL combine `--lines` and `--follow` by streaming backfill before live events
When both `--lines N` and `--follow` are set, the CLI SHALL first emit the last N buffered items as NDJSON and then transition to streaming live events without duplicating the boundary item.

#### Scenario: Backfill-then-follow composes cleanly
- **WHEN** a caller runs `archon tail <session-id> --lines 50 --follow`
- **THEN** the CLI MUST emit up to 50 historical items as single-line JSON entries on stdout
- **AND** the CLI MUST then transition to live event streaming without re-emitting the most recent historical item
- **AND** the combined output MUST remain valid NDJSON (one complete JSON object per line, `\n` terminated)
