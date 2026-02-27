package daemon

import (
	"context"
	"testing"
	"time"

	"control/internal/types"
)

type testHistoryConversationAdapter struct {
	provider string
	calls    int
	items    []map[string]any
}

func (a *testHistoryConversationAdapter) Provider() string { return a.provider }

func (a *testHistoryConversationAdapter) History(context.Context, historyDeps, *types.Session, *types.SessionMeta, int) ([]map[string]any, error) {
	a.calls++
	return a.items, nil
}

func TestSessionServiceHistoryDelegatesToRegisteredConversationAdapter(t *testing.T) {
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
	adapter := &testHistoryConversationAdapter{
		provider: "mock-provider",
		items:    []map[string]any{{"type": "log", "text": "ok"}},
	}
	service := NewSessionService(
		nil,
		&Stores{Sessions: store},
		nil,
		WithConversationAdapters(newConversationAdapterRegistry(adapter)),
	)

	items, err := service.History(context.Background(), "s1", 50)
	if err != nil {
		t.Fatalf("expected history success, got err=%v", err)
	}
	if adapter.calls != 1 {
		t.Fatalf("expected adapter history to be called once, got %d", adapter.calls)
	}
	if len(items) != 1 || items[0]["text"] != "ok" {
		t.Fatalf("unexpected history items: %#v", items)
	}
}
