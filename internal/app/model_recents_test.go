package app

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/types"
)

func TestApplySelectionStateEntersRecentsMode(t *testing.T) {
	m := NewModel(nil)
	handled, _, _ := m.applySelectionState(&sidebarItem{kind: sidebarRecentsAll})
	if !handled {
		t.Fatalf("expected recents selection to be handled")
	}
	if m.mode != uiModeRecents {
		t.Fatalf("expected recents mode, got %v", m.mode)
	}
	if !strings.Contains(m.contentRaw, "Recents overview") {
		t.Fatalf("expected recents content to render, got %q", m.contentRaw)
	}
}

func TestDismissSelectedRecentsReadyRemovesQueueItem(t *testing.T) {
	m := NewModel(nil)
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
	m.recents.ObserveMeta(m.sessionMeta, now)
	m.mode = uiModeRecents
	m.recentsSelectedSessionID = "s1"

	if !m.dismissSelectedRecentsReady() {
		t.Fatalf("expected dismiss to succeed")
	}
	if m.recents.IsReady("s1") {
		t.Fatalf("expected s1 to be removed from ready queue")
	}
}

func TestRecentsCardRendersControlsAboveBubble(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.showRecents = true
	m.width = 120
	m.height = 40
	m.viewport.SetWidth(90)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
	}
	m.recents.StartRun("s1", "turn-0", now.Add(-time.Minute))
	m.recentsSelectedSessionID = "s1"
	m.recentsPreviews = map[string]recentsPreview{
		"s1": {Revision: "turn-1", Preview: "assistant preview"},
	}
	state := m.recentsState()
	rendered := xansi.Strip(m.renderRecentsContent(state))
	replyIndex := strings.Index(rendered, "[Reply]")
	bubbleIndex := strings.Index(rendered, "assistant preview")
	if replyIndex < 0 {
		t.Fatalf("expected reply control in recents card, got %q", rendered)
	}
	if bubbleIndex < 0 {
		t.Fatalf("expected assistant preview text in recents bubble, got %q", rendered)
	}
	if replyIndex > bubbleIndex {
		t.Fatalf("expected controls above bubble text, got %q", rendered)
	}
}

func TestRecentsTurnCompletedMessageMovesRunToReady(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", LastTurnID: "turn-42"},
	}
	m.recents.StartRun("s1", "turn-42", now)
	m.recentsCompletionWatching["s1"] = "turn-42"

	handled, cmd := m.reduceStateMessages(recentsTurnCompletedMsg{
		id:           "s1",
		expectedTurn: "turn-42",
		turnID:       "turn-42",
	})
	if !handled {
		t.Fatalf("expected recents completion message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command")
	}
	if m.recents.IsRunning("s1") {
		t.Fatalf("expected s1 to leave running after completion")
	}
	if !m.recents.IsReady("s1") {
		t.Fatalf("expected s1 to move into ready")
	}
	if _, watching := m.recentsCompletionWatching["s1"]; watching {
		t.Fatalf("expected completion watcher to clear")
	}
}
