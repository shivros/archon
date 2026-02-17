package app

import (
	"strings"
	"testing"
	"time"

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
