package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

func TestMouseReducerRecentsReplyControlClickStartsInlineReply(t *testing.T) {
	m := setupRecentsMouseModel(t, true)
	x, y := findVisualTokenInBody(t, m, "[Reply]")

	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !handled {
		t.Fatalf("expected recents reply control click to be handled")
	}
	if got := strings.TrimSpace(m.recentsReplySessionID); got != "s1" {
		t.Fatalf("expected inline reply target s1, got %q", got)
	}
}

func TestMouseReducerRecentsExpandControlClickTogglesSession(t *testing.T) {
	m := setupRecentsMouseModel(t, true)
	x, y := findVisualTokenInBody(t, m, "[Expand]")

	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !handled {
		t.Fatalf("expected recents expand control click to be handled")
	}
	if !m.recentsExpandedSessions["s1"] {
		t.Fatalf("expected s1 recents card to be expanded")
	}
}

func TestMouseReducerRecentsDismissControlClickRemovesReadyItem(t *testing.T) {
	m := setupRecentsMouseModel(t, true)
	x, y := findVisualTokenInBody(t, m, "[Dismiss]")

	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !handled {
		t.Fatalf("expected recents dismiss control click to be handled")
	}
	if m.recents.IsReady("s1") {
		t.Fatalf("expected s1 to be dismissed from ready")
	}
}

func TestMouseReducerRecentsOpenControlClickQueuesSelectionCommand(t *testing.T) {
	m := setupRecentsMouseModel(t, false)
	x, y := findVisualTokenInBody(t, m, "[Open]")

	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !handled {
		t.Fatalf("expected recents open control click to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected recents open to queue a follow-up command")
	}
}

func setupRecentsMouseModel(t *testing.T, ready bool) *Model {
	t.Helper()
	m := NewModel(nil)
	m.resize(120, 40)
	now := time.Now().UTC()
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-a1"},
	}
	m.recents.StartRun("s1", "turn-u1", now.Add(-time.Minute))
	if ready {
		m.recents.ObserveMeta(m.sessionMeta, now)
	}
	m.recentsPreviews = map[string]recentsPreview{
		"s1": {Revision: "turn-a1", Preview: "assistant preview", Full: "assistant preview"},
	}
	m.applySidebarItems()
	m.mode = uiModeRecents
	m.recentsSelectedSessionID = "s1"
	m.refreshRecentsContent()
	return &m
}
