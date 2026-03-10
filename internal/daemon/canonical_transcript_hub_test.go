package daemon

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"control/internal/daemon/transcriptadapters"
	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

type hubTestIngressFactory struct {
	mu     sync.Mutex
	opens  int
	handle TranscriptIngressHandle
	err    error
}

func (f *hubTestIngressFactory) Open(context.Context, string, string) (TranscriptIngressHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	return f.handle, f.err
}

func (f *hubTestIngressFactory) OpenCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opens
}

type hubTestMapper struct{}

func (hubTestMapper) MapItem(string, transcriptadapters.MappingContext, map[string]any) []transcriptdomain.TranscriptEvent {
	return nil
}

func (hubTestMapper) MapEvent(_ string, _ transcriptadapters.MappingContext, event types.CodexEvent) []transcriptdomain.TranscriptEvent {
	text := event.Method
	if text == "" {
		return nil
	}
	return []transcriptdomain.TranscriptEvent{{
		Kind: transcriptdomain.TranscriptEventDelta,
		Delta: []transcriptdomain.Block{{
			Kind: "assistant",
			Role: "assistant",
			Text: text,
		}},
	}}
}

type scriptedIngressStep struct {
	handle TranscriptIngressHandle
	err    error
}

type scriptedHubIngressFactory struct {
	mu    sync.Mutex
	steps []scriptedIngressStep
	opens int
}

func (f *scriptedHubIngressFactory) Open(context.Context, string, string) (TranscriptIngressHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	if len(f.steps) == 0 {
		return TranscriptIngressHandle{}, errors.New("unexpected ingress open")
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	return step.handle, step.err
}

func (f *scriptedHubIngressFactory) OpenCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opens
}

type neverReconnectPolicy struct{}

func (neverReconnectPolicy) NextAttempt(current int, hadTraffic bool) (int, bool) {
	_ = hadTraffic
	return current + 1, false
}

type nonLifecycleHub struct{}

func (nonLifecycleHub) Subscribe(context.Context, transcriptdomain.RevisionToken) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	ch := make(chan transcriptdomain.TranscriptEvent)
	close(ch)
	return ch, func() {}, nil
}

func (nonLifecycleHub) Snapshot() transcriptdomain.TranscriptSnapshot {
	return transcriptdomain.TranscriptSnapshot{}
}

func (nonLifecycleHub) Close() error { return nil }

func TestDefaultTranscriptReconnectPolicyUsesFallbackLimit(t *testing.T) {
	policy := NewDefaultTranscriptReconnectPolicy(0)
	attempt := 0
	for i := 1; i <= defaultTranscriptHubMaxReconnects; i++ {
		next, shouldReconnect := policy.NextAttempt(attempt, false)
		if !shouldReconnect {
			t.Fatalf("expected reconnect allowed for attempt %d", i)
		}
		if next != i {
			t.Fatalf("expected next attempt %d, got %d", i, next)
		}
		attempt = next
	}

	next, shouldReconnect := policy.NextAttempt(attempt, false)
	if shouldReconnect {
		t.Fatalf("expected reconnect denied after fallback max reconnects")
	}
	if next != attempt+1 {
		t.Fatalf("expected monotonic attempt counter, got %d", next)
	}

	resetAttempt, resetReconnect := policy.NextAttempt(next, true)
	if !resetReconnect || resetAttempt != 0 {
		t.Fatalf("expected hadTraffic=true to reset attempt counter, got attempt=%d reconnect=%v", resetAttempt, resetReconnect)
	}
}

func TestCanonicalTranscriptHubHelpersHandleNilReceivers(t *testing.T) {
	var hub *canonicalTranscriptHub
	hub.bindLifecycleObserver(nil, "ignored")
	hub.setReconnectPolicy(nil)
	next, ok := hub.reconnectPolicyOrDefault().NextAttempt(defaultTranscriptHubMaxReconnects, false)
	if ok || next != defaultTranscriptHubMaxReconnects+1 {
		t.Fatalf("expected default policy behavior from nil receiver fallback, got next=%d ok=%v", next, ok)
	}
	if hub.transitionRuntimeState(hubStateReady) {
		t.Fatalf("expected nil receiver transition to fail")
	}
}

func TestCanonicalTranscriptHubTransitionRuntimeStateRejectsInvalidTransitions(t *testing.T) {
	hub := &canonicalTranscriptHub{runtimeState: hubStateClosed}
	if hub.transitionRuntimeState(hubStateReady) {
		t.Fatalf("expected closed -> ready transition to be rejected")
	}
	if hub.runtimeState != hubStateClosed {
		t.Fatalf("expected runtime state to remain closed, got %q", hub.runtimeState)
	}

	hub.runtimeState = ""
	if hub.transitionRuntimeState(hubStateReconnecting) {
		t.Fatalf("expected implicit starting -> reconnecting transition to be rejected")
	}
	if hub.runtimeState != "" {
		t.Fatalf("expected runtime state unchanged on rejected transition, got %q", hub.runtimeState)
	}
}

func TestCanonicalTranscriptHubEmitHubStatusEdgeCases(t *testing.T) {
	hub := &canonicalTranscriptHub{
		sessionID:        "s-status-edge",
		provider:         "codex",
		runtimeState:     hubStateStarting,
		projectorFactory: NewDefaultTranscriptProjectorFactory(),
	}
	projector := NewTranscriptProjector("s-status-edge", "codex", "")

	calls := 0
	if hub.emitHubStatus(projector, func(transcriptdomain.TranscriptEvent) bool {
		calls++
		return true
	}, transcriptdomain.StreamStatus("invalid")) {
		t.Fatalf("expected invalid stream status to be rejected")
	}
	if calls != 0 {
		t.Fatalf("expected invalid status to short-circuit before emit, got %d emits", calls)
	}

	hub.runtimeState = hubStateClosed
	if hub.emitHubStatus(projector, func(transcriptdomain.TranscriptEvent) bool {
		calls++
		return true
	}, transcriptdomain.StreamStatusReady) {
		t.Fatalf("expected emit to fail when runtime transition is invalid")
	}
	if calls != 0 {
		t.Fatalf("expected no emit attempt for invalid transition, got %d emits", calls)
	}

	hub.runtimeState = hubStateStarting
	if hub.emitHubStatus(projector, func(transcriptdomain.TranscriptEvent) bool { return false }, transcriptdomain.StreamStatusClosed) {
		t.Fatalf("expected emitHubStatus to report false when downstream emit fails")
	}
	if hub.runtimeState != hubStateClosed {
		t.Fatalf("expected closed runtime state after failed closed emit, got %q", hub.runtimeState)
	}
}

func TestNormalizeTranscriptIngressHandleDefaultsClose(t *testing.T) {
	normalized := normalizeTranscriptIngressHandle(TranscriptIngressHandle{})
	if normalized.Close == nil {
		t.Fatalf("expected normalizeTranscriptIngressHandle to provide a no-op close")
	}
	normalized.Close()

	closed := false
	normalized = normalizeTranscriptIngressHandle(TranscriptIngressHandle{
		Close: func() { closed = true },
	})
	normalized.Close()
	if !closed {
		t.Fatalf("expected normalizeTranscriptIngressHandle to preserve non-nil close callback")
	}
}

func TestIsCanonicalTranscriptHubExplicitlyClosedSupportsInterfaceProbing(t *testing.T) {
	if isCanonicalTranscriptHubExplicitlyClosed(nonLifecycleHub{}) {
		t.Fatalf("expected non-canonical implementation probe to return false")
	}
	hub := &canonicalTranscriptHub{}
	if isCanonicalTranscriptHubExplicitlyClosed(hub) {
		t.Fatalf("expected fresh hub to report not explicitly closed")
	}
}

func TestCanonicalTranscriptHubRegistryHubForSessionNilReceiver(t *testing.T) {
	var registry *defaultCanonicalTranscriptHubRegistry
	if _, err := registry.HubForSession(context.Background(), "s-nil-registry", "codex"); err == nil {
		t.Fatalf("expected nil registry receiver to return error")
	}
}

func TestCanonicalTranscriptHubRegistryReturnsSingletonPerSession(t *testing.T) {
	events := make(chan types.CodexEvent)
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		Events:          events,
		FollowAvailable: true,
		Close:           func() {},
	}}
	registry := NewDefaultCanonicalTranscriptHubRegistry(factory, hubTestMapper{}, nil)

	h1, err := registry.HubForSession(context.Background(), "s1", "codex")
	if err != nil {
		t.Fatalf("HubForSession first: %v", err)
	}
	h2, err := registry.HubForSession(context.Background(), "s1", "codex")
	if err != nil {
		t.Fatalf("HubForSession second: %v", err)
	}
	c1, ok := h1.(*canonicalTranscriptHub)
	if !ok {
		t.Fatalf("expected concrete hub type")
	}
	c2, ok := h2.(*canonicalTranscriptHub)
	if !ok {
		t.Fatalf("expected concrete hub type")
	}
	if c1 != c2 {
		t.Fatalf("expected registry singleton hub instance per session")
	}
	_ = registry.CloseAll()
}

func TestCanonicalTranscriptHubRegistryConstructorAcceptsExplicitDeps(t *testing.T) {
	registry := NewDefaultCanonicalTranscriptHubRegistry(
		&hubTestIngressFactory{},
		hubTestMapper{},
		NewDefaultTranscriptProjectorFactory(),
	)
	if registry == nil {
		t.Fatalf("expected registry")
	}
	if err := registry.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
}

func TestCanonicalTranscriptHubRegistryRejectsProviderMismatchForSession(t *testing.T) {
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		FollowAvailable: true,
		Close:           func() {},
	}}
	registry := NewDefaultCanonicalTranscriptHubRegistry(factory, hubTestMapper{}, nil)

	if _, err := registry.HubForSession(context.Background(), "s-provider", "codex"); err != nil {
		t.Fatalf("initial HubForSession: %v", err)
	}
	_, err := registry.HubForSession(context.Background(), "s-provider", "claude")
	if err == nil {
		t.Fatalf("expected provider mismatch conflict")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected service error, got %T", err)
	}
	if svcErr.Kind != ServiceErrorConflict {
		t.Fatalf("expected conflict error kind, got %q", svcErr.Kind)
	}
}

func TestCanonicalTranscriptHubRegistryCloseSessionRemovesHub(t *testing.T) {
	events := make(chan types.CodexEvent)
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		Events:          events,
		FollowAvailable: true,
		Close:           func() {},
	}}
	registry := NewDefaultCanonicalTranscriptHubRegistry(factory, hubTestMapper{}, nil)

	hub1Raw, err := registry.HubForSession(context.Background(), "s-close", "codex")
	if err != nil {
		t.Fatalf("HubForSession first: %v", err)
	}
	hub1 := hub1Raw.(*canonicalTranscriptHub)

	ch, cancel, err := hub1.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()
	_ = awaitEventKind(t, ch, transcriptdomain.TranscriptEventStreamStatus)

	if err := registry.CloseSession("s-close"); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				goto closed
			}
		case <-timeout:
			t.Fatalf("timed out waiting for stream close after CloseSession")
		}
	}
closed:

	hub2Raw, err := registry.HubForSession(context.Background(), "s-close", "codex")
	if err != nil {
		t.Fatalf("HubForSession second: %v", err)
	}
	hub2 := hub2Raw.(*canonicalTranscriptHub)
	if hub1 == hub2 {
		t.Fatalf("expected new hub instance after CloseSession")
	}
}

func TestCanonicalTranscriptHubRegistryCloseSessionNoopCases(t *testing.T) {
	registry := NewDefaultCanonicalTranscriptHubRegistry(&hubTestIngressFactory{}, hubTestMapper{}, nil)
	if err := registry.CloseSession(" "); err != nil {
		t.Fatalf("expected blank CloseSession noop, got %v", err)
	}
	if err := registry.CloseSession("missing"); err != nil {
		t.Fatalf("expected missing CloseSession noop, got %v", err)
	}
}

func TestCanonicalTranscriptHubRegistryEvictsIdleHubAfterLastSubscriberLeaves(t *testing.T) {
	events := make(chan types.CodexEvent)
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		Events:          events,
		FollowAvailable: true,
		Close:           func() {},
	}}
	registry := newDefaultCanonicalTranscriptHubRegistryWithIdleTTL(factory, hubTestMapper{}, nil, 40*time.Millisecond)

	hubRaw, err := registry.HubForSession(context.Background(), "s-idle-evict", "codex")
	if err != nil {
		t.Fatalf("HubForSession: %v", err)
	}
	hub := hubRaw.(*canonicalTranscriptHub)
	ch, cancel, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	_ = awaitEventKind(t, ch, transcriptdomain.TranscriptEventStreamStatus)
	cancel()

	awaitCondition(t, 2*time.Second, func() bool {
		registry.mu.Lock()
		defer registry.mu.Unlock()
		_, ok := registry.hubs["s-idle-evict"]
		return !ok
	})
}

func TestCanonicalTranscriptHubRegistryCancelsIdleEvictionWhenSubscriberReattaches(t *testing.T) {
	events := make(chan types.CodexEvent)
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		Events:          events,
		FollowAvailable: true,
		Close:           func() {},
	}}
	registry := newDefaultCanonicalTranscriptHubRegistryWithIdleTTL(factory, hubTestMapper{}, nil, 120*time.Millisecond)

	hubRaw, err := registry.HubForSession(context.Background(), "s-idle-cancel", "codex")
	if err != nil {
		t.Fatalf("HubForSession first: %v", err)
	}
	hub1 := hubRaw.(*canonicalTranscriptHub)
	ch1, cancel1, err := hub1.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe first: %v", err)
	}
	_ = awaitEventKind(t, ch1, transcriptdomain.TranscriptEventStreamStatus)
	cancel1()

	time.Sleep(45 * time.Millisecond)
	hubRaw, err = registry.HubForSession(context.Background(), "s-idle-cancel", "codex")
	if err != nil {
		t.Fatalf("HubForSession reattach: %v", err)
	}
	hub2 := hubRaw.(*canonicalTranscriptHub)
	if hub1 != hub2 {
		t.Fatalf("expected hub reuse before idle eviction")
	}
	_, cancel2, err := hub2.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe reattach: %v", err)
	}
	defer cancel2()

	time.Sleep(160 * time.Millisecond)
	hubRaw, err = registry.HubForSession(context.Background(), "s-idle-cancel", "codex")
	if err != nil {
		t.Fatalf("HubForSession after idle window: %v", err)
	}
	hub3 := hubRaw.(*canonicalTranscriptHub)
	if hub3 != hub1 {
		t.Fatalf("expected reattached subscriber to cancel eviction and keep same hub")
	}

	cancel2()
	awaitCondition(t, 2*time.Second, func() bool {
		registry.mu.Lock()
		defer registry.mu.Unlock()
		_, ok := registry.hubs["s-idle-cancel"]
		return !ok
	})
}

func TestCanonicalTranscriptHubRegistryCreatesFreshHubAfterIdleEviction(t *testing.T) {
	events := make(chan types.CodexEvent)
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		Events:          events,
		FollowAvailable: true,
		Close:           func() {},
	}}
	registry := newDefaultCanonicalTranscriptHubRegistryWithIdleTTL(factory, hubTestMapper{}, nil, 40*time.Millisecond)

	hubRaw, err := registry.HubForSession(context.Background(), "s-idle-fresh", "codex")
	if err != nil {
		t.Fatalf("HubForSession first: %v", err)
	}
	hub1 := hubRaw.(*canonicalTranscriptHub)
	ch, cancel, err := hub1.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe first: %v", err)
	}
	_ = awaitEventKind(t, ch, transcriptdomain.TranscriptEventStreamStatus)
	cancel()

	awaitCondition(t, 2*time.Second, func() bool {
		registry.mu.Lock()
		defer registry.mu.Unlock()
		_, ok := registry.hubs["s-idle-fresh"]
		return !ok
	})

	hubRaw, err = registry.HubForSession(context.Background(), "s-idle-fresh", "codex")
	if err != nil {
		t.Fatalf("HubForSession second: %v", err)
	}
	hub2 := hubRaw.(*canonicalTranscriptHub)
	if hub1 == hub2 {
		t.Fatalf("expected fresh hub instance after idle eviction")
	}
	_ = hub2.Close()
}

func TestCanonicalTranscriptHubRegistryDetachIsSafeUnderConcurrentSubscribers(t *testing.T) {
	events := make(chan types.CodexEvent)
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		Events:          events,
		FollowAvailable: true,
		Close:           func() {},
	}}
	registry := newDefaultCanonicalTranscriptHubRegistryWithIdleTTL(factory, hubTestMapper{}, nil, 30*time.Millisecond)

	hubRaw, err := registry.HubForSession(context.Background(), "s-concurrent-detach", "codex")
	if err != nil {
		t.Fatalf("HubForSession: %v", err)
	}
	hub := hubRaw.(*canonicalTranscriptHub)
	cancels := make([]func(), 0, 24)
	for i := 0; i < 24; i++ {
		ch, cancel, subErr := hub.Subscribe(context.Background(), "")
		if subErr != nil {
			t.Fatalf("Subscribe %d: %v", i, subErr)
		}
		cancels = append(cancels, cancel)
		if i == 0 {
			_ = awaitEventKind(t, ch, transcriptdomain.TranscriptEventStreamStatus)
		}
	}

	var wg sync.WaitGroup
	for _, cancel := range cancels {
		cancelFn := cancel
		wg.Add(1)
		go func() {
			defer wg.Done()
			cancelFn()
		}()
	}
	wg.Wait()

	awaitCondition(t, 2*time.Second, func() bool {
		registry.mu.Lock()
		defer registry.mu.Unlock()
		lifecycle, ok := registry.hubs["s-concurrent-detach"]
		if !ok || lifecycle == nil {
			return true
		}
		return lifecycle.subscriberCount == 0
	})
}

func TestCanonicalTranscriptHubRegistryIgnoresStaleHubClosedCallback(t *testing.T) {
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		FollowAvailable: false,
		Close:           func() {},
	}}
	registry := newDefaultCanonicalTranscriptHubRegistryWithIdleTTL(factory, hubTestMapper{}, nil, 250*time.Millisecond)

	hub1Raw, err := registry.HubForSession(context.Background(), "s-stale-close", "codex")
	if err != nil {
		t.Fatalf("HubForSession first: %v", err)
	}
	hub1 := hub1Raw.(*canonicalTranscriptHub)

	registry.mu.Lock()
	oldInstanceID := registry.hubs["s-stale-close"].instanceID
	registry.mu.Unlock()

	if err := registry.CloseSession("s-stale-close"); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	hub2Raw, err := registry.HubForSession(context.Background(), "s-stale-close", "codex")
	if err != nil {
		t.Fatalf("HubForSession second: %v", err)
	}
	hub2 := hub2Raw.(*canonicalTranscriptHub)
	if hub1 == hub2 {
		t.Fatalf("expected second hub to be a fresh instance")
	}

	registry.HubClosed("s-stale-close", oldInstanceID)
	hub3Raw, err := registry.HubForSession(context.Background(), "s-stale-close", "codex")
	if err != nil {
		t.Fatalf("HubForSession third: %v", err)
	}
	hub3 := hub3Raw.(*canonicalTranscriptHub)
	if hub2 != hub3 {
		t.Fatalf("expected stale close callback to leave active hub intact")
	}
}

func TestCanonicalTranscriptHubRegistryIgnoresStaleSubscriberLifecycleCallbacks(t *testing.T) {
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		FollowAvailable: true,
		Close:           func() {},
	}}
	registry := newDefaultCanonicalTranscriptHubRegistryWithIdleTTL(factory, hubTestMapper{}, nil, 5*time.Second)

	_, err := registry.HubForSession(context.Background(), "s-stale-subscriber-callback", "codex")
	if err != nil {
		t.Fatalf("HubForSession first: %v", err)
	}
	registry.mu.Lock()
	oldInstanceID := registry.hubs["s-stale-subscriber-callback"].instanceID
	registry.mu.Unlock()

	if err := registry.CloseSession("s-stale-subscriber-callback"); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	_, err = registry.HubForSession(context.Background(), "s-stale-subscriber-callback", "codex")
	if err != nil {
		t.Fatalf("HubForSession second: %v", err)
	}

	registry.SubscriberAttached("s-stale-subscriber-callback", oldInstanceID)
	registry.mu.Lock()
	lifecycle := registry.hubs["s-stale-subscriber-callback"]
	if lifecycle == nil {
		registry.mu.Unlock()
		t.Fatalf("expected lifecycle entry for second hub instance")
	}
	if lifecycle.subscriberCount != 0 {
		registry.mu.Unlock()
		t.Fatalf("expected stale attach callback to be ignored, got subscriberCount=%d", lifecycle.subscriberCount)
	}
	currentInstanceID := lifecycle.instanceID
	registry.mu.Unlock()

	registry.SubscriberAttached("s-stale-subscriber-callback", currentInstanceID)
	registry.SubscriberDetached("s-stale-subscriber-callback", oldInstanceID)
	registry.mu.Lock()
	lifecycle = registry.hubs["s-stale-subscriber-callback"]
	if lifecycle == nil {
		registry.mu.Unlock()
		t.Fatalf("expected lifecycle after stale detach")
	}
	if lifecycle.subscriberCount != 1 {
		registry.mu.Unlock()
		t.Fatalf("expected stale detach callback to be ignored, got subscriberCount=%d", lifecycle.subscriberCount)
	}
	if lifecycle.idleTimer != nil {
		registry.mu.Unlock()
		t.Fatalf("expected no idle timer while active subscriber remains")
	}
	registry.mu.Unlock()

	registry.SubscriberDetached("s-stale-subscriber-callback", currentInstanceID)
	registry.mu.Lock()
	lifecycle = registry.hubs["s-stale-subscriber-callback"]
	if lifecycle == nil {
		registry.mu.Unlock()
		t.Fatalf("expected lifecycle to remain until idle eviction/close")
	}
	if lifecycle.subscriberCount != 0 {
		registry.mu.Unlock()
		t.Fatalf("expected subscriber count to reach zero after valid detach, got %d", lifecycle.subscriberCount)
	}
	if lifecycle.idleTimer == nil {
		registry.mu.Unlock()
		t.Fatalf("expected idle timer after valid detach")
	}
	registry.mu.Unlock()

	if err := registry.CloseSession("s-stale-subscriber-callback"); err != nil {
		t.Fatalf("CloseSession cleanup: %v", err)
	}
}

func TestCanonicalTranscriptHubRegistryCloseAllHandlesNilLifecycleEntries(t *testing.T) {
	registry := newDefaultCanonicalTranscriptHubRegistryWithIdleTTL(&hubTestIngressFactory{}, hubTestMapper{}, nil, time.Second)
	registry.hubs["nil-entry"] = nil
	registry.hubs["nil-hub"] = &canonicalTranscriptHubLifecycle{}

	if err := registry.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if len(registry.hubs) != 0 {
		t.Fatalf("expected CloseAll to clear registry map, got %d entries", len(registry.hubs))
	}
}

func TestCanonicalTranscriptHubRegistryHubForSessionValidatesInputs(t *testing.T) {
	registry := NewDefaultCanonicalTranscriptHubRegistry(&hubTestIngressFactory{}, hubTestMapper{}, nil)
	if _, err := registry.HubForSession(context.Background(), " ", "codex"); err == nil {
		t.Fatalf("expected missing session id validation error")
	}
	if _, err := registry.HubForSession(context.Background(), "s1", " "); err == nil {
		t.Fatalf("expected missing provider validation error")
	}
}

func TestNewCanonicalTranscriptHubValidatesInputs(t *testing.T) {
	if _, err := newCanonicalTranscriptHub(" ", "codex", &hubTestIngressFactory{}, hubTestMapper{}, nil); err == nil {
		t.Fatalf("expected session validation error")
	}
	if _, err := newCanonicalTranscriptHub("s1", " ", &hubTestIngressFactory{}, hubTestMapper{}, nil); err == nil {
		t.Fatalf("expected provider validation error")
	}
}

func TestNewCanonicalTranscriptHubDefaultsMapperAndProjectorFactory(t *testing.T) {
	hub, err := newCanonicalTranscriptHub("s-defaults", "codex", &hubTestIngressFactory{}, nil, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	if hub.mapper == nil {
		t.Fatalf("expected default mapper")
	}
	if hub.projectorFactory == nil {
		t.Fatalf("expected default projector factory")
	}
}

func TestCanonicalTranscriptHubFanoutSharesSingleIngressAttach(t *testing.T) {
	events := make(chan types.CodexEvent, 8)
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		Events:          events,
		FollowAvailable: true,
		Close:           func() {},
	}}
	registry := NewDefaultCanonicalTranscriptHubRegistry(factory, hubTestMapper{}, nil)
	hubRaw, err := registry.HubForSession(context.Background(), "s-fanout", "codex")
	if err != nil {
		t.Fatalf("HubForSession: %v", err)
	}
	hub := hubRaw.(*canonicalTranscriptHub)

	ch1, cancel1, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe 1: %v", err)
	}
	defer cancel1()
	ch2, cancel2, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe 2: %v", err)
	}
	defer cancel2()

	_ = awaitEventKind(t, ch1, transcriptdomain.TranscriptEventStreamStatus)

	events <- types.CodexEvent{Method: "hello"}
	d1 := awaitEventKind(t, ch1, transcriptdomain.TranscriptEventDelta)
	d2 := awaitEventKind(t, ch2, transcriptdomain.TranscriptEventDelta)
	if len(d1.Delta) != 1 || d1.Delta[0].Text != "hello" {
		t.Fatalf("unexpected subscriber1 delta: %#v", d1)
	}
	if len(d2.Delta) != 1 || d2.Delta[0].Text != "hello" {
		t.Fatalf("unexpected subscriber2 delta: %#v", d2)
	}
	if factory.OpenCount() != 1 {
		t.Fatalf("expected one ingress open, got %d", factory.OpenCount())
	}

	close(events)
	_ = hub.Close()
}

func TestCanonicalTranscriptHubSnapshotReturnsCopy(t *testing.T) {
	events := make(chan types.CodexEvent, 2)
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		Events:          events,
		FollowAvailable: true,
		Close:           func() {},
	}}
	hubRaw, err := NewDefaultCanonicalTranscriptHubRegistry(factory, hubTestMapper{}, nil).
		HubForSession(context.Background(), "s-snapshot", "codex")
	if err != nil {
		t.Fatalf("HubForSession: %v", err)
	}
	hub := hubRaw.(*canonicalTranscriptHub)
	ch, cancel, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()
	_ = awaitEventKind(t, ch, transcriptdomain.TranscriptEventStreamStatus)
	events <- types.CodexEvent{Method: "snapshot-test"}
	_ = awaitEventKind(t, ch, transcriptdomain.TranscriptEventDelta)

	snapshot := hub.Snapshot()
	if len(snapshot.Blocks) != 1 {
		t.Fatalf("expected one snapshot block, got %#v", snapshot.Blocks)
	}
	snapshot.Blocks[0].Text = "mutated"
	refreshed := hub.Snapshot()
	if refreshed.Blocks[0].Text != "snapshot-test" {
		t.Fatalf("expected snapshot copy semantics, got %#v", refreshed.Blocks[0])
	}

	close(events)
	_ = hub.Close()
}

func TestCanonicalTranscriptHubSnapshotNilReceiver(t *testing.T) {
	var hub *canonicalTranscriptHub
	snapshot := hub.Snapshot()
	if snapshot.SessionID != "" || snapshot.Revision != "" || len(snapshot.Blocks) != 0 {
		t.Fatalf("expected zero snapshot for nil hub, got %#v", snapshot)
	}
}

func TestCanonicalTranscriptHubEmitsReplaceForLateSubscriber(t *testing.T) {
	events := make(chan types.CodexEvent, 8)
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		Events:          events,
		FollowAvailable: true,
		Close:           func() {},
	}}
	hubRaw, err := NewDefaultCanonicalTranscriptHubRegistry(factory, hubTestMapper{}, nil).
		HubForSession(context.Background(), "s-replay", "codex")
	if err != nil {
		t.Fatalf("HubForSession: %v", err)
	}
	hub := hubRaw.(*canonicalTranscriptHub)

	ch1, cancel1, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe 1: %v", err)
	}
	defer cancel1()
	_ = awaitEventKind(t, ch1, transcriptdomain.TranscriptEventStreamStatus)
	events <- types.CodexEvent{Method: "first"}
	_ = awaitEventKind(t, ch1, transcriptdomain.TranscriptEventDelta)

	ch2, cancel2, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe 2: %v", err)
	}
	defer cancel2()
	replace := awaitEventKind(t, ch2, transcriptdomain.TranscriptEventReplace)
	if replace.Replace == nil || len(replace.Replace.Blocks) != 1 || replace.Replace.Blocks[0].Text != "first" {
		t.Fatalf("expected replace snapshot with first delta, got %#v", replace)
	}

	events <- types.CodexEvent{Method: "second"}
	delta := awaitEventKind(t, ch2, transcriptdomain.TranscriptEventDelta)
	if len(delta.Delta) != 1 || delta.Delta[0].Text != "second" {
		t.Fatalf("expected live delta after replace, got %#v", delta)
	}

	close(events)
	_ = hub.Close()
}

func TestCanonicalTranscriptHubSubscribeFailsWhenIngressUnavailable(t *testing.T) {
	hub, err := newCanonicalTranscriptHub("s-ingress", "codex", nil, hubTestMapper{}, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	if _, _, err := hub.Subscribe(context.Background(), ""); err == nil {
		t.Fatalf("expected ingress unavailable error")
	}
}

func TestCanonicalTranscriptHubSubscribeNilReceiver(t *testing.T) {
	var hub *canonicalTranscriptHub
	if _, _, err := hub.Subscribe(context.Background(), ""); err == nil {
		t.Fatalf("expected nil receiver subscribe error")
	}
}

func TestCanonicalTranscriptHubSubscribeSkipsInvalidReplaySnapshot(t *testing.T) {
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		FollowAvailable: false,
		Close:           func() {},
	}}
	hub, err := newCanonicalTranscriptHub("s-invalid-replay", "codex", factory, hubTestMapper{}, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	// Intentionally invalid snapshot payload (empty text) so replay validation fails.
	hub.updateState(transcriptdomain.TranscriptSnapshot{
		SessionID: "s-invalid-replay",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
		Blocks: []transcriptdomain.Block{{
			Kind: "assistant",
			Text: "",
		}},
	}, transcriptdomain.MustParseRevisionToken("2"))

	ch, cancel, err := hub.Subscribe(context.Background(), transcriptdomain.MustParseRevisionToken("1"))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()
	first := awaitEventKind(t, ch, transcriptdomain.TranscriptEventStreamStatus)
	if first.StreamStatus != transcriptdomain.StreamStatusReady {
		t.Fatalf("expected ready status, got %#v", first)
	}
}

func TestCanonicalTranscriptHubSubscribeFailsAfterClose(t *testing.T) {
	hub, err := newCanonicalTranscriptHub("s-closed", "codex", &hubTestIngressFactory{
		handle: TranscriptIngressHandle{FollowAvailable: false, Close: func() {}},
	}, hubTestMapper{}, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	if err := hub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, _, err := hub.Subscribe(context.Background(), ""); err == nil {
		t.Fatalf("expected closed hub subscribe error")
	}
}

func TestCanonicalTranscriptHubCloseIsIdempotent(t *testing.T) {
	hub, err := newCanonicalTranscriptHub("s-close-idempotent", "codex", &hubTestIngressFactory{
		handle: TranscriptIngressHandle{FollowAvailable: false, Close: func() {}},
	}, hubTestMapper{}, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	if err := hub.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := hub.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if !hub.isExplicitlyClosed() {
		t.Fatalf("expected hub explicit close flag")
	}
}

func TestCanonicalTranscriptHubCloseNilReceiver(t *testing.T) {
	var hub *canonicalTranscriptHub
	if err := hub.Close(); err != nil {
		t.Fatalf("expected nil receiver close to be noop, got %v", err)
	}
}

func TestCanonicalTranscriptHubIsExplicitlyClosedStates(t *testing.T) {
	var nilHub *canonicalTranscriptHub
	if !nilHub.isExplicitlyClosed() {
		t.Fatalf("expected nil hub to report closed")
	}

	hub, err := newCanonicalTranscriptHub("s-open", "codex", &hubTestIngressFactory{}, hubTestMapper{}, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	if hub.isExplicitlyClosed() {
		t.Fatalf("expected open hub to report not closed")
	}
}

func TestCanonicalTranscriptHubDegradesWhenFollowUnavailable(t *testing.T) {
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		FollowAvailable: false,
		Close:           func() {},
	}}
	hubRaw, err := NewDefaultCanonicalTranscriptHubRegistry(factory, hubTestMapper{}, nil).
		HubForSession(context.Background(), "s-offline", "claude")
	if err != nil {
		t.Fatalf("HubForSession: %v", err)
	}
	hub := hubRaw.(*canonicalTranscriptHub)
	ch, cancel, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	first := awaitEventKind(t, ch, transcriptdomain.TranscriptEventStreamStatus)
	if first.StreamStatus != transcriptdomain.StreamStatusReady {
		t.Fatalf("expected ready status first, got %#v", first)
	}
	second := awaitEventKind(t, ch, transcriptdomain.TranscriptEventStreamStatus)
	if second.StreamStatus != transcriptdomain.StreamStatusClosed {
		t.Fatalf("expected closed status second, got %#v", second)
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected channel close")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for channel close")
	}
	_ = hub.Close()
}

func TestCanonicalTranscriptHubEmitsReconnectingDuringRecoverableIngressRestart(t *testing.T) {
	events1 := make(chan types.CodexEvent)
	close(events1)
	events2 := make(chan types.CodexEvent, 1)
	events2 <- types.CodexEvent{Method: "after-reconnect"}
	close(events2)

	factory := &scriptedHubIngressFactory{
		steps: []scriptedIngressStep{
			{
				handle: TranscriptIngressHandle{
					Events:          events1,
					FollowAvailable: true,
					Reconnectable:   true,
					Close:           func() {},
				},
			},
			{
				handle: TranscriptIngressHandle{
					Events:          events2,
					FollowAvailable: true,
					Reconnectable:   false,
					Close:           func() {},
				},
			},
		},
	}
	hub, err := newCanonicalTranscriptHub("s-reconnect", "codex", factory, hubTestMapper{}, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	ch, cancel, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	statuses := collectStreamStatusesUntilClosed(t, ch)
	if len(statuses) < 4 {
		t.Fatalf("expected ready/reconnecting/ready/closed status sequence, got %#v", statuses)
	}
	if statuses[0] != transcriptdomain.StreamStatusReady || statuses[1] != transcriptdomain.StreamStatusReconnecting {
		t.Fatalf("expected ready then reconnecting, got %#v", statuses)
	}
}

func TestCanonicalTranscriptHubEmitsReadyAgainAfterReconnect(t *testing.T) {
	events1 := make(chan types.CodexEvent)
	close(events1)
	events2 := make(chan types.CodexEvent, 1)
	events2 <- types.CodexEvent{Method: "second-ready"}
	close(events2)

	factory := &scriptedHubIngressFactory{
		steps: []scriptedIngressStep{
			{
				handle: TranscriptIngressHandle{
					Events:          events1,
					FollowAvailable: true,
					Reconnectable:   true,
					Close:           func() {},
				},
			},
			{
				handle: TranscriptIngressHandle{
					Events:          events2,
					FollowAvailable: true,
					Reconnectable:   false,
					Close:           func() {},
				},
			},
		},
	}
	hub, err := newCanonicalTranscriptHub("s-ready-again", "codex", factory, hubTestMapper{}, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	ch, cancel, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	statuses := collectStreamStatusesUntilClosed(t, ch)
	readyCount := 0
	for _, status := range statuses {
		if status == transcriptdomain.StreamStatusReady {
			readyCount++
		}
	}
	if readyCount < 2 {
		t.Fatalf("expected ready to be emitted again after reconnect, got %#v", statuses)
	}
}

func TestCanonicalTranscriptHubEmitsErrorBeforeClosedOnTerminalIngressFailure(t *testing.T) {
	events1 := make(chan types.CodexEvent)
	close(events1)
	factory := &scriptedHubIngressFactory{
		steps: []scriptedIngressStep{
			{
				handle: TranscriptIngressHandle{
					Events:          events1,
					FollowAvailable: true,
					Reconnectable:   true,
					Close:           func() {},
				},
			},
			{err: errors.New("terminal ingress failure")},
		},
	}
	hub, err := newCanonicalTranscriptHub("s-terminal-error", "codex", factory, hubTestMapper{}, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	ch, cancel, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	statuses := collectStreamStatusesUntilClosed(t, ch)
	errorIdx := -1
	closedIdx := -1
	for i, status := range statuses {
		if status == transcriptdomain.StreamStatusError && errorIdx == -1 {
			errorIdx = i
		}
		if status == transcriptdomain.StreamStatusClosed && closedIdx == -1 {
			closedIdx = i
		}
	}
	if errorIdx == -1 || closedIdx == -1 || errorIdx > closedIdx {
		t.Fatalf("expected error before closed, got %#v", statuses)
	}
}

func TestCanonicalTranscriptHubEmitsClosedExactlyOnce(t *testing.T) {
	events1 := make(chan types.CodexEvent)
	close(events1)
	factory := &scriptedHubIngressFactory{
		steps: []scriptedIngressStep{
			{
				handle: TranscriptIngressHandle{
					Events:          events1,
					FollowAvailable: true,
					Reconnectable:   true,
					Close:           func() {},
				},
			},
			{err: errors.New("terminal ingress failure")},
		},
	}
	hub, err := newCanonicalTranscriptHub("s-closed-once", "codex", factory, hubTestMapper{}, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	ch, cancel, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	statuses := collectStreamStatusesUntilClosed(t, ch)
	closedCount := 0
	for _, status := range statuses {
		if status == transcriptdomain.StreamStatusClosed {
			closedCount++
		}
	}
	if closedCount != 1 {
		t.Fatalf("expected closed exactly once, got %d from %#v", closedCount, statuses)
	}
}

func TestCanonicalTranscriptHubUsesInjectedReconnectPolicy(t *testing.T) {
	events1 := make(chan types.CodexEvent)
	close(events1)
	factory := &scriptedHubIngressFactory{
		steps: []scriptedIngressStep{
			{
				handle: TranscriptIngressHandle{
					Events:          events1,
					FollowAvailable: true,
					Reconnectable:   true,
					Close:           func() {},
				},
			},
		},
	}
	hub, err := newCanonicalTranscriptHub("s-policy-reconnect", "codex", factory, hubTestMapper{}, nil)
	if err != nil {
		t.Fatalf("newCanonicalTranscriptHub: %v", err)
	}
	hub.setReconnectPolicy(neverReconnectPolicy{})
	ch, cancel, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	statuses := collectStreamStatusesUntilClosed(t, ch)
	if len(statuses) < 3 {
		t.Fatalf("expected ready/error/closed statuses, got %#v", statuses)
	}
	if statuses[1] != transcriptdomain.StreamStatusError || statuses[len(statuses)-1] != transcriptdomain.StreamStatusClosed {
		t.Fatalf("expected injected policy to force terminal error then close, got %#v", statuses)
	}
	if factory.OpenCount() != 1 {
		t.Fatalf("expected no reconnect attach with injected policy, got %d opens", factory.OpenCount())
	}
}

func TestCanonicalTranscriptHubSlowSubscriberDoesNotBlock(t *testing.T) {
	events := make(chan types.CodexEvent, 1024)
	factory := &hubTestIngressFactory{handle: TranscriptIngressHandle{
		Events:          events,
		FollowAvailable: true,
		Close:           func() {},
	}}
	hubRaw, err := NewDefaultCanonicalTranscriptHubRegistry(factory, hubTestMapper{}, nil).
		HubForSession(context.Background(), "s-slow", "codex")
	if err != nil {
		t.Fatalf("HubForSession: %v", err)
	}
	hub := hubRaw.(*canonicalTranscriptHub)

	slowCh, cancelSlow, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("slow subscribe: %v", err)
	}
	defer cancelSlow()
	fastCh, cancelFast, err := hub.Subscribe(context.Background(), "")
	if err != nil {
		t.Fatalf("fast subscribe: %v", err)
	}
	defer cancelFast()

	const total = 500
	for i := 0; i < total; i++ {
		events <- types.CodexEvent{Method: strconv.Itoa(i)}
	}
	close(events)

	lastText := ""
	count := 0
	timeout := time.After(4 * time.Second)
loop:
	for {
		select {
		case event, ok := <-fastCh:
			if !ok {
				break loop
			}
			if event.Kind != transcriptdomain.TranscriptEventDelta || len(event.Delta) == 0 {
				continue
			}
			count++
			lastText = event.Delta[0].Text
		case <-timeout:
			t.Fatalf("timed out reading fast subscriber")
		}
	}
	if count == 0 {
		t.Fatalf("expected fast subscriber deltas")
	}
	if lastText != strconv.Itoa(total-1) {
		t.Fatalf("expected fast subscriber to receive latest event %d, got %q", total-1, lastText)
	}

	select {
	case _, ok := <-slowCh:
		if ok {
			// slow channel may not be closed immediately if test consumption leaves buffer space.
		}
	case <-time.After(250 * time.Millisecond):
	}
	_ = hub.Close()
}

func TestShouldReplaySnapshotRules(t *testing.T) {
	if shouldReplaySnapshot("", "") {
		t.Fatalf("expected no replay for empty current revision")
	}
	if !shouldReplaySnapshot("", transcriptdomain.MustParseRevisionToken("2")) {
		t.Fatalf("expected replay for zero after revision")
	}
	if !shouldReplaySnapshot(transcriptdomain.MustParseRevisionToken("1"), transcriptdomain.MustParseRevisionToken("2")) {
		t.Fatalf("expected replay when current is newer")
	}
	if shouldReplaySnapshot(transcriptdomain.MustParseRevisionToken("2"), transcriptdomain.MustParseRevisionToken("2")) {
		t.Fatalf("expected no replay on equal revisions")
	}
	if !shouldReplaySnapshot(transcriptdomain.RevisionToken("bad token"), transcriptdomain.MustParseRevisionToken("2")) {
		t.Fatalf("expected replay when comparison fails")
	}
}

func awaitCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for condition")
}

func collectStreamStatusesUntilClosed(
	t *testing.T,
	ch <-chan transcriptdomain.TranscriptEvent,
) []transcriptdomain.StreamStatus {
	t.Helper()
	statuses := make([]transcriptdomain.StreamStatus, 0, 8)
	timeout := time.After(2 * time.Second)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return statuses
			}
			if event.Kind == transcriptdomain.TranscriptEventStreamStatus {
				statuses = append(statuses, event.StreamStatus)
			}
		case <-timeout:
			t.Fatalf("timed out collecting stream statuses")
		}
	}
}

func awaitEventKind(
	t *testing.T,
	ch <-chan transcriptdomain.TranscriptEvent,
	kind transcriptdomain.TranscriptEventKind,
) transcriptdomain.TranscriptEvent {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed waiting for %s", kind)
			}
			if event.Kind == kind {
				return event
			}
		case <-timeout:
			t.Fatalf("timed out waiting for event kind %s", kind)
		}
	}
}
