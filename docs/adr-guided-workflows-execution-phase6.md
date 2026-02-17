# ADR: Guided Workflow Execution Guardrails and Quality/Commit Controls (Phase 6)

## Status
Accepted (Phase 6)

## Context
Phases 1-5 delivered run orchestration, policy pauses, notifications, and UI supervision. Execution still lacked:

- explicit capability checks for automated steps
- bounded retry handling for command failures
- integrated quality gates (tests/lint/typecheck hooks)
- commit execution with conventional commit validation and approval gating
- richer auditability of step outcomes and decisions

## Decision
We introduced execution controls in the guided workflow engine:

- `ExecutionControls` with:
  - capability flags (`quality_checks`, `commit`)
  - bounded retry policy (`max_attempts`)
  - quality hooks (tests/lint/typecheck)
  - commit behavior (`require_approval`, commit message)
- built-in guarded handlers for:
  - `quality_checks`: executes configured hooks with bounded retries
  - `commit`: validates conventional commit format and executes commit command
- safe failure semantics:
  - capability-denied paths fail explicitly with audit evidence
  - command failures are retried only within policy bounds
  - failure details are recorded on step/run
- approval gating for commit:
  - when configured, run policy is hardened to require pre-commit approval checkpoint before commit execution
- stronger audit trail:
  - per-run `audit_trail` captures run/phase/step/decision actions, outcomes, attempts, and detail

## Consequences
Positive:

- Automated behavior is explicit, bounded, and explainable.
- Quality/commit steps become testable integration points instead of opaque no-ops.
- Approval-controlled commit paths are now auditable end-to-end.

Tradeoffs:

- Default quality hook commands are conservative placeholders and may need project-specific tuning.
- Commit execution remains runner-driven; real VCS integration policy is environment-dependent.
