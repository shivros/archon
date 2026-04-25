## 1. Template Contract

- [x] 1.1 Audit workflow template loading and `GET /v1/workflow-templates` so custom-file replacement and built-in fallback behavior match the spec.
- [x] 1.2 Add or tighten tests for custom template replacement and built-in default exposure.

## 2. Run Lifecycle Contract

- [x] 2.1 Audit run creation/list/get/start/pause/resume/stop/rename/dismiss/undismiss/decision handlers so they match the documented API contract.
- [x] 2.2 Add or tighten tests for dependency-queued runs, display-prompt resolution, dismissal visibility, and best-effort stop interruption.
- [x] 2.3 Audit restart persistence so saved runs/timelines restore correctly and in-flight runs surface as interrupted failures after restart.

## 3. Metrics Contract

- [x] 3.1 Audit workflow metrics snapshot and reset handlers so telemetry exposure and zeroing behavior match the spec.
- [x] 3.2 Add or tighten tests for metrics persistence and reset semantics.

## 4. Documentation And Verification

- [x] 4.1 Update `README.md` and guided-workflow ADRs where necessary so the documented API and behavior match the OpenSpec contract.
- [x] 4.2 Run focused tests for `internal/guidedworkflows`, `internal/daemon`, and `internal/client` workflow coverage.
- [x] 4.3 Run `openspec validate add-guided-workflow-run-contracts`.
