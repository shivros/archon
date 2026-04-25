## ADDED Requirements

### Requirement: Archon SHALL register Hermes as a first-class provider
Archon SHALL register a `hermes` provider definition in the providers registry with runtime `RuntimeACP`, launched as `hermes acp`. The definition SHALL advertise capabilities that match what ACP and the Hermes ACP server actually implement: `SupportsEvents`, `SupportsInterrupt`, `SupportsApprovals`, and `SupportsGuidedWorkflowDispatch` SHALL be true; `SupportsFileSearch`, `UsesItems`, and `NoProcess` SHALL be false.

#### Scenario: Hermes appears in the provider registry
- **WHEN** a caller invokes the archon provider registry to enumerate providers
- **THEN** the registry MUST include a `hermes` entry whose runtime is `RuntimeACP`
- **AND** the entry MUST list `hermes` as a command candidate

#### Scenario: Hermes capabilities reflect the ACP + Hermes contract
- **WHEN** a caller queries the capabilities for provider `hermes`
- **THEN** the response MUST report `SupportsEvents`, `SupportsInterrupt`, `SupportsApprovals`, and `SupportsGuidedWorkflowDispatch` as true
- **AND** the response MUST report `SupportsFileSearch`, `UsesItems`, and `NoProcess` as false

#### Scenario: Unknown Hermes command is surfaced as a resolution error
- **WHEN** `hermes` is not on `$PATH` and no command override is configured
- **THEN** archon MUST fail the session start with an error identifying the missing binary
- **AND** archon MUST NOT leave a partially started subprocess behind

### Requirement: Archon SHALL start Hermes as a per-session stdio ACP subprocess
For each archon session that selects the `hermes` provider, archon SHALL spawn a `hermes acp` subprocess, complete the ACP `initialize` handshake as the client, and create a Hermes session via `session/new` using the archon session's working directory. The returned ACP `sessionId` SHALL be stored as the provider process's thread identifier for use by the session service.

#### Scenario: Successful start returns a usable provider process
- **WHEN** archon starts a new Hermes session
- **THEN** archon MUST spawn `hermes acp` with the session's working directory
- **AND** archon MUST send `initialize` advertising baseline client capabilities and wait for a successful response
- **AND** archon MUST call `session/new` and store the returned `sessionId` on the provider process

#### Scenario: Initialization failure aborts the session
- **WHEN** the `initialize` handshake fails or times out
- **THEN** archon MUST terminate the Hermes subprocess
- **AND** archon MUST surface the initialization error to the caller without registering the session

### Requirement: Archon SHALL stream Hermes transcript events in real time
Archon SHALL translate ACP `session/update` notifications emitted during a Hermes turn into canonical transcript events so that the user sees assistant deltas, tool-call activity, plans, and thinking output as they arrive, rather than only at turn completion.

#### Scenario: Agent message chunks render as assistant deltas
- **WHEN** Hermes emits a `session/update` with `sessionUpdate: "agent_message_chunk"`
- **THEN** archon MUST produce an assistant-delta transcript event carrying that chunk's content
- **AND** the event MUST be dispatched before the turn completes

#### Scenario: Tool call events render as tool-call transcript entries
- **WHEN** Hermes emits a `session/update` with `sessionUpdate: "tool_call"` followed by one or more `"tool_call_update"` notifications for the same `toolCallId`
- **THEN** archon MUST produce a started tool-call transcript event for the initial notification
- **AND** archon MUST produce tool-call update events reflecting each status and content change
- **AND** archon MUST mark the tool call as completed when its final update has status `completed`

#### Scenario: Plan notifications render as plan entries
- **WHEN** Hermes emits a `session/update` with `sessionUpdate: "plan"`
- **THEN** archon MUST produce a plan transcript event carrying the plan entries and their priorities and statuses

#### Scenario: Thinking chunks render as thinking output
- **WHEN** Hermes emits a `session/update` with `sessionUpdate: "agent_thought_chunk"`
- **THEN** archon MUST produce a thinking transcript event carrying that chunk's content

#### Scenario: Unknown sessionUpdate variants are preserved, not discarded
- **WHEN** Hermes emits a `session/update` whose `sessionUpdate` discriminator archon does not yet recognise
- **THEN** archon MUST record the raw payload on a generic passthrough event
- **AND** archon MUST NOT fail the turn

### Requirement: Archon SHALL surface Hermes approval requests through its existing approvals flow
When Hermes issues a `session/request_permission` request (for example, to approve a dangerous terminal command), archon SHALL route that request to its existing approvals UI, MUST NOT auto-approve, and SHALL return the user's decision to Hermes before Hermes proceeds.

#### Scenario: Pending approval is shown to the user
- **WHEN** Hermes issues `session/request_permission` during a turn
- **THEN** archon MUST emit an approval-pending transcript event carrying the tool call's description and options
- **AND** archon MUST NOT respond to Hermes until the user has made a choice

#### Scenario: User decision maps to ACP permission outcome
- **WHEN** the user selects an approval option
- **THEN** archon MUST respond to the originating `session/request_permission` request with the matching ACP outcome
- **AND** the selection MUST map to the ACP semantics of `allow_once`, `allow_always`, or `deny`

#### Scenario: Turn cancellation cancels the pending approval
- **WHEN** the turn is cancelled while a `session/request_permission` request is outstanding
- **THEN** archon MUST respond to that request with the `cancelled` outcome
- **AND** archon MUST NOT leave the agent waiting for a permission response

### Requirement: Archon SHALL support interrupting a Hermes turn
Archon's interrupt action for a Hermes session SHALL send a `session/cancel` notification for the ACP session id, and the in-flight `session/prompt` call SHALL resolve with a `cancelled` stop reason that archon surfaces as a turn-cancelled transcript event.

#### Scenario: Interrupt during a streaming turn ends the turn as cancelled
- **WHEN** archon interrupts a Hermes session whose `session/prompt` is in flight
- **THEN** archon MUST send a `session/cancel` notification for that session id
- **AND** the in-flight prompt call MUST resolve with `stopReason: "cancelled"`
- **AND** archon MUST produce a turn-cancelled transcript event

#### Scenario: Interrupt after turn completion is a no-op
- **WHEN** archon interrupts a Hermes session whose last `session/prompt` has already completed
- **THEN** archon MUST NOT send `session/cancel`
- **AND** archon MUST NOT produce a cancellation transcript event

### Requirement: Archon SHALL scope Hermes session resume to the live subprocess
Archon SHALL NOT claim cross-restart session resume for Hermes. Within a live `hermes acp` subprocess, archon MAY call `session/load` to resume a previously created session id. If the subprocess has exited or the session id is not known to it, archon SHALL surface the session as ended rather than silently starting a fresh session.

#### Scenario: Resume within a live subprocess succeeds
- **WHEN** archon resumes a Hermes session while the owning subprocess is still running
- **AND** the ACP agent advertises the `loadSession` capability
- **THEN** archon MUST issue `session/load` with the stored session id
- **AND** archon MUST continue the session without creating a new ACP session id

#### Scenario: Resume against a dead subprocess is surfaced as session-ended
- **WHEN** archon attempts to resume a Hermes session whose owning subprocess has exited
- **THEN** archon MUST surface the session to the user as ended
- **AND** archon MUST NOT silently create a new Hermes session under the old session id

### Requirement: Archon SHALL allow Hermes command, args, model, and env overrides via config
Archon SHALL expose a `CoreHermesProviderConfig` (command, args, default model, allowed models, extra env) parallel in shape to `CoreCodexProviderConfig` and `CoreClaudeProviderConfig`. Resolving the Hermes command SHALL honor a configured override before falling back to `hermes` on `$PATH`, matching the existing `ProviderCommand` resolution pattern.

#### Scenario: Command override is used when configured
- **WHEN** the core config sets a custom command for the `hermes` provider
- **THEN** archon MUST resolve that command path before falling back to the default `hermes` candidate

#### Scenario: Extra env vars are passed to the subprocess
- **WHEN** the core config supplies additional environment variables for `hermes`
- **THEN** archon MUST include those variables in the Hermes subprocess environment
- **AND** archon MUST preserve the caller's existing environment variables
