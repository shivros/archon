package app

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/client"
	"control/internal/types"
)

func TestDefaultHistoryLoadPolicyUsesSinglePassLines(t *testing.T) {
	m := NewModel(nil)
	if got := m.historyFetchLinesInitial(); got != defaultInitialHistoryLines {
		t.Fatalf("expected initial history lines %d, got %d", defaultInitialHistoryLines, got)
	}
}

func TestLoadSelectedSessionSkipsHistoryBackfillByDefault(t *testing.T) {
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
	if len(batch) != 4 {
		t.Fatalf("expected 4 commands (history, approvals, stream/items, events), got %d", len(batch))
	}
}

func TestStartSessionClearsPreviousContentAndSkipsBackfillCommand(t *testing.T) {
	m := NewModel(nil)
	m.setSnapshotBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "old reply should clear"}})

	session := &types.Session{
		ID:       "s1",
		Provider: "codex",
		Status:   types.SessionStatusRunning,
		Title:    "Session",
	}
	handled, cmd := m.reduceStateMessages(startSessionMsg{session: session})
	if !handled {
		t.Fatalf("expected start session message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected follow-up commands for started session")
	}
	if blocks := m.currentBlocks(); len(blocks) != 0 {
		t.Fatalf("expected previous transcript blocks to clear, got %#v", blocks)
	}
	if raw := strings.Join(m.currentLines(), "\n"); strings.Contains(raw, "old reply should clear") {
		t.Fatalf("expected previous transcript text to clear, got %q", raw)
	}
	if !m.loading {
		t.Fatalf("expected loading to start")
	}
	if m.loadingKey != "sess:s1" {
		t.Fatalf("expected loading key sess:s1, got %q", m.loadingKey)
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected batch command, got %T", msg)
	}
	expected := 2 // fetch sessions + initial history
	if shouldStreamItems(session.Provider) {
		expected += 2 // approvals + items stream
	} else if isActiveStatus(session.Status) {
		if session.Provider == "codex" {
			expected++ // events stream
		} else {
			expected++ // log stream
		}
	}
	if session.Provider == "codex" {
		expected++ // history polling safety refresh
	}
	expected++ // recents state save debounce
	if len(batch) != expected {
		t.Fatalf("expected %d start-session commands without backfill, got %d", expected, len(batch))
	}
}

func TestApplyProviderSelectionClearsPreviousContent(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "old reply should clear"}})
	m.newSession = &newSessionTarget{workspaceID: "ws1"}

	cmd := m.applyProviderSelection("codex")
	if cmd == nil {
		t.Fatalf("expected provider options fetch command")
	}
	lines := strings.Join(m.currentLines(), "\n")
	if strings.Contains(lines, "old reply should clear") {
		t.Fatalf("expected previous transcript text to clear, got %q", lines)
	}
	if !strings.Contains(lines, "New session. Send your first message to start.") {
		t.Fatalf("expected new-session placeholder, got %q", lines)
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode, got %v", m.mode)
	}
}

func TestActiveStreamTargetIDEmptyDuringNewSession(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.newSession = &newSessionTarget{
		workspaceID: "ws1",
		provider:    "codex",
	}

	if got := m.activeStreamTargetID(); got != "" {
		t.Fatalf("expected empty stream target while new session is pending, got %q", got)
	}
}

func TestHistoryPollUsesInitialHistoryLines(t *testing.T) {
	history := &historyRecorderAPI{}
	m := newPhase0ModelWithSession("codex")
	m.sessionHistoryAPI = history
	m.mode = uiModeCompose
	if m.compose != nil {
		m.compose.Enter("s1", "Session")
	}

	handled, cmd := m.reduceStateMessages(historyPollMsg{
		id:        "s1",
		key:       "sess:s1",
		attempt:   0,
		minAgents: -1,
	})
	if !handled {
		t.Fatalf("expected history poll message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected history poll command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("expected batch command, got %T", msg)
	}
	_ = batch[0]()
	if len(history.calls) != 1 || history.calls[0] != defaultInitialHistoryLines {
		t.Fatalf("expected one history call with %d lines, got %#v", defaultInitialHistoryLines, history.calls)
	}
}

type historyRecorderAPI struct {
	calls []int
}

func (h *historyRecorderAPI) History(_ context.Context, _ string, lines int) (*client.TailItemsResponse, error) {
	h.calls = append(h.calls, lines)
	return &client.TailItemsResponse{Items: []map[string]any{}}, nil
}
