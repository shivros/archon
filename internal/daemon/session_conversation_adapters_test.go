package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"control/internal/store"
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

func TestOpenCodeConversationAdapterApproveRepliesPermission(t *testing.T) {
	var (
		receivedPath string
		receivedBody map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		auth := r.Header.Get("Authorization")
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte("opencode:token-123"))
		if auth != want {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENCODE_BASE_URL", server.URL)
	t.Setenv("OPENCODE_TOKEN", "token-123")
	ctx := context.Background()
	base := t.TempDir()
	approvalStore := store.NewFileApprovalStore(filepath.Join(base, "approvals.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions.json"))

	session := &types.Session{
		ID:        "s-open",
		Provider:  "opencode",
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: session,
		Source:  sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("seed session record: %v", err)
	}
	params, _ := json.Marshal(map[string]any{"permission_id": "perm-123"})
	_, err = approvalStore.Upsert(ctx, &types.Approval{
		SessionID: session.ID,
		RequestID: 77,
		Method:    "tool/requestUserInput",
		Params:    params,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:  sessionStore,
		Approvals: approvalStore,
	}, nil, nil)

	if err := service.Approve(ctx, session.ID, 77, "accept", nil, nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if receivedPath != "/permission/perm-123/reply" {
		t.Fatalf("unexpected permission reply path: %q", receivedPath)
	}
	if decision := receivedBody["decision"]; decision != "accept" {
		t.Fatalf("unexpected approval decision payload: %#v", receivedBody)
	}
	_, ok, err := approvalStore.Get(ctx, session.ID, 77)
	if err != nil {
		t.Fatalf("get approval after delete: %v", err)
	}
	if ok {
		t.Fatalf("expected approval to be deleted after successful reply")
	}
}

func TestOpenCodeConversationAdapterSubscribeEventsSyncsApprovals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/event":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("expected http flusher")
			}
			send := func(payload map[string]any) {
				data, _ := json.Marshal(payload)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", string(data))
				flusher.Flush()
			}
			send(map[string]any{
				"type": "permission.updated",
				"properties": map[string]any{
					"id":        "perm-abc",
					"sessionID": "remote-s-1",
					"type":      "command",
					"title":     "run command",
					"metadata": map[string]any{
						"command": "echo one",
					},
				},
			})
			send(map[string]any{
				"type": "permission.replied",
				"properties": map[string]any{
					"permissionID": "perm-abc",
					"sessionID":    "remote-s-1",
					"response":     "once",
				},
			})
			send(map[string]any{
				"type": "session.idle",
				"properties": map[string]any{
					"sessionID": "remote-s-1",
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENCODE_BASE_URL", server.URL)
	ctx := context.Background()
	base := t.TempDir()
	approvalStore := store.NewFileApprovalStore(filepath.Join(base, "approvals.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

	session := &types.Session{
		ID:        "s-open-events",
		Provider:  "opencode",
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: session,
		Source:  sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("seed session record: %v", err)
	}
	_, err = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:         session.ID,
		ProviderSessionID: "remote-s-1",
	})
	if err != nil {
		t.Fatalf("seed session meta: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
		Approvals:   approvalStore,
	}, nil, nil)

	streamCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events, closeFn, err := service.SubscribeEvents(streamCtx, session.ID)
	if err != nil {
		t.Fatalf("SubscribeEvents: %v", err)
	}
	defer closeFn()

	receivedApproval := false
	receivedResolved := false
	for i := 0; i < 3; i++ {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatalf("stream closed early")
			}
			switch event.Method {
			case "item/commandExecution/requestApproval":
				receivedApproval = true
			case "permission/replied":
				receivedResolved = true
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for mapped events")
		}
	}
	if !receivedApproval || !receivedResolved {
		t.Fatalf("expected approval lifecycle events, got approval=%v resolved=%v", receivedApproval, receivedResolved)
	}

	approvals, err := approvalStore.ListBySession(ctx, session.ID)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(approvals) != 0 {
		t.Fatalf("expected approval store to be reconciled, got %#v", approvals)
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
