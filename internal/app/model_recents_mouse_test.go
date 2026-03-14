package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

type recentsMouseSessionAPI struct {
	stubInterruptSessionAPI
}

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

func TestMouseReducerRecentsInterruptControlClickQueuesInterrupt(t *testing.T) {
	m := setupRecentsMouseModel(t, false)
	sessionAPI := &recentsMouseSessionAPI{}
	m.sessionAPI = sessionAPI
	x, y := findVisualTokenInBody(t, m, "[Interrupt]")

	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !handled {
		t.Fatalf("expected recents interrupt control click to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected recents interrupt to queue a follow-up command")
	}
	msg := m.pendingMouseCmd()
	interrupt, ok := msg.(interruptMsg)
	if !ok {
		t.Fatalf("expected interruptMsg, got %T", msg)
	}
	if interrupt.id != "s1" {
		t.Fatalf("expected interrupt target s1, got %q", interrupt.id)
	}
	if len(sessionAPI.calls) != 1 || sessionAPI.calls[0] != "s1" {
		t.Fatalf("expected interrupt call for s1, got %#v", sessionAPI.calls)
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

func TestMouseReducerRecentsReplyInputWheelDoesNotScrollViewport(t *testing.T) {
	m := setupRecentsMouseModel(t, true)
	if !m.startRecentsReply() {
		t.Fatalf("expected recents reply to start")
	}
	layout := m.resolveMouseLayout()
	m.viewport.SetContent(strings.Repeat("line\n", 220))
	m.viewport.SetYOffset(20)
	beforeOffset := m.viewport.YOffset()
	y := m.viewport.Height() + 2

	handled := m.reduceMouseWheel(tea.MouseClickMsg{Button: tea.MouseWheelUp, X: layout.rightStart, Y: y}, layout, -1)
	if !handled {
		t.Fatalf("expected wheel event over recents input to be handled")
	}
	if got := m.viewport.YOffset(); got != beforeOffset {
		t.Fatalf("expected recents input wheel to avoid viewport scroll, got %d want %d", got, beforeOffset)
	}
}

func TestMouseReducerRecentsReplyInputClickFocusesInput(t *testing.T) {
	m := setupRecentsMouseModel(t, true)
	if !m.startRecentsReply() {
		t.Fatalf("expected recents reply to start")
	}
	if m.recentsReplyInput == nil || m.input == nil {
		t.Fatalf("expected recents input controllers")
	}
	m.recentsReplyInput.Blur()
	m.input.FocusSidebar()
	layout := m.resolveMouseLayout()
	y := m.viewport.Height() + 2

	handled := m.reduceInputFocusLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: y}, layout)
	if !handled {
		t.Fatalf("expected recents input click to be handled")
	}
	if !m.recentsReplyInput.Focused() {
		t.Fatalf("expected recents reply input to be focused")
	}
	if !m.input.IsChatFocused() {
		t.Fatalf("expected input controller focus to switch to chat")
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
