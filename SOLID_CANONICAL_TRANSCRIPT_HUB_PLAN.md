# SOLID Plan: Canonical Transcript Hub (Direct Cutover)

## Decision

We are doing a direct cutover to a session-scoped canonical transcript hub.

- No feature flags.
- No progressive rollout.
- No dual follow runtimes.
- No provider-specific follow behavior in upper layers.

`CanonicalTranscriptHub` becomes the only owner of live canonical transcript state per session.

---

## What Changes From Today

Current runtime in code:

- `SessionService.SubscribeTranscript` delegates to `canonical_transcript_follow_service.go`.
- `canonical_transcript_follow_service.go` creates projector + mapping loop per subscriber.
- `transcript_transport.go` selects provider-native follow source per call.

Target runtime:

- One hub per session in a registry.
- One ingress attachment per session hub.
- One mapper + one projector pipeline per session hub.
- N subscribers receive fanout from the same canonical stream.

This removes per-subscriber runtime ownership and makes revisions deterministic.

---

## SOLID Mapping

### SRP

- `SessionService`: validate request, resolve session/provider, delegate.
- `CanonicalTranscriptHubRegistry`: lifecycle and singleton ownership per session.
- `CanonicalTranscriptHub`: live stream runtime, projection, fanout, revision ownership.
- `TranscriptIngress`: native attach semantics only.
- `TranscriptMapper`: native -> canonical mapping only.
- `TranscriptProjector`: canonical state reduction only.
- `TranscriptSnapshotReader`: persisted snapshot reconstruction only.

### OCP

Adding a provider requires only adapter/ingress work.
No changes required in hub orchestration, API handlers, or UI stream consumers.

### LSP

Every provider must satisfy one hub-facing attach contract:

- attach success with events or items,
- explicit follow-unavailable,
- close/error semantics normalized.

### ISP

- API/UI depend only on snapshot + stream contracts.
- Hub depends only on narrow ports (ingress, mapper, projector factory, fanout policy).

### DIP

- `SessionService` depends on `CanonicalTranscriptHubRegistry` and `TranscriptSnapshotReader` interfaces.
- Hub depends on `TranscriptIngressFactory` and `TranscriptMapper`, not provider managers.

---

## Target Architecture

### 1) Hub Registry

**New file:** `internal/daemon/canonical_transcript_hub_registry.go`

Responsibilities:

- `map[sessionID]*hubRuntime`
- create/get hub atomically
- refcount subscriber attach/detach
- idle eviction and explicit close

Proposed interface:

```go
type CanonicalTranscriptHubRegistry interface {
    HubForSession(ctx context.Context, session *types.Session) (CanonicalTranscriptHub, error)
    CloseSession(sessionID string) error
    CloseAll() error
}
```

### 2) Canonical Hub

**New file:** `internal/daemon/canonical_transcript_hub.go`

Responsibilities:

- own one ingress attachment
- own mapping + projection loop
- own revision sequencing
- keep latest snapshot in memory
- fanout canonical events to subscribers
- emit stream lifecycle events (`ready`, `closed`, `reconnecting`, `error`)

Proposed interface:

```go
type CanonicalTranscriptHub interface {
    Subscribe(ctx context.Context, after transcriptdomain.RevisionToken) (<-chan transcriptdomain.TranscriptEvent, func(), error)
    Snapshot() transcriptdomain.TranscriptSnapshot
    Close() error
}
```

### 3) Ingress Port

**Refactor:** `internal/daemon/transcript_transport.go` + `internal/daemon/transcript_ports.go`

- keep existing selector behavior but expose as `TranscriptIngressFactory` + `TranscriptIngressHandle`
- hide event-vs-item branching behind ingress boundary

```go
type TranscriptIngressFactory interface {
    Open(ctx context.Context, sessionID, provider string) (TranscriptIngressHandle, error)
}

type TranscriptIngressHandle struct {
    Events          <-chan types.CodexEvent
    Items           <-chan map[string]any
    FollowAvailable bool
    Close           func()
}
```

### 4) Follow Opener Adapter

**Refactor:** `internal/daemon/canonical_transcript_follow_service.go`

- stop owning mapper/projector/event loop
- become thin adapter:
  - resolve hub via registry
  - call `hub.Subscribe(after)`

### 5) SessionService Delegation Boundary

**Refactor:** `internal/daemon/transcript_service.go` and `internal/daemon/session_service.go`

- `GetTranscriptSnapshot`: session validation + delegate to snapshot reader
- `SubscribeTranscript`: session validation + delegate to follow opener backed by hub registry
- no transcript lifecycle logic remains here

---

## Runtime Semantics

### Hub State Machine

States:

1. `starting`
2. `ready`
3. `reconnecting`
4. `closed`
5. `error`

Rules:

- only hub transitions states
- state transitions emit canonical `stream.status`
- terminal close always emits `stream.status=closed` once

### Revision Ownership

- only hub calls `NextRevision()` for live streams
- subscriber attach never allocates revisions directly
- snapshot reconstruction can have independent revisions, but live follow revision authority is always the hub instance

### Late Subscriber Replay

Initial implementation (no event journal):

- hub stores `currentSnapshot` + `currentRevision`
- subscriber attach with empty/stale `after` gets `transcript.replace` immediately
- then subscriber is attached to live delta fanout

### Persisted / Non-Live Sessions

If follow is unavailable but session exists:

- emit `stream.status=ready`
- emit `stream.status=closed`
- close stream cleanly

This preserves separation between existence and liveness.

### Fanout and Slow Subscriber Policy

- each subscriber has bounded buffered channel
- hub loop is single-threaded for ingest/map/project/fanout ordering
- slow subscriber policy: close subscriber when buffer limit is exceeded
- one slow subscriber must never stall hub progression

---

## End-to-End Implementation Sequence

## Phase 1: Add Contracts and Registry Skeleton

Objective:
Introduce hub contracts without changing API behavior.

Code changes:

- Add `internal/daemon/canonical_transcript_hub.go` (interface + runtime skeleton)
- Add `internal/daemon/canonical_transcript_hub_registry.go` (registry skeleton)
- Extend `internal/daemon/transcript_ports.go` with hub + ingress ports
- Add service option in `internal/daemon/session_service.go`:
  - `WithCanonicalTranscriptHubRegistry(...)`

Tests:

- new registry unit tests for singleton-per-session
- session service option guard/set tests

Exit criteria:

- code compiles with registry injected
- existing transcript tests still pass

## Phase 2: Move Live Runtime Ownership Into Hub

Objective:
Move attach/map/project/fanout loop from follow service into hub.

Code changes:

- implement hub run loop in `canonical_transcript_hub.go`
- wire ingress factory adapter from current `TranscriptTransportSelector`
- convert `canonical_transcript_follow_service.go` to pure adapter over hub subscribe

Tests:

- hub emits `ready` on startup
- hub propagates mapped deltas from events/items
- no duplicate ingress attach for two subscribers

Exit criteria:

- follow service contains no projector loop
- two subscribers share one ingress attachment in tests

## Phase 3: Late Subscriber Replace Replay

Objective:
Guarantee convergence for delayed subscribers.

Code changes:

- keep hub `snapshot` + `revision` fields authoritative
- on subscribe:
  - compare `after` vs current revision
  - emit `transcript.replace` when stale/empty
- keep `canonical_transcript_snapshot_service.go` for REST snapshot

Tests:

- stale subscriber receives replace then live events
- up-to-date subscriber receives only live events
- revisions remain monotonic

Exit criteria:

- deterministic convergence behavior for recents + compose concurrent consumers

## Phase 4: Normalize Provider Ingress Behavior

Objective:
Ensure codex/claude/opencode/kilo obey identical hub-facing follow semantics.

Code changes:

- refine `transcript_transport.go` into ingress factory semantics
- normalize unavailable/terminal/recoverable errors from provider adapters
- update `session_conversation_adapters.go` mapping where needed

Tests:

- provider parity integration tests (all supported providers)
- persisted but non-live session always emits ready->closed

Exit criteria:

- hub behavior is provider-agnostic above ingress layer

## Phase 5: Make SessionService and API Layers Pure Orchestration

Objective:
Lock architectural boundaries.

Code changes:

- slim `transcript_service.go` delegation paths
- keep `api_transcript_handlers.go` transport-only SSE marshalling
- remove any remaining transcript lifecycle logic from service/handlers

Tests:

- transcript service integration tests updated to assert delegation boundaries
- API stream tests pass unchanged contract

Exit criteria:

- no transcript runtime ownership above hub

## Phase 6: UI Semantics Hardening

Objective:
Align UI to hub semantics as independent subscribers.

Code changes:

- verify/reinforce handling in:
  - `internal/app/transcript_stream.go`
  - `internal/app/model_request_activity.go`
  - `internal/app/transcript_stream_health.go`
  - `internal/app/model_update_messages.go`
- ensure reconnect path expects canonical `replace` + `stream.status`

Tests:

- compose + recents simultaneous subscribe parity
- reconnect does not duplicate content
- stale snapshot supersession still respected

Exit criteria:

- multiple UI surfaces follow same session without divergence

## Phase 7: Dead Code Removal and Finalization

Objective:
Remove obsolete per-subscriber runtime assumptions.

Code changes:

- delete superseded logic in `canonical_transcript_follow_service.go`
- remove unused transport abstractions replaced by ingress ports
- clean constructor wiring in `NewSessionService` defaults

Tests:

- full daemon + app transcript test suites
- provider integration tests

Exit criteria:

- single runtime path remains for live transcript follow

---

## Concrete Test Matrix

### Unit

- registry singleton and close/evict behavior
- hub state transitions and status events
- fanout consistency across N subscribers
- slow subscriber isolation
- replace replay on stale attach
- ready->closed degradation for non-live sessions

### Integration (daemon)

- two simultaneous subscribers get same canonical ordering
- recents + compose shared session follow with one native attach
- provider parity: codex, claude, opencode, kilocode
- reconnect/reattach preserves coherent revision stream

### Integration (app)

- snapshot then stream merge remains monotonic
- stream close sets controller closed state correctly
- reconnect resumes from active revision without duplication

### Regression

- `GET /v1/sessions/:id/transcript` unchanged
- `GET /v1/sessions/:id/transcript/stream?follow=1` unchanged
- revision validation and JSON contract tests stay green

---

## Delivery Rules

- Direct cutover only.
- No flag scaffolding.
- No compatibility dual-path runtime.
- No rollback branch inside production code; rollback is git revert if needed.

---

## Definition of Done

Done when all are true:

1. One live canonical hub instance max per session.
2. One ingress attach max per live session hub.
3. All subscribers receive identical canonical ordering from shared runtime.
4. `SessionService` and API handlers are orchestration-only for transcript paths.
5. Provider-specific behavior is hidden behind ingress + mapper adapters.
6. Late subscribers converge through `transcript.replace` + live deltas.
7. Persisted/non-live sessions degrade uniformly (`ready` then `closed`).
8. Full transcript unit/integration suites pass across daemon + app.
