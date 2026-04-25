## Why

Archon already ships with two background automation features that materially affect user experience: asynchronous title generation and daemon-side notifications. Both are documented and test-covered, but neither has an OpenSpec contract, which makes subtle regressions in queueing, compare-and-set behavior, or notification resolution much easier to miss.

## What Changes

- Add a contract for asynchronous session and workflow title generation, including enablement, queueing, compare-and-set updates, and best-effort failure handling.
- Add a contract for daemon-side notifications, including layered override precedence, method selection, script-command execution, and deduplication.
- Lock the documented integration points where session/workflow creation enqueue title-generation work and notification settings can be overridden per worktree or session.

## Capabilities

### New Capabilities
- `title-generation`: Defines Archon's asynchronous session and workflow title-generation behavior.
- `notifications`: Defines Archon's daemon-side notification resolution and dispatch contract.

### Modified Capabilities

## Impact

- **Affected code:**
  - `internal/daemon/title_generation*.go`
  - `internal/daemon/title_generation_provider*.go`
  - `internal/daemon/notification_*.go`
  - `internal/daemon/api.go`
  - `internal/daemon/api_sessions*.go`
  - `internal/daemon/api_workflow_runs*.go`
  - `internal/config/settings.go`
  - `README.md`
- **Affected behavior:** No new product features are introduced; this change specifies and hardens the existing automation surfaces.
- **Dependencies:** Existing OpenRouter-backed title generator, notification sinks, and persisted session/workflow metadata.
- **Out of scope for this change:**
  - Adding new title-generation providers
  - Adding new notification sink types beyond the current supported methods
  - UI-specific toast behavior, which remains separate from daemon notifications
