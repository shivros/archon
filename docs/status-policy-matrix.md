# Status Policy Matrix

This document defines how UI status updates should be emitted from `internal/app`.

## Intent

- Keep status behavior consistent across controllers/reducers/stream handlers.
- Prevent toast spam for high-frequency background updates.
- Make severity explicit at the callsite (`validation`, `copy`, `background`, etc).

## API Entry Point

All status updates flow through:

- `Model.setStatusByEvent(...)` in `internal/app/model_status_policy.go`.

Avoid writing `m.status = ...` directly outside that file.

## Event Matrix

| Event helper | Event kind | Toast | Level | Typical usage |
| --- | --- | --- | --- | --- |
| `setStatusMessage` | `statusEventStatusOnly` | No | n/a | Mode hints, navigation text, ephemeral UI labels |
| `setValidationStatus` | `statusEventValidationWarning` | Yes | warning | Missing required input/selection, invalid preconditions |
| `setBackgroundStatus` | `statusEventBackgroundInfo` | No | info | Frequent async progress (`streaming`, `tail updated`) |
| `setBackgroundError` | `statusEventBackgroundError` | Yes | error | Async fetch/stream failures |
| `setApprovalStatus` | `statusEventApprovalWarning` | Yes | warning | Approval-required prompts |
| `setCopyStatusInfo` | `statusEventCopyInfo` | Yes | info | Successful copy |
| `setCopyStatusWarning` | `statusEventCopyWarning` | Yes | warning | Copy requested with no valid target |
| `setCopyStatusError` | `statusEventCopyError` | Yes | error | Clipboard failure |
| `setStatusInfo` | `statusEventActionInfo` | Yes | info | Discrete successful user actions |
| `setStatusWarning` | `statusEventActionWarning` | Yes | warning | Non-validation warnings tied to actions |
| `setStatusError` | `statusEventActionError` | Yes | error | Action failures |

## Usage Guidance

- Use `setStatusMessage` for high-churn states where toasts would be noisy.
- Prefer `setValidationStatus` for user-correctable issues.
- Prefer `setBackgroundError` for async pipeline failures (polling/stream/fetch).
- Use copy-specific helpers for clipboard flows (consistency and future tuning).
- If adding a new event category, update:
  - `statusEvent` constants,
  - `statusPolicyForEvent`,
  - `docs/status-policy-matrix.md`,
  - tests in `internal/app/model_status_policy_test.go`.
