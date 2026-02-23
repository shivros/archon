# Architecture

This note documents the current flow so refactors can preserve behavior.

## Request/Response Flow

```
UI (internal/app) -> typed HTTP/SSE client (internal/client)
                   -> daemon API (internal/daemon)
                   -> provider/session runtime (codex, claude, custom)
```

1. `cmd/archon/main.go` starts either the daemon or the Bubble Tea UI.
2. The UI `Model` in `internal/app/model.go` coordinates modes, selection, rendering, polling, and stream consumption.
3. The UI talks to the daemon through interfaces in `internal/app/api.go`, backed by `internal/client.Client`.
4. The client uses REST endpoints under `/v1/...` and SSE endpoints for live streams:
   - `/v1/sessions/:id/tail?follow=1` for log stream chunks
   - `/v1/sessions/:id/events?follow=1` for codex events
   - `/v1/sessions/:id/items?follow=1` for item-based providers
5. `internal/daemon/api.go` handles HTTP transport/routing and delegates to services (`SessionService`, workspace/state services).
6. `SessionService` and `SessionManager` orchestrate provider adapters:
   - codex provider (`provider_codex.go`)
   - claude provider (`provider_claude.go`)
   - opencode/kilocode server provider (`provider_opencode.go`)
   - generic process provider (`provider_exec.go`)
7. Guided workflow orchestration is isolated behind `internal/guidedworkflows` and wired through daemon adapters (`guided_workflows_bridge.go`), so it can later move to a plugin boundary.
8. Guided workflow run lifecycle HTTP endpoints are exposed by daemon handlers in `internal/daemon/api_workflow_runs_handlers.go` (`/v1/workflow-runs` and `/v1/workflow-runs/:id/...`).
9. Guided workflow policy evaluation (confidence-weighted with hard/conditional gates) is part of run progression in `internal/guidedworkflows/policy.go` and persisted into run decision metadata.
10. Turn-completed notifications are also consumed by the guided workflow bridge to advance matching runs and publish actionable decision-needed notifications when policy pauses are triggered.
11. Guided workflow rollout guardrails are read from core config (`guided_workflows.rollout`) and translated by daemon bridge adapters into run-service options (max active runs, automation controls, retries, commit approval requirements).
12. Guided workflow telemetry lives inside `internal/guidedworkflows` as a snapshot API (`GetRunMetrics`), is persisted through daemon adapters into app state, and is exposed at `GET /v1/workflow-runs/metrics` with operational reset support at `POST /v1/workflow-runs/metrics/reset`.
13. Guided workflow templates are sourced from `~/.archon/workflow_templates.json`; when present, that file fully replaces built-in defaults (no merge). Built-in defaults are defined in `internal/guidedworkflows/default_workflow_templates.json` and are used only when no user template file exists.
14. Guided workflow step execution supports prompt dispatch through an injected `StepPromptDispatcher`; when dispatched, steps move to `awaiting_turn` and are only completed by turn-completed events, preserving turn-driven progression.
15. Workflow template steps may optionally include `runtime_options` (for example model/reasoning/access). These are applied as per-turn overrides during step dispatch and, on successful send, become the session default runtime options for later turns.

## Streaming and Persistence

- Streaming state in UI is consumed via:
  - `StreamController` (log chunks),
  - `CodexStreamController` (event stream + approvals),
  - `ItemStreamController` (item stream providers).
- Persistent app/session metadata is stored by daemon-backed stores in `internal/store` and retrieved by the UI through snapshot calls (`sessions`, `history`, `approvals`, app state).
- UI keeps a transcript cache keyed by sidebar selection so switching sessions is fast while still reconciling with history snapshots.

## Status and Toast Policy

- UI status/toast behavior is centralized in `internal/app/model_status_policy.go`.
- Event categories and toast severity rules are documented in `docs/status-policy-matrix.md`.
- New status patterns should extend the policy table instead of writing direct `m.status = ...` assignments.

## Selection Focus Flow

- Selection transitions are orchestrated by `SelectionTransitionService` in `internal/app/selection_navigation.go`.
- Selection history intent is governed by `SelectionOriginPolicy`.
- Pane ownership rules are governed by `SelectionFocusPolicy` in `internal/app/selection_focus_policy.go`.
- The default focus policy keeps workflow opens user-driven and makes explicit session selections leave guided workflow mode.
- Keep policy implementations deterministic and side-effect free so event and polling paths behave consistently.
- Add a new policy when behavior needs to vary by selection source or mode without changing transition orchestration.
- Update transition orchestration only when introducing a new lifecycle phase that cannot be represented as policy decisions.

## Phase 0 Baseline Contract

Phase 0 must keep behavior stable for:

- streaming updates and close states,
- compose/send local state transitions,
- session selection load/reset behavior,
- approval visibility in both polling and event-driven paths.
