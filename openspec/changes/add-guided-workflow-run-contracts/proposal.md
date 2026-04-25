## Why

Guided workflows are one of the largest product surfaces in the repo, with dedicated docs, daemon APIs, persistence, policy, and UI flows, but there is still no OpenSpec coverage for the core run contracts. That gap makes it hard to evolve templates, lifecycle endpoints, and telemetry intentionally.

## What Changes

- Add a contract for how guided workflow templates are sourced and exposed through the daemon.
- Add a contract for guided workflow run creation, lifecycle actions, decisions, dependencies, visibility, and restart persistence.
- Add a contract for guided workflow metrics and reset behavior.
- Lock the documented user-prompt, display-prompt, and dismissal behaviors that the UI and API clients already rely on.

## Capabilities

### New Capabilities
- `guided-workflow-templates`: Defines where workflow templates come from and how they are exposed through the daemon.
- `guided-workflow-run-lifecycle`: Defines guided workflow run creation, lifecycle actions, dependency handling, visibility, and persistence semantics.
- `guided-workflow-run-metrics`: Defines workflow telemetry snapshot and reset behavior.

### Modified Capabilities

## Impact

- **Affected code:**
  - `internal/guidedworkflows/*`
  - `internal/daemon/api_workflow_runs_handlers.go`
  - `internal/daemon/guided_workflows_bridge.go`
  - `internal/client/client.go`
  - `internal/client/client_workflow_runs_test.go`
  - `README.md`
  - guided-workflow ADRs in `docs/`
- **Affected behavior:** This change is intended to formalize existing behavior, not add a new guided-workflow feature family.
- **Dependencies:** Existing workflow-run service, persistence layer, workflow template loaders, and telemetry snapshot storage.
- **Out of scope for this change:**
  - Reworking the policy engine itself
  - Changing the guided-workflow UI flow in Bubble Tea
  - Moving guided workflows to a plugin boundary
