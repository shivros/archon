# Canonical Transcript Hub Closeout Checklist

This checklist closes the remaining gap between the current implementation and
[`SOLID_CANONICAL_TRANSCRIPT_HUB_PLAN.md`](./SOLID_CANONICAL_TRANSCRIPT_HUB_PLAN.md).

Current assessment: the hub architecture is in place, but three areas still
need explicit closure:

1. registry-owned lifecycle management
2. full hub state-machine ownership
3. app-level proof that multiple UI consumers share one canonical follow runtime

The goal is not more abstraction. The goal is to finish the ownership model so
the hub is the unambiguous live authority for a session's canonical transcript.

---

## 1. Finish Registry Lifecycle Ownership

### Gap

`internal/daemon/canonical_transcript_hub_registry.go` currently provides
singleton lookup plus explicit `CloseSession` and `CloseAll`, but it does not
yet own the full lifecycle described in the plan:

- subscriber-aware hub retention
- automatic cleanup after the last subscriber detaches
- idle eviction for inactive hubs

Today, a hub can outlive all subscribers until something calls `CloseSession`
or `CloseAll`.

### Closeout Objective

Make the registry the single owner of hub lifecycle policy.

The registry should decide:

- when a hub is created
- when a hub is retained
- when a hub is eligible for eviction
- when a closed hub is removed from the registry

The hub should not need to know whether it is the last retained hub in the
process. That is registry policy, not stream-runtime policy.

### File Targets

- `internal/daemon/transcript_ports.go`
- `internal/daemon/canonical_transcript_hub_registry.go`
- `internal/daemon/canonical_transcript_hub.go`

### Concrete Changes

1. Extend the registry contract so it can observe subscriber detach.

Recommended shape:

```go
type CanonicalTranscriptHubRegistry interface {
    HubForSession(ctx context.Context, sessionID, provider string) (CanonicalTranscriptHub, error)
    SubscriberAttached(sessionID string)
    SubscriberDetached(sessionID string)
    CloseSession(sessionID string) error
    CloseAll() error
}
```

If you want to keep the public interface smaller, make these internal methods on
the default registry and wire them through the hub's unsubscribe path. The key
thing is the behavior, not the exact method names.

2. Add per-session lifecycle metadata in the registry.

Suggested fields:

- `subscriberCount`
- `lastDetachedAt`
- `idleTimer` or `evictAt`

3. Evict hubs after the last subscriber leaves and the idle window expires.

Recommended policy:

- when `subscriberCount` drops to `0`, arm idle eviction
- if a new subscriber attaches before eviction, cancel the timer
- if the timer fires and the hub is still idle, close it and delete it from the registry

4. Remove closed hubs from the registry even when closure originates inside the
hub.

This avoids stale registry entries for hubs that terminate after transport
failure or explicit internal shutdown.

### Acceptance Criteria

- a session with no subscribers does not retain a live hub indefinitely
- a second subscriber joining the same active session reuses the existing hub
- a subscriber rejoining before idle eviction reuses the same hub
- a subscriber rejoining after idle eviction gets a fresh hub
- registry state remains correct under concurrent attach/detach

### Test Additions

Add to `internal/daemon/canonical_transcript_hub_test.go`:

- `TestCanonicalTranscriptHubRegistryEvictsIdleHubAfterLastSubscriberLeaves`
- `TestCanonicalTranscriptHubRegistryCancelsIdleEvictionWhenSubscriberReattaches`
- `TestCanonicalTranscriptHubRegistryCreatesFreshHubAfterIdleEviction`
- `TestCanonicalTranscriptHubRegistryDetachIsSafeUnderConcurrentSubscribers`

---

## 2. Finish the Hub State Machine

### Gap

`internal/daemon/canonical_transcript_hub.go` currently covers:

- `ready`
- `closed`

The plan still calls for the hub to own the full stream lifecycle:

- `starting`
- `ready`
- `reconnecting`
- `closed`
- `error`

Those states already exist in `internal/daemon/transcriptdomain/types.go`, but
the runtime does not yet appear to model the full transition set explicitly.

### Closeout Objective

Make the hub the only authority for live stream state transitions.

Upper layers should not infer reconnecting or terminal failure from transport
details. They should only observe canonical `stream.status` events emitted by
the hub.

### File Targets

- `internal/daemon/canonical_transcript_hub.go`
- `internal/daemon/transcript_transport.go`
- `internal/daemon/transcriptdomain/types.go`
- `internal/daemon/transcriptdomain/validation.go`

### Concrete Changes

1. Add an internal hub runtime state enum.

Suggested private states:

- `hubStateStarting`
- `hubStateReady`
- `hubStateReconnecting`
- `hubStateClosed`
- `hubStateError`

2. Define explicit transition rules.

Recommended baseline:

- startup emits `ready` once the ingress loop is active
- transient ingress interruption emits `reconnecting`
- successful reattach emits `ready`
- terminal transport failure emits `error`
- all terminal exits emit `closed` exactly once

3. Move retry/reattach policy behind the ingress boundary or a hub-local retry
loop.

The hub should decide when a transport interruption is recoverable versus
terminal. `transcript_transport.go` can still select the provider-native source,
but the hub should own the canonical state progression.

4. Preserve deterministic revision ownership for lifecycle events.

Every emitted `stream.status` event should continue to consume a single hub
revision in order.

### Acceptance Criteria

- reconnecting is observable as canonical transcript state, not inferred
- terminal failure produces a canonical `error` event before closure
- `closed` is emitted once and only once
- subscribers that join after reconnect still see a valid replace-plus-live flow
- provider-native transport quirks do not leak to app or API callers

### Test Additions

Add to `internal/daemon/canonical_transcript_hub_test.go`:

- `TestCanonicalTranscriptHubEmitsReconnectingDuringRecoverableIngressRestart`
- `TestCanonicalTranscriptHubEmitsReadyAgainAfterReconnect`
- `TestCanonicalTranscriptHubEmitsErrorBeforeClosedOnTerminalIngressFailure`
- `TestCanonicalTranscriptHubEmitsClosedExactlyOnce`

Add or extend integration proof in
`internal/daemon/transcript_service_integration_test.go`:

- `TestSessionServiceSubscribeTranscriptPropagatesReconnectLifecycleFromHub`
- `TestSessionServiceSubscribeTranscriptPropagatesTerminalHubErrorAsCanonicalStatus`

---

## 3. Prove Shared Runtime at the App Layer

### Gap

The daemon-level hub tests already prove shared fanout, but the plan's product
goal is stronger: multiple UI consumers should be able to follow the same
session through one canonical runtime without drift or duplicate native attach.

That still needs an app-level proof.

### Closeout Objective

Prove that the app can have multiple session observers for the same session and
still behave as one transcript client from the daemon's perspective.

The practical scenario to prove is:

- compose view is open for session `s1`
- recents is also watching session `s1`
- both consume the same canonical transcript stream semantics
- daemon follow attach happens once

### File Targets

- `internal/app/model_history_progressive_test.go`
- `internal/app/model_recents_test.go`
- `internal/app/session_bootstrap_policy_test.go`
- `internal/app/commands.go`

### Concrete Changes

1. Add an app-facing test double that counts transcript stream opens per session.

2. Drive one scenario where compose and recents both subscribe to the same
session.

3. Assert:

- one transcript stream attach for that session
- both surfaces observe consistent transcript state
- no duplicate ready/replace handling causes divergent UI state

4. If the app intentionally maintains separate local consumers over one shared
daemon stream abstraction, document that explicitly in the test.

### Acceptance Criteria

- compose and recents can observe the same session without double-follow attach
- preview/completion behavior remains correct for recents
- compose transcript rendering does not regress
- reconnect behavior does not create duplicate local watchers

### Test Additions

Add to `internal/app/model_history_progressive_test.go`:

- `TestUnifiedBootstrapSharesTranscriptFollowBetweenComposeAndRecents`

Add to `internal/app/model_recents_test.go`:

- `TestRecentsAndComposeObserveSameSessionWithoutDuplicateTranscriptAttach`

Add to `internal/app/session_bootstrap_policy_test.go` if needed:

- `TestSessionBootstrapPolicyPrefersSharedTranscriptFollowForSameSession`

---

## 4. Optional But Worth Doing Before Declaring Done

These are not the core missing behaviors, but they will make the closeout more
complete and easier to keep closed.

### Documentation Alignment

Update:

- `SOLID_CANONICAL_TRANSCRIPT_HUB_PLAN.md`
- `docs/architecture.md`

Add a short closeout note that states:

- one hub per session
- one ingress attachment per active hub
- registry owns retention and eviction
- hub owns canonical lifecycle and revision authority
- app depends only on transcript snapshot + transcript stream contracts

### Naming Cleanup

Sweep for comments or tests that still describe the follow runtime as
per-subscriber, especially in:

- `internal/daemon/canonical_transcript_follow_service.go`
- `internal/app/model_history_progressive_test.go`
- `internal/app/commands.go`

---

## 5. Definition Of Done

The hub plan can be considered complete when all of the following are true:

1. Registry lifecycle is subscriber-aware and evicts idle hubs automatically.
2. Hub runtime owns `ready`, `reconnecting`, `error`, and `closed` semantics.
3. App-level tests prove multi-subscriber same-session behavior without
duplicate follow attach.
4. Existing snapshot/follow integration tests still pass.
5. Docs describe the same architecture that the code actually implements.

At that point, the hub is not just present. It is the fully closed canonical
ownership boundary the plan was aiming for.
