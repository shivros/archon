## Context

Guided workflows span template loading, daemon HTTP endpoints, run orchestration, persistence, and notification/decision loops. The repo already includes several ADRs and a large test matrix, so the change should capture the current contract in a few coherent spec areas instead of scattering it across many tiny documents.

## Goals / Non-Goals

**Goals:**
- Specify template source precedence and template-list exposure.
- Specify run lifecycle, visibility, dependency, and restart behavior.
- Specify telemetry snapshot/reset behavior.

**Non-Goals:**
- Rewriting the workflow engine or policy model.
- Freezing every internal timeline event field.
- Specifying the entire TUI workflow UX.

## Decisions

- Split the contract into three capabilities:
  - template source and listing
  - run lifecycle and persistence
  - metrics
- Treat `user_prompt` and `display_user_prompt` as distinct, load-bearing fields:
  - `user_prompt` preserves authored workflow intent when present
  - `display_user_prompt` may be resolved from linked session metadata for legacy or session-owned runs
- Specify dismiss/undismiss as both run-visibility behavior and workflow-owned session-visibility synchronization, because the API and UI already depend on that coupling.
- Capture restart behavior at the service-contract level:
  - persisted runs and timelines survive restart
  - in-flight running runs do not silently resume and instead surface as interrupted failures

## Risks / Trade-offs

- **The workflow subsystem is broad** -> Grouping the requirements into three capabilities keeps the spec navigable while still covering the major contract boundaries.
- **Some behavior is policy-driven and may evolve** -> The spec focuses on externally visible lifecycle outcomes rather than internal evaluator implementation.
- **Persistence semantics can be subtle** -> Explicit restart scenarios make those expectations intentional instead of incidental.
