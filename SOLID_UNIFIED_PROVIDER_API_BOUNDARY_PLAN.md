# SOLID Plan: Unified Provider API Boundary

## Overview

This plan unifies all providers at the **daemon/API boundary** while preserving provider-specific internals behind adapters.
Goal: the UI consumes one delivery model (snapshot + follow stream) regardless of provider.

Primary outcomes:
- Remove provider-branching from UI load/stream logic.
- Standardize loading, reconnect, completion, and error semantics.
- Reduce regressions caused by divergent provider transport behavior.

---

## Problem Statement

Current boundary behavior differs by provider:
- Codex path: `history/tail/events`
- Claude/OpenCode/Kilo path: `items` stream + snapshots

This creates duplicated UI state machines and inconsistent behavior under refresh/reconnect/session updates.

Observed failure classes:
- Reload churn and visual popping during active turns.
- Stream closure without equivalent reconnect behavior.
- Snapshot/stream merge differences causing apparent message disappearance.

---

## SOLID Mapping

### SRP
- Split concerns into:
1. Provider ingestion (provider-specific)
2. Canonical transcript projection (provider-agnostic)
3. API transport streaming/snapshot serving

### OCP
- Add new providers by implementing ingestion/projection adapters only.
- No UI or API contract changes for additional providers.

### LSP
- All provider adapters must satisfy one canonical contract for turn/transcript events.
- UI can treat providers interchangeably at boundary level.

### ISP
- Use narrow interfaces:
1. `TranscriptSnapshotReader`
2. `TranscriptUpdateSubscriber`
3. `TurnLifecyclePublisher`
4. `ApprovalStatePublisher`

### DIP
- UI and API handlers depend on boundary interfaces, not provider runtimes.
- Provider-specific code depends on abstractions for canonical event emission.

---

## Target Boundary Contract

Introduce a canonical session transcript contract exposed by API/service:

1. Snapshot
- `GET /v1/sessions/:id/transcript` (or compatibility wrapper)
- Returns:
  - `revision`
  - `provider`
  - `blocks` (canonical block schema)
  - `capabilities` (approvals/events/interrupt/support flags)
  - `turn_state` (idle|running|completed|failed, optional turn id)

2. Follow stream
- `GET /v1/sessions/:id/transcript/stream?follow=1`
- Emits canonical events:
  - `transcript.delta`
  - `transcript.replace`
  - `turn.started`
  - `turn.completed`
  - `approval.pending`
  - `approval.resolved`
  - `stream.status` (ready|closed|reconnecting|error)
  - `heartbeat`

3. Revisions
- Monotonic revision token per session transcript state.
- UI applies only latest revision and ignores stale events.

4. Completion semantics
- Unified turn completion signal regardless of provider transport.
- No provider-specific completion inference in UI.

---

## Architecture Changes

### New Core Components

1. `CanonicalTranscriptService`
- Builds and serves canonical transcript snapshots.
- Owns revision generation and consistency rules.

2. `CanonicalTranscriptHub`
- Broadcasts canonical update events to subscribers.
- Handles subscription lifecycle and replay-on-subscribe policy.

3. `ProviderTranscriptAdapter` (per provider runtime family)
- Converts provider-native artifacts/events into canonical events.
- Persists canonical transcript state through service interfaces.

4. `SessionReloadDecisionPolicy`
- Defines when selection reload is required.
- Excludes volatile metadata (`LastActiveAt`) from reload triggers.

### Existing Component Adjustments

1. API stream handlers
- Add unified transcript stream endpoint.
- Keep old endpoints during migration via compatibility adapters.

2. UI model
- Replace provider-specific bootstrap/load branches with one transcript bootstrap path.
- Single loading-complete trigger from canonical stream/snapshot readiness.

3. Request activity/reconnect
- Reconnect policy moved to provider-agnostic boundary logic keyed by `stream.status` and revision gaps.

---

## Phased Delivery

## Phase 0: Instrumentation and Safety Rails

Objective: Make behavior observable before structural change.

Tasks:
1. Add metrics for:
- session reload reason
- stream close reason
- reconnect attempts/outcomes
- revision supersession drops
2. Add debug logs for canonical boundary trial paths.
3. Add feature flag:
- `unified_transcript_boundary_enabled`

Acceptance:
- Can attribute UI transcript resets to explicit reasons in logs/metrics.

---

## Phase 1: Canonical Domain Model

Objective: Define provider-agnostic transcript/event types.

Tasks:
1. Create canonical types package:
- `CanonicalBlock`
- `CanonicalTranscriptSnapshot`
- `CanonicalTranscriptEvent`
- `CanonicalTurnState`
2. Add schema validation and conversion helpers.
3. Write contract tests (goldens) for canonical events.

Acceptance:
- Canonical model compiles and contract tests pass.
- No runtime behavior change yet.

---

## Phase 2: Adapter Layer (Provider -> Canonical)

Objective: Move provider variability behind adapters.

Tasks:
1. Implement `ProviderTranscriptAdapter` for:
- codex
- claude
- opencode/kilocode (shared base + variant hooks)
2. Feed canonical hub from existing provider event/item sources.
3. Normalize turn lifecycle and approval signals.

Acceptance:
- For identical conversation fixtures, adapters emit canonical-equivalent transcript output.

---

## Phase 3: Unified Snapshot/Stream API

Objective: Expose one functional boundary for UI consumption.

Tasks:
1. Implement transcript snapshot endpoint/service call.
2. Implement transcript follow stream endpoint/service call.
3. Keep legacy endpoints as compatibility wrappers translating from canonical service.

Acceptance:
- Unified endpoints support all providers.
- Legacy endpoints continue functioning without regressions.

---

## Phase 4: UI Migration to Unified Boundary

Objective: Remove provider branch paths from UI session load/stream.

Tasks:
1. Replace `history/tail/items/events` bootstrap branching with one transcript bootstrap.
2. Single loading lifecycle:
- loading starts on selection start
- loading ends on first valid snapshot or stream ready/update
3. Replace provider-specific reconnect checks with boundary `stream.status`.
4. Gate with feature flag and rollout by cohort.

Acceptance:
- No provider-specific selection bootstrap branches remain in UI model.
- Existing UX parity verified on all built-in providers.

---

## Phase 5: Reload Policy Hardening

Objective: Eliminate metadata-churn reload loops.

Tasks:
1. Introduce `SessionReloadDecisionPolicy` that ignores volatile metadata fields for transcript reload.
2. Reload only on semantic transcript-impacting changes:
- selection change
- explicit revision conflict
- provider capability change requiring UI mode change
3. Add debounce/coalescing for session metadata updates.

Acceptance:
- Active-session refresh no longer causes repeated transcript reset/loading under normal turn activity.

---

## Phase 6: Legacy Path Removal

Objective: Reduce maintenance burden.

Tasks:
1. Remove deprecated provider-specific UI load logic.
2. Remove legacy API internals no longer used.
3. Retain compatibility shim only if external clients require it.

Acceptance:
- Single boundary pathway is default and fully covered by tests.

---

## Testing Strategy

### Unit
1. Canonical event/state reducers.
2. Adapter conversion for each provider family.
3. Reload decision policy edge cases.

### Integration
1. Session selection while active turn running (all providers).
2. Stream close/reconnect behavior with no message loss.
3. Snapshot + stream consistency under rapid updates.

### Regression suites (must pass)
1. Spinner clears exactly once per session load unless explicit error.
2. No pop-in/out on active codex thread.
3. Active claude/opencode/kilo stream updates visible without manual reselection.
4. Messages persist across refresh and do not disappear due to merge churn.

---

## Risk Register

1. Event ordering mismatch across providers
- Mitigation: revision sequencing and stale drop policy at canonical hub.

2. Temporary dual-path complexity during migration
- Mitigation: strict phase gates + feature flag + parity tests.

3. Performance regressions from canonicalization
- Mitigation: benchmark adapter throughput and snapshot serialization.

4. Hidden external reliance on legacy endpoints
- Mitigation: compatibility wrappers + deprecation telemetry.

---

## Rollout Plan

1. Internal canary with unified boundary enabled.
2. Provider-by-provider enablement:
- codex first
- claude
- opencode/kilocode
3. Full default enablement after parity and stability thresholds.
4. Remove legacy paths after one release cycle of clean telemetry.

---

## Definition of Done

1. UI consumes one transcript boundary path for all providers.
2. Provider-specific logic is isolated to adapter implementations.
3. Reload loops and stream-lifecycle inconsistencies are resolved by policy, not provider conditionals.
4. Legacy compatibility either removed or explicitly scoped and monitored.
