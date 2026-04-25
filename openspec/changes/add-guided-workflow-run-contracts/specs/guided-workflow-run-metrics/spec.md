## ADDED Requirements

### Requirement: The daemon SHALL expose a workflow metrics snapshot endpoint
Archon SHALL expose `GET /v1/workflow-runs/metrics` as the current guided-workflow telemetry snapshot for operational monitoring.

#### Scenario: Metrics endpoint returns the current snapshot
- **WHEN** a caller performs `GET /v1/workflow-runs/metrics`
- **THEN** the daemon MUST return the current workflow metrics snapshot as JSON
- **AND** the snapshot MUST include whether telemetry is enabled plus the currently accumulated counters

### Requirement: Workflow metrics SHALL support operational reset through `POST /v1/workflow-runs/metrics/reset`
Archon SHALL expose a reset endpoint that zeroes the current workflow metrics snapshot and returns the reset state.

#### Scenario: Metrics reset zeroes counters
- **WHEN** a caller performs `POST /v1/workflow-runs/metrics/reset`
- **THEN** the daemon MUST reset the current metrics counters
- **AND** the response MUST contain the zeroed snapshot that is now active

### Requirement: Guided workflow metrics SHALL survive daemon restarts when telemetry persistence is enabled
Workflow metrics are part of the daemon's persisted operational state. Restarting the daemon SHALL NOT discard the accumulated metrics snapshot unless an explicit reset has occurred or telemetry persistence has been disabled.

#### Scenario: Persisted metrics restore after restart
- **WHEN** guided workflow metrics have been persisted and the daemon restarts
- **THEN** the metrics snapshot returned by `GET /v1/workflow-runs/metrics` after restart MUST reflect the persisted counters rather than resetting silently
