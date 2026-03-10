# SOLID Canonical Transcript Hub Closeout Execution Plan

## Scope

Complete the canonical transcript hub architecture end-to-end with a direct cutover:

- no feature flags
- no progressive rollout
- no dual live transcript runtimes

## Target Outcome

A session has exactly one canonical live transcript authority in-process:

1. Registry owns lifecycle policy (create, retain, evict, cleanup).
2. Hub owns canonical stream lifecycle semantics (`ready`, `reconnecting`, `error`, `closed`).
3. App compose + recents observers share one same-session transcript follow runtime.

## SOLID Guardrails

### SRP

- Registry: lifecycle + retention policy only.
- Hub: ingest/map/project/revision/fanout runtime only.
- SessionService/follow service: validation + delegation only.
- App model: UI state composition over snapshot/stream contracts only.

### OCP

- Provider-specific differences stay in ingress/mapper adapters.
- Hub and app observer model stay provider-agnostic.

### LSP

- Every ingress implementation must expose normalized follow contracts:
  - availability
  - channels/events
  - close semantics
  - reconnect capability signaling

### ISP

- App depends only on `GetTranscriptSnapshot` + `TranscriptStream`.
- Hub depends only on ingress/mapper/projector ports plus a narrow lifecycle observer.
- Public registry consumers are not required to depend on lifecycle bookkeeping methods.

### DIP

- Runtime orchestration depends on interfaces (`CanonicalTranscriptHubRegistry`, `CanonicalTranscriptHubLifecycleObserver`, `TranscriptIngressFactory`, `TranscriptMapper`, `TranscriptReconnectPolicy`) and injected dependencies.

## Workstreams

## 1) Registry Lifecycle Closure

### Changes

- Keep `CanonicalTranscriptHubRegistry` minimal (`HubForSession`, `CloseSession`, `CloseAll`).
- Add internal lifecycle observer contract (`CanonicalTranscriptHubLifecycleObserver`) for:
  - `SubscriberAttached(sessionID, hubInstanceID)`
  - `SubscriberDetached(sessionID, hubInstanceID)`
  - `HubClosed(sessionID, hubInstanceID)`
- Add per-session lifecycle metadata:
  - hub interface/provider binding
  - instance ID
  - subscriber count
  - last-detached timestamp
  - idle eviction timer
- Implement idle eviction policy:
  - arm timer when count reaches 0
  - cancel timer on reattach
  - close + remove hub when idle timer fires
- Remove registry entries when hub closes internally via hub->observer close callback.
- Ignore stale lifecycle callbacks by matching `hubInstanceID` before mutating registry state.

### Tests

- `TestCanonicalTranscriptHubRegistryEvictsIdleHubAfterLastSubscriberLeaves`
- `TestCanonicalTranscriptHubRegistryCancelsIdleEvictionWhenSubscriberReattaches`
- `TestCanonicalTranscriptHubRegistryCreatesFreshHubAfterIdleEviction`
- `TestCanonicalTranscriptHubRegistryDetachIsSafeUnderConcurrentSubscribers`

## 2) Hub State Machine Closure

### Changes

- Add explicit internal runtime states:
  - `starting`, `ready`, `reconnecting`, `error`, `closed`
- Enforce explicit transition rules in one place.
- Emit canonical `stream.status` events for lifecycle transitions with hub-owned revision advancement.
- Add reconnect loop semantics for recoverable ingress interruptions.
- Inject reconnect behavior via `TranscriptReconnectPolicy` instead of hardcoded retry limits.
- Emit terminal `error` before `closed` on unrecoverable reconnect failure.
- Guarantee `closed` is emitted once.

### Tests

- `TestCanonicalTranscriptHubEmitsReconnectingDuringRecoverableIngressRestart`
- `TestCanonicalTranscriptHubEmitsReadyAgainAfterReconnect`
- `TestCanonicalTranscriptHubEmitsErrorBeforeClosedOnTerminalIngressFailure`
- `TestCanonicalTranscriptHubEmitsClosedExactlyOnce`
- `TestSessionServiceSubscribeTranscriptPropagatesReconnectLifecycleFromHub`
- `TestSessionServiceSubscribeTranscriptPropagatesTerminalHubErrorAsCanonicalStatus`

## 3) App Shared-Runtime Proof

### Changes

- Add transcript API test double that counts stream opens per session.
- Make recents completion watching prefer shared same-session transcript follow when compose stream is already active.
- Propagate completion signals from shared transcript stream into recents completion state transitions.
- Preserve recents preview/completion behavior while preventing duplicate same-session follow attaches.

### Tests

- `TestUnifiedBootstrapSharesTranscriptFollowBetweenComposeAndRecents`
- `TestRecentsAndComposeObserveSameSessionWithoutDuplicateTranscriptAttach`
- `TestSessionBootstrapPolicyPrefersSharedTranscriptFollowForSameSession`

## 4) Documentation Alignment

### Changes

- Update `SOLID_CANONICAL_TRANSCRIPT_HUB_PLAN.md` with closeout status and direct-cutover note.
- Update `docs/architecture.md` with canonical hub ownership boundary and lifecycle responsibilities.

## Verification Plan

1. Run targeted daemon tests for registry lifecycle, hub runtime state machine, and follow-service integration.
2. Run targeted app tests for shared same-session follow behavior across compose/recents.
3. Run full `go test ./internal/daemon ./internal/app` to confirm no regressions.
4. Confirm docs reflect implemented architecture (no stale per-subscriber ownership language).

## Definition of Done

1. Registry lifecycle is subscriber-aware and evicts idle hubs automatically.
2. Hub runtime owns canonical lifecycle statuses and emits deterministic revisions.
3. App-level tests prove same-session compose/recents sharing without duplicate transcript attach.
4. Existing daemon + app transcript suites pass.
5. Architecture docs and SOLID plan describe the implemented ownership model.
