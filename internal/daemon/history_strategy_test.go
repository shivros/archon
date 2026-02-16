package daemon

import (
	"context"
	"testing"
	"time"

	"control/internal/types"
)

func TestSessionServiceHistoryDelegatesToRegisteredHistoryStrategy(t *testing.T) {
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
	strategy := &testConversationHistoryStrategy{
		provider: "mock-provider",
		items:    []map[string]any{{"type": "log", "text": "ok"}},
	}
	service := NewSessionService(
		nil,
		&Stores{Sessions: store},
		nil,
		nil,
		WithSessionHistoryStrategies(newConversationHistoryStrategyRegistry(strategy)),
	)

	items, err := service.History(context.Background(), "s1", 50)
	if err != nil {
		t.Fatalf("expected history success, got err=%v", err)
	}
	if strategy.calls != 1 {
		t.Fatalf("expected strategy to be called once, got %d", strategy.calls)
	}
	if len(items) != 1 || items[0]["text"] != "ok" {
		t.Fatalf("unexpected history items: %#v", items)
	}
}

type testConversationHistoryStrategy struct {
	provider string
	calls    int
	items    []map[string]any
}

func (s *testConversationHistoryStrategy) Provider() string {
	return s.provider
}

func (s *testConversationHistoryStrategy) History(context.Context, *SessionService, *types.Session, *types.SessionMeta, string, int) ([]map[string]any, error) {
	s.calls++
	return s.items, nil
}
