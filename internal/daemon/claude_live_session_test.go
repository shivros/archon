package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		publisher.mu.Lock()
		calls := publisher.calls
		publisher.mu.Unlock()
		if calls == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected completion publish from background worker")
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

func TestClaudeLiveSessionSendReturnsQuicklyAndProcessesInBackground(t *testing.T) {
	transport := &blockingClaudeTransport{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	publisher := &stubClaudeTurnCompletionPublisher{}
	orchestrator := claudeSendOrchestrator{
		validator:           stubClaudeInputValidator{text: "hello"},
		transport:           transport,
		turnIDs:             stubTurnIDGenerator{id: "claude-turn-xyz"},
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

	start := time.Now()
	turnID, err := session.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "hello"}}, nil)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if turnID != "claude-turn-xyz" {
		t.Fatalf("unexpected turnID: %q", turnID)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("expected fast accepted response, elapsed=%v", elapsed)
	}

	select {
	case <-transport.started:
	case <-time.After(time.Second):
		t.Fatalf("background transport did not start")
	}

	publisher.mu.Lock()
	callsBeforeRelease := publisher.calls
	publisher.mu.Unlock()
	if callsBeforeRelease != 0 {
		t.Fatalf("expected no completion publish while transport blocked, got %d", callsBeforeRelease)
	}

	close(transport.release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		publisher.mu.Lock()
		calls := publisher.calls
		publisher.mu.Unlock()
		if calls == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected completion publish after background transport released")
}

func TestClaudeLiveSessionBackgroundFailurePublishesFailureAndClearsActiveTurn(t *testing.T) {
	notifier := &stubTurnCompletionNotifier{}
	repository := &recordingTurnArtifactRepository{}
	session := &claudeLiveSession{
		sessionID: "s1",
		session:   &types.Session{ID: "s1", Provider: "claude"},
		failureReporter: defaultClaudeTurnFailureReporter{
			sessionID:    "s1",
			providerName: "claude",
			repository:   repository,
			notifier:     notifier,
		},
		orchestrator: claudeSendOrchestrator{
			validator: stubClaudeInputValidator{text: "hello"},
			transport: stubClaudeTransport{err: errors.New("transport failed")},
			turnIDs:   stubTurnIDGenerator{id: "turn-1"},
		},
	}

	_, err := session.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "hello"}}, nil)
	if err != nil {
		t.Fatalf("expected accepted turn despite background failure, got %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		notifier.mu.Lock()
		calls := notifier.calls
		event := notifier.lastEvent
		notifier.mu.Unlock()
		if calls > 0 {
			if event.Status != "failed" {
				t.Fatalf("expected failure status, got %q", event.Status)
			}
			if event.TurnID != "turn-1" {
				t.Fatalf("expected turn id turn-1, got %q", event.TurnID)
			}
			if strings.TrimSpace(event.Error) == "" {
				t.Fatalf("expected error details in failure notification")
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	deadline = time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if session.ActiveTurnID() == "" {
			goto verifyArtifacts
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected active turn to clear after failure, got %q", session.ActiveTurnID())

verifyArtifacts:
	items := repository.Snapshot()
	if len(items) == 0 {
		t.Fatalf("expected failure item artifact")
	}
	if asString(items[0]["type"]) != "log" {
		t.Fatalf("expected log item, got %#v", items[0])
	}
	if asString(items[0]["turn_id"]) != "turn-1" {
		t.Fatalf("expected turn_id on failure item, got %#v", items[0]["turn_id"])
	}
}

func TestClaudeLiveSessionQueuesTurnsPerSession(t *testing.T) {
	transport := &queuedClaudeTransport{
		startedText: make(chan string, 2),
		allow:       make(chan struct{}, 2),
	}
	session := &claudeLiveSession{
		sessionID: "s1",
		session:   &types.Session{ID: "s1", Provider: "claude"},
		orchestrator: claudeSendOrchestrator{
			validator: stubClaudeInputValidator{text: "default"},
			transport: transport,
			turnIDs:   &sequenceTurnIDGenerator{ids: []string{"turn-1", "turn-2"}},
		},
	}

	// Use payload text to identify execution order.
	session.orchestrator.validator = inputTextValidator{}

	if _, err := session.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "one"}}, nil); err != nil {
		t.Fatalf("first StartTurn: %v", err)
	}
	if _, err := session.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "two"}}, nil); err != nil {
		t.Fatalf("second StartTurn: %v", err)
	}

	select {
	case got := <-transport.startedText:
		if got != "one" {
			t.Fatalf("expected first queued turn to start with input 'one', got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("first queued turn did not start")
	}

	select {
	case got := <-transport.startedText:
		t.Fatalf("unexpected concurrent second start before first completed: %q", got)
	case <-time.After(80 * time.Millisecond):
		// Expected: second turn remains queued.
	}

	transport.allow <- struct{}{}
	select {
	case got := <-transport.startedText:
		if got != "two" {
			t.Fatalf("expected second queued turn to start with input 'two', got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("second queued turn did not start after first completed")
	}
	transport.allow <- struct{}{}
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

func TestClaudeLiveSessionSessionIDAccessor(t *testing.T) {
	session := &claudeLiveSession{sessionID: "session-123"}
	if got := session.SessionID(); got != "session-123" {
		t.Fatalf("expected session id accessor to return session-123, got %q", got)
	}
}

func TestClaudeLiveSessionInterruptRequiresManager(t *testing.T) {
	session := &claudeLiveSession{
		sessionID: "s1",
	}
	err := session.Interrupt(context.Background())
	if err == nil {
		t.Fatalf("expected unavailable error")
	}
	expectServiceErrorKind(t, err, ServiceErrorUnavailable)
}

func TestClaudeLiveSessionInterruptDelegatesToManager(t *testing.T) {
	manager := &SessionManager{
		sessions: map[string]*sessionRuntime{
			"s1": {
				interrupt: func() error { return nil },
			},
		},
	}
	session := &claudeLiveSession{
		sessionID: "s1",
		manager:   manager,
	}
	if err := session.Interrupt(context.Background()); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
}

func TestClaudeLiveSessionStartTurnQueueFull(t *testing.T) {
	session := &claudeLiveSession{
		sessionID: "s1",
		session:   &types.Session{ID: "s1", Provider: "claude"},
		orchestrator: claudeSendOrchestrator{
			validator: stubClaudeInputValidator{text: "hello"},
			turnIDs:   stubTurnIDGenerator{id: "turn-overflow"},
		},
		scheduler: &stubClaudeTurnScheduler{enqueueErr: unavailableError("queue full", nil)},
	}
	_, err := session.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "hello"}}, nil)
	if err == nil {
		t.Fatalf("expected queue full error")
	}
	expectServiceErrorKind(t, err, ServiceErrorUnavailable)
}

func TestClaudeLiveSessionTransportSendCoverage(t *testing.T) {
	t.Run("nil_manager", func(t *testing.T) {
		transport := claudeLiveSessionTransport{}
		err := transport.Send(context.Background(), claudeSendContext{}, &types.Session{ID: "s1"}, nil, []byte("x"), nil)
		expectServiceErrorKind(t, err, ServiceErrorUnavailable)
	})

	t.Run("nil_session", func(t *testing.T) {
		transport := claudeLiveSessionTransport{manager: &SessionManager{}}
		err := transport.Send(context.Background(), claudeSendContext{}, nil, nil, []byte("x"), nil)
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
	})

	t.Run("direct_send_success", func(t *testing.T) {
		sessionID := "s-send-success"
		manager := &SessionManager{
			baseDir: t.TempDir(),
			sessions: map[string]*sessionRuntime{
				sessionID: {
					session: &types.Session{ID: sessionID, Provider: "claude"},
					send: func([]byte) error {
						return nil
					},
				},
			},
		}
		transport := claudeLiveSessionTransport{manager: manager}
		err := transport.Send(context.Background(), claudeSendContext{}, &types.Session{
			ID:       sessionID,
			Provider: "claude",
			Cwd:      t.TempDir(),
		}, nil, []byte("x"), nil)
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
	})
}

func TestClaudeLiveSessionStateStoreAndCompletionReader(t *testing.T) {
	t.Run("state_store_persists", func(t *testing.T) {
		metaStore := failingSessionMetaStore{}
		storeImpl := claudeLiveSessionStateStore{
			stores: &Stores{SessionMeta: &metaStore},
		}
		storeImpl.SaveTurnState(context.Background(), "s1", "turn-1")
		meta, ok, err := metaStore.Get(context.Background(), "s1")
		if err != nil {
			t.Fatalf("meta get: %v", err)
		}
		if !ok || meta == nil || meta.LastTurnID != "turn-1" {
			t.Fatalf("expected persisted turn metadata, got %#v", meta)
		}
	})

	t.Run("state_store_ignores_invalid", func(t *testing.T) {
		storeImpl := claudeLiveSessionStateStore{}
		storeImpl.SaveTurnState(context.Background(), "", "")
	})

	t.Run("completion_reader_nil_repo", func(t *testing.T) {
		reader := claudeLiveSessionCompletionReader{}
		items, err := reader.ReadSessionItems("s1", 10)
		if err != nil {
			t.Fatalf("ReadSessionItems: %v", err)
		}
		if items != nil {
			t.Fatalf("expected nil items for nil repo, got %#v", items)
		}
	})

	t.Run("completion_reader_delegates_repo", func(t *testing.T) {
		repo := &recordingTurnArtifactRepository{}
		_ = repo.AppendItems("s1", []map[string]any{{"type": "assistant", "text": "ok"}})
		reader := claudeLiveSessionCompletionReader{repository: repo}
		items, err := reader.ReadSessionItems("s1", 10)
		if err != nil {
			t.Fatalf("ReadSessionItems: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected delegated items, got %#v", items)
		}
	})
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
	mu        sync.Mutex
	calls     int
	lastEvent TurnCompletionEvent
}

type stubClaudeTurnScheduler struct {
	enqueueErr error
	enqueued   []claudeTurnJob
}

func (s *stubClaudeTurnScheduler) Enqueue(job claudeTurnJob) error {
	if s.enqueueErr != nil {
		return s.enqueueErr
	}
	s.enqueued = append(s.enqueued, job)
	return nil
}

func (s *stubClaudeTurnScheduler) Close() {}

func (s *stubTurnCompletionNotifier) NotifyTurnCompleted(_ context.Context, sessionID, turnID, provider string, meta *types.SessionMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.lastEvent = TurnCompletionEvent{
		SessionID: sessionID,
		TurnID:    turnID,
		Provider:  provider,
	}
}

func (s *stubTurnCompletionNotifier) NotifyTurnCompletedEvent(_ context.Context, event TurnCompletionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.lastEvent = event
}

type blockingClaudeTransport struct {
	started chan struct{}
	release chan struct{}
}

func (t *blockingClaudeTransport) Send(
	context.Context,
	claudeSendContext,
	*types.Session,
	*types.SessionMeta,
	[]byte,
	*types.SessionRuntimeOptions,
) error {
	select {
	case t.started <- struct{}{}:
	default:
	}
	<-t.release
	return nil
}

type recordingTurnArtifactRepository struct {
	mu    sync.Mutex
	items []map[string]any
}

func (r *recordingTurnArtifactRepository) ReadItems(string, int) ([]map[string]any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]map[string]any, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, cloneItemMap(item))
	}
	return out, nil
}

func (r *recordingTurnArtifactRepository) AppendItems(_ string, items []map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range items {
		r.items = append(r.items, cloneItemMap(item))
	}
	return nil
}

func (r *recordingTurnArtifactRepository) Snapshot() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]map[string]any, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, cloneItemMap(item))
	}
	return out
}

type queuedClaudeTransport struct {
	startedText chan string
	allow       chan struct{}
}

func (t *queuedClaudeTransport) Send(
	_ context.Context,
	_ claudeSendContext,
	_ *types.Session,
	_ *types.SessionMeta,
	payload []byte,
	_ *types.SessionRuntimeOptions,
) error {
	text, err := extractClaudeUserText(payload)
	if err != nil {
		return err
	}
	t.startedText <- strings.TrimSpace(text)
	<-t.allow
	return nil
}

type sequenceTurnIDGenerator struct {
	mu  sync.Mutex
	ids []string
}

func (g *sequenceTurnIDGenerator) NewTurnID(string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.ids) == 0 {
		return ""
	}
	id := g.ids[0]
	g.ids = g.ids[1:]
	return id
}

type inputTextValidator struct{}

func (inputTextValidator) TextFromInput(input []map[string]any) (string, error) {
	text := strings.TrimSpace(extractTextInput(input))
	if text == "" {
		return "", invalidError("text input is required", nil)
	}
	return text, nil
}

func TestClaudeLiveSessionFailurePublishesItemStreamSignal(t *testing.T) {
	manager := &SessionManager{
		sessions: map[string]*sessionRuntime{
			"s1": {
				itemsHub: newItemHub(),
			},
		},
	}
	baseDir := t.TempDir()
	repository := &fileSessionItemsRepository{
		baseDirResolver: func() (string, error) { return baseDir, nil },
		broadcastItems:  manager.BroadcastItems,
	}

	itemsCh, cancel, err := manager.SubscribeItems("s1")
	if err != nil {
		t.Fatalf("SubscribeItems: %v", err)
	}
	defer cancel()

	session := &claudeLiveSession{
		sessionID: "s1",
		session:   &types.Session{ID: "s1", Provider: "claude"},
		manager:   manager,
		failureReporter: defaultClaudeTurnFailureReporter{
			sessionID:    "s1",
			providerName: "claude",
			repository:   repository,
			notifier:     &stubTurnCompletionNotifier{},
			debugWriter:  manager,
		},
		orchestrator: claudeSendOrchestrator{
			validator: stubClaudeInputValidator{text: "hello"},
			transport: stubClaudeTransport{err: errors.New("boom")},
			turnIDs:   stubTurnIDGenerator{id: "turn-stream-fail"},
		},
	}

	if _, err := session.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "hello"}}, nil); err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	select {
	case item := <-itemsCh:
		if asString(item["type"]) != "log" {
			t.Fatalf("expected log item, got %#v", item)
		}
		if asString(item["turn_id"]) != "turn-stream-fail" {
			t.Fatalf("expected turn id in stream item, got %#v", item["turn_id"])
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected streamed failure item")
	}

	items, err := repository.ReadItems("s1", 20)
	if err != nil {
		t.Fatalf("ReadItems: %v", err)
	}
	if len(items) == 0 {
		t.Fatalf("expected persisted failure items")
	}
	if asString(items[0]["turn_id"]) != "turn-stream-fail" {
		t.Fatalf("expected persisted turn id, got %#v", items[0]["turn_id"])
	}
}
