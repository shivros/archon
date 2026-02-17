# ADR: Guided Workflow Engine Skeleton (Phase 2)

## Status
Accepted (Phase 2)

## Context
Phase 1 introduced config gating and basic module boundaries for guided workflows.
Phase 2 needs runnable domain and lifecycle foundations without committing to final execution logic.

## Decision
We added a domain model and deterministic in-memory engine in `internal/guidedworkflows`:

- Domain entities:
  - `WorkflowTemplate`
  - `WorkflowRun`
  - `PhaseRun`
  - `StepRun`
  - `CheckpointDecision`
- Built-in template:
  - `SOLID Phase Delivery` (`solid_phase_delivery`)
  - step sequence:
    - `phase plan`
    - `implementation`
    - `SOLID audit`
    - `mitigation plan`
    - `mitigation implementation`
    - `test gap audit`
    - `test implementation`
    - `quality checks`
    - `commit`
- Engine skeleton:
  - deterministic `Advance(...)` state transition with no-op step handlers by default
  - optional per-step handlers for future real execution
- Run lifecycle service (`RunService`):
  - `CreateRun`
  - `StartRun`
  - `PauseRun`
  - `ResumeRun`
  - `GetRun`
  - `GetRunTimeline`
  - plus internal progression helper `AdvanceRun`

Daemon integration:

- New lifecycle handlers in `internal/daemon/api_workflow_runs_handlers.go`:
  - `POST /v1/workflow-runs`
  - `POST /v1/workflow-runs/:id/start`
  - `POST /v1/workflow-runs/:id/pause`
  - `POST /v1/workflow-runs/:id/resume`
  - `GET /v1/workflow-runs/:id`
  - `GET /v1/workflow-runs/:id/timeline`

## Consequences
Positive:

- End-to-end no-op workflow execution path is runnable in dev/test.
- State transitions are explicit and testable.
- API/service layers are shaped for later persistence and plugin extraction.

Tradeoffs:

- Run state is in-memory only (non-durable).
- Step handlers are skeleton no-ops by default.
- No scheduling/worker loop yet; progression is lifecycle-triggered and service-driven.
