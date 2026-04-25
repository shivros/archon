## Why

Archon already relies on `start`, `kill`, and snapshot `tail` as the basic CLI session lifecycle, but only the newer session-management commands have OpenSpec coverage. That leaves core automation paths undocumented even though the README, command help, and tests already treat them as stable behavior.

## What Changes

- Capture the current CLI contract for `archon start`, including required provider selection, request-shaping flags, and success output.
- Capture the current CLI contract for `archon kill`, including its positional session-id input and `ok` success output.
- Capture the current snapshot contract for `archon tail`, including `--lines`, JSON-array output, and the distinction between snapshot mode and the separately specified follow mode.
- Lock the shared error-surfacing expectations for these commands so future refactors do not accidentally change stderr/exit behavior.

## Capabilities

### New Capabilities
- `cli-session-start`: Defines how `archon start` validates input, forwards start-session options to the daemon, and reports success or failure.
- `cli-session-kill`: Defines how `archon kill` targets an existing session and reports the result.
- `cli-session-tail`: Defines how `archon tail` fetches buffered output as a JSON snapshot, including `--lines`.

### Modified Capabilities

## Impact

- **Affected code:**
  - `cmd/archon/command_start.go`
  - `cmd/archon/command_kill.go`
  - `cmd/archon/command_tail.go`
  - `cmd/archon/client_adapter.go`
  - `cmd/archon/commands_test.go`
  - `README.md`
- **Affected behavior:** No net-new product behavior is intended; this change documents and hardens the existing lifecycle command surface.
- **Dependencies:** Existing daemon session endpoints and the current client adapter methods.
- **Out of scope for this change:**
  - `archon tail --follow`, which is already covered by `add-cli-tail-follow`
  - `archon send`, `archon session`, approvals, and `archon ps --json`, which already have dedicated changes
