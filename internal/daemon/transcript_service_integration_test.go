package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"control/internal/daemon/transcriptadapters"
	"control/internal/daemon/transcriptdomain"
	"control/internal/store"
	"control/internal/types"
)

type fixedTranscriptMapper struct {
	itemEvents  []transcriptdomain.TranscriptEvent
	eventEvents []transcriptdomain.TranscriptEvent
}

func (m fixedTranscriptMapper) MapItem(string, transcriptadapters.MappingContext, map[string]any) []transcriptdomain.TranscriptEvent {
	out := make([]transcriptdomain.TranscriptEvent, len(m.itemEvents))
	copy(out, m.itemEvents)
	return out
}

func (m fixedTranscriptMapper) MapEvent(string, transcriptadapters.MappingContext, types.CodexEvent) []transcriptdomain.TranscriptEvent {
	out := make([]transcriptdomain.TranscriptEvent, len(m.eventEvents))
	copy(out, m.eventEvents)
	return out
}

type fixedTranscriptTransportSelector struct {
	transport       transcriptTransport
	followAvailable bool
	err             error
}

func (s fixedTranscriptTransportSelector) Select(context.Context, string, string) (transcriptTransportSelection, error) {
	if s.err != nil {
		return transcriptTransportSelection{}, s.err
	}
	followAvailable := s.followAvailable
	if !followAvailable && (s.transport.eventsCh != nil || s.transport.itemsCh != nil) {
		followAvailable = true
	}
	return transcriptTransportSelection{
		transport:       s.transport,
		followAvailable: followAvailable,
	}, nil
}

type scriptedTranscriptIngressStep struct {
	handle TranscriptIngressHandle
	err    error
}

type scriptedTranscriptIngressFactory struct {
	mu    sync.Mutex
	steps []scriptedTranscriptIngressStep
	opens int
}

func (f *scriptedTranscriptIngressFactory) Open(context.Context, string, string) (TranscriptIngressHandle, error) {
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

func (f *scriptedTranscriptIngressFactory) OpenCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opens
}

type integrationNeverReconnectPolicy struct{}

func (integrationNeverReconnectPolicy) NextAttempt(current int, hadTraffic bool) (int, bool) {
	_ = hadTraffic
	return current + 1, false
}

type stubTranscriptSnapshotReader struct {
	snapshot transcriptdomain.TranscriptSnapshot
	err      error

	calls     int
	lastID    string
	lastProv  string
	lastLines int
}

func (s *stubTranscriptSnapshotReader) ReadSnapshot(_ context.Context, sessionID, provider string, lines int) (transcriptdomain.TranscriptSnapshot, error) {
	s.calls++
	s.lastID = sessionID
	s.lastProv = provider
	s.lastLines = lines
	if s.err != nil {
		return transcriptdomain.TranscriptSnapshot{}, s.err
	}
	return s.snapshot, nil
}

type stubTranscriptFollowOpener struct {
	events []transcriptdomain.TranscriptEvent
	err    error

	calls     int
	lastID    string
	lastProv  string
	lastAfter transcriptdomain.RevisionToken
}

func (s *stubTranscriptFollowOpener) OpenFollow(_ context.Context, sessionID, provider string, after transcriptdomain.RevisionToken) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	s.calls++
	s.lastID = sessionID
	s.lastProv = provider
	s.lastAfter = after
	if s.err != nil {
		return nil, nil, s.err
	}
	ch := make(chan transcriptdomain.TranscriptEvent, len(s.events))
	for _, event := range s.events {
		ch <- event
	}
	close(ch)
	return ch, func() {}, nil
}

func TestSessionServiceGetTranscriptSnapshotFromHistoryLogs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sessionID := "sess-transcript"
	sessionsDir := filepath.Join(home, ".archon", "sessions", sessionID)
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "stdout.log"), []byte("hello from log\n"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionsDir, "stderr.log"), []byte(""), 0o644); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        sessionID,
			Provider:  "custom",
			Cmd:       "custom",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	svc := NewSessionService(nil, &Stores{Sessions: index}, nil)
	snapshot, err := svc.GetTranscriptSnapshot(context.Background(), sessionID, 200)
	if err != nil {
		t.Fatalf("GetTranscriptSnapshot: %v", err)
	}
	if snapshot.SessionID != sessionID {
		t.Fatalf("expected session id %q, got %q", sessionID, snapshot.SessionID)
	}
	if len(snapshot.Blocks) == 0 {
		t.Fatalf("expected transcript blocks from history logs")
	}
	if snapshot.Revision.String() == "" {
		t.Fatalf("expected revision token")
	}
}

func TestSessionServiceGetTranscriptSnapshotRequiresSessionID(t *testing.T) {
	svc := NewSessionService(nil, nil, nil)
	if _, err := svc.GetTranscriptSnapshot(context.Background(), " ", 200); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestSessionServiceGetTranscriptSnapshotUsesInjectedSnapshotReader(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s-snapshot-injected",
			Provider:  "  CoDex  ",
			Cmd:       "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	reader := &stubTranscriptSnapshotReader{
		snapshot: transcriptdomain.TranscriptSnapshot{
			SessionID: "s-snapshot-injected",
			Revision:  transcriptdomain.MustParseRevisionToken("rev-snapshot"),
			Blocks: []transcriptdomain.Block{{
				Kind: "assistant",
				Role: "assistant",
				Text: "snapshot from injected reader",
			}},
		},
	}
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil, WithTranscriptSnapshotReader(reader))
	snapshot, err := svc.GetTranscriptSnapshot(context.Background(), "s-snapshot-injected", 11)
	if err != nil {
		t.Fatalf("GetTranscriptSnapshot: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("expected snapshot reader to be called once, got %d", reader.calls)
	}
	if reader.lastID != "s-snapshot-injected" {
		t.Fatalf("expected snapshot reader session id, got %q", reader.lastID)
	}
	if reader.lastProv != "codex" {
		t.Fatalf("expected normalized provider codex, got %q", reader.lastProv)
	}
	if reader.lastLines != 11 {
		t.Fatalf("expected forwarded lines=11, got %d", reader.lastLines)
	}
	if len(snapshot.Blocks) != 1 || snapshot.Blocks[0].Text != "snapshot from injected reader" {
		t.Fatalf("expected injected snapshot payload, got %#v", snapshot.Blocks)
	}
}

func TestSessionServiceSubscribeTranscriptAcceptsLexicalAfterRevision(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s1",
			Provider:  "custom",
			Cmd:       "custom",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	events := make(chan types.CodexEvent)
	close(events)
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptMapper(fixedTranscriptMapper{}),
		WithTranscriptTransportSelector(fixedTranscriptTransportSelector{transport: transcriptTransport{eventsCh: events}}),
	)

	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s1", transcriptdomain.MustParseRevisionToken("rev-A"))
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()
	first, ok := <-ch
	if !ok {
		t.Fatalf("expected ready event")
	}
	if first.Kind != transcriptdomain.TranscriptEventStreamStatus {
		t.Fatalf("expected stream status event, got %q", first.Kind)
	}
	if _, err := transcriptdomain.ParseRevisionToken(first.Revision.String()); err != nil {
		t.Fatalf("expected valid revision token: %v", err)
	}
}

func TestSessionServiceSubscribeTranscriptUsesInjectedFollowOpener(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s-follow-injected",
			Provider:  " CoDeX ",
			Cmd:       "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	opener := &stubTranscriptFollowOpener{
		events: []transcriptdomain.TranscriptEvent{{
			Kind:         transcriptdomain.TranscriptEventStreamStatus,
			StreamStatus: transcriptdomain.StreamStatusReady,
			Revision:     transcriptdomain.MustParseRevisionToken("rev-follow-ready"),
		}},
	}
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil, WithTranscriptFollowOpener(opener))
	after := transcriptdomain.MustParseRevisionToken("rev-base")
	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s-follow-injected", after)
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()

	streamEvents := collectTranscriptEvents(ch)
	if len(streamEvents) != 1 {
		t.Fatalf("expected injected follow events, got %#v", streamEvents)
	}
	if streamEvents[0].Kind != transcriptdomain.TranscriptEventStreamStatus || streamEvents[0].StreamStatus != transcriptdomain.StreamStatusReady {
		t.Fatalf("expected ready stream status from injected opener, got %#v", streamEvents[0])
	}
	if opener.calls != 1 {
		t.Fatalf("expected follow opener to be called once, got %d", opener.calls)
	}
	if opener.lastID != "s-follow-injected" {
		t.Fatalf("expected follow opener session id, got %q", opener.lastID)
	}
	if opener.lastProv != "codex" {
		t.Fatalf("expected normalized provider codex, got %q", opener.lastProv)
	}
	if opener.lastAfter.String() != after.String() {
		t.Fatalf("expected after revision %q, got %q", after.String(), opener.lastAfter.String())
	}
}

func TestSessionServiceSubscribeTranscriptUsesMapperAndSelector(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s2",
			Provider:  "custom",
			Cmd:       "custom",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	events := make(chan types.CodexEvent, 1)
	events <- types.CodexEvent{Method: "turn/started"}
	close(events)
	mapper := fixedTranscriptMapper{
		eventEvents: []transcriptdomain.TranscriptEvent{{
			Kind: transcriptdomain.TranscriptEventDelta,
			Delta: []transcriptdomain.Block{{
				Kind: "assistant",
				Text: "mapped",
			}},
		}},
	}
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptMapper(mapper),
		WithTranscriptTransportSelector(fixedTranscriptTransportSelector{transport: transcriptTransport{eventsCh: events}}),
	)

	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s2", "")
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()

	seenDelta := false
	for event := range ch {
		if event.Kind == transcriptdomain.TranscriptEventDelta {
			seenDelta = true
			break
		}
	}
	if !seenDelta {
		t.Fatalf("expected mapped delta event to flow through transcript stream")
	}
}

func TestSessionServiceSubscribeTranscriptTransportError(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{ID: "s3", Provider: "custom", Cmd: "custom", Status: types.SessionStatusInactive, CreatedAt: now},
		Source:  sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptTransportSelector(fixedTranscriptTransportSelector{err: invalidError("boom", nil)}),
	)
	if _, _, err := svc.SubscribeTranscript(context.Background(), "s3", ""); err == nil {
		t.Fatalf("expected transport selection error")
	}
}

func TestSessionServiceSubscribeTranscriptDegradesGracefullyWhenSessionIsPersistedButNotLive(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s-claude-persisted",
			Provider:  "claude",
			Cmd:       "claude",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	manager, err := NewSessionManager(t.TempDir())
	if err != nil {
		t.Fatalf("new session manager: %v", err)
	}

	svc := NewSessionService(manager, &Stores{Sessions: index}, nil)
	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s-claude-persisted", "")
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()

	streamEvents := collectTranscriptEvents(ch)
	if len(streamEvents) != 2 {
		t.Fatalf("expected ready and closed events, got %#v", streamEvents)
	}
	if streamEvents[0].Kind != transcriptdomain.TranscriptEventStreamStatus || streamEvents[0].StreamStatus != transcriptdomain.StreamStatusReady {
		t.Fatalf("expected initial ready status, got %#v", streamEvents[0])
	}
	if streamEvents[1].Kind != transcriptdomain.TranscriptEventStreamStatus || streamEvents[1].StreamStatus != transcriptdomain.StreamStatusClosed {
		t.Fatalf("expected terminal closed status, got %#v", streamEvents[1])
	}
	if !streamEvents[1].Revision.IsZero() && streamEvents[0].Revision.String() == streamEvents[1].Revision.String() {
		t.Fatalf("expected closed event to advance revision, got ready=%q closed=%q", streamEvents[0].Revision.String(), streamEvents[1].Revision.String())
	}
}

func TestSessionServiceSubscribeTranscriptPropagatesReconnectLifecycleFromHub(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s-reconnect-lifecycle",
			Provider:  "codex",
			Cmd:       "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	events1 := make(chan types.CodexEvent)
	close(events1)
	events2 := make(chan types.CodexEvent, 1)
	events2 <- types.CodexEvent{Method: "reconnected"}
	close(events2)
	ingress := &scriptedTranscriptIngressFactory{
		steps: []scriptedTranscriptIngressStep{
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
	mapper := fixedTranscriptMapper{
		eventEvents: []transcriptdomain.TranscriptEvent{{
			Kind: transcriptdomain.TranscriptEventDelta,
			Delta: []transcriptdomain.Block{{
				Kind: "assistant_message",
				Role: "assistant",
				Text: "mapped after reconnect",
			}},
		}},
	}
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptMapper(mapper),
		WithTranscriptIngressFactory(ingress),
	)

	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s-reconnect-lifecycle", "")
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()

	streamEvents := collectTranscriptEvents(ch)
	statuses := make([]transcriptdomain.StreamStatus, 0, 6)
	for _, event := range streamEvents {
		if event.Kind == transcriptdomain.TranscriptEventStreamStatus {
			statuses = append(statuses, event.StreamStatus)
		}
	}
	if len(statuses) < 4 {
		t.Fatalf("expected ready/reconnecting/ready/closed lifecycle statuses, got %#v", statuses)
	}
	if statuses[0] != transcriptdomain.StreamStatusReady || statuses[1] != transcriptdomain.StreamStatusReconnecting || statuses[2] != transcriptdomain.StreamStatusReady {
		t.Fatalf("expected ready -> reconnecting -> ready sequence, got %#v", statuses)
	}
}

func TestSessionServiceSubscribeTranscriptPropagatesTerminalHubErrorAsCanonicalStatus(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s-terminal-hub-error",
			Provider:  "codex",
			Cmd:       "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	events1 := make(chan types.CodexEvent)
	close(events1)
	ingress := &scriptedTranscriptIngressFactory{
		steps: []scriptedTranscriptIngressStep{
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
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptMapper(fixedTranscriptMapper{}),
		WithTranscriptIngressFactory(ingress),
	)

	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s-terminal-hub-error", "")
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()

	streamEvents := collectTranscriptEvents(ch)
	statuses := make([]transcriptdomain.StreamStatus, 0, 6)
	for _, event := range streamEvents {
		if event.Kind == transcriptdomain.TranscriptEventStreamStatus {
			statuses = append(statuses, event.StreamStatus)
		}
	}
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
		t.Fatalf("expected canonical error status before closed, got %#v", statuses)
	}
}

func TestSessionServiceSubscribeTranscriptUsesInjectedReconnectPolicyEndToEnd(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s-policy-injected",
			Provider:  "codex",
			Cmd:       "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	events1 := make(chan types.CodexEvent)
	close(events1)
	ingress := &scriptedTranscriptIngressFactory{
		steps: []scriptedTranscriptIngressStep{
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
					FollowAvailable: false,
					Close:           func() {},
				},
			},
		},
	}
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptMapper(fixedTranscriptMapper{}),
		WithTranscriptIngressFactory(ingress),
		WithTranscriptReconnectPolicy(integrationNeverReconnectPolicy{}),
	)

	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s-policy-injected", "")
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()

	streamEvents := collectTranscriptEvents(ch)
	statuses := make([]transcriptdomain.StreamStatus, 0, len(streamEvents))
	for _, event := range streamEvents {
		if event.Kind == transcriptdomain.TranscriptEventStreamStatus {
			statuses = append(statuses, event.StreamStatus)
		}
	}
	if len(statuses) < 3 {
		t.Fatalf("expected ready/error/closed statuses, got %#v", statuses)
	}
	if statuses[0] != transcriptdomain.StreamStatusReady || statuses[1] != transcriptdomain.StreamStatusError || statuses[len(statuses)-1] != transcriptdomain.StreamStatusClosed {
		t.Fatalf("expected injected policy lifecycle ready->error->closed, got %#v", statuses)
	}
	for _, status := range statuses {
		if status == transcriptdomain.StreamStatusReconnecting {
			t.Fatalf("expected injected never-reconnect policy to suppress reconnecting status, got %#v", statuses)
		}
	}
	if ingress.OpenCount() != 1 {
		t.Fatalf("expected single ingress open when injected policy denies reconnect, got %d", ingress.OpenCount())
	}
}

func TestSessionServiceProviderSupportsTranscriptStreaming(t *testing.T) {
	svc := NewSessionService(nil, nil, nil)
	if !svc.providerSupportsTranscriptStreaming("codex") {
		t.Fatalf("expected codex to support transcript streaming")
	}
	if svc.providerSupportsTranscriptStreaming("custom") {
		t.Fatalf("expected custom to not support transcript streaming")
	}
}

func TestSessionServiceGetTranscriptSnapshotMappingProducesValidJSON(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{ID: "s4", Provider: "custom", Cmd: "custom", Status: types.SessionStatusInactive, CreatedAt: now},
		Source:  sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionDir := filepath.Join(home, ".archon", "sessions", "s4")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "stdout.log"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write stdout: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "stderr.log"), []byte(""), 0o644); err != nil {
		t.Fatalf("write stderr: %v", err)
	}
	svc := NewSessionService(nil, &Stores{Sessions: index}, nil)
	snapshot, err := svc.GetTranscriptSnapshot(context.Background(), "s4", 10)
	if err != nil {
		t.Fatalf("GetTranscriptSnapshot: %v", err)
	}
	if _, err := json.Marshal(snapshot); err != nil {
		t.Fatalf("expected snapshot JSON marshal to succeed: %v", err)
	}
}

func TestSessionServiceSubscribeTranscriptFiltersCodexControlNoiseAndEmitsAssistantDelta(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s-codex-live",
			Provider:  "codex",
			Cmd:       "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	events := make(chan types.CodexEvent, 8)
	events <- types.CodexEvent{Method: "codex/event/mcp_startup_complete", Params: json.RawMessage(`{"msg":{"type":"mcp_startup_complete"}}`)}
	events <- types.CodexEvent{Method: "codex/event/mcp_startup_update", Params: json.RawMessage(`{"msg":{"type":"mcp_startup_update"}}`)}
	events <- types.CodexEvent{Method: "thread/started", Params: json.RawMessage(`{"threadId":"thread-1"}`)}
	events <- types.CodexEvent{Method: "thread/status/changed", Params: json.RawMessage(`{"threadId":"thread-1","status":{"type":"active"}}`)}
	events <- types.CodexEvent{
		Method: "item/agentMessage/delta",
		Params: json.RawMessage(`{"threadId":"thread-1","itemId":"msg_1","delta":"hello from assistant"}`),
	}
	events <- types.CodexEvent{Method: "thread/status/changed", Params: json.RawMessage(`{"threadId":"thread-1","status":{"type":"idle"}}`)}
	close(events)

	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptMapper(NewDefaultTranscriptMapper(nil)),
		WithTranscriptTransportSelector(fixedTranscriptTransportSelector{
			transport: transcriptTransport{eventsCh: events},
		}),
	)

	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s-codex-live", "")
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()

	streamEvents := collectTranscriptEvents(ch)
	if len(streamEvents) == 0 {
		t.Fatalf("expected transcript stream events")
	}

	var hasAssistantDelta bool
	var hasReadyStatus bool
	for _, event := range streamEvents {
		if event.Kind == transcriptdomain.TranscriptEventDelta {
			if len(event.Delta) == 1 && event.Delta[0].Role == "assistant" && event.Delta[0].Text == "hello from assistant" {
				hasAssistantDelta = true
			}
			if len(event.Delta) == 1 && event.Delta[0].Kind == "provider_event" {
				t.Fatalf("unexpected provider_event control noise in transcript delta: %#v", event.Delta[0])
			}
		}
		if event.Kind == transcriptdomain.TranscriptEventStreamStatus && event.StreamStatus == transcriptdomain.StreamStatusReady {
			hasReadyStatus = true
		}
	}

	if !hasAssistantDelta {
		t.Fatalf("expected assistant delta event from live codex stream, got %#v", streamEvents)
	}
	if !hasReadyStatus {
		t.Fatalf("expected at least one stream ready status event")
	}
}

func TestSessionServiceSubscribeTranscriptMixedLiveNoiseKeepsOnlyTranscriptRelevantEvents(t *testing.T) {
	index := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	now := time.Now().UTC()
	_, err := index.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s-codex-mixed",
			Provider:  "codex",
			Cmd:       "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	events := make(chan types.CodexEvent, 12)
	reqID := 9
	events <- types.CodexEvent{Method: "account/rateLimits/updated", Params: json.RawMessage(`{"limits":[]}`)}
	events <- types.CodexEvent{Method: "thread/status/changed", Params: json.RawMessage(`{"threadId":"thread-1","status":{"type":"active"}}`)}
	events <- types.CodexEvent{Method: "codex/event/token_count", Params: json.RawMessage(`{"total":123}`)}
	events <- types.CodexEvent{Method: "item/fileChange/requestApproval", ID: &reqID}
	events <- types.CodexEvent{
		Method: "item/agentMessage/delta",
		Params: json.RawMessage(`{"threadId":"thread-1","itemId":"msg_2","delta":"assistant reply"}`),
	}
	events <- types.CodexEvent{Method: "thread/tokenUsage/updated", Params: json.RawMessage(`{"input_tokens":10}`)}
	events <- types.CodexEvent{Method: "thread/status/changed", Params: json.RawMessage(`{"threadId":"thread-1","status":{"type":"idle"}}`)}
	close(events)

	svc := NewSessionService(nil, &Stores{Sessions: index}, nil,
		WithTranscriptMapper(NewDefaultTranscriptMapper(nil)),
		WithTranscriptTransportSelector(fixedTranscriptTransportSelector{
			transport: transcriptTransport{eventsCh: events},
		}),
	)

	ch, cancel, err := svc.SubscribeTranscript(context.Background(), "s-codex-mixed", "")
	if err != nil {
		t.Fatalf("SubscribeTranscript: %v", err)
	}
	defer cancel()

	streamEvents := collectTranscriptEvents(ch)
	if len(streamEvents) == 0 {
		t.Fatalf("expected transcript stream events")
	}

	var hasAssistantDelta bool
	var hasApprovalPending bool
	var hasReadyStatus bool
	for _, event := range streamEvents {
		switch event.Kind {
		case transcriptdomain.TranscriptEventDelta:
			if len(event.Delta) == 1 && event.Delta[0].Text == "assistant reply" && event.Delta[0].Role == "assistant" {
				hasAssistantDelta = true
				continue
			}
			t.Fatalf("unexpected transcript delta from mixed noise stream: %#v", event)
		case transcriptdomain.TranscriptEventApprovalPending:
			if event.Approval != nil && event.Approval.RequestID == reqID {
				hasApprovalPending = true
				continue
			}
			t.Fatalf("unexpected approval payload: %#v", event)
		case transcriptdomain.TranscriptEventStreamStatus:
			if event.StreamStatus == transcriptdomain.StreamStatusReady {
				hasReadyStatus = true
			}
		case transcriptdomain.TranscriptEventHeartbeat:
		default:
			t.Fatalf("unexpected transcript event kind from mixed noise stream: %#v", event)
		}
	}

	if !hasAssistantDelta {
		t.Fatalf("expected assistant delta event in mixed noise stream, got %#v", streamEvents)
	}
	if !hasApprovalPending {
		t.Fatalf("expected approval pending event in mixed noise stream, got %#v", streamEvents)
	}
	if !hasReadyStatus {
		t.Fatalf("expected ready status in mixed noise stream, got %#v", streamEvents)
	}
}

func collectTranscriptEvents(ch <-chan transcriptdomain.TranscriptEvent) []transcriptdomain.TranscriptEvent {
	events := make([]transcriptdomain.TranscriptEvent, 0, 8)
	for event := range ch {
		events = append(events, event)
	}
	return events
}
