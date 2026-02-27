package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"control/internal/types"
)

func TestOpenCodeLiveSessionStartTurnAcceptsPromptPending(t *testing.T) {
	const providerSessionID = "remote-live-pending"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/"+providerSessionID+"/message" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case http.MethodPost:
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != "/tmp/live-pending" {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			// Exceed client timeout so prompt service returns errOpenCodePromptPending.
			time.Sleep(80 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{
		BaseURL:  server.URL,
		Username: "opencode",
		Timeout:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}

	ls := &openCodeLiveSession{
		sessionID:    "s-live-pending",
		providerName: "opencode",
		providerID:   providerSessionID,
		directory:    "/tmp/live-pending",
		client:       client,
	}

	turnID, err := ls.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "hello"}}, nil)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if strings.TrimSpace(turnID) == "" {
		t.Fatalf("expected non-empty turn id")
	}
	if got := strings.TrimSpace(ls.ActiveTurnID()); got != strings.TrimSpace(turnID) {
		t.Fatalf("expected active turn %q, got %q", turnID, got)
	}
}

func TestOpenCodeLiveSessionPublishesTurnFailurePayload(t *testing.T) {
	eventStream := make(chan types.CodexEvent, 1)
	notifier := &captureOpenCodeNotificationPublisher{}
	ls := &openCodeLiveSession{
		sessionID:    "sess-open-failure",
		providerName: "opencode",
		events:       eventStream,
		hub:          newCodexSubscriberHub(),
		turnNotifier: NewTurnCompletionNotifier(notifier, nil),
	}
	ls.start()
	eventStream <- types.CodexEvent{
		Method: "turn/completed",
		Params: json.RawMessage(`{"turn":{"id":"turn-1","status":"failed","error":{"message":"unsupported model"}}}`),
	}
	close(eventStream)

	deadline := time.Now().Add(250 * time.Millisecond)
	for notifier.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	notifications := notifier.Events()
	if len(notifications) != 1 {
		t.Fatalf("expected one turn completion notification, got %d", len(notifications))
	}
	event := notifications[0]
	if event.Trigger != types.NotificationTriggerTurnCompleted {
		t.Fatalf("unexpected trigger: %q", event.Trigger)
	}
	if got := strings.TrimSpace(asString(event.Payload["turn_status"])); got != "failed" {
		t.Fatalf("expected turn_status=failed, got %q", got)
	}
	if got := strings.TrimSpace(asString(event.Payload["turn_error"])); got != "unsupported model" {
		t.Fatalf("expected turn_error payload, got %q", got)
	}
}

func TestOpenCodeLiveSessionSetNotificationPublisherUsesNotifierInterface(t *testing.T) {
	aware := &stubAwareTurnCompletionNotifier{}
	ls := &openCodeLiveSession{
		sessionID:    "sess-open-aware",
		providerName: "opencode",
		turnNotifier: aware,
	}
	publisher := &captureOpenCodeNotificationPublisher{}
	ls.SetNotificationPublisher(publisher)
	if aware.setCalls != 1 {
		t.Fatalf("expected SetNotificationPublisher to be delegated once, got %d", aware.setCalls)
	}
	if aware.publisher != publisher {
		t.Fatalf("expected delegated publisher to be stored on notifier")
	}
}

func TestOpenCodeLiveSessionPublishTurnCompletedNoNotifierNoPanic(t *testing.T) {
	ls := &openCodeLiveSession{
		sessionID:    "sess-open-nil",
		providerName: "opencode",
		turnNotifier: nil,
	}
	ls.publishTurnCompleted(turnEventParams{
		TurnID: "turn-nil",
		Status: "failed",
		Error:  "ignored",
	})
}

func TestOpenCodeLiveSessionStoreApprovalPersists(t *testing.T) {
	approvalID := 42
	store := &captureApprovalStorage{}
	ls := &openCodeLiveSession{
		sessionID:     "sess-open-approval",
		approvalStore: store,
	}
	ls.storeApproval(types.CodexEvent{
		ID:     &approvalID,
		Method: "item/commandExecution/requestApproval",
		Params: json.RawMessage(`{"permission_id":"perm-1"}`),
	})
	if store.called != 1 {
		t.Fatalf("expected approval store call, got %d", store.called)
	}
	if store.sessionID != "sess-open-approval" || store.requestID != approvalID {
		t.Fatalf("unexpected approval store call args: %#v", store)
	}
}

func TestOpenCodeLiveSessionBasicAccessorsAndInterrupt(t *testing.T) {
	ls := &openCodeLiveSession{
		sessionID: "sess-open-basic",
		client:    &openCodeClient{},
		hub:       newCodexSubscriberHub(),
	}
	if got := ls.SessionID(); got != "sess-open-basic" {
		t.Fatalf("expected session id accessor, got %q", got)
	}
	if ls.isClosed() {
		t.Fatalf("expected session to start open")
	}
	ch, cancel := ls.Events()
	if ch == nil || cancel == nil {
		t.Fatalf("expected events subscription handles")
	}
	cancel()
	if err := ls.Interrupt(context.Background()); err == nil {
		t.Fatalf("expected interrupt to delegate and fail when session service missing")
	}
	ls.Close()
	if !ls.isClosed() {
		t.Fatalf("expected session to be closed after Close()")
	}
}

func TestOpenCodeLiveSessionPublishTurnCompletedIncludesArtifactPayload(t *testing.T) {
	notifier := &captureOpenCodeNotificationPublisher{}
	ls := &openCodeLiveSession{
		sessionID:    "sess-open-artifacts",
		providerName: "opencode",
		turnNotifier: NewTurnCompletionNotifier(notifier, nil),
		artifactSync: stubTurnArtifactSynchronizer{
			result: TurnArtifactSyncResult{
				Output:                 "assistant output",
				ArtifactsPersisted:     true,
				AssistantArtifactCount: 2,
				Source:                 "test_sync",
			},
		},
	}

	ls.publishTurnCompleted(turnEventParams{
		TurnID: "turn-artifacts",
		Status: "completed",
	})
	notifications := notifier.Events()
	if len(notifications) != 1 {
		t.Fatalf("expected one turn completion notification, got %d", len(notifications))
	}
	event := notifications[0]
	if got := strings.TrimSpace(asString(event.Payload["turn_output"])); got != "assistant output" {
		t.Fatalf("expected turn_output payload, got %q", got)
	}
	if got := strings.TrimSpace(asString(event.Payload["artifact_sync_source"])); got != "test_sync" {
		t.Fatalf("expected artifact_sync_source payload, got %q", got)
	}
	if persisted, _ := event.Payload["artifacts_persisted"].(bool); !persisted {
		t.Fatalf("expected artifacts_persisted=true, got %#v", event.Payload["artifacts_persisted"])
	}
	if count, _ := asInt(event.Payload["assistant_artifact_count"]); count != 2 {
		t.Fatalf("expected assistant_artifact_count=2, got %#v", event.Payload["assistant_artifact_count"])
	}
}

type captureOpenCodeNotificationPublisher struct {
	mu     sync.Mutex
	events []types.NotificationEvent
}

func (p *captureOpenCodeNotificationPublisher) Publish(event types.NotificationEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
}

func (p *captureOpenCodeNotificationPublisher) Len() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

func (p *captureOpenCodeNotificationPublisher) Events() []types.NotificationEvent {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]types.NotificationEvent, len(p.events))
	copy(out, p.events)
	return out
}

type stubAwareTurnCompletionNotifier struct {
	publisher NotificationPublisher
	setCalls  int
}

type stubTurnArtifactSynchronizer struct {
	result TurnArtifactSyncResult
}

func (s stubTurnArtifactSynchronizer) SyncTurnArtifacts(_ context.Context, turn turnEventParams) TurnArtifactSyncResult {
	if strings.TrimSpace(s.result.Output) == "" {
		s.result.Output = strings.TrimSpace(turn.Output)
	}
	return s.result
}

func (s *stubAwareTurnCompletionNotifier) NotifyTurnCompleted(context.Context, string, string, string, *types.SessionMeta) {
}

func (s *stubAwareTurnCompletionNotifier) NotifyTurnCompletedEvent(context.Context, TurnCompletionEvent) {
}

func (s *stubAwareTurnCompletionNotifier) SetNotificationPublisher(notifier NotificationPublisher) {
	s.publisher = notifier
	s.setCalls++
}

type captureApprovalStorage struct {
	called    int
	sessionID string
	requestID int
	method    string
	params    json.RawMessage
}

func (s *captureApprovalStorage) StoreApproval(_ context.Context, sessionID string, requestID int, method string, params json.RawMessage) error {
	s.called++
	s.sessionID = sessionID
	s.requestID = requestID
	s.method = method
	s.params = params
	return nil
}

func (s *captureApprovalStorage) GetApproval(_ context.Context, _ string, _ int) (*types.Approval, bool, error) {
	return nil, false, nil
}

func (s *captureApprovalStorage) DeleteApproval(_ context.Context, _ string, _ int) error {
	return nil
}

func TestOpenCodeLiveSessionRespondSuccess(t *testing.T) {
	const providerSessionID = "remote-respond"
	replyReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/permissions/") {
			replyReceived = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{
		BaseURL:  server.URL,
		Username: "opencode",
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}

	store := &respondApprovalStorage{
		approvals: map[int]*types.Approval{
			42: {
				SessionID: "s1",
				RequestID: 42,
				Method:    "item/commandExecution/requestApproval",
				Params:    json.RawMessage(`{"permission_id":"perm-abc"}`),
			},
		},
	}

	ls := &openCodeLiveSession{
		sessionID:     "s1",
		providerName:  "opencode",
		providerID:    providerSessionID,
		directory:     "/tmp/respond-test",
		client:        client,
		approvalStore: store,
	}

	err = ls.Respond(context.Background(), 42, map[string]any{
		"decision": "accept",
	})
	if err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if !replyReceived {
		t.Fatal("expected ReplyPermission to be called")
	}
	if store.deleteCalled != 1 {
		t.Fatalf("expected approval to be deleted, deleteCalled=%d", store.deleteCalled)
	}
	if _, ok := store.approvals[42]; ok {
		t.Fatal("expected approval 42 to be removed from store")
	}
}

func TestOpenCodeLiveSessionRespondApprovalNotFound(t *testing.T) {
	store := &respondApprovalStorage{approvals: map[int]*types.Approval{}}
	ls := &openCodeLiveSession{
		sessionID:     "s1",
		approvalStore: store,
	}

	err := ls.Respond(context.Background(), 99, map[string]any{"decision": "accept"})
	if err == nil {
		t.Fatal("expected error for missing approval")
	}
}

func TestOpenCodeLiveSessionRespondMissingPermissionID(t *testing.T) {
	store := &respondApprovalStorage{
		approvals: map[int]*types.Approval{
			1: {
				SessionID: "s1",
				RequestID: 1,
				Params:    json.RawMessage(`{}`),
			},
		},
	}
	ls := &openCodeLiveSession{
		sessionID:     "s1",
		approvalStore: store,
	}

	err := ls.Respond(context.Background(), 1, map[string]any{"decision": "accept"})
	if err == nil {
		t.Fatal("expected error for missing permission_id")
	}
}

func TestOpenCodeLiveSessionRespondNilApprovalStore(t *testing.T) {
	ls := &openCodeLiveSession{
		sessionID: "s1",
	}
	err := ls.Respond(context.Background(), 1, map[string]any{"decision": "accept"})
	if err == nil {
		t.Fatal("expected error for nil approval store")
	}
}

type respondApprovalStorage struct {
	approvals    map[int]*types.Approval
	deleteCalled int
}

func (s *respondApprovalStorage) StoreApproval(_ context.Context, sessionID string, requestID int, method string, params json.RawMessage) error {
	if s.approvals == nil {
		s.approvals = map[int]*types.Approval{}
	}
	s.approvals[requestID] = &types.Approval{
		SessionID: sessionID,
		RequestID: requestID,
		Method:    method,
		Params:    params,
	}
	return nil
}

func (s *respondApprovalStorage) GetApproval(_ context.Context, _ string, requestID int) (*types.Approval, bool, error) {
	if s.approvals == nil {
		return nil, false, nil
	}
	a, ok := s.approvals[requestID]
	return a, ok, nil
}

func (s *respondApprovalStorage) DeleteApproval(_ context.Context, _ string, requestID int) error {
	s.deleteCalled++
	if s.approvals != nil {
		delete(s.approvals, requestID)
	}
	return nil
}
