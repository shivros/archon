# ADR: Guided Workflow Foundation (Phase 1)

## Status
Accepted (Phase 1)

## Context
Archon needs guided LLM workflows that can be enabled by config today and later extracted into a true plugin with minimal rewrites.

Product constraints for this phase:

- feature is opt-in and never auto-starts by default
- default checkpoint style is `confidence_weighted`
- default mode is `guarded_autopilot`
- existing turn-end notifications should be the event source for workflow decision logic
- disabled state must preserve current behavior

## Decision
We introduced a dedicated `internal/guidedworkflows` module with a narrow orchestration boundary:

- `guidedworkflows.Orchestrator` interface:
  - `StartRun(...)` for explicit run starts from worktree/workspace context
  - `OnTurnEvent(...)` for turn-completed event ingestion
  - `Config()` / `Enabled()` for feature gating and diagnostics
- `guidedworkflows.Config` and `NormalizeConfig(...)` for stable defaults and normalization
- `guidedworkflows.New(...)` factory that returns:
  - a disabled no-op orchestrator when feature is off
  - an enabled guarded-autopilot orchestrator scaffold when feature is on

Daemon wiring is isolated through adapters:

- `newGuidedWorkflowOrchestrator(coreCfg)` maps core config to module config
- `NewGuidedWorkflowNotificationPublisher(...)` decorates the existing notification publisher
  - forwards all notifications unchanged
  - forwards `turn.completed` events to guided workflow orchestration when enabled

No API/UI behavior is auto-triggered in this phase. Explicit run invocation is available via service boundary (`SessionService.StartGuidedWorkflowRun`) for upcoming phases.

## Consequences
Positive:

- Guided workflow code is modular and can be moved behind a plugin transport later.
- Disabled config path is a true no-op, minimizing regression risk.
- Existing notification infrastructure remains the single event source for turn completion.

Tradeoffs:

- The enabled orchestrator is currently a scaffold (run creation + event hook), not full decisioning.
- Decision request emission and risk/model signals are deferred to subsequent phases.

## Plugin Extraction Path
To extract later without major rewrites:

1. Keep `Orchestrator` as the daemon-facing contract.
2. Replace in-process `guidedworkflows.New(...)` with a plugin client implementation.
3. Keep `NewGuidedWorkflowNotificationPublisher(...)` as the bridge so event routing remains unchanged.
4. Move run state from in-memory scaffold to plugin-owned persistence/transport.
