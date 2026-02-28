package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"control/internal/store"
	"control/internal/types"
)

type stubOrchestratorTurnCompletionStrategy struct {
	shouldPublish bool
	source        string
}

func (s stubOrchestratorTurnCompletionStrategy) ShouldPublishCompletion(int, []map[string]any) bool {
	return s.shouldPublish
}

func (s stubOrchestratorTurnCompletionStrategy) Source() string {
	if s.source == "" {
		return "stub_source"
	}
	return s.source
}

type stubClaudeCompletionReader struct {
	items []map[string]any
	err   error
}

func (s stubClaudeCompletionReader) ReadSessionItems(string, int) ([]map[string]any, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.items, nil
}

type stubClaudeTransport struct {
	err error
}

func (s stubClaudeTransport) Send(context.Context, claudeSendContext, *types.Session, *types.SessionMeta, []byte, *types.SessionRuntimeOptions) error {
	return s.err
}

type stubClaudeTurnStateStore struct {
	savedSessionID string
	savedTurnID    string
}

func (s *stubClaudeTurnStateStore) SaveTurnState(_ context.Context, sessionID, turnID string) {
	s.savedSessionID = sessionID
	s.savedTurnID = turnID
}

type stubClaudeTurnCompletionPublisher struct {
	mu     sync.Mutex
	calls  int
	turnID string
	source string
}

func (s *stubClaudeTurnCompletionPublisher) PublishTurnCompleted(_ *types.Session, _ *types.SessionMeta, turnID, source string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.turnID = turnID
	s.source = source
}

type stubClaudeInputValidator struct {
	text string
	err  error
}

func (s stubClaudeInputValidator) TextFromInput([]map[string]any) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.text, nil
}

type stubTurnIDGenerator struct {
	id string
}

func (s stubTurnIDGenerator) NewTurnID(string) string {
	return s.id
}

func TestDefaultClaudeCompletionDecisionPolicyUsesStrategySource(t *testing.T) {
	policy := defaultClaudeCompletionDecisionPolicy{strategy: stubOrchestratorTurnCompletionStrategy{
		shouldPublish: true,
		source:        "strategy_source",
	}}
	publish, source := policy.Decide(0, []map[string]any{{"type": "agentMessage"}}, nil)
	if !publish {
		t.Fatalf("expected publish=true")
	}
	if source != "strategy_source" {
		t.Fatalf("expected strategy source, got %q", source)
	}
}

func TestDefaultClaudeCompletionDecisionPolicyFallsBackToSyncSource(t *testing.T) {
	policy := defaultClaudeCompletionDecisionPolicy{strategy: stubOrchestratorTurnCompletionStrategy{shouldPublish: false}}
	publish, source := policy.Decide(0, nil, nil)
	if !publish {
		t.Fatalf("expected publish=true")
	}
	if source != "claude_sync_send_completed" {
		t.Fatalf("expected sync completion source, got %q", source)
	}
}

func TestDefaultClaudeCompletionDecisionPolicySuppressesOnSendError(t *testing.T) {
	policy := defaultClaudeCompletionDecisionPolicy{strategy: stubOrchestratorTurnCompletionStrategy{shouldPublish: true}}
	publish, source := policy.Decide(0, nil, errors.New("boom"))
	if publish {
		t.Fatalf("expected publish=false on send error")
	}
	if source != "" {
		t.Fatalf("expected empty source on send error, got %q", source)
	}
}

func TestReadClaudeCompletionItemsHandlesNilReaderAndErrors(t *testing.T) {
	items, err := readClaudeCompletionItems(nil, "s1")
	if err != nil {
		t.Fatalf("expected nil error for nil reader, got %v", err)
	}
	if items != nil {
		t.Fatalf("expected nil items for nil reader, got %#v", items)
	}
	_, err = readClaudeCompletionItems(stubClaudeCompletionReader{err: errors.New("read failed")}, "s1")
	if err == nil {
		t.Fatalf("expected read error")
	}
}

func TestDefaultTurnIDGeneratorUsesProviderPrefix(t *testing.T) {
	gen := defaultTurnIDGenerator{}
	id := gen.NewTurnID("claude")
	if len(id) == 0 {
		t.Fatalf("expected non-empty turn id")
	}
	if id[:12] != "claude-turn-" {
		t.Fatalf("expected claude prefix, got %q", id)
	}
}

func TestDefaultTurnIDGeneratorFallsBackForEmptyProvider(t *testing.T) {
	gen := defaultTurnIDGenerator{}
	id := gen.NewTurnID("   ")
	if len(id) == 0 {
		t.Fatalf("expected non-empty turn id")
	}
	if !strings.HasPrefix(id, "provider-turn-") {
		t.Fatalf("expected provider prefix fallback, got %q", id)
	}
}

func TestDefaultClaudeInputValidatorSuccess(t *testing.T) {
	v := defaultClaudeInputValidator{}
	text, err := v.TextFromInput([]map[string]any{{"type": "text", "text": " hello "}})
	if err != nil {
		t.Fatalf("TextFromInput: %v", err)
	}
	if strings.TrimSpace(text) != "hello" {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestClaudeSendOrchestratorSendUsesDefaultPolicyWhenNil(t *testing.T) {
	state := &stubClaudeTurnStateStore{}
	pub := &stubClaudeTurnCompletionPublisher{}
	o := claudeSendOrchestrator{
		validator:           stubClaudeInputValidator{text: "hello"},
		transport:           stubClaudeTransport{},
		turnIDs:             stubTurnIDGenerator{id: "claude-turn-1"},
		stateStore:          state,
		completionReader:    stubClaudeCompletionReader{items: []map[string]any{{"type": "agentMessage"}}, err: nil},
		completionPublisher: pub,
	}
	sendCtx := claudeSendContext{Manager: &SessionManager{}}
	session := &types.Session{ID: "s1", Provider: "claude"}
	turnID, err := o.Send(context.Background(), sendCtx, session, nil, []map[string]any{{"type": "text", "text": "hi"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if turnID != "claude-turn-1" {
		t.Fatalf("unexpected turn id: %q", turnID)
	}
	if state.savedSessionID != "s1" || state.savedTurnID != "claude-turn-1" {
		t.Fatalf("unexpected saved state: session=%q turn=%q", state.savedSessionID, state.savedTurnID)
	}
	if pub.calls != 1 || pub.turnID != "claude-turn-1" || pub.source != "claude_sync_send_completed" {
		t.Fatalf("unexpected publish: calls=%d turn=%q source=%q", pub.calls, pub.turnID, pub.source)
	}
}

func TestClaudeSendOrchestratorSendValidatorError(t *testing.T) {
	o := claudeSendOrchestrator{
		validator: stubClaudeInputValidator{err: errors.New("bad input")},
	}
	sendCtx := claudeSendContext{Manager: &SessionManager{}}
	session := &types.Session{ID: "s1", Provider: "claude"}
	_, err := o.Send(context.Background(), sendCtx, session, nil, []map[string]any{{"type": "text", "text": "hi"}})
	if err == nil || !strings.Contains(err.Error(), "bad input") {
		t.Fatalf("expected validator error, got %v", err)
	}
}

func TestClaudeSendOrchestratorSendRejectsEmptyTurnID(t *testing.T) {
	o := claudeSendOrchestrator{
		validator: stubClaudeInputValidator{text: "hello"},
		transport: stubClaudeTransport{},
		turnIDs:   stubTurnIDGenerator{id: "   "},
	}
	sendCtx := claudeSendContext{Manager: &SessionManager{}}
	session := &types.Session{ID: "s1", Provider: "claude"}
	_, err := o.Send(context.Background(), sendCtx, session, nil, []map[string]any{{"type": "text", "text": "hi"}})
	expectServiceErrorKind(t, err, ServiceErrorUnavailable)
}

func TestClaudeSendOrchestratorSendPropagatesTransportError(t *testing.T) {
	o := claudeSendOrchestrator{
		validator: stubClaudeInputValidator{text: "hello"},
		transport: stubClaudeTransport{err: errors.New("send failed")},
		turnIDs:   stubTurnIDGenerator{id: "claude-turn-1"},
	}
	sendCtx := claudeSendContext{Manager: &SessionManager{}}
	session := &types.Session{ID: "s1", Provider: "claude"}
	_, err := o.Send(context.Background(), sendCtx, session, nil, []map[string]any{{"type": "text", "text": "hi"}})
	if err == nil || !strings.Contains(err.Error(), "send failed") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestClaudeSendOrchestratorSendNoPublisherStillSucceeds(t *testing.T) {
	o := claudeSendOrchestrator{
		validator:        stubClaudeInputValidator{text: "hello"},
		transport:        stubClaudeTransport{},
		turnIDs:          stubTurnIDGenerator{id: "claude-turn-1"},
		completionReader: stubClaudeCompletionReader{items: []map[string]any{{"type": "agentMessage"}}, err: nil},
	}
	sendCtx := claudeSendContext{Manager: &SessionManager{}}
	session := &types.Session{ID: "s1", Provider: "claude"}
	turnID, err := o.Send(context.Background(), sendCtx, session, nil, []map[string]any{{"type": "text", "text": "hi"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if turnID != "claude-turn-1" {
		t.Fatalf("unexpected turn id: %q", turnID)
	}
}

func TestClaudeSendOrchestratorSendToleratesCompletionReadError(t *testing.T) {
	pub := &stubClaudeTurnCompletionPublisher{}
	o := claudeSendOrchestrator{
		validator:           stubClaudeInputValidator{text: "hello"},
		transport:           stubClaudeTransport{},
		turnIDs:             stubTurnIDGenerator{id: "claude-turn-1"},
		completionReader:    stubClaudeCompletionReader{err: errors.New("read failed")},
		completionPublisher: pub,
	}
	sendCtx := claudeSendContext{Manager: &SessionManager{}}
	session := &types.Session{ID: "s1", Provider: "claude"}
	_, err := o.Send(context.Background(), sendCtx, session, nil, []map[string]any{{"type": "text", "text": "hi"}})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if pub.calls != 1 || pub.source != "claude_sync_send_completed" {
		t.Fatalf("expected sync fallback publish, got calls=%d source=%q", pub.calls, pub.source)
	}
}

func TestDefaultClaudeSendTransportBranches(t *testing.T) {
	transport := defaultClaudeSendTransport{}
	t.Run("nil_manager", func(t *testing.T) {
		err := transport.Send(context.Background(), claudeSendContext{}, &types.Session{ID: "s1"}, nil, []byte("x"), nil)
		expectServiceErrorKind(t, err, ServiceErrorUnavailable)
	})
	t.Run("nil_session", func(t *testing.T) {
		err := transport.Send(context.Background(), claudeSendContext{Manager: &SessionManager{}}, nil, nil, []byte("x"), nil)
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
	})
	t.Run("send_non_session_not_found", func(t *testing.T) {
		sessionID := "s-send-fail"
		manager := &SessionManager{
			baseDir: t.TempDir(),
			sessions: map[string]*sessionRuntime{
				sessionID: {
					session: &types.Session{ID: sessionID, Provider: "claude"},
					send: func([]byte) error {
						return errors.New("network down")
					},
				},
			},
		}
		sendCtx := claudeSendContext{Manager: manager}
		err := transport.Send(context.Background(), sendCtx, &types.Session{
			ID:       sessionID,
			Provider: "claude",
			Cwd:      t.TempDir(),
		}, nil, []byte("x"), nil)
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
		if !strings.Contains(err.Error(), "network down") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("resume_then_second_send_failure", func(t *testing.T) {
		sessionID := "s-retry-send-fail"
		sendCalls := 0
		manager := &SessionManager{
			baseDir: t.TempDir(),
			sessions: map[string]*sessionRuntime{
				sessionID: {
					session: &types.Session{ID: sessionID, Provider: "claude"},
					send: func([]byte) error {
						sendCalls++
						if sendCalls == 1 {
							return ErrSessionNotFound
						}
						return errors.New("second send failed")
					},
				},
			},
		}
		sendCtx := claudeSendContext{Manager: manager}
		err := transport.Send(context.Background(), sendCtx, &types.Session{
			ID:       sessionID,
			Provider: "claude",
			Cwd:      t.TempDir(),
		}, &types.SessionMeta{
			SessionID:         sessionID,
			ProviderSessionID: "provider-sess-1",
		}, []byte("x"), nil)
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
	})
	t.Run("resume_then_second_send_success", func(t *testing.T) {
		sessionID := "s-retry-send-success"
		sendCalls := 0
		manager := &SessionManager{
			baseDir: t.TempDir(),
			sessions: map[string]*sessionRuntime{
				sessionID: {
					session: &types.Session{ID: sessionID, Provider: "claude"},
					send: func([]byte) error {
						sendCalls++
						if sendCalls == 1 {
							return ErrSessionNotFound
						}
						return nil
					},
				},
			},
		}
		sendCtx := claudeSendContext{Manager: manager}
		err := transport.Send(context.Background(), sendCtx, &types.Session{
			ID:       sessionID,
			Provider: "claude",
			Cwd:      t.TempDir(),
		}, &types.SessionMeta{
			SessionID:         sessionID,
			ProviderSessionID: "provider-sess-1",
		}, []byte("x"), nil)
		if err != nil {
			t.Fatalf("Send: %v", err)
		}
		if sendCalls != 2 {
			t.Fatalf("expected 2 send calls, got %d", sendCalls)
		}
	})
}

func TestSessionServiceClaudeTurnStateStoreSaveTurnStateGuards(t *testing.T) {
	s := sessionServiceClaudeTurnStateStore{}
	s.SaveTurnState(context.Background(), "s1", "t1")
	s = sessionServiceClaudeTurnStateStore{service: &SessionService{}}
	s.SaveTurnState(context.Background(), "s1", "t1")
}

func TestSessionServiceClaudeTurnStateStoreSaveTurnStatePersists(t *testing.T) {
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json"))
	service := &SessionService{stores: &Stores{SessionMeta: metaStore}}
	s := sessionServiceClaudeTurnStateStore{service: service}
	s.SaveTurnState(context.Background(), "s-turn", "turn-1")
	meta, ok, err := metaStore.Get(context.Background(), "s-turn")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok || meta == nil {
		t.Fatalf("expected persisted meta")
	}
	if meta.LastTurnID != "turn-1" {
		t.Fatalf("expected last turn id persisted, got %q", meta.LastTurnID)
	}
	if meta.LastActiveAt == nil || meta.LastActiveAt.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("expected last active timestamp persisted, got %#v", meta.LastActiveAt)
	}
}

func TestSessionServiceClaudeTurnStateStoreSaveTurnStateEmptyIDsNoOp(t *testing.T) {
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json"))
	service := &SessionService{stores: &Stores{SessionMeta: metaStore}}
	s := sessionServiceClaudeTurnStateStore{service: service}
	s.SaveTurnState(context.Background(), "   ", "turn-1")
	s.SaveTurnState(context.Background(), "s-empty-turn", "   ")
	meta, ok, err := metaStore.Get(context.Background(), "s-empty-turn")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok || meta != nil {
		t.Fatalf("expected empty turn id to no-op, got meta=%#v", meta)
	}
}
