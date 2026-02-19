# ADR: Guided Workflow Checkpoint Policy Engine (Phase 3)

## Status
Accepted (Phase 3)

## Context
Phase 2 provided a runnable workflow engine skeleton, but decisioning for auto-continue versus human checkpoint was still missing.
Phase 3 introduces explainable policy evaluation so workflow runs can pause intentionally when risk signals are present.

## Decision
We added a confidence-weighted checkpoint policy engine in `internal/guidedworkflows/policy.go` with:

- deterministic scoring and explicit `continue` / `pause` actions
- reason codes and user-facing reason messages
- severity and tier classification (`low..critical`, `tier_0..tier_3`)
- hard gates and conditional gates

Supported gates:

- ambiguity/blocker detected
- confidence below threshold
- high blast radius
- sensitive files touched
- pre-commit approval required
- failing checks

Policy sources:

- global defaults from core config (`[guided_workflows.policy]`)
- per-run overrides (`CreateRunRequest.policy_overrides`)

Run lifecycle integration:

- policy is evaluated before each step advance
- each evaluation is recorded in `checkpoint_decisions`
- latest decision is exposed at `run.latest_decision`
- pause decisions switch run state to `paused` and emit checkpoint timeline events

## Consequences
Positive:

- Decisions are explainable and reproducible in tests.
- Existing no-op execution remains functional under defaults.
- API consumers can tune behavior per run without changing global config.

Tradeoffs:

- Policy inputs are currently static/default at runtime except the built-in pre-commit signal and explicit test inputs.
- A real signal pipeline (from analysis/check tools/model telemetry) is still a follow-up.
