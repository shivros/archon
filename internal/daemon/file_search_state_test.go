package daemon

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestApplyFileSearchRuntimeEventMarksTerminalFailedState(t *testing.T) {
	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)
	current := &types.FileSearchSession{
		ID:       "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex"},
		Query:    "main",
		Limit:    5,
		Status:   types.FileSearchStatusActive,
	}
	next, event, terminal := applyFileSearchRuntimeEvent(current, types.FileSearchEvent{
		Kind:     types.FileSearchEventFailed,
		SearchID: "fs-1",
		Error:    "boom",
	}, now)
	if !terminal {
		t.Fatalf("expected failed event to be terminal")
	}
	if next.Status != types.FileSearchStatusFailed || next.ClosedAt == nil {
		t.Fatalf("unexpected next session: %#v", next)
	}
	if event.Status != types.FileSearchStatusFailed || event.OccurredAt == nil || !event.OccurredAt.Equal(now) {
		t.Fatalf("unexpected normalized event: %#v", event)
	}
}

func TestNewFileSearchSessionCoalescesSnapshotDefaults(t *testing.T) {
	createdAt := time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC)
	session := newFileSearchSession(&types.FileSearchSession{
		Query:  "snapshot-query",
		Status: types.FileSearchStatusCreated,
	}, "fs-1", "codex", types.FileSearchScope{Provider: "codex"}, "fallback", 7, createdAt)
	if session.ID != "fs-1" || session.Provider != "codex" || session.Query != "snapshot-query" || session.Limit != 7 || session.CreatedAt != createdAt {
		t.Fatalf("unexpected session: %#v", session)
	}
}

func TestApplyFileSearchCommandUpdatePromotesCreatedToActive(t *testing.T) {
	now := time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC)
	query := "next"
	limit := 8
	next, event := applyFileSearchCommandUpdate(&types.FileSearchSession{
		ID:       "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex"},
		Status:   types.FileSearchStatusCreated,
	}, types.FileSearchUpdateRequest{
		Query: &query,
		Limit: &limit,
	}, nil, now)
	if next.Status != types.FileSearchStatusActive || next.Query != "next" || next.Limit != 8 || next.UpdatedAt == nil || !next.UpdatedAt.Equal(now) {
		t.Fatalf("unexpected updated session: %#v", next)
	}
	if event.Kind != types.FileSearchEventUpdated || event.Status != types.FileSearchStatusActive {
		t.Fatalf("unexpected update event: %#v", event)
	}
}

func TestApplyFileSearchCloseMarksClosed(t *testing.T) {
	now := time.Date(2026, 3, 27, 12, 30, 0, 0, time.UTC)
	next, event := applyFileSearchClose(&types.FileSearchSession{
		ID:       "fs-1",
		Provider: "codex",
		Status:   types.FileSearchStatusActive,
	}, now)
	if next.Status != types.FileSearchStatusClosed || next.ClosedAt == nil || !next.ClosedAt.Equal(now) {
		t.Fatalf("unexpected closed session: %#v", next)
	}
	if event.Kind != types.FileSearchEventClosed || event.Status != types.FileSearchStatusClosed {
		t.Fatalf("unexpected close event: %#v", event)
	}
}

func TestNormalizeFileSearchEventFallsBackToSessionAndKind(t *testing.T) {
	now := time.Date(2026, 3, 27, 13, 0, 0, 0, time.UTC)
	event := normalizeFileSearchEvent(types.FileSearchEvent{
		Kind: types.FileSearchEventResults,
	}, &types.FileSearchSession{
		ID:       "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex", SessionID: "sess-1"},
		Query:    "main",
		Limit:    6,
		Status:   types.FileSearchStatusCreated,
	}, now)
	if event.SearchID != "fs-1" || event.Provider != "codex" || event.Query != "main" || event.Limit != 6 || event.Status != types.FileSearchStatusActive {
		t.Fatalf("unexpected normalized event: %#v", event)
	}
}

func TestCoalesceFileSearchSessionUsesFallbackValues(t *testing.T) {
	createdAt := time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC)
	merged := coalesceFileSearchSession(&types.FileSearchSession{
		Query: "query",
	}, &types.FileSearchSession{
		ID:        "fs-1",
		Provider:  "codex",
		Scope:     types.FileSearchScope{Provider: "codex"},
		Limit:     4,
		Status:    types.FileSearchStatusActive,
		CreatedAt: createdAt,
	})
	if merged.ID != "fs-1" || merged.Provider != "codex" || merged.Limit != 4 || merged.CreatedAt != createdAt {
		t.Fatalf("unexpected merged session: %#v", merged)
	}
}
