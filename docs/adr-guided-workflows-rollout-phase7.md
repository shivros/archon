# ADR: Guided Workflows Phase 7 (Rollout + Telemetry)

## Status
Accepted

## Context
Phase 6 added execution controls and auditability, but guided workflows still needed:

- explicit rollout guardrails for safe enablement
- user/developer documentation for day-1 usage
- telemetry for product iteration and operational tuning

## Decision
We introduced Phase 7 controls in four layers:

1. Core config rollout schema (`[guided_workflows.rollout]`)
   - `telemetry_enabled`
   - `max_active_runs`
   - `automation_enabled`
   - `allow_quality_checks`
   - `allow_commit`
   - `require_commit_approval`
   - `max_retry_attempts`

2. Daemon bridge mapping
   - translates core config into `guidedworkflows.ExecutionControls`
   - passes max-active-run guardrail and telemetry flag via run-service options
   - provides a persistence adapter backed by app-state storage

3. Run-service telemetry snapshot
   - tracks `runs_started`, `runs_completed`, `runs_failed`
   - tracks `pause_count` and `pause_rate`
   - tracks approval latency (`avg`/`max` ms)
   - tracks intervention causes from policy/user interventions

4. Transport endpoint
   - `GET /v1/workflow-runs/metrics`
   - `POST /v1/workflow-runs/metrics/reset` for manual metric window resets
   - returns current metrics snapshot for iteration/operations

## Consequences
### Positive
- Safe defaults: automation remains opt-in; manual start remains explicit.
- Rollout risk is constrained by `max_active_runs` and commit-approval defaults.
- Product teams can observe pause rates, approvals, and intervention causes without parsing logs.

### Tradeoffs
- Metrics are aggregate counts, not per-workspace series.
- Persistence currently reuses app-state storage, so telemetry writes share the same state-save path as UI state updates.

## Extension Points
- Replace in-memory metrics with persistent time-series sink.
- Add per-workspace/worktree metrics dimensions.
- Export metrics via broader observability integrations while keeping current endpoint as stable API.
