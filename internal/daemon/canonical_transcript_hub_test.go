package daemon

import (
	"context"
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
