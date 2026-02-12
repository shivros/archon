package daemon

import (
	"context"
	"testing"
	"time"

	"control/internal/types"
)

func TestSessionServiceDelegatesToRegisteredConversationAdapter(t *testing.T) {
	store := &stubSessionIndexStore{
		records: map[string]*types.SessionRecord{
			"s1": {
				Session: &types.Session{
					ID:        "s1",
					Provider:  "mock-provider",
					Cwd:       "/tmp/mock",
					Status:    types.SessionStatusRunning,
					CreatedAt: time.Now().UTC(),
				},
				Source: sessionSourceInternal,
			},
		},
	}
	adapter := &testConversationAdapter{
		provider:   "mock-provider",
		sendTurnID: "turn-123",
	}
	service := NewSessionService(nil, &Stores{Sessions: store}, nil, nil)
	service.adapters = newConversationAdapterRegistry(adapter)

	turnID, err := service.SendMessage(context.Background(), "s1", []map[string]any{
		{"type": "text", "text": "hello"},
	})
	if err != nil {
		t.Fatalf("expected send delegation success, got err=%v", err)
	}
	if turnID != "turn-123" {
		t.Fatalf("expected turn id turn-123, got %q", turnID)
	}
	if adapter.sendCalls != 1 {
		t.Fatalf("expected adapter send to be called once, got %d", adapter.sendCalls)
	}
}

func TestConversationAdapterContractHistoryRequiresSession(t *testing.T) {
	registry := newConversationAdapterRegistry()
	service := NewSessionService(nil, nil, nil, nil)

	for _, provider := range []string{"codex", "claude", "opencode", "kilocode"} {
		t.Run(provider, func(t *testing.T) {
			adapter := registry.adapterFor(provider)
			items, err := adapter.History(context.Background(), service, nil, nil, sessionSourceInternal, 10)
			if items != nil {
				t.Fatalf("expected nil items for invalid call, got %#v", items)
			}
			expectServiceErrorKind(t, err, ServiceErrorInvalid)
		})
	}
}

func TestConversationAdapterContractSendUnavailableWithoutRuntime(t *testing.T) {
	for _, provider := range []string{"codex", "claude", "opencode", "kilocode"} {
		t.Run(provider, func(t *testing.T) {
			store := &stubSessionIndexStore{
				records: map[string]*types.SessionRecord{
					"s1": {
						Session: &types.Session{
							ID:        "s1",
							Provider:  provider,
							Cwd:       "/tmp/adapter",
							Status:    types.SessionStatusRunning,
							CreatedAt: time.Now().UTC(),
						},
						Source: sessionSourceInternal,
					},
				},
			}
			service := NewSessionService(nil, &Stores{Sessions: store}, nil, nil)
			_, err := service.SendMessage(context.Background(), "s1", []map[string]any{
				{"type": "text", "text": "hello"},
			})
			expectServiceErrorKind(t, err, ServiceErrorUnavailable)
		})
	}
}

func expectServiceErrorKind(t *testing.T, err error, kind ServiceErrorKind) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected service error kind %s, got nil", kind)
	}
	serviceErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected *ServiceError, got %T (%v)", err, err)
	}
	if serviceErr.Kind != kind {
		t.Fatalf("expected service error kind %s, got %s (%v)", kind, serviceErr.Kind, err)
	}
}

type stubSessionIndexStore struct {
	records map[string]*types.SessionRecord
}

func (s *stubSessionIndexStore) ListRecords(context.Context) ([]*types.SessionRecord, error) {
	out := make([]*types.SessionRecord, 0, len(s.records))
	for _, record := range s.records {
		out = append(out, record)
	}
	return out, nil
}

func (s *stubSessionIndexStore) GetRecord(_ context.Context, sessionID string) (*types.SessionRecord, bool, error) {
	record, ok := s.records[sessionID]
	if !ok {
		return nil, false, nil
	}
	return record, true, nil
}

func (s *stubSessionIndexStore) UpsertRecord(_ context.Context, record *types.SessionRecord) (*types.SessionRecord, error) {
	if s.records == nil {
		s.records = map[string]*types.SessionRecord{}
	}
	if record != nil && record.Session != nil {
		s.records[record.Session.ID] = record
	}
	return record, nil
}

func (s *stubSessionIndexStore) DeleteRecord(_ context.Context, sessionID string) error {
	delete(s.records, sessionID)
	return nil
}

type testConversationAdapter struct {
	provider   string
	sendTurnID string
	sendCalls  int
}

func (a *testConversationAdapter) Provider() string {
	return a.provider
}

func (a *testConversationAdapter) History(context.Context, *SessionService, *types.Session, *types.SessionMeta, string, int) ([]map[string]any, error) {
	return []map[string]any{{"type": "log", "text": "ok"}}, nil
}

func (a *testConversationAdapter) SendMessage(_ context.Context, _ *SessionService, _ *types.Session, _ *types.SessionMeta, _ []map[string]any) (string, error) {
	a.sendCalls++
	return a.sendTurnID, nil
}

func (a *testConversationAdapter) SubscribeEvents(context.Context, *SessionService, *types.Session, *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	ch := make(chan types.CodexEvent)
	close(ch)
	return ch, func() {}, nil
}

func (a *testConversationAdapter) Approve(context.Context, *SessionService, *types.Session, *types.SessionMeta, int, string, []string, map[string]any) error {
	return nil
}

func (a *testConversationAdapter) Interrupt(context.Context, *SessionService, *types.Session, *types.SessionMeta) error {
	return nil
}
