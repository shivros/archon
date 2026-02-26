package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestConversationAdapterContractHistoryRequiresSession(t *testing.T) {
	registry := newConversationAdapterRegistry()
	service := NewSessionService(nil, nil, nil)

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

func TestClaudeCompletionStrategyOrDefault(t *testing.T) {
	adapter := claudeConversationAdapter{}
	if _, ok := adapter.completionStrategyOrDefault().(claudeItemDeltaCompletionStrategy); !ok {
		t.Fatalf("expected default claude completion strategy")
	}
	custom := stubTurnCompletionStrategy{shouldPublish: true, source: "custom"}
	adapter.completionStrategy = custom
	if got := adapter.completionStrategyOrDefault(); got.Source() != "custom" {
		t.Fatalf("expected custom strategy source, got %q", got.Source())
	}
}

func TestSessionServiceClaudeCompletionIONilService(t *testing.T) {
	io := sessionServiceClaudeCompletionIO{}
	items, err := io.ReadSessionItems("s1", 10)
	if err != nil {
		t.Fatalf("ReadSessionItems err: %v", err)
	}
	if items != nil {
		t.Fatalf("expected nil items for nil service, got %#v", items)
	}
	io.PublishTurnCompleted(nil, nil, "", "")
}

func TestClaudeConversationAdapterSendMessageDoesNotPublishCompletionWithoutAssistantOutput(t *testing.T) {
	ctx := context.Background()
	sessionID := "s-claude-no-complete"
	baseDir := t.TempDir()
	session := &types.Session{
		ID:        sessionID,
		Provider:  "claude",
		Cwd:       t.TempDir(),
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	sessionStore := &stubSessionIndexStore{
		records: map[string]*types.SessionRecord{
			sessionID: {
				Session: session,
				Source:  sessionSourceInternal,
			},
		},
	}
	var service *SessionService
	manager := &SessionManager{
		baseDir: baseDir,
		sessions: map[string]*sessionRuntime{
			sessionID: {
				session: session,
				send: func([]byte) error {
					return service.appendSessionItems(sessionID, []map[string]any{
						{"type": "userMessage", "content": []map[string]any{{"type": "text", "text": "hello"}}},
					})
				},
			},
		},
	}
	publisher := &captureSessionServiceNotificationPublisher{}
	service = NewSessionService(manager, &Stores{Sessions: sessionStore}, nil, WithNotificationPublisher(publisher))

	turnID, err := service.SendMessage(ctx, sessionID, []map[string]any{
		{"type": "text", "text": "hello"},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if turnID != "" {
		t.Fatalf("expected empty turn id for claude send, got %q", turnID)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("expected no turn-completed event without assistant output, got %#v", publisher.events)
	}
}

func TestClaudeConversationAdapterSendMessageRequiresProviderSessionIDOnRecovery(t *testing.T) {
	ctx := context.Background()
	sessionID := "s-claude-missing-provider-session"
	session := &types.Session{
		ID:        sessionID,
		Provider:  "claude",
		Cwd:       t.TempDir(),
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	service := NewSessionService(&SessionManager{
		baseDir: t.TempDir(),
		sessions: map[string]*sessionRuntime{
			sessionID: {
				session: session,
				send: func([]byte) error {
					return ErrSessionNotFound
				},
			},
		},
	}, &Stores{
		Sessions: &stubSessionIndexStore{
			records: map[string]*types.SessionRecord{
				sessionID: {Session: session, Source: sessionSourceInternal},
			},
		},
	}, nil)
	_, err := service.SendMessage(ctx, sessionID, []map[string]any{
		{"type": "text", "text": "hello"},
	})
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
	if !strings.Contains(err.Error(), "provider session id not available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClaudeConversationAdapterSendMessageRequiresCwdOnRecovery(t *testing.T) {
	ctx := context.Background()
	sessionID := "s-claude-missing-cwd"
	session := &types.Session{
		ID:        sessionID,
		Provider:  "claude",
		Cwd:       "   ",
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	service := NewSessionService(&SessionManager{
		baseDir: t.TempDir(),
		sessions: map[string]*sessionRuntime{
			sessionID: {
				session: session,
				send: func([]byte) error {
					return ErrSessionNotFound
				},
			},
		},
	}, &Stores{
		Sessions: &stubSessionIndexStore{
			records: map[string]*types.SessionRecord{
				sessionID: {Session: session, Source: sessionSourceInternal},
			},
		},
		SessionMeta: &failingSessionMetaStore{
			entry: &types.SessionMeta{
				SessionID:         sessionID,
				ProviderSessionID: "provider-sess-1",
			},
		},
	}, nil)
	_, err := service.SendMessage(ctx, sessionID, []map[string]any{
		{"type": "text", "text": "hello"},
	})
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
	if !strings.Contains(err.Error(), "session cwd is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClaudeConversationAdapterSendMessageRecoveryAdditionalDirectoriesError(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	if err := ensureDir(repoDir); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{
		RepoPath:              repoDir,
		AdditionalDirectories: []string{"../missing"},
	})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}
	sessionID := "s-claude-dirs-err"
	session := &types.Session{
		ID:        sessionID,
		Provider:  "claude",
		Cwd:       repoDir,
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	service := NewSessionService(&SessionManager{
		baseDir: t.TempDir(),
		sessions: map[string]*sessionRuntime{
			sessionID: {
				session: session,
				send: func([]byte) error {
					return ErrSessionNotFound
				},
			},
		},
	}, &Stores{
		Sessions: &stubSessionIndexStore{
			records: map[string]*types.SessionRecord{
				sessionID: {Session: session, Source: sessionSourceInternal},
			},
		},
		SessionMeta: &failingSessionMetaStore{
			entry: &types.SessionMeta{
				SessionID:         sessionID,
				ProviderSessionID: "provider-sess-1",
				WorkspaceID:       ws.ID,
			},
		},
		Workspaces: workspaceStore,
	}, nil)
	_, err = service.SendMessage(ctx, sessionID, []map[string]any{
		{"type": "text", "text": "hello"},
	})
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
	if !strings.Contains(strings.ToLower(err.Error()), "no such file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClaudeConversationAdapterSendMessageRecoveryResumeError(t *testing.T) {
	ctx := context.Background()
	sessionID := "s-claude-resume-err"
	session := &types.Session{
		ID:        sessionID,
		Provider:  "claude",
		Cwd:       t.TempDir(),
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	baseFile := filepath.Join(t.TempDir(), "sessions-base-file")
	if err := os.WriteFile(baseFile, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	service := NewSessionService(&SessionManager{
		baseDir:  baseFile,
		sessions: map[string]*sessionRuntime{},
	}, &Stores{
		Sessions: &stubSessionIndexStore{
			records: map[string]*types.SessionRecord{
				sessionID: {Session: session, Source: sessionSourceInternal},
			},
		},
		SessionMeta: &failingSessionMetaStore{
			entry: &types.SessionMeta{
				SessionID:         sessionID,
				ProviderSessionID: "provider-sess-1",
			},
		},
	}, nil)
	_, err := service.SendMessage(ctx, sessionID, []map[string]any{
		{"type": "text", "text": "hello"},
	})
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
}

func TestClaudeConversationAdapterSendMessageRecoverySecondSendFailure(t *testing.T) {
	ctx := context.Background()
	sessionID := "s-claude-retry-send-fail"
	session := &types.Session{
		ID:        sessionID,
		Provider:  "claude",
		Cwd:       t.TempDir(),
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	sendCalls := 0
	service := NewSessionService(&SessionManager{
		baseDir: t.TempDir(),
		sessions: map[string]*sessionRuntime{
			sessionID: {
				session: session,
				send: func([]byte) error {
					sendCalls++
					if sendCalls == 1 {
						return ErrSessionNotFound
					}
					return errors.New("second send failed")
				},
			},
		},
	}, &Stores{
		Sessions: &stubSessionIndexStore{
			records: map[string]*types.SessionRecord{
				sessionID: {Session: session, Source: sessionSourceInternal},
			},
		},
		SessionMeta: &failingSessionMetaStore{
			entry: &types.SessionMeta{
				SessionID:         sessionID,
				ProviderSessionID: "provider-sess-1",
			},
		},
	}, nil)
	_, err := service.SendMessage(ctx, sessionID, []map[string]any{
		{"type": "text", "text": "hello"},
	})
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
	if !strings.Contains(err.Error(), "second send failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClaudeConversationAdapterSendMessageRecoverySecondSendSuccess(t *testing.T) {
	ctx := context.Background()
	sessionID := "s-claude-retry-send-ok"
	session := &types.Session{
		ID:        sessionID,
		Provider:  "claude",
		Cwd:       t.TempDir(),
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	sendCalls := 0
	service := NewSessionService(&SessionManager{
		baseDir: t.TempDir(),
		sessions: map[string]*sessionRuntime{
			sessionID: {
				session: session,
				send: func([]byte) error {
					sendCalls++
					if sendCalls == 1 {
						return ErrSessionNotFound
					}
					return nil
				},
			},
		},
	}, &Stores{
		Sessions: &stubSessionIndexStore{
			records: map[string]*types.SessionRecord{
				sessionID: {Session: session, Source: sessionSourceInternal},
			},
		},
		SessionMeta: &failingSessionMetaStore{
			entry: &types.SessionMeta{
				SessionID:         sessionID,
				ProviderSessionID: "provider-sess-1",
			},
		},
	}, nil)
	if _, err := service.SendMessage(ctx, sessionID, []map[string]any{
		{"type": "text", "text": "hello"},
	}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if sendCalls != 2 {
		t.Fatalf("expected two send attempts, got %d", sendCalls)
	}
}

func TestClaudeConversationAdapterSendMessageUsesInjectedCompletionStrategy(t *testing.T) {
	ctx := context.Background()
	sessionID := "s-claude-injected-strategy"
	baseDir := t.TempDir()
	session := &types.Session{
		ID:        sessionID,
		Provider:  "claude",
		Cwd:       t.TempDir(),
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	sessionStore := &stubSessionIndexStore{
		records: map[string]*types.SessionRecord{
			sessionID: {
				Session: session,
				Source:  sessionSourceInternal,
			},
		},
	}
	var service *SessionService
	manager := &SessionManager{
		baseDir: baseDir,
		sessions: map[string]*sessionRuntime{
			sessionID: {
				session: session,
				send: func([]byte) error {
					return service.appendSessionItems(sessionID, []map[string]any{
						{"type": "userMessage", "content": []map[string]any{{"type": "text", "text": "hello"}}},
					})
				},
			},
		},
	}
	publisher := &captureSessionServiceNotificationPublisher{}
	service = NewSessionService(manager, &Stores{Sessions: sessionStore}, nil, WithNotificationPublisher(publisher))
	service.adapters = newConversationAdapterRegistry(claudeConversationAdapter{
		fallback: defaultConversationAdapter{},
		completionStrategy: stubTurnCompletionStrategy{
			shouldPublish: true,
			source:        "injected_strategy",
		},
	})

	_, err := service.SendMessage(ctx, sessionID, []map[string]any{
		{"type": "text", "text": "hello"},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("expected one completion event from injected strategy, got %d", len(publisher.events))
	}
	if publisher.events[0].Source != "injected_strategy" {
		t.Fatalf("unexpected completion source: %q", publisher.events[0].Source)
	}
}

func TestClaudeConversationAdapterSendMessagePublishesCompletionAfterAssistantOutput(t *testing.T) {
	ctx := context.Background()
	sessionID := "s-claude-complete"
	baseDir := t.TempDir()
	session := &types.Session{
		ID:        sessionID,
		Provider:  "claude",
		Cwd:       t.TempDir(),
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	sessionStore := &stubSessionIndexStore{
		records: map[string]*types.SessionRecord{
			sessionID: {
				Session: session,
				Source:  sessionSourceInternal,
			},
		},
	}
	var service *SessionService
	manager := &SessionManager{
		baseDir: baseDir,
		sessions: map[string]*sessionRuntime{
			sessionID: {
				session: session,
				send: func([]byte) error {
					return service.appendSessionItems(sessionID, []map[string]any{
						{"type": "userMessage", "content": []map[string]any{{"type": "text", "text": "hello"}}},
						{"type": "agentMessage", "text": "done"},
					})
				},
			},
		},
	}
	publisher := &captureSessionServiceNotificationPublisher{}
	service = NewSessionService(manager, &Stores{Sessions: sessionStore}, nil, WithNotificationPublisher(publisher))

	turnID, err := service.SendMessage(ctx, sessionID, []map[string]any{
		{"type": "text", "text": "hello"},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if turnID != "" {
		t.Fatalf("expected empty turn id for claude send, got %q", turnID)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("expected one turn-completed event, got %d", len(publisher.events))
	}
	event := publisher.events[0]
	if event.Trigger != types.NotificationTriggerTurnCompleted {
		t.Fatalf("unexpected trigger: %q", event.Trigger)
	}
	if event.Source != "claude_items_post_send" {
		t.Fatalf("expected claude post-send completion source, got %q", event.Source)
	}
}

func TestClaudeConversationAdapterUnsupportedOperationsReturnInvalid(t *testing.T) {
	adapter := newClaudeConversationAdapter(defaultConversationAdapter{})
	_, _, err := adapter.SubscribeEvents(context.Background(), nil, &types.Session{ID: "s1"}, nil)
	expectServiceErrorKind(t, err, ServiceErrorInvalid)
	if err := adapter.Approve(context.Background(), nil, &types.Session{ID: "s1"}, nil, 1, "accept", nil, nil); err == nil {
		t.Fatalf("expected approve to return error")
	} else {
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
	}
	if err := adapter.Interrupt(context.Background(), nil, &types.Session{ID: "s1"}, nil); err == nil {
		t.Fatalf("expected interrupt to return error")
	} else {
		expectServiceErrorKind(t, err, ServiceErrorInvalid)
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

func TestOpenCodeConversationAdapterApproveForwardsResponsesToSessionEndpoint(t *testing.T) {
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
	sessionMetaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

	session := &types.Session{
		ID:        "s-open-session-endpoint",
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
	_, err = sessionMetaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:         session.ID,
		ProviderSessionID: "remote-s-1",
	})
	if err != nil {
		t.Fatalf("seed session meta: %v", err)
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
		Sessions:    sessionStore,
		SessionMeta: sessionMetaStore,
		Approvals:   approvalStore,
	}, nil, nil)

	if err := service.Approve(ctx, session.ID, 77, "accept", []string{"first answer", "second answer"}, nil); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if receivedPath != "/session/remote-s-1/permissions/perm-123" {
		t.Fatalf("unexpected permission reply path: %q", receivedPath)
	}
	if response := strings.TrimSpace(asString(receivedBody["response"])); response != "once" {
		t.Fatalf("unexpected approval response payload: %#v", receivedBody)
	}
	rawResponses, _ := receivedBody["responses"].([]any)
	if len(rawResponses) != 2 || asString(rawResponses[0]) != "first answer" || asString(rawResponses[1]) != "second answer" {
		t.Fatalf("unexpected approval responses payload: %#v", receivedBody)
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

func TestOpenCodeConversationAdapterSendMessageReconcilesHistoryOnSendFailure(t *testing.T) {
	const (
		sessionID         = "s-open-send-reconcile"
		providerSessionID = "remote-s-send"
		directory         = "/tmp/open-send-reconcile"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/" + providerSessionID + "/message":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"info": map[string]any{
						"id":        "msg-user-send",
						"role":      "user",
						"createdAt": "2026-02-13T02:00:00Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "hello send"},
					},
				},
				{
					"info": map[string]any{
						"id":        "msg-assistant-send",
						"role":      "assistant",
						"createdAt": "2026-02-13T02:00:01Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "recovered send reply"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENCODE_BASE_URL", server.URL)
	rememberOpenCodeRuntimeBaseURL("opencode", server.URL)
	sessionsBaseDir := filepath.Join(home, ".archon", "sessions")
	if err := os.MkdirAll(sessionsBaseDir, 0o700); err != nil {
		t.Fatalf("mkdir sessions dir: %v", err)
	}

	ctx := context.Background()
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:         sessionID,
		ProviderSessionID: providerSessionID,
	}); err != nil {
		t.Fatalf("seed session meta: %v", err)
	}

	session := &types.Session{
		ID:        sessionID,
		Provider:  "opencode",
		Cwd:       directory,
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	manager := &SessionManager{
		baseDir: sessionsBaseDir,
		sessions: map[string]*sessionRuntime{
			sessionID: {
				session: session,
				send: func([]byte) error {
					return errors.New("simulated send timeout")
				},
			},
		},
	}
	service := NewSessionService(manager, &Stores{SessionMeta: metaStore}, nil)

	_, err := service.SendMessage(ctx, sessionID, []map[string]any{
		{"type": "text", "text": "hello send"},
	})
	expectServiceErrorKind(t, err, ServiceErrorInvalid)

	items, _, readErr := service.readSessionItems(sessionID, 50)
	if readErr != nil {
		t.Fatalf("readSessionItems: %v", readErr)
	}
	if len(items) == 0 {
		t.Fatalf("expected reconciled items, got none")
	}
	foundAssistant := false
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(asString(item["type"]))) != "assistant" {
			continue
		}
		if strings.TrimSpace(asString(item["provider_message_id"])) == "msg-assistant-send" {
			foundAssistant = true
			break
		}
	}
	if !foundAssistant {
		t.Fatalf("expected assistant recovery in items, got %#v", items)
	}
}

func TestOpenCodeConversationAdapterLiveTurnResumesOnSessionNotFound(t *testing.T) {
	ctx := context.Background()
	session := &types.Session{
		ID:        "s-open-live-resume",
		Provider:  "opencode",
		Cwd:       t.TempDir(),
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	sessionStore := &stubSessionIndexStore{
		records: map[string]*types.SessionRecord{
			session.ID: {
				Session: session,
				Source:  sessionSourceInternal,
			},
		},
	}
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "session_meta.json"))
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:         session.ID,
		ProviderSessionID: "remote-live-resume",
	}); err != nil {
		t.Fatalf("seed session meta: %v", err)
	}
	manager := &SessionManager{
		baseDir: t.TempDir(),
		sessions: map[string]*sessionRuntime{
			session.ID: {session: session},
		},
	}
	live := &stubLiveManager{
		startTurnResults: []stubLiveTurnResult{
			{err: ErrSessionNotFound},
			{turnID: "turn-live-recovered"},
		},
	}
	service := NewSessionService(manager, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)
	service.liveManager = live

	turnID, err := service.SendMessage(ctx, session.ID, []map[string]any{
		{"type": "text", "text": "hello"},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if turnID != "turn-live-recovered" {
		t.Fatalf("expected recovered turn id, got %q", turnID)
	}
	if live.startTurnCalls != 2 {
		t.Fatalf("expected two live start attempts, got %d", live.startTurnCalls)
	}
}

func TestOpenCodeConversationAdapterSubscribeEventsRecoversMissedAssistantOnStreamClose(t *testing.T) {
	const (
		sessionID         = "s-open-event-reconcile"
		providerSessionID = "remote-s-events"
		directory         = "/tmp/open-event-reconcile"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/event":
			if got := strings.TrimSpace(r.URL.Query().Get("parentID")); got != providerSessionID {
				http.Error(w, "missing parentID", http.StatusBadRequest)
				return
			}
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			if flusher, ok := w.(http.Flusher); ok {
				_, _ = w.Write([]byte(":\n\n"))
				flusher.Flush()
			}
			return
		case "/session/" + providerSessionID + "/message":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"info": map[string]any{
						"id":        "msg-user-event",
						"role":      "user",
						"createdAt": "2026-02-13T02:10:00Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "hello event"},
					},
				},
				{
					"info": map[string]any{
						"id":        "msg-assistant-event",
						"role":      "assistant",
						"createdAt": "2026-02-13T02:10:01Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "recovered event reply"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("OPENCODE_BASE_URL", server.URL)
	rememberOpenCodeRuntimeBaseURL("opencode", server.URL)
	service := NewSessionService(nil, nil, nil)
	adapter := openCodeConversationAdapter{providerName: "opencode"}
	session := &types.Session{
		ID:        sessionID,
		Provider:  "opencode",
		Cwd:       directory,
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	meta := &types.SessionMeta{
		SessionID:         sessionID,
		ProviderSessionID: providerSessionID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	events, closeFn, err := adapter.SubscribeEvents(ctx, service, session, meta)
	if err != nil {
		t.Fatalf("SubscribeEvents: %v", err)
	}
	defer closeFn()

	methods := make([]string, 0, 8)
	for event := range events {
		methods = append(methods, event.Method)
	}
	if len(methods) == 0 {
		t.Fatalf("expected recovered events, got none")
	}
	if !containsString(methods, "item/agentMessage/delta") {
		t.Fatalf("expected recovered assistant delta event, got %v", methods)
	}
	if !containsString(methods, "turn/completed") {
		t.Fatalf("expected recovered turn completion event, got %v", methods)
	}

	items, _, readErr := service.readSessionItems(sessionID, 50)
	if readErr != nil {
		t.Fatalf("readSessionItems: %v", readErr)
	}
	if len(items) == 0 {
		t.Fatalf("expected recovered items, got none")
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
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

type stubLiveTurnResult struct {
	turnID string
	err    error
}

type stubLiveManager struct {
	startTurnResults []stubLiveTurnResult
	startTurnCalls   int
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
	ch := make(chan types.CodexEvent)
	close(ch)
	return ch, func() {}, nil
}

func (s *stubLiveManager) Respond(context.Context, *types.Session, *types.SessionMeta, int, map[string]any) error {
	return nil
}

func (s *stubLiveManager) Interrupt(context.Context, *types.Session, *types.SessionMeta) error {
	return nil
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

func (a *testConversationAdapter) Provider() string {
	return a.provider
}

func (a *testConversationAdapter) History(context.Context, *SessionService, *types.Session, *types.SessionMeta, string, int) ([]map[string]any, error) {
	return []map[string]any{{"type": "log", "text": "ok"}}, nil
}

func (a *testConversationAdapter) SendMessage(_ context.Context, _ *SessionService, _ *types.Session, meta *types.SessionMeta, _ []map[string]any) (string, error) {
	a.sendCalls++
	if meta != nil {
		a.lastRuntimeOptions = types.CloneRuntimeOptions(meta.RuntimeOptions)
	} else {
		a.lastRuntimeOptions = nil
	}
	a.runtimeOptionsBySend = append(a.runtimeOptionsBySend, types.CloneRuntimeOptions(a.lastRuntimeOptions))
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
