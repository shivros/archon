# SOLID Plan: Queueable Guided Workflow Chaining

## 1. Objective
Implement first-class guided workflow chaining so a workflow run can be queued behind one or more existing workflow runs, across any workspace/worktree in Archon, with no chain-depth limit.

Example target behavior:
- Run B (subpackage B) can depend on Run A (subpackage A).
- Run B stays queued until Run A reaches `completed`.
- When Run A completes, Run B auto-starts.
- This can be repeated for arbitrarily long chains.

This is a direct product behavior change (no feature flags, no progressive rollout path for this capability).

## 2. Product Semantics (Final Contract)
1. A new run may declare dependencies on any existing guided workflow run IDs in Archon.
2. `StartRun` on a dependency-free run behaves exactly as today.
3. `StartRun` on a run with unmet dependencies transitions to `queued` (not `running`).
4. A queued run auto-transitions to `running` when all dependencies are `completed`.
5. Dependencies are success-gated: `failed` or `stopped` upstream runs keep downstream queued with a blocking reason.
6. If a failed/stopped upstream run is later resumed and then completes, downstream queued runs can proceed.
7. Dismissal does not break dependency resolution (dependencies key off run ID, not visibility).
8. Chaining depth is unbounded; implementation must be iterative (no recursive depth assumptions).

## 3. SOLID Architecture

### 3.1 Single Responsibility
Split current run-service concerns into explicit dependency components:
- `RunDependencyValidator`: validates dependency references and graph rules.
- `RunDependencyIndex`: maintains reverse index (`upstream -> dependents`).
- `RunDependencyEvaluator`: determines readiness/blocking state from current run statuses.
- `QueuedRunActivator`: promotes ready queued runs to running and triggers dispatch queue.

`InMemoryRunService` remains orchestration glue, not the place for dependency algorithm details.

### 3.2 Open/Closed
Define dependency condition strategy now, defaulting to `completed`:
- `DependencyCondition` interface (or enum + evaluator strategy).
- Initial supported condition: `on_completed`.
- Future conditions (`on_stopped`, `on_failed`, etc.) can be added without editing core lifecycle flow.

### 3.3 Liskov Substitution
Keep existing `RunLifecycleService` behavior valid for callers that do not use dependencies:
- Old create/start flows still work.
- New dependency-aware logic is additive and must not change no-dependency semantics.

### 3.4 Interface Segregation
Avoid bloated lifecycle interfaces by adding narrow dependency-focused interfaces:
- `RunDependencyQueryService` for dependency state reads.
- `RunDependencyMutationService` for enqueue/dependency assignment if needed.

### 3.5 Dependency Inversion
`InMemoryRunService` depends on abstractions, not concrete maps/algorithms:
- Inject validator/evaluator/index via `RunServiceOption` defaults.
- Keep persistence and queue mechanics behind existing interfaces (`RunPersistenceService`, `DispatchQueue`).

## 4. Domain Model Changes
Files:
- `internal/guidedworkflows/domain.go`
- `internal/guidedworkflows/service.go`

Additions:
1. `WorkflowRunStatusQueued`.
2. `WorkflowRun.Dependencies []RunDependency`.
3. `WorkflowRun.DependencyState RunDependencyState`.
4. `CreateRunRequest.DependsOnRunIDs []string`.

New structs:
- `RunDependency`:
  - `RunID string`
  - `Condition string` (initial value `completed`)
- `RunDependencyState`:
  - `Ready bool`
  - `Blocking bool`
  - `Reason string`
  - `Unmet []RunDependencySnapshot`
  - `LastEvaluatedAt time.Time`
- `RunDependencySnapshot`:
  - `RunID string`
  - `RequiredCondition string`
  - `ObservedStatus WorkflowRunStatus`
  - `Satisfied bool`

## 5. Backend Lifecycle Design

### 5.1 Creation
- Validate each dependency run ID exists in run store.
- Reject self-reference.
- Normalize, dedupe, and persist dependency list.
- Build reverse index entries for each dependency.

### 5.2 Start
Current `StartRun` evolves as:
1. If no dependencies: existing `running` transition + dispatch.
2. If dependencies exist:
  - evaluate readiness.
  - if not ready: set `status=queued`, persist dependency state, emit `run_queued` timeline/audit.
  - if ready: same as normal start.

### 5.3 Auto-Activation
When any run status changes:
1. Query dependents from reverse index.
2. Enqueue each dependent for dependency re-evaluation via existing dispatch queue (`reason=dependency_changed`).
3. During queued dependent advance:
  - if still unmet: remain queued, refresh dependency state.
  - if ready: transition to `running`, emit `run_started_from_queue`, then continue normal step dispatch.

### 5.4 Terminal and Recovery Semantics
- `completed` upstream satisfies dependency.
- `failed/stopped` upstream sets downstream `DependencyState.Blocking=true` with clear reason.
- If upstream later re-enters running and eventually completes, downstream can unblock automatically.

### 5.5 Restart Safety
`restoreRuns(...)` must:
- rebuild dependency reverse index from persisted runs.
- preserve `queued` runs (do not mark queued as interrupted failure).
- enqueue one dependency reconciliation sweep after restore.

## 6. API and Client Contract
Files:
- `internal/daemon/api.go`
- `internal/daemon/api_workflow_runs_handlers.go`
- `internal/client/dto.go`
- `internal/client/client.go`

Changes:
1. Extend create payload:
- `depends_on_run_ids: []string`
2. Return dependency fields in run payload (list/get/timeline contexts unchanged).
3. Keep start endpoint unchanged (`POST /v1/workflow-runs/:id/start`); it now may return `status=queued`.
4. Error mapping additions:
- invalid dependency run ID -> `400`
- dependency graph violation -> `409`

No new rollout toggles are introduced.

## 7. TUI / UX Plan
Files:
- `internal/app/guided_workflow_controller.go`
- `internal/app/model_guided_workflow.go`
- `internal/app/model_update_messages.go`
- `internal/app/commands.go`
- relevant tests in `internal/app/*guided_workflow*test.go`

Behavior:
1. Setup flow adds a dependency picker fed from existing workflow runs.
2. User can select one or multiple upstream runs across workspaces/worktrees.
3. Create request includes `depends_on_run_ids`.
4. Existing automatic create->start flow remains.
5. If start returns queued:
- status line: `guided workflow queued: waiting for dependencies`
- live panel shows unmet dependency statuses and blocking reason.
6. Update `runStatusText(...)` to include `queued`.

## 8. Persistence and Migration
Files:
- `internal/store/workflow_run_store.go`
- `internal/store/bbolt_workflow_run_store.go`
- store tests

Plan:
1. Backward-compatible JSON field additions (old snapshots load with empty dependencies).
2. Keep migration non-destructive; no mandatory manual migration step.
3. Optional: bump `workflowRunSchemaVersion` to `2` for explicit format traceability.

## 9. Observability
Files:
- `internal/guidedworkflows/service.go`
- `internal/daemon/api_workflow_runs_handlers.go`

Add telemetry/audit events:
- `run_queued`
- `run_dependency_rechecked`
- `run_dependency_blocked`
- `run_started_from_queue`

Metrics additions:
- `RunsQueued`
- `RunsAutoStartedFromQueue`
- `DependencyBlocks`
- `DependencyRechecks`

## 10. Testing Strategy

### 10.1 Guided Workflow Service Tests
File: `internal/guidedworkflows/service_test.go`
- create run with valid/invalid dependencies.
- start dependency-free run -> running.
- start dependent run with unmet dependency -> queued.
- upstream completion auto-starts downstream.
- chain A -> B -> C executes in order.
- upstream failed keeps dependent queued+blocked.
- upstream resume + completion unblocks dependent.
- restart restore preserves queued runs and re-indexes dependencies.

### 10.2 API Tests
File: `internal/daemon/api_workflow_runs_test.go`
- POST create with `depends_on_run_ids` round-trips in responses.
- start endpoint returning queued is handled as success.
- invalid dependency IDs return expected status/error payload.

### 10.3 Client Tests
File: `internal/client/client_workflow_runs_test.go`
- DTO serialization/deserialization for dependency fields.

### 10.4 App Tests
Files:
- `internal/app/model_guided_workflow_test.go`
- `internal/app/guided_workflow_controller_test.go`
- `internal/app/model_update_messages_test.go`

Coverage:
- dependency selection included in create request.
- queued start status messaging.
- queued run rendering of unmet dependencies.

## 11. Delivery Phases

### Phase 1: Domain + Service Core
- Add status/fields/types.
- Implement validator/index/evaluator/activation internals.
- Add unit tests for lifecycle and chain behavior.

### Phase 2: API + Client
- Extend create contracts and response mapping.
- Add API/client tests.

### Phase 3: TUI
- Dependency picker in guided setup.
- Queued state rendering + status text updates.
- App-level tests.

### Phase 4: Hardening
- Restart reconciliation.
- Telemetry and timeline audit completeness.
- End-to-end integration scenario test (A->B->C).

## 12. Definition of Done
1. Users can queue a guided workflow behind any existing guided workflow run in Archon.
2. Downstream run auto-starts when upstream completes successfully.
3. Multi-hop chains (3+ depth) work deterministically.
4. No chain depth cap is enforced.
5. Behavior survives daemon restarts.
6. No new feature flags or progressive rollout controls are introduced.
7. Existing non-dependent workflow behavior remains unchanged.
