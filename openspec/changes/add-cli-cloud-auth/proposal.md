## Why

Cloud login is already a first-class documented CLI workflow, but there is no OpenSpec contract for the three commands that expose it. That means a device-flow regression in `login`, `whoami`, or `logout` could slip through without an intentional spec change even though users and docs already depend on the current behavior.

## What Changes

- Add a CLI contract for `archon login`, including fallback instructions, optional browser launch, polling behavior, and successful link output.
- Add a CLI contract for `archon whoami`, including linked and unlinked human-readable output.
- Add a CLI contract for `archon logout`, including full and partial unlink messaging.
- Lock shared error-handling expectations for the cloud-auth commands.

## Capabilities

### New Capabilities
- `cli-cloud-auth`: Defines the user-visible contract for `archon login`, `archon whoami`, and `archon logout`.

### Modified Capabilities

## Impact

- **Affected code:**
  - `cmd/archon/command_login.go`
  - `cmd/archon/command_whoami.go`
  - `cmd/archon/command_logout.go`
  - `cmd/archon/client_adapter.go`
  - `cmd/archon/commands_test.go`
  - `README.md`
- **Affected behavior:** No new cloud features are introduced; this change specifies and hardens the existing device-auth flow.
- **Dependencies:** Existing daemon-backed cloud-auth APIs and persisted cloud credential state.
- **Out of scope for this change:**
  - Changes to the remote Archon Cloud API
  - Altering the device-flow UX beyond what is needed to align implementation with the documented contract
