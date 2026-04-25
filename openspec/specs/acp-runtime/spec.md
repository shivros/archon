## ADDED Requirements

### Requirement: ACP client SHALL speak stdio JSON-RPC 2.0 with newline-delimited framing
Archon SHALL provide a reusable ACP client substrate that communicates with an ACP-speaking agent subprocess over its standard input and standard output, using UTF-8 encoded JSON-RPC 2.0 messages delimited by a single `\n` character, with no embedded newlines inside a message. The client MUST reserve the subprocess's `stdout` for ACP traffic only and MUST NOT interpret `stderr` output as protocol data.

#### Scenario: Well-formed message is framed and sent on a single line
- **WHEN** the client sends a JSON-RPC request, response, or notification
- **THEN** the client MUST emit the JSON object followed by exactly one `\n` to the agent's `stdin`
- **AND** the emitted payload MUST contain no embedded newline characters

#### Scenario: Agent output is parsed line-by-line from stdout
- **WHEN** the agent writes a complete line ending in `\n` to its `stdout`
- **THEN** the client MUST parse it as a JSON-RPC message
- **AND** the client MUST continue reading subsequent messages without loss

#### Scenario: Malformed JSON or oversized lines do not crash the client
- **WHEN** the agent writes a line that fails JSON parsing
- **THEN** the client MUST log the failure to its debug sink
- **AND** the client MUST continue reading subsequent lines from the agent

#### Scenario: stderr output is forwarded, not interpreted
- **WHEN** the agent writes data to its `stderr`
- **THEN** the client MUST forward that data to the configured stderr sink
- **AND** the client MUST NOT attempt to parse it as JSON-RPC

### Requirement: ACP client SHALL correlate requests to responses by JSON-RPC id
The client SHALL assign a unique, monotonically increasing numeric `id` to each outgoing request and SHALL deliver the matching response (or error) to the caller that made the request, supporting concurrent in-flight requests on a single subprocess.

#### Scenario: Concurrent requests receive their own responses
- **WHEN** two callers issue requests against the same client concurrently
- **THEN** each caller MUST receive the response whose `id` matches its request
- **AND** neither caller MUST receive the other caller's response

#### Scenario: Error responses are surfaced to the originating caller
- **WHEN** the agent returns a JSON-RPC error object for a request id
- **THEN** the client MUST return that error to the caller that sent that request id
- **AND** the client MUST NOT treat a JSON-RPC error as a transport failure

#### Scenario: Context cancellation aborts a pending request
- **WHEN** a caller's context is cancelled before the agent responds
- **THEN** the client MUST return a context-cancellation error to that caller
- **AND** the client MUST discard any subsequent response for that request id without panicking

### Requirement: ACP client SHALL fan out notifications to subscribers
The client SHALL deliver JSON-RPC notifications received from the agent (notably `session/update`) to all active subscribers, without blocking the read loop when a subscriber is slow.

#### Scenario: Multiple subscribers each observe a notification
- **WHEN** the agent emits a `session/update` notification
- **AND** two subscribers are attached
- **THEN** both subscribers MUST receive that notification

#### Scenario: A slow subscriber does not stall the read loop
- **WHEN** one subscriber stops draining its channel while notifications continue to arrive
- **THEN** the client MUST continue to process inbound messages and serve other subscribers
- **AND** the client MUST either drop or buffer notifications for the stalled subscriber under a documented policy, not deadlock

#### Scenario: Unknown sessionUpdate variants are preserved
- **WHEN** the agent emits a `session/update` whose `sessionUpdate` discriminator is unknown to the client
- **THEN** the client MUST deliver the notification to subscribers with the unknown payload retained as raw JSON
- **AND** the client MUST NOT drop the notification

### Requirement: ACP client SHALL dispatch agent→client requests to registered handlers
The client SHALL accept incoming JSON-RPC requests from the agent (e.g. `session/request_permission`, `fs/read_text_file`, `fs/write_text_file`, `terminal/*`) and SHALL route each to a handler registered for that method, returning the handler's result or a `method_not_found` error if none is registered.

#### Scenario: Registered handler returns a response
- **WHEN** the agent issues an incoming request for a method that has a registered handler
- **THEN** the client MUST invoke the handler with the request parameters
- **AND** the client MUST return the handler's result (or error) to the agent under the original request id

#### Scenario: Unregistered method returns method_not_found
- **WHEN** the agent issues an incoming request for a method with no registered handler
- **THEN** the client MUST respond with a JSON-RPC error whose code indicates `method_not_found`
- **AND** the client MUST NOT crash or disconnect

### Requirement: ACP client SHALL initialize before issuing session methods
The client SHALL send an `initialize` request at the start of every connection, advertising its client info, client capabilities, and the protocol version it supports. It SHALL refuse to issue any session-level method until the agent has responded successfully.

#### Scenario: Successful initialize unlocks session calls
- **WHEN** the client connects to the agent and the agent responds to `initialize`
- **THEN** the client MUST record the agent-reported capabilities and info
- **AND** the client MUST allow subsequent `session/new`, `session/load`, `session/prompt`, and `session/cancel` calls

#### Scenario: Version mismatch fails loudly
- **WHEN** the agent's `initialize` response reports a protocol version the client does not support
- **THEN** the client MUST terminate the connection
- **AND** the client MUST surface a descriptive error to the caller identifying the version mismatch

### Requirement: ACP client SHALL support session cancellation via session/cancel notification
The client SHALL expose an operation that sends a `session/cancel` notification for a given `sessionId`, and SHALL ensure that any in-flight `session/prompt` call for that session resolves with a `cancelled` stop reason from the agent.

#### Scenario: Cancel while a prompt is in flight resolves with cancelled
- **WHEN** a caller has a `session/prompt` request outstanding
- **AND** another caller invokes cancel on the same `sessionId`
- **THEN** the client MUST send a `session/cancel` notification to the agent
- **AND** the pending `session/prompt` caller MUST eventually receive a response with `stopReason: "cancelled"`

### Requirement: ACP client SHALL shut down gracefully
The client SHALL expose a `Close` operation that closes the agent's stdin, waits for the process to exit, and returns once the process has terminated. If the process does not exit within a bounded timeout it SHALL be killed.

#### Scenario: Graceful shutdown drains stdin and waits for exit
- **WHEN** the caller invokes `Close`
- **THEN** the client MUST close the agent's stdin
- **AND** the client MUST wait for the agent process to exit under a bounded timeout

#### Scenario: Unresponsive process is killed on timeout
- **WHEN** the agent process does not exit within the shutdown timeout after stdin is closed
- **THEN** the client MUST kill the process
- **AND** the client MUST return an error indicating the forced termination
