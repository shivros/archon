package app

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/client"
)

func TestDefaultHistoryLoadPolicyUsesProgressiveLines(t *testing.T) {
	m := NewModel(nil)
	if got := m.historyFetchLinesInitial(); got != defaultInitialHistoryLines {
		t.Fatalf("expected initial history lines %d, got %d", defaultInitialHistoryLines, got)
	}
	if got := m.historyFetchLinesBackfill(); got != maxViewportLines {
		t.Fatalf("expected backfill lines %d, got %d", maxViewportLines, got)
	}
}

func TestHistoryBackfillMsgFetchesConfiguredLines(t *testing.T) {
	history := &historyRecorderAPI{}
	m := NewModel(nil)
	m.sessionHistoryAPI = history
	m.pendingSessionKey = "sess:s1"

	handled, cmd := m.reduceStateMessages(historyBackfillMsg{id: "s1", key: "sess:s1", lines: 777})
	if !handled {
		t.Fatalf("expected history backfill message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected history fetch command")
	}
	msg, ok := cmd().(historyMsg)
	if !ok {
		t.Fatalf("expected historyMsg, got %T", cmd())
	}
	if msg.key != "sess:s1" {
		t.Fatalf("expected history key sess:s1, got %q", msg.key)
	}
	if len(history.calls) != 1 || history.calls[0] != 777 {
		t.Fatalf("expected one history call with 777 lines, got %#v", history.calls)
	}
}

func TestHistoryBackfillMsgIgnoresStaleSelection(t *testing.T) {
	history := &historyRecorderAPI{}
	m := NewModel(nil)
	m.sessionHistoryAPI = history
	m.pendingSessionKey = "sess:other"

	handled, cmd := m.reduceStateMessages(historyBackfillMsg{id: "s1", key: "sess:s1", lines: 777})
	if !handled {
		t.Fatalf("expected history backfill message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for stale selection")
	}
	if len(history.calls) != 0 {
		t.Fatalf("expected no history calls, got %#v", history.calls)
	}
}

func TestLoadSelectedSessionSchedulesHistoryBackfill(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	item := m.selectedItem()
	if item == nil || item.session == nil {
		t.Fatalf("expected selected session item")
	}

	cmd := m.loadSelectedSession(item)
	if cmd == nil {
		t.Fatalf("expected load command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected load batch, got %T", msg)
	}
	if len(batch) != 5 {
		t.Fatalf("expected 5 commands (history, backfill, approvals, stream, events), got %d", len(batch))
	}
}

type historyRecorderAPI struct {
	calls []int
}

func (h *historyRecorderAPI) History(_ context.Context, _ string, lines int) (*client.TailItemsResponse, error) {
	h.calls = append(h.calls, lines)
	return &client.TailItemsResponse{Items: []map[string]any{}}, nil
}
