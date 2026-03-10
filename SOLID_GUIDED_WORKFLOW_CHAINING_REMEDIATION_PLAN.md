# SOLID Remediation Plan: Guided Workflow Chaining Findings

## 1. Goal
Address the previously identified SOLID gaps in the chaining plan before/while implementation so the resulting system is strongly typed, dependency-inverted, interface-minimal, and migration-safe.

This plan remediates the five findings from the prior audit.

## 2. Findings to Remediate
1. No implementation-level SOLID verification path existed yet.
2. Dependency condition used weak string typing.
3. Proposed interfaces risked ISP violation (possible interface sprawl).
4. DIP wiring/composition responsibilities were underspecified.
5. Persistence version bump was optional, leaving contract ambiguity.

## 3. Remediation Architecture

### 3.1 Strong Domain Typing (Fixes Finding #2)
Decisions:
- Introduce typed dependency condition:
  - `type DependencyCondition string`
  - constants:
    - `DependencyConditionOnCompleted DependencyCondition = "on_completed"`
- Replace raw string fields with typed fields:
  - `RunDependency.Condition DependencyCondition`
  - `RunDependencySnapshot.RequiredCondition DependencyCondition`
- Add normalization/validation helpers:
  - `NormalizeDependencyCondition(raw string) (DependencyCondition, bool)`
  - `ValidateDependencyCondition(condition DependencyCondition) error`

Guardrails:
- Reject unknown values at API boundary (`400`).
- Default condition only at create-time normalization, not implicitly during evaluation.

### 3.2 Consumer-Driven Interfaces (Fixes Finding #3)
Decisions:
- Do not add new public service interfaces until at least one concrete consumer requires them.
- Keep dependency logic behind internal components, not expanded daemon-facing contracts.

Implementation shape:
- Internal-only contracts in `internal/guidedworkflows`:
  - `dependencyValidator`
  - `dependencyGraphIndex`
  - `dependencyEvaluator`
  - `queuedRunActivator`
- `RunLifecycleService` remains externally unchanged except additive request/response fields.

Guardrails:
- Add an “Interface Introduction Checklist” in code comments/tests:
  - at least one call site,
  - at least one test double,
  - measurable cohesion benefit.

### 3.3 Explicit Composition Root (Fixes Finding #4)
Decisions:
- `newGuidedWorkflowRunService(...)` in daemon bridge is the composition root.
- Construction flow is explicit and ordered:
  1. Build defaults for validator/index/evaluator/activator.
  2. Inject into `NewRunService(...)` via dedicated `RunServiceOption`s.
  3. Keep all runtime orchestration in `InMemoryRunService`, all policy/algorithm in injected collaborators.

New options (internal):
- `WithDependencyValidator(...)`
- `WithDependencyGraphIndex(...)`
- `WithDependencyEvaluator(...)`
- `WithQueuedRunActivator(...)`

Guardrails:
- `NewRunService` always resolves nil collaborators to safe defaults.
- Service tests assert behavior with both defaults and injected test doubles.

### 3.4 Mandatory Snapshot Versioning (Fixes Finding #5)
Decisions:
- Bump workflow run snapshot schema version from `1` to `2`.
- Version bump is required in this change set.

Migration policy:
- Backward read compatibility:
  - v1 snapshots deserialize with zero-value dependency fields.
  - on write, snapshots are persisted as v2.
- No destructive migration.
- Add explicit tests for v1-read/v2-write round-trip.

### 3.5 Implementation SOLID Audit Loop (Fixes Finding #1)
Decisions:
- Introduce a mandatory post-implementation SOLID audit checkpoint before merge.

Audit checklist:
- SRP: dependency algorithms isolated from lifecycle orchestration.
- OCP: adding `DependencyCondition` variants requires new evaluator strategy, not lifecycle rewrites.
- LSP: no-dependency runs remain behavior-identical.
- ISP: no new public interfaces without direct consumers.
- DIP: service depends on collaborator abstractions via options.

Evidence required:
- passing test suite sections,
- file-level architecture notes,
- explicit mapping of each principle to concrete code locations.

## 4. Work Plan

### Phase A: Domain and Contracts
Files:
- `internal/guidedworkflows/domain.go`
- `internal/daemon/api.go`
- `internal/client/dto.go`

Tasks:
1. Add `DependencyCondition` type/const/normalizer.
2. Update dependency model fields to typed condition.
3. Extend create request DTO with dependency IDs (and optional condition extension structure if needed).
4. Add API error for invalid dependency condition.

### Phase B: Internal Collaborators and Wiring
Files:
- `internal/guidedworkflows/service.go`
- `internal/guidedworkflows/*dependency*.go` (new)
- `internal/daemon/guided_workflows_bridge.go`

Tasks:
1. Implement validator/index/evaluator/activator as internal collaborators.
2. Add `RunServiceOption` injection points.
3. Wire defaults in `NewRunService`.
4. Wire composition in daemon bridge explicitly.

### Phase C: Persistence Versioning
Files:
- `internal/store/workflow_run_store.go`
- `internal/store/bbolt_workflow_run_store.go`
- store tests

Tasks:
1. Set `workflowRunSchemaVersion = 2`.
2. Ensure legacy v1 snapshots read successfully.
3. Ensure new writes persist v2 consistently.
4. Add regression tests for mixed-version data.

### Phase D: Verification and Audit Closure
Files:
- `internal/guidedworkflows/service_test.go`
- `internal/daemon/api_workflow_runs_test.go`
- `internal/client/client_workflow_runs_test.go`
- `docs/architecture.md` (targeted update)

Tasks:
1. Add principle-aligned test coverage for typed condition, no-dependency compatibility, and collaborator injection.
2. Execute SOLID audit checklist with code references.
3. Update architecture doc with final collaborator boundaries.

## 5. Acceptance Criteria
1. Dependency conditions are type-safe end-to-end (no raw string comparison in lifecycle core).
2. Public service interfaces are not expanded without consumer justification.
3. Composition root and collaborator wiring are explicit and test-covered.
4. Workflow run snapshot schema is version 2 with backward-compatible reads.
5. A post-implementation SOLID audit document/checklist is completed with file references.

## 6. Risks and Mitigations
- Risk: Over-abstraction introduces complexity.
  - Mitigation: keep collaborator interfaces internal and minimal.
- Risk: Backward compatibility regressions in run restore.
  - Mitigation: add v1 fixture tests and restart-reconciliation tests.
- Risk: Scope drift into broader workflow redesign.
  - Mitigation: confine changes to dependency/chaining pathways and persistence versioning.

## 7. Execution Order
1. Domain typing + request validation.
2. Internal collaborator extraction + wiring.
3. Persistence schema v2 + migration tests.
4. API/client/app behavior verification.
5. Final SOLID audit and architecture note update.
