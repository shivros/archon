package daemon

import (
	"context"
	"testing"

	"control/internal/types"
)

func TestClaudeLiveSessionStartTurnReturnsTurnID(t *testing.T) {
	publisher := &stubClaudeTurnCompletionPublisher{}
	orchestrator := claudeSendOrchestrator{
		validator:           stubClaudeInputValidator{text: "hello"},
		transport:           stubClaudeTransport{},
		turnIDs:             stubTurnIDGenerator{id: "claude-turn-abc"},
		stateStore:          &stubClaudeTurnStateStore{},
		completionReader:    stubClaudeCompletionReader{},
		completionPublisher: publisher,
		completionPolicy:    defaultClaudeCompletionDecisionPolicy{strategy: claudeItemDeltaCompletionStrategy{}},
	}
	session := &claudeLiveSession{
		sessionID:    "s1",
		session:      &types.Session{ID: "s1", Provider: "claude"},
		orchestrator: orchestrator,
	}

	turnID, err := session.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "hello"}}, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if turnID != "claude-turn-abc" {
		t.Fatalf("expected turnID claude-turn-abc, got %q", turnID)
	}
	if session.ActiveTurnID() != "claude-turn-abc" {
		t.Fatalf("expected ActiveTurnID claude-turn-abc, got %q", session.ActiveTurnID())
	}
	if publisher.calls != 1 {
		t.Fatalf("expected 1 completion publish, got %d", publisher.calls)
	}
}

func TestClaudeLiveSessionStartTurnOnClosedSession(t *testing.T) {
	session := &claudeLiveSession{
		sessionID: "s1",
		session:   &types.Session{ID: "s1", Provider: "claude"},
		closed:    true,
	}

	_, err := session.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "hello"}}, nil)
	if err == nil {
		t.Fatal("expected error for closed session")
	}
}

func TestClaudeLiveSessionStartTurnTransportError(t *testing.T) {
	orchestrator := claudeSendOrchestrator{
		validator:        stubClaudeInputValidator{text: "hello"},
		transport:        stubClaudeTransport{err: unavailableError("fail", nil)},
		turnIDs:          stubTurnIDGenerator{id: "claude-turn-xyz"},
		stateStore:       &stubClaudeTurnStateStore{},
		completionReader: stubClaudeCompletionReader{},
	}
	session := &claudeLiveSession{
		sessionID:    "s1",
		session:      &types.Session{ID: "s1", Provider: "claude"},
		orchestrator: orchestrator,
	}

	_, err := session.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "hello"}}, nil)
	if err == nil {
		t.Fatal("expected error from transport failure")
	}
	if session.ActiveTurnID() != "" {
		t.Fatalf("expected empty ActiveTurnID after error, got %q", session.ActiveTurnID())
	}
}

func TestClaudeLiveSessionActiveTurnIDTracksLastTurn(t *testing.T) {
	publisher := &stubClaudeTurnCompletionPublisher{}
	stateStore := &stubClaudeTurnStateStore{}
	session := &claudeLiveSession{
		sessionID: "s1",
		session:   &types.Session{ID: "s1", Provider: "claude"},
		orchestrator: claudeSendOrchestrator{
			validator:           stubClaudeInputValidator{text: "hello"},
			transport:           stubClaudeTransport{},
			turnIDs:             stubTurnIDGenerator{id: "turn-1"},
			stateStore:          stateStore,
			completionReader:    stubClaudeCompletionReader{},
			completionPublisher: publisher,
			completionPolicy:    defaultClaudeCompletionDecisionPolicy{strategy: claudeItemDeltaCompletionStrategy{}},
		},
	}

	if session.ActiveTurnID() != "" {
		t.Fatalf("expected empty initial ActiveTurnID, got %q", session.ActiveTurnID())
	}

	_, err := session.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "hello"}}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.ActiveTurnID() != "turn-1" {
		t.Fatalf("expected ActiveTurnID turn-1, got %q", session.ActiveTurnID())
	}
}

func TestClaudeLiveSessionEventsReturnsClosed(t *testing.T) {
	session := &claudeLiveSession{
		sessionID: "s1",
		session:   &types.Session{ID: "s1", Provider: "claude"},
	}

	ch, cancel := session.Events()
	defer cancel()

	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed")
	}
}

func TestClaudeLiveSessionClose(t *testing.T) {
	session := &claudeLiveSession{
		sessionID: "s1",
		session:   &types.Session{ID: "s1", Provider: "claude"},
	}

	if session.IsClosed() {
		t.Fatal("expected session to not be closed initially")
	}
	session.Close()
	if !session.IsClosed() {
		t.Fatal("expected session to be closed after Close()")
	}
}

func TestClaudeLiveSessionSetSessionMeta(t *testing.T) {
	session := &claudeLiveSession{
		sessionID: "s1",
		session:   &types.Session{ID: "s1", Provider: "claude"},
	}

	meta := &types.SessionMeta{SessionID: "s1", WorkspaceID: "ws1"}
	session.SetSessionMeta(meta)

	session.mu.Lock()
	got := session.meta
	session.mu.Unlock()

	if got == nil || got.WorkspaceID != "ws1" {
		t.Fatalf("expected meta with WorkspaceID ws1, got %+v", got)
	}
	if got == meta {
		t.Fatal("expected meta to be cloned, not the same pointer")
	}
}

func TestClaudeLiveSessionFactoryNilSession(t *testing.T) {
	factory := newClaudeLiveSessionFactory(nil, nil, nil, nil, nil)
	_, err := factory.CreateTurnCapable(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil session")
	}
}

func TestClaudeLiveSessionFactoryNilManager(t *testing.T) {
	factory := newClaudeLiveSessionFactory(nil, nil, nil, nil, nil)
	_, err := factory.CreateTurnCapable(context.Background(), &types.Session{ID: "s1"}, nil)
	if err == nil {
		t.Fatal("expected error for nil manager")
	}
}

func TestClaudeLiveSessionFactoryProviderName(t *testing.T) {
	factory := newClaudeLiveSessionFactory(nil, nil, nil, nil, nil)
	if factory.ProviderName() != "claude" {
		t.Fatalf("expected provider name 'claude', got %q", factory.ProviderName())
	}
}

func TestClaudeLiveSessionCompletionPublisher(t *testing.T) {
	notifier := &stubTurnCompletionNotifier{}
	publisher := claudeLiveSessionCompletionPublisher{notifier: notifier}

	session := &types.Session{ID: "s1", Provider: "claude"}
	meta := &types.SessionMeta{SessionID: "s1"}
	publisher.PublishTurnCompleted(session, meta, "turn-1", "test_source")

	if notifier.calls != 1 {
		t.Fatalf("expected 1 notification, got %d", notifier.calls)
	}
	if notifier.lastEvent.SessionID != "s1" {
		t.Fatalf("expected sessionID s1, got %q", notifier.lastEvent.SessionID)
	}
	if notifier.lastEvent.TurnID != "turn-1" {
		t.Fatalf("expected turnID turn-1, got %q", notifier.lastEvent.TurnID)
	}
	if notifier.lastEvent.Source != "test_source" {
		t.Fatalf("expected source test_source, got %q", notifier.lastEvent.Source)
	}
}

func TestClaudeLiveSessionCompletionPublisherNilNotifier(t *testing.T) {
	publisher := claudeLiveSessionCompletionPublisher{}
	publisher.PublishTurnCompleted(&types.Session{ID: "s1"}, nil, "turn-1", "src")
	// Should not panic
}

func TestClaudeLiveSessionCompletionPublisherNilSession(t *testing.T) {
	notifier := &stubTurnCompletionNotifier{}
	publisher := claudeLiveSessionCompletionPublisher{notifier: notifier}
	publisher.PublishTurnCompleted(nil, nil, "turn-1", "src")
	if notifier.calls != 0 {
		t.Fatalf("expected 0 notifications for nil session, got %d", notifier.calls)
	}
}

func TestClaudeLiveSessionCompletionPublisherDeferredWiring(t *testing.T) {
	// Reproduces the daemon.go initialization order: turnNotifier is created
	// with nil publisher, used in a claudeLiveSessionCompletionPublisher, then
	// receives the real publisher via SetNotificationPublisher after construction.
	turnNotifier := NewTurnCompletionNotifier(nil, nil)
	publisher := claudeLiveSessionCompletionPublisher{notifier: turnNotifier}

	// Before wiring: publish should be silently dropped (notifier.notifier == nil).
	session := &types.Session{ID: "s1", Provider: "claude"}
	publisher.PublishTurnCompleted(session, nil, "turn-dropped", "test_source")

	// Wire the real notification publisher.
	capture := &capturingNotificationPublisher{}
	turnNotifier.SetNotificationPublisher(capture)

	// After wiring: publish should flow through to the notification publisher.
	publisher.PublishTurnCompleted(session, nil, "turn-delivered", "test_source")

	if len(capture.events) != 1 {
		t.Fatalf("expected 1 captured event after wiring, got %d", len(capture.events))
	}
	if capture.events[0].TurnID != "turn-delivered" {
		t.Fatalf("expected turnID turn-delivered, got %q", capture.events[0].TurnID)
	}
	if capture.events[0].SessionID != "s1" {
		t.Fatalf("expected sessionID s1, got %q", capture.events[0].SessionID)
	}
}

type capturingNotificationPublisher struct {
	events []types.NotificationEvent
}

func (c *capturingNotificationPublisher) Publish(event types.NotificationEvent) {
	c.events = append(c.events, event)
}

func (c *capturingNotificationPublisher) Close() {}

type stubTurnCompletionNotifier struct {
	calls     int
	lastEvent TurnCompletionEvent
}

func (s *stubTurnCompletionNotifier) NotifyTurnCompleted(_ context.Context, sessionID, turnID, provider string, meta *types.SessionMeta) {
	s.calls++
	s.lastEvent = TurnCompletionEvent{
		SessionID: sessionID,
		TurnID:    turnID,
		Provider:  provider,
	}
}

func (s *stubTurnCompletionNotifier) NotifyTurnCompletedEvent(_ context.Context, event TurnCompletionEvent) {
	s.calls++
	s.lastEvent = event
}
