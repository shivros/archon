package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/logging"
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
	service := NewSessionService(nil, &Stores{Sessions: store}, nil)
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

func TestSessionServiceSendMessageWithOptionsMergesAndPersistsRuntimeOptions(t *testing.T) {
	ctx := context.Background()
	sessionStore := &stubSessionIndexStore{
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
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json"))
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID: "s1",
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model:  "baseline-model",
			Access: types.AccessOnRequest,
		},
	}); err != nil {
		t.Fatalf("seed session meta: %v", err)
	}
	adapter := &testConversationAdapter{
		provider:   "mock-provider",
		sendTurnID: "turn-override",
	}
	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)
	service.adapters = newConversationAdapterRegistry(adapter)

	turnID, err := service.SendMessageWithOptions(ctx, "s1", []map[string]any{
		{"type": "text", "text": "hello"},
	}, SendMessageOptions{
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model:     "override-model",
			Reasoning: types.ReasoningHigh,
		},
		PersistRuntimeOption: true,
	})
	if err != nil {
		t.Fatalf("SendMessageWithOptions: %v", err)
	}
	if turnID != "turn-override" {
		t.Fatalf("expected turn id turn-override, got %q", turnID)
	}
	if adapter.lastRuntimeOptions == nil {
		t.Fatalf("expected adapter to receive merged runtime options")
	}
	if adapter.lastRuntimeOptions.Model != "override-model" {
		t.Fatalf("expected merged model override, got %q", adapter.lastRuntimeOptions.Model)
	}
	if adapter.lastRuntimeOptions.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected merged reasoning override, got %q", adapter.lastRuntimeOptions.Reasoning)
	}
	if adapter.lastRuntimeOptions.Access != types.AccessOnRequest {
		t.Fatalf("expected baseline access to remain merged, got %q", adapter.lastRuntimeOptions.Access)
	}
	meta, ok, err := metaStore.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("get persisted meta: %v", err)
	}
	if !ok || meta == nil || meta.RuntimeOptions == nil {
		t.Fatalf("expected persisted runtime options")
	}
	if meta.RuntimeOptions.Model != "override-model" {
		t.Fatalf("expected persisted model override, got %q", meta.RuntimeOptions.Model)
	}
	if meta.RuntimeOptions.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected persisted reasoning override, got %q", meta.RuntimeOptions.Reasoning)
	}
	if meta.RuntimeOptions.Access != types.AccessOnRequest {
		t.Fatalf("expected persisted merged access, got %q", meta.RuntimeOptions.Access)
	}
}

func TestSessionServiceSendMessageWithOptionsFailsWhenRuntimePersistenceFails(t *testing.T) {
	ctx := context.Background()
	sessionStore := &stubSessionIndexStore{
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
	metaStore := &failingSessionMetaStore{
		entry: &types.SessionMeta{
			SessionID: "s1",
			RuntimeOptions: &types.SessionRuntimeOptions{
				Model: "baseline-model",
			},
		},
		upsertErr: errors.New("disk full"),
	}
	adapter := &testConversationAdapter{
		provider:   "mock-provider",
		sendTurnID: "turn-override",
	}
	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)
	service.adapters = newConversationAdapterRegistry(adapter)

	_, err := service.SendMessageWithOptions(ctx, "s1", []map[string]any{
		{"type": "text", "text": "hello"},
	}, SendMessageOptions{
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model: "override-model",
		},
		PersistRuntimeOption: true,
	})
	if err == nil {
		t.Fatalf("expected SendMessageWithOptions to fail when runtime persistence fails")
	}
	expectServiceErrorKind(t, err, ServiceErrorUnavailable)
	if !errors.Is(err, ErrRuntimeOptionsPersistFailed) {
		t.Fatalf("expected ErrRuntimeOptionsPersistFailed, got %v", err)
	}
}

func TestSessionServiceSendMessageUsesPersistedRuntimeOptionsOnSubsequentSends(t *testing.T) {
	ctx := context.Background()
	sessionStore := &stubSessionIndexStore{
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
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json"))
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID: "s1",
		RuntimeOptions: &types.SessionRuntimeOptions{
			Access: types.AccessOnRequest,
		},
	}); err != nil {
		t.Fatalf("seed session meta: %v", err)
	}
	adapter := &testConversationAdapter{
		provider:   "mock-provider",
		sendTurnID: "turn-runtime",
	}
	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)
	service.adapters = newConversationAdapterRegistry(adapter)

	_, err := service.SendMessageWithOptions(ctx, "s1", []map[string]any{
		{"type": "text", "text": "step 1"},
	}, SendMessageOptions{
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model:     "gpt-5.3-codex",
			Reasoning: types.ReasoningExtraHigh,
		},
		PersistRuntimeOption: true,
	})
	if err != nil {
		t.Fatalf("SendMessageWithOptions step 1: %v", err)
	}
	_, err = service.SendMessage(ctx, "s1", []map[string]any{
		{"type": "text", "text": "step 2"},
	})
	if err != nil {
		t.Fatalf("SendMessage step 2: %v", err)
	}
	if len(adapter.runtimeOptionsBySend) != 2 {
		t.Fatalf("expected two send calls, got %d", len(adapter.runtimeOptionsBySend))
	}
	first := adapter.runtimeOptionsBySend[0]
	second := adapter.runtimeOptionsBySend[1]
	if first == nil {
		t.Fatalf("expected first send runtime options")
	}
	if second == nil {
		t.Fatalf("expected second send to inherit persisted runtime options")
	}
	if first.Model != "gpt-5.3-codex" || second.Model != "gpt-5.3-codex" {
		t.Fatalf("expected persisted model to be reused, got first=%q second=%q", first.Model, second.Model)
	}
	if first.Reasoning != types.ReasoningExtraHigh || second.Reasoning != types.ReasoningExtraHigh {
		t.Fatalf("expected persisted reasoning to be reused, got first=%q second=%q", first.Reasoning, second.Reasoning)
	}
	if first.Access != types.AccessOnRequest || second.Access != types.AccessOnRequest {
		t.Fatalf("expected baseline access to remain, got first=%q second=%q", first.Access, second.Access)
	}
}

func TestSessionServicePersistRuntimeOptionsAfterSendValidationAndNilContext(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var service *SessionService
		err := service.persistRuntimeOptionsAfterSend(context.Background(), "s1", &types.SessionRuntimeOptions{Model: "m"})
		if !errors.Is(err, ErrRuntimeOptionsPersistFailed) {
			t.Fatalf("expected ErrRuntimeOptionsPersistFailed, got %v", err)
		}
	})

	t.Run("missing meta store", func(t *testing.T) {
		service := NewSessionService(nil, &Stores{}, nil)
		err := service.persistRuntimeOptionsAfterSend(context.Background(), "s1", &types.SessionRuntimeOptions{Model: "m"})
		if !errors.Is(err, ErrRuntimeOptionsPersistFailed) {
			t.Fatalf("expected ErrRuntimeOptionsPersistFailed, got %v", err)
		}
	})

	t.Run("empty session id", func(t *testing.T) {
		service := NewSessionService(nil, &Stores{
			SessionMeta: store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json")),
		}, nil, nil)
		err := service.persistRuntimeOptionsAfterSend(context.Background(), "   ", &types.SessionRuntimeOptions{Model: "m"})
		if !errors.Is(err, ErrRuntimeOptionsPersistFailed) {
			t.Fatalf("expected ErrRuntimeOptionsPersistFailed, got %v", err)
		}
	})

	t.Run("nil runtime options is no-op", func(t *testing.T) {
		service := NewSessionService(nil, &Stores{
			SessionMeta: store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json")),
		}, nil, nil)
		if err := service.persistRuntimeOptionsAfterSend(context.Background(), "s1", nil); err != nil {
			t.Fatalf("expected nil runtime options to no-op, got %v", err)
		}
	})

	t.Run("nil context persists successfully", func(t *testing.T) {
		metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json"))
		service := NewSessionService(nil, &Stores{
			SessionMeta: metaStore,
		}, nil, nil)
		want := &types.SessionRuntimeOptions{
			Model:     "gpt-5.2-codex",
			Reasoning: types.ReasoningHigh,
		}
		if err := service.persistRuntimeOptionsAfterSend(nil, "s1", want); err != nil {
			t.Fatalf("persistRuntimeOptionsAfterSend: %v", err)
		}
		meta, ok, err := metaStore.Get(context.Background(), "s1")
		if err != nil {
			t.Fatalf("get persisted meta: %v", err)
		}
		if !ok || meta == nil || meta.RuntimeOptions == nil {
			t.Fatalf("expected persisted runtime options")
		}
		if meta.RuntimeOptions.Model != want.Model || meta.RuntimeOptions.Reasoning != want.Reasoning {
			t.Fatalf("unexpected persisted runtime options: %#v", meta.RuntimeOptions)
		}
	})
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
			service := NewSessionService(nil, &Stores{Sessions: store}, nil)
			_, err := service.SendMessage(context.Background(), "s1", []map[string]any{
				{"type": "text", "text": "hello"},
			})
			expectServiceErrorKind(t, err, ServiceErrorUnavailable)
		})
	}
}

type stubTurnCompletionStrategy struct {
	shouldPublish bool
	source        string
}

func (s stubTurnCompletionStrategy) ShouldPublishCompletion(int, []map[string]any) bool {
	return s.shouldPublish
}

func (s stubTurnCompletionStrategy) Source() string {
	if strings.TrimSpace(s.source) == "" {
		return "stub_source"
	}
	return s.source
}

type stubClaudeCompletionIO struct {
	items    []map[string]any
	readErr  error
	publishN int
}

func (s *stubClaudeCompletionIO) ReadSessionItems(string, int) ([]map[string]any, error) {
	if s == nil {
		return nil, nil
	}
	if s.readErr != nil {
		return nil, s.readErr
	}
	return s.items, nil
}

func (s *stubClaudeCompletionIO) PublishTurnCompleted(*types.Session, *types.SessionMeta, string, string) {
	if s == nil {
		return
	}
	s.publishN++
}

func TestClaudeItemDeltaCompletionStrategy(t *testing.T) {
	strategy := claudeItemDeltaCompletionStrategy{}
	tests := []struct {
		name        string
		before      int
		items       []map[string]any
		wantPublish bool
	}{
		{
			name:        "user_only_delta",
			before:      1,
			items:       []map[string]any{{"type": "userMessage"}, {"type": "userMessage"}},
			wantPublish: false,
		},
		{
			name:        "assistant_delta",
			before:      1,
			items:       []map[string]any{{"type": "userMessage"}, {"type": "agentMessage"}},
			wantPublish: true,
		},
		{
			name:        "reasoning_delta",
			before:      0,
			items:       []map[string]any{{"type": "reasoning"}},
			wantPublish: true,
		},
		{
			name:        "negative_before_is_normalized",
			before:      -1,
			items:       []map[string]any{{"type": "agentMessage"}},
			wantPublish: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strategy.ShouldPublishCompletion(tt.before, tt.items)
			if got != tt.wantPublish {
				t.Fatalf("ShouldPublishCompletion mismatch: got=%v want=%v", got, tt.wantPublish)
			}
		})
	}
	if strategy.Source() != "claude_items_post_send" {
		t.Fatalf("unexpected strategy source: %q", strategy.Source())
	}
}

func TestClaudeCompletionProbeItemCount(t *testing.T) {
	if got := claudeCompletionProbeItemCount(nil, "s1"); got != 0 {
		t.Fatalf("expected zero count for nil IO, got %d", got)
	}
	if got := claudeCompletionProbeItemCount(&stubClaudeCompletionIO{}, "   "); got != 0 {
		t.Fatalf("expected zero count for empty session id, got %d", got)
	}
	if got := claudeCompletionProbeItemCount(&stubClaudeCompletionIO{readErr: errors.New("boom")}, "s1"); got != 0 {
		t.Fatalf("expected zero count on read error, got %d", got)
	}
	if got := claudeCompletionProbeItemCount(&stubClaudeCompletionIO{
		items: []map[string]any{{"type": "userMessage"}, {"type": "agentMessage"}},
	}, "s1"); got != 2 {
		t.Fatalf("expected count 2, got %d", got)
	}
}

func TestClaudeCompletionProbeHasTerminalOutput(t *testing.T) {
	strategy := claudeItemDeltaCompletionStrategy{}
	io := &stubClaudeCompletionIO{
		items: []map[string]any{{"type": "userMessage"}, {"type": "agentMessage"}},
	}
	if got := claudeCompletionProbeHasTerminalOutput(nil, strategy, "s1", 0); got {
		t.Fatalf("expected false for nil IO")
	}
	if got := claudeCompletionProbeHasTerminalOutput(io, nil, "s1", 0); got {
		t.Fatalf("expected false for nil strategy")
	}
	if got := claudeCompletionProbeHasTerminalOutput(io, strategy, "   ", 0); got {
		t.Fatalf("expected false for empty session id")
	}
	if got := claudeCompletionProbeHasTerminalOutput(&stubClaudeCompletionIO{readErr: errors.New("boom")}, strategy, "s1", 0); got {
		t.Fatalf("expected false on read error")
	}
	if got := claudeCompletionProbeHasTerminalOutput(&stubClaudeCompletionIO{items: nil}, strategy, "s1", 0); got {
		t.Fatalf("expected false on empty items")
	}
	if got := claudeCompletionProbeHasTerminalOutput(io, strategy, "s1", 5); got {
		t.Fatalf("expected false when baseline exceeds len and no new items exist")
	}
	if got := claudeCompletionProbeHasTerminalOutput(io, strategy, "s1", -1); !got {
		t.Fatalf("expected true when negative baseline is normalized and assistant item exists")
	}
}

func TestClaudeCompletionItemSignalsTurnCompletion(t *testing.T) {
	tests := []struct {
		name string
		item map[string]any
		want bool
	}{
		{name: "nil", item: nil, want: false},
		{name: "agent_message", item: map[string]any{"type": "agentMessage"}, want: true},
		{name: "agent_message_delta", item: map[string]any{"type": "agentMessageDelta"}, want: true},
		{name: "agent_message_end", item: map[string]any{"type": "agentMessageEnd"}, want: true},
		{name: "assistant", item: map[string]any{"type": "assistant"}, want: true},
		{name: "reasoning", item: map[string]any{"type": "reasoning"}, want: true},
		{name: "result", item: map[string]any{"type": "result"}, want: true},
		{name: "unknown", item: map[string]any{"type": "userMessage"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := claudeCompletionItemSignalsTurnCompletion(tt.item); got != tt.want {
				t.Fatalf("claudeCompletionItemSignalsTurnCompletion() = %v, want %v", got, tt.want)
			}
		})
	}
}

type stubClaudeCompletionDecisionPolicy struct {
	publish bool
	source  string
}

func (s stubClaudeCompletionDecisionPolicy) Decide(int, []map[string]any, error) (bool, string) {
	return s.publish, s.source
}

func TestLiveManagerConversationSenderRequiresLiveManager(t *testing.T) {
	adapter := liveManagerConversationSender{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}

	_, err := adapter.SendMessage(context.Background(), sendDeps{}, session, nil, []map[string]any{{"type": "text", "text": "hello"}})
	expectServiceErrorKind(t, err, ServiceErrorUnavailable)
}

func TestLiveManagerConversationSenderDelegatesToLiveManager(t *testing.T) {
	adapter := liveManagerConversationSender{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}
	live := &stubLiveManager{
		startTurnResults: []stubLiveTurnResult{
			{turnID: "turn-live-1"},
		},
	}
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json"))
	deps := sendDeps{
		liveManager:      live,
		sessionMetaStore: metaStore,
	}

	turnID, err := adapter.SendMessage(context.Background(), deps, session, nil, []map[string]any{{"type": "text", "text": "hello"}})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if turnID != "turn-live-1" {
		t.Fatalf("expected turn-live-1, got %q", turnID)
	}
	if live.startTurnCalls != 1 {
		t.Fatalf("expected 1 StartTurn call, got %d", live.startTurnCalls)
	}
}

func TestLiveManagerConversationEventSubscriberRequiresLiveManager(t *testing.T) {
	adapter := liveManagerConversationEventSubscriber{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}

	_, _, err := adapter.SubscribeEvents(context.Background(), eventDeps{}, session, nil)
	expectServiceErrorKind(t, err, ServiceErrorUnavailable)
}

func TestLiveManagerConversationApproverRequiresLiveManager(t *testing.T) {
	adapter := liveManagerConversationApprover{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}

	err := adapter.Approve(context.Background(), approvalDeps{}, session, nil, 1, "accept", nil, nil)
	expectServiceErrorKind(t, err, ServiceErrorUnavailable)
}

func TestLiveManagerConversationInterrupterRequiresLiveManager(t *testing.T) {
	adapter := liveManagerConversationInterrupter{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}

	err := adapter.Interrupt(context.Background(), interruptDeps{}, session, nil)
	expectServiceErrorKind(t, err, ServiceErrorUnavailable)
}

func TestLiveManagerConversationSenderRequiresSession(t *testing.T) {
	adapter := liveManagerConversationSender{providerName: "test"}

	_, err := adapter.SendMessage(context.Background(), sendDeps{}, nil, nil, []map[string]any{{"type": "text", "text": "hello"}})
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
}

func TestLiveManagerConversationSenderStartTurnError(t *testing.T) {
	adapter := liveManagerConversationSender{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}
	live := &stubLiveManager{
		startTurnResults: []stubLiveTurnResult{
			{err: errors.New("start failed")},
		},
	}
	deps := sendDeps{liveManager: live}

	_, err := adapter.SendMessage(context.Background(), deps, session, nil, []map[string]any{{"type": "text", "text": "hello"}})
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
	if !strings.Contains(err.Error(), "start failed") {
		t.Fatalf("expected wrapped error message, got %v", err)
	}
}

func TestLiveManagerConversationEventSubscriberRequiresSession(t *testing.T) {
	adapter := liveManagerConversationEventSubscriber{providerName: "test"}
	live := &stubLiveManager{}
	deps := eventDeps{liveManager: live}

	_, _, err := adapter.SubscribeEvents(context.Background(), deps, nil, nil)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
}

func TestLiveManagerConversationEventSubscriberError(t *testing.T) {
	adapter := liveManagerConversationEventSubscriber{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}
	live := &stubLiveManager{subscribeErr: errors.New("subscribe failed")}
	deps := eventDeps{liveManager: live}

	_, _, err := adapter.SubscribeEvents(context.Background(), deps, session, nil)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
	if !strings.Contains(err.Error(), "subscribe failed") {
		t.Fatalf("expected wrapped error message, got %v", err)
	}
}

func TestLiveManagerConversationEventSubscriberDelegatesToLiveManager(t *testing.T) {
	adapter := liveManagerConversationEventSubscriber{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}
	live := &stubLiveManager{}
	deps := eventDeps{liveManager: live}

	ch, cancel, err := adapter.SubscribeEvents(context.Background(), deps, session, nil)
	if err != nil {
		t.Fatalf("SubscribeEvents: %v", err)
	}
	if ch == nil {
		t.Fatalf("expected non-nil channel")
	}
	if cancel == nil {
		t.Fatalf("expected non-nil cancel func")
	}
	cancel()
}

func TestLiveManagerConversationInterrupterDelegatesToLiveManager(t *testing.T) {
	adapter := liveManagerConversationInterrupter{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}
	live := &stubLiveManager{}
	deps := interruptDeps{liveManager: live}

	err := adapter.Interrupt(context.Background(), deps, session, nil)
	if err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if live.interruptCalls != 1 {
		t.Fatalf("expected 1 Interrupt call, got %d", live.interruptCalls)
	}
}

func TestLiveManagerConversationApproverRequiresSession(t *testing.T) {
	adapter := liveManagerConversationApprover{providerName: "test"}
	live := &stubLiveManager{}
	deps := approvalDeps{liveManager: live}

	err := adapter.Approve(context.Background(), deps, nil, nil, 1, "accept", nil, nil)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
}

func TestLiveManagerConversationApproverHappyPath(t *testing.T) {
	adapter := liveManagerConversationApprover{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}
	live := &stubLiveManager{}
	approvals := &stubApprovalStore{}
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json"))
	deps := approvalDeps{
		liveManager:      live,
		approvalStore:    approvals,
		sessionMetaStore: metaStore,
	}

	responses := []string{"yes", "confirmed"}
	acceptSettings := map[string]any{"always": true}
	err := adapter.Approve(context.Background(), deps, session, nil, 42, "accept", responses, acceptSettings)
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}

	// Verify Respond was called with responses and acceptSettings
	if live.respondCalls != 1 {
		t.Fatalf("expected 1 Respond call, got %d", live.respondCalls)
	}
	if live.lastRespondArgs["decision"] != "accept" {
		t.Fatalf("expected decision=accept, got %v", live.lastRespondArgs["decision"])
	}
	if _, ok := live.lastRespondArgs["responses"]; !ok {
		t.Fatalf("expected responses in Respond args")
	}
	if _, ok := live.lastRespondArgs["acceptSettings"]; !ok {
		t.Fatalf("expected acceptSettings in Respond args")
	}

	// Verify approval deletion
	if len(approvals.deleteCalls) != 1 {
		t.Fatalf("expected 1 approval delete call, got %d", len(approvals.deleteCalls))
	}
	if approvals.deleteCalls[0].sessionID != "s1" || approvals.deleteCalls[0].requestID != 42 {
		t.Fatalf("unexpected delete args: %+v", approvals.deleteCalls[0])
	}

	// Verify meta LastActiveAt was updated
	meta, ok, err := metaStore.Get(context.Background(), "s1")
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if !ok || meta == nil || meta.LastActiveAt == nil {
		t.Fatalf("expected LastActiveAt to be set after approve")
	}
}

func TestLiveManagerConversationApproverRespondError(t *testing.T) {
	adapter := liveManagerConversationApprover{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}
	live := &stubLiveManager{respondErr: errors.New("respond failed")}
	deps := approvalDeps{liveManager: live}

	err := adapter.Approve(context.Background(), deps, session, nil, 1, "accept", nil, nil)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
	if !strings.Contains(err.Error(), "respond failed") {
		t.Fatalf("expected wrapped error message, got %v", err)
	}
}

func TestLiveManagerConversationInterrupterRequiresSession(t *testing.T) {
	adapter := liveManagerConversationInterrupter{providerName: "test"}
	live := &stubLiveManager{}
	deps := interruptDeps{liveManager: live}

	err := adapter.Interrupt(context.Background(), deps, nil, nil)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
}

func TestLiveManagerConversationInterrupterError(t *testing.T) {
	adapter := liveManagerConversationInterrupter{providerName: "test"}
	session := &types.Session{ID: "s1", Provider: "test"}
	live := &stubLiveManager{interruptErr: errors.New("interrupt failed")}
	deps := interruptDeps{liveManager: live}

	err := adapter.Interrupt(context.Background(), deps, session, nil)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
	if !strings.Contains(err.Error(), "interrupt failed") {
		t.Fatalf("expected wrapped error message, got %v", err)
	}
}

func TestOpenCodeConversationAdapterHistoryFetchesRemoteMessages(t *testing.T) {
	cases := []struct {
		name        string
		provider    string
		baseURLVars []string
	}{
		{name: "opencode", provider: "opencode", baseURLVars: []string{"OPENCODE_BASE_URL"}},
		{name: "kilocode", provider: "kilocode", baseURLVars: []string{"KILOCODE_BASE_URL"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			const directory = "/tmp/open-history-worktree"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/session/remote-s-1/message":
					if r.Method != http.MethodGet {
						http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
						return
					}
					if got := r.URL.Query().Get("directory"); got != directory {
						http.Error(w, "missing directory", http.StatusBadRequest)
						return
					}
					writeJSON(w, http.StatusOK, []map[string]any{
						{
							"info": map[string]any{
								"id":        "msg-user-1",
								"role":      "user",
								"createdAt": "2026-02-13T01:00:00Z",
							},
							"parts": []map[string]any{
								{"type": "text", "text": "hello remote"},
							},
						},
						{
							"info": map[string]any{
								"id":        "msg-assistant-1",
								"role":      "assistant",
								"createdAt": "2026-02-13T01:00:01Z",
							},
							"parts": []map[string]any{
								{"type": "text", "text": "remote reply"},
							},
						},
					})
					return
				default:
					http.NotFound(w, r)
					return
				}
			}))
			defer server.Close()

			for _, envName := range tc.baseURLVars {
				t.Setenv(envName, server.URL)
			}

			ctx := context.Background()
			base := t.TempDir()
			sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions.json"))
			metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

			session := &types.Session{
				ID:        "s-open-history-" + tc.provider,
				Provider:  tc.provider,
				Cwd:       directory,
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
			}, nil, nil)

			items, err := service.History(ctx, session.ID, 200)
			if err != nil {
				t.Fatalf("History: %v", err)
			}
			if len(items) != 2 {
				t.Fatalf("expected 2 remote items, got %#v", items)
			}
			if items[0]["type"] != "userMessage" {
				t.Fatalf("expected first item to be userMessage, got %#v", items[0])
			}
			if items[1]["type"] != "assistant" {
				t.Fatalf("expected second item to be assistant, got %#v", items[1])
			}
			msg, _ := items[1]["message"].(map[string]any)
			if msg == nil || msg["content"] == nil {
				t.Fatalf("expected assistant content, got %#v", items[1])
			}
		})
	}
}

func TestOpenCodeConversationAdapterHistoryFallsBackWithoutDirectory(t *testing.T) {
	var (
		sawDirectoryAttempt bool
		sawFallbackAttempt  bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/remote-s-2/message":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if directory := strings.TrimSpace(r.URL.Query().Get("directory")); directory != "" {
				sawDirectoryAttempt = true
				http.NotFound(w, r)
				return
			}
			sawFallbackAttempt = true
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"info": map[string]any{
						"id":        "msg-assistant-2",
						"role":      "assistant",
						"createdAt": "2026-02-13T01:00:02Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "fallback reply"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	t.Setenv("OPENCODE_BASE_URL", server.URL)
	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

	session := &types.Session{
		ID:        "s-open-history-fallback",
		Provider:  "opencode",
		Cwd:       "/tmp/rejected-directory",
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
		ProviderSessionID: "remote-s-2",
	})
	if err != nil {
		t.Fatalf("seed session meta: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)

	items, err := service.History(ctx, session.ID, 50)
	if err != nil {
		t.Fatalf("History fallback: %v", err)
	}
	if !sawDirectoryAttempt || !sawFallbackAttempt {
		t.Fatalf("expected directory and fallback attempts, got directory=%v fallback=%v", sawDirectoryAttempt, sawFallbackAttempt)
	}
	if len(items) != 1 || items[0]["type"] != "assistant" {
		t.Fatalf("expected fallback assistant item, got %#v", items)
	}
}

func TestOpenCodeConversationAdapterHistoryBackfillsMissingItemsWithoutDuplicates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/remote-s-3/message":
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"info": map[string]any{
						"id":        "msg-user-3",
						"role":      "user",
						"createdAt": "2026-02-13T01:05:00Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "hello backfill"},
					},
				},
				{
					"info": map[string]any{
						"id":        "msg-assistant-3",
						"role":      "assistant",
						"createdAt": "2026-02-13T01:05:01Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "assistant backfill"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENCODE_BASE_URL", server.URL)

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

	session := &types.Session{
		ID:        "s-open-history-backfill",
		Provider:  "opencode",
		Cwd:       "/tmp/open-backfill",
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
		ProviderSessionID: "remote-s-3",
	})
	if err != nil {
		t.Fatalf("seed session meta: %v", err)
	}

	sessionsRoot := filepath.Join(home, ".archon", "sessions", session.ID)
	if err := os.MkdirAll(sessionsRoot, 0o700); err != nil {
		t.Fatalf("mkdir sessions root: %v", err)
	}
	initial := map[string]any{
		"type":                "userMessage",
		"provider_message_id": "msg-user-3",
		"provider_created_at": "2026-02-13T01:05:00Z",
		"content": []map[string]any{
			{"type": "text", "text": "hello backfill"},
		},
	}
	data, _ := json.Marshal(initial)
	if err := os.WriteFile(filepath.Join(sessionsRoot, "items.jsonl"), append(data, '\n'), 0o600); err != nil {
		t.Fatalf("seed items file: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)

	for i := 0; i < 2; i++ {
		items, err := service.History(ctx, session.ID, 50)
		if err != nil {
			t.Fatalf("History pass %d: %v", i+1, err)
		}
		if len(items) != 2 {
			t.Fatalf("expected two remote items on pass %d, got %#v", i+1, items)
		}
	}

	persisted, _, err := service.readSessionItems(session.ID, 50)
	if err != nil {
		t.Fatalf("readSessionItems: %v", err)
	}
	if len(persisted) != 2 {
		t.Fatalf("expected backfilled items without duplicates, got %#v", persisted)
	}
}

func TestUnsupportedConversationPortsReturnInvalidErrors(t *testing.T) {
	t.Run("sender", func(t *testing.T) {
		sender := unsupportedConversationSender{}
		if sender.Provider() != "*" {
			t.Fatalf("expected wildcard provider, got %q", sender.Provider())
		}
		_, err := sender.SendMessage(context.Background(), sendDeps{}, &types.Session{ID: "s1"}, nil, []map[string]any{{"type": "text", "text": "hello"}})
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
	})

	t.Run("subscriber", func(t *testing.T) {
		subscriber := unsupportedConversationEventSubscriber{}
		if subscriber.Provider() != "*" {
			t.Fatalf("expected wildcard provider, got %q", subscriber.Provider())
		}
		_, _, err := subscriber.SubscribeEvents(context.Background(), eventDeps{}, &types.Session{ID: "s1"}, nil)
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
	})

	t.Run("approver", func(t *testing.T) {
		approver := unsupportedConversationApprover{}
		if approver.Provider() != "*" {
			t.Fatalf("expected wildcard provider, got %q", approver.Provider())
		}
		err := approver.Approve(context.Background(), approvalDeps{}, &types.Session{ID: "s1"}, nil, 1, "accept", nil, nil)
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
	})

	t.Run("interrupter", func(t *testing.T) {
		interrupter := unsupportedConversationInterrupter{}
		if interrupter.Provider() != "*" {
			t.Fatalf("expected wildcard provider, got %q", interrupter.Provider())
		}
		err := interrupter.Interrupt(context.Background(), interruptDeps{}, &types.Session{ID: "s1"}, nil)
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
	})
}

func TestConversationAdapterRegistryNilAndEmptyFallbackPaths(t *testing.T) {
	var nilRegistry *conversationAdapterRegistry
	_, err := nilRegistry.senderFor("unknown").SendMessage(context.Background(), sendDeps{}, &types.Session{ID: "s1"}, nil, []map[string]any{{"type": "text", "text": "hello"}})
	expectServiceErrorKind(t, err, ServiceErrorInvalid)

	_, _, err = nilRegistry.eventsFor("unknown").SubscribeEvents(context.Background(), eventDeps{}, &types.Session{ID: "s1"}, nil)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)

	err = nilRegistry.approverFor("unknown").Approve(context.Background(), approvalDeps{}, &types.Session{ID: "s1"}, nil, 1, "accept", nil, nil)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)

	err = nilRegistry.interrupterFor("unknown").Interrupt(context.Background(), interruptDeps{}, &types.Session{ID: "s1"}, nil)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)

	items, err := nilRegistry.historyFor("unknown").History(context.Background(), historyDeps{
		readSessionLogs: func(string, int) ([]string, error) { return []string{"fallback"}, nil },
	}, &types.Session{ID: "s1", Provider: "unknown"}, nil, 20)
	if err != nil {
		t.Fatalf("history fallback: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one fallback item, got %#v", items)
	}

	emptyRegistry := &conversationAdapterRegistry{
		senders:      map[string]conversationSender{},
		history:      map[string]conversationHistoryReader{},
		subscribers:  map[string]conversationEventSubscriber{},
		approvers:    map[string]conversationApprover{},
		interrupters: map[string]conversationInterrupter{},
	}
	_, err = emptyRegistry.senderFor("unknown").SendMessage(context.Background(), sendDeps{}, &types.Session{ID: "s1"}, nil, []map[string]any{{"type": "text", "text": "hello"}})
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
}

func TestConversationAdapterRegistryUsesConfiguredFallbackMembers(t *testing.T) {
	registry := &conversationAdapterRegistry{
		fallbackSender:      testSenderPort{provider: "fb-sender", turnID: "t-fallback"},
		fallbackHistory:     testHistoryPort{provider: "fb-history", items: []map[string]any{{"type": "log", "text": "from-fallback"}}},
		fallbackSubscriber:  testEventSubscriberPort{provider: "fb-subscriber"},
		fallbackApprover:    testApproverPort{provider: "fb-approver"},
		fallbackInterrupter: testInterrupterPort{provider: "fb-interrupter"},
		senders:             map[string]conversationSender{},
		history:             map[string]conversationHistoryReader{},
		subscribers:         map[string]conversationEventSubscriber{},
		approvers:           map[string]conversationApprover{},
		interrupters:        map[string]conversationInterrupter{},
	}

	turnID, err := registry.senderFor("unknown").SendMessage(context.Background(), sendDeps{}, &types.Session{ID: "s1"}, nil, []map[string]any{{"type": "text", "text": "hello"}})
	if err != nil {
		t.Fatalf("fallback sender err: %v", err)
	}
	if turnID != "t-fallback" {
		t.Fatalf("expected fallback sender turn id, got %q", turnID)
	}

	items, err := registry.historyFor("unknown").History(context.Background(), historyDeps{}, &types.Session{ID: "s1", Provider: "unknown"}, nil, 5)
	if err != nil {
		t.Fatalf("fallback history err: %v", err)
	}
	if len(items) != 1 || items[0]["text"] != "from-fallback" {
		t.Fatalf("unexpected fallback history: %#v", items)
	}

	ch, cancel, err := registry.eventsFor("unknown").SubscribeEvents(context.Background(), eventDeps{}, &types.Session{ID: "s1"}, nil)
	if err != nil {
		t.Fatalf("fallback subscriber err: %v", err)
	}
	if ch == nil || cancel == nil {
		t.Fatalf("expected fallback subscriber channel+cancel")
	}
	cancel()

	err = registry.approverFor("unknown").Approve(context.Background(), approvalDeps{}, &types.Session{ID: "s1"}, nil, 1, "accept", nil, nil)
	if err != nil {
		t.Fatalf("fallback approver err: %v", err)
	}

	err = registry.interrupterFor("unknown").Interrupt(context.Background(), interruptDeps{}, &types.Session{ID: "s1"}, nil)
	if err != nil {
		t.Fatalf("fallback interrupter err: %v", err)
	}
}

func TestHistoryReadersAdditionalBranches(t *testing.T) {
	t.Run("default_history_nil_logs_reader", func(t *testing.T) {
		reader := defaultHistoryReader{}
		_, err := reader.History(context.Background(), historyDeps{}, &types.Session{ID: "s1", Provider: "unknown"}, nil, 10)
		expectServiceErrorKind(t, err, ServiceErrorUnavailable)
	})

	t.Run("default_history_nil_session", func(t *testing.T) {
		reader := defaultHistoryReader{}
		_, err := reader.History(context.Background(), historyDeps{}, nil, nil, 10)
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
	})

	t.Run("codex_tail_unavailable", func(t *testing.T) {
		reader := codexHistoryReader{providerName: "codex", fallback: defaultHistoryReader{}}
		_, err := reader.History(context.Background(), historyDeps{}, &types.Session{ID: "s1", Provider: "codex"}, &types.SessionMeta{ThreadID: "thread-1"}, 10)
		expectServiceErrorKind(t, err, ServiceErrorUnavailable)
	})

	t.Run("open_code_nil_session", func(t *testing.T) {
		reader := openCodeHistoryReader{providerName: "opencode", fallback: defaultHistoryReader{}}
		_, err := reader.History(context.Background(), historyDeps{}, nil, nil, 10)
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
	})

	t.Run("open_code_remote_empty_falls_back_to_logs", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/session/remote-empty/message":
				writeJSON(w, http.StatusOK, []map[string]any{})
				return
			default:
				http.NotFound(w, r)
				return
			}
		}))
		defer server.Close()
		t.Setenv("OPENCODE_BASE_URL", server.URL)

		reader := openCodeHistoryReader{providerName: "opencode", fallback: defaultHistoryReader{}}
		items, err := reader.History(context.Background(), historyDeps{
			readSessionLogs: func(string, int) ([]string, error) {
				return []string{"local-fallback-log"}, nil
			},
		}, &types.Session{
			ID:       "s1",
			Provider: "opencode",
			Cwd:      "/tmp/opencode-empty",
		}, &types.SessionMeta{
			SessionID:         "s1",
			ProviderSessionID: "remote-empty",
		}, 20)
		if err != nil {
			t.Fatalf("History: %v", err)
		}
		if len(items) != 1 || items[0]["text"] != "local-fallback-log" {
			t.Fatalf("expected local fallback item, got %#v", items)
		}
	})

	t.Run("open_code_remote_error_falls_back_to_logs", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer server.Close()
		t.Setenv("OPENCODE_BASE_URL", server.URL)

		reader := openCodeHistoryReader{providerName: "opencode", fallback: defaultHistoryReader{}}
		items, err := reader.History(context.Background(), historyDeps{
			readSessionLogs: func(string, int) ([]string, error) {
				return []string{"local-fallback-on-error"}, nil
			},
		}, &types.Session{
			ID:       "s2",
			Provider: "opencode",
			Cwd:      "/tmp/opencode-error",
		}, &types.SessionMeta{
			SessionID:         "s2",
			ProviderSessionID: "remote-error",
		}, 20)
		if err != nil {
			t.Fatalf("History: %v", err)
		}
		if len(items) != 1 || items[0]["text"] != "local-fallback-on-error" {
			t.Fatalf("expected local fallback item on error, got %#v", items)
		}
	})
}

func TestLiveManagerPortProviderNames(t *testing.T) {
	if got := (liveManagerConversationSender{providerName: "p1"}).Provider(); got != "p1" {
		t.Fatalf("unexpected sender provider: %q", got)
	}
	if got := (liveManagerConversationEventSubscriber{providerName: "p2"}).Provider(); got != "p2" {
		t.Fatalf("unexpected subscriber provider: %q", got)
	}
	if got := (liveManagerConversationApprover{providerName: "p3"}).Provider(); got != "p3" {
		t.Fatalf("unexpected approver provider: %q", got)
	}
	if got := (liveManagerConversationInterrupter{providerName: "p4"}).Provider(); got != "p4" {
		t.Fatalf("unexpected interrupter provider: %q", got)
	}
	if got := (defaultHistoryReader{}).Provider(); got != "*" {
		t.Fatalf("unexpected default history provider: %q", got)
	}
	if got := (codexHistoryReader{providerName: "codex"}).Provider(); got != "codex" {
		t.Fatalf("unexpected codex history provider: %q", got)
	}
	if got := (openCodeHistoryReader{providerName: "opencode"}).Provider(); got != "opencode" {
		t.Fatalf("unexpected opencode history provider: %q", got)
	}
}

func TestOpenCodeHistoryReconcilerLogHelpers(t *testing.T) {
	// nil logger should be a no-op
	openCodeHistoryReconciler{}.logWarn("warn")
	openCodeHistoryReconciler{}.logDebug("debug")

	var debugBuf bytes.Buffer
	debugLogger := logging.New(&debugBuf, logging.Debug)
	rec := openCodeHistoryReconciler{logger: debugLogger}
	rec.logWarn("warn_message")
	rec.logDebug("debug_message")
	logs := debugBuf.String()
	if !strings.Contains(logs, "warn_message") || !strings.Contains(logs, "debug_message") {
		t.Fatalf("expected both warn and debug messages in logs, got %q", logs)
	}

	var infoBuf bytes.Buffer
	infoLogger := logging.New(&infoBuf, logging.Info)
	rec = openCodeHistoryReconciler{logger: infoLogger}
	rec.logDebug("debug_filtered")
	if strings.Contains(infoBuf.String(), "debug_filtered") {
		t.Fatalf("expected debug message to be filtered at info level, got %q", infoBuf.String())
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
	provider             string
	sendTurnID           string
	sendCalls            int
	lastRuntimeOptions   *types.SessionRuntimeOptions
	runtimeOptionsBySend []*types.SessionRuntimeOptions
}

type testSenderPort struct {
	provider string
	turnID   string
}

func (p testSenderPort) Provider() string { return p.provider }

func (p testSenderPort) SendMessage(context.Context, sendDeps, *types.Session, *types.SessionMeta, []map[string]any) (string, error) {
	return p.turnID, nil
}

type testHistoryPort struct {
	provider string
	items    []map[string]any
}

func (p testHistoryPort) Provider() string { return p.provider }

func (p testHistoryPort) History(context.Context, historyDeps, *types.Session, *types.SessionMeta, int) ([]map[string]any, error) {
	return p.items, nil
}

type testEventSubscriberPort struct {
	provider string
}

func (p testEventSubscriberPort) Provider() string { return p.provider }

func (p testEventSubscriberPort) SubscribeEvents(context.Context, eventDeps, *types.Session, *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	ch := make(chan types.CodexEvent)
	close(ch)
	return ch, func() {}, nil
}

type testApproverPort struct {
	provider string
}

func (p testApproverPort) Provider() string { return p.provider }

func (p testApproverPort) Approve(context.Context, approvalDeps, *types.Session, *types.SessionMeta, int, string, []string, map[string]any) error {
	return nil
}

type testInterrupterPort struct {
	provider string
}

func (p testInterrupterPort) Provider() string { return p.provider }

func (p testInterrupterPort) Interrupt(context.Context, interruptDeps, *types.Session, *types.SessionMeta) error {
	return nil
}

type stubLiveTurnResult struct {
	turnID string
	err    error
}

type stubLiveManager struct {
	startTurnResults []stubLiveTurnResult
	startTurnCalls   int

	subscribeErr    error
	respondErr      error
	respondCalls    int
	lastRespondArgs map[string]any
	interruptErr    error
	interruptCalls  int
}

func (s *stubLiveManager) StartTurn(_ context.Context, _ *types.Session, _ *types.SessionMeta, _ []map[string]any, _ *types.SessionRuntimeOptions) (string, error) {
	if s.startTurnCalls >= len(s.startTurnResults) {
		return "", nil
	}
	result := s.startTurnResults[s.startTurnCalls]
	s.startTurnCalls++
	return result.turnID, result.err
}

func (s *stubLiveManager) Subscribe(_ *types.Session, _ *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	if s.subscribeErr != nil {
		return nil, nil, s.subscribeErr
	}
	ch := make(chan types.CodexEvent)
	close(ch)
	return ch, func() {}, nil
}

func (s *stubLiveManager) Respond(_ context.Context, _ *types.Session, _ *types.SessionMeta, _ int, result map[string]any) error {
	s.respondCalls++
	s.lastRespondArgs = result
	return s.respondErr
}

func (s *stubLiveManager) Interrupt(context.Context, *types.Session, *types.SessionMeta) error {
	s.interruptCalls++
	return s.interruptErr
}

func (s *stubLiveManager) SetNotificationPublisher(NotificationPublisher) {}

type failingSessionMetaStore struct {
	entry     *types.SessionMeta
	upsertErr error
}

func (s *failingSessionMetaStore) List(context.Context) ([]*types.SessionMeta, error) {
	if s == nil || s.entry == nil {
		return []*types.SessionMeta{}, nil
	}
	copy := *s.entry
	return []*types.SessionMeta{&copy}, nil
}

func (s *failingSessionMetaStore) Get(_ context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	if s == nil || s.entry == nil || s.entry.SessionID != sessionID {
		return nil, false, nil
	}
	copy := *s.entry
	return &copy, true, nil
}

func (s *failingSessionMetaStore) Upsert(_ context.Context, _ *types.SessionMeta) (*types.SessionMeta, error) {
	if s == nil || s.upsertErr == nil {
		return nil, nil
	}
	return nil, s.upsertErr
}

func (s *failingSessionMetaStore) Delete(context.Context, string) error {
	return nil
}

type stubApprovalStore struct {
	deleteCalls []struct {
		sessionID string
		requestID int
	}
}

func (s *stubApprovalStore) ListBySession(context.Context, string) ([]*types.Approval, error) {
	return nil, nil
}

func (s *stubApprovalStore) Get(context.Context, string, int) (*types.Approval, bool, error) {
	return nil, false, nil
}

func (s *stubApprovalStore) Upsert(context.Context, *types.Approval) (*types.Approval, error) {
	return nil, nil
}

func (s *stubApprovalStore) Delete(_ context.Context, sessionID string, requestID int) error {
	s.deleteCalls = append(s.deleteCalls, struct {
		sessionID string
		requestID int
	}{sessionID, requestID})
	return nil
}

func (s *stubApprovalStore) DeleteSession(context.Context, string) error {
	return nil
}

func (a *testConversationAdapter) Provider() string {
	return a.provider
}

func (a *testConversationAdapter) History(context.Context, historyDeps, *types.Session, *types.SessionMeta, int) ([]map[string]any, error) {
	return []map[string]any{{"type": "log", "text": "adapter-history"}}, nil
}

func (a *testConversationAdapter) SendMessage(_ context.Context, _ sendDeps, _ *types.Session, meta *types.SessionMeta, _ []map[string]any) (string, error) {
	a.sendCalls++
	if meta != nil {
		a.lastRuntimeOptions = types.CloneRuntimeOptions(meta.RuntimeOptions)
	} else {
		a.lastRuntimeOptions = nil
	}
	a.runtimeOptionsBySend = append(a.runtimeOptionsBySend, types.CloneRuntimeOptions(a.lastRuntimeOptions))
	return a.sendTurnID, nil
}

func (a *testConversationAdapter) SubscribeEvents(context.Context, eventDeps, *types.Session, *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	ch := make(chan types.CodexEvent)
	close(ch)
	return ch, func() {}, nil
}

func (a *testConversationAdapter) Approve(context.Context, approvalDeps, *types.Session, *types.SessionMeta, int, string, []string, map[string]any) error {
	return nil
}

func (a *testConversationAdapter) Interrupt(context.Context, interruptDeps, *types.Session, *types.SessionMeta) error {
	return nil
}
