package app

import (
	"strings"
	"testing"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestMessageSelectionEnterWithV(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "one"},
		{Role: ChatRoleAgent, Text: "two"},
	})

	handled, cmd := m.reduceViewToggleKeys(tea.KeyPressMsg{Text: "v"})
	if !handled {
		t.Fatalf("expected v to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command")
	}
	if !m.messageSelectActive {
		t.Fatalf("expected message selection to be active")
	}
	if m.messageSelectIndex < 0 || m.messageSelectIndex >= len(m.contentBlocks) {
		t.Fatalf("unexpected selected index %d", m.messageSelectIndex)
	}
	if !strings.Contains(m.status, "selected") {
		t.Fatalf("expected selected status, got %q", m.status)
	}
}

func TestMessageSelectionMoveAndExit(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "one"},
		{Role: ChatRoleAgent, Text: "two"},
	})
	m.enterMessageSelection()
	m.messageSelectIndex = 0

	handled, cmd := m.reduceMessageSelectionKey(tea.KeyPressMsg{Text: "j"})
	if !handled || cmd != nil {
		t.Fatalf("expected j to be handled without command")
	}
	if m.messageSelectIndex != 1 {
		t.Fatalf("expected selected index to move to 1, got %d", m.messageSelectIndex)
	}

	handled, cmd = m.reduceMessageSelectionKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled || cmd != nil {
		t.Fatalf("expected esc to be handled without command")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to deactivate")
	}
}

func TestMessageSelectionCopyUsesPlainText(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "   ", Status: ChatStatusSending}})
	m.enterMessageSelection()
	m.messageSelectIndex = 0

	handled, cmd := m.reduceMessageSelectionKey(tea.KeyPressMsg{Text: "y"})
	if !handled || cmd != nil {
		t.Fatalf("expected y to be handled without command")
	}
	if m.status != "nothing to copy" {
		t.Fatalf("expected plain-text copy path status, got %q", m.status)
	}
}

func TestMessageSelectionRenderShowsVisibleMarker(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "hello"}})
	before := xansi.Strip(m.renderedText)
	if strings.Contains(before, "Selected") {
		t.Fatalf("did not expect selected marker before entering mode: %q", before)
	}

	m.enterMessageSelection()
	after := xansi.Strip(m.renderedText)
	if !strings.Contains(after, "Selected") {
		t.Fatalf("expected selected marker after entering mode: %q", after)
	}
}

func TestMessageSelectionExitUsesRemappedToggleCommand(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "one"},
		{Role: ChatRoleAgent, Text: "two"},
	})
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandToggleMessageSelect: "ctrl+j",
	}))
	m.enterMessageSelection()

	handled, cmd := m.reduceMessageSelectionKey(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected remapped toggle command to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to deactivate")
	}
}

func TestMessageSelectionEnterExitsToCompose(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	if m.sidebar != nil {
		m.sidebar.SelectBySessionID("s1")
	}
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "one"},
		{Role: ChatRoleAgent, Text: "two"},
	})
	m.enterMessageSelection()
	if !m.messageSelectActive {
		t.Fatalf("expected message selection to be active")
	}
	sessionID := m.selectedSessionID()
	if sessionID != "s1" {
		t.Fatalf("expected selected session s1, got %q", sessionID)
	}

	handled, _ := m.reduceMessageSelectionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to be deactivated")
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode after enter, got %d", m.mode)
	}
}

func TestMessageSelectionQuitUsesRemappedQuitCommand(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "two"},
	})
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandQuit: "ctrl+q",
	}))
	m.enterMessageSelection()

	handled, cmd := m.reduceMessageSelectionKey(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected remapped quit command to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg from command")
	}
}
