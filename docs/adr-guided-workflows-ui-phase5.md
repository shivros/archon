# ADR: Guided Workflow Supervision UI (Phase 5)

## Status
Accepted (Phase 5)

## Context
Phases 1-4 established configuration, run lifecycle/state machine, policy decisions, and pause notifications. The app still needed a manual, in-product UI to:

- start guided runs from task/worktree context
- supervise progression with a readable timeline
- act on pause decisions without a settings surface

## Decision
We added a dedicated guided workflow UI mode in the TUI with modular controller boundaries:

- context entry points:
  - worktree context menu: `Start Guided Workflow`
  - session/task context menu: `Start Guided Workflow`
- guided workflow screens:
  - `Workflow Launcher`
  - `Run Setup` (template + policy sensitivity presets)
  - `Live Timeline` (phase/step state + timeline artifacts)
  - `Decision Inbox` actions (`approve_continue`, `request_revision`, `pause_run`)
  - `Post-run Summary`
- explicit explainability:
  - rendered "why paused" / "why continued" explanation from decision metadata reasons
- API boundary:
  - app-side `GuidedWorkflowAPI` interface and dedicated workflow commands/messages
  - isolated `GuidedWorkflowUIController` for future plugin extraction

## Consequences
Positive:

- Users can manually start and supervise guided runs without enabling auto-start.
- Policy pauses are visible and actionable directly in the guided view.
- UI code is kept in an isolated controller + API interface for plugin extraction.

Tradeoffs:

- Decision inbox currently comes from run polling + decision metadata, not from a standalone notification inbox view.
- Sensitivity presets currently tune threshold overrides only (not full gate matrices).
