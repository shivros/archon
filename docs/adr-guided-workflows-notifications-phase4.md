# ADR: Guided Workflow Decision Notifications and Actions (Phase 4)

## Status
Accepted (Phase 4)

## Context
Phase 3 introduced policy evaluation and pause decisions, but users still needed a practical loop to:

- receive checkpoint decisions through existing notifications
- respond with explicit decision actions
- avoid repeated prompts for duplicate turn events

## Decision
We integrated guided workflow pause handling with the existing notification/event path:

- `turn.completed` events now drive guided workflow progression for matching active runs
- policy pause outcomes emit a decision-needed notification event with actionable payload
- the event includes reason, confidence/risk summary, recommended action, and available actions
- repeated turn events and repeated pause notifications are deduplicated (turn receipts + decision receipts)

We also added explicit decision actions on runs:

- `approve_continue`
- `request_revision`
- `pause_run`

exposed through `POST /v1/workflow-runs/:id/decision`.

## Consequences
Positive:

- Decision loop is now end-to-end: pause -> notify -> explicit action.
- Existing notification infrastructure remains the transport.
- Event replay does not spam duplicate decision notifications.

Tradeoffs:

- Notification trigger remains `turn.completed` with `status=decision_needed` for compatibility; a dedicated trigger may be introduced later.
- Turn-driven progression currently uses in-memory receipts and is process-local.
