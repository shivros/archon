package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSearchReducerEscExitsSearchMode(t *testing.T) {
	m := NewModel(nil)
	m.enterSearch()

	handled, cmd := m.reduceSearchModeKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected search reducer to handle esc")
	}
	if cmd != nil {
		t.Fatalf("expected no command on esc")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
	if m.status != "search canceled" {
		t.Fatalf("expected cancel status, got %q", m.status)
	}
}

func TestComposeReducerEnterEmptyShowsValidation(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("")
	if m.chatInput != nil {
		m.chatInput.SetValue("   ")
	}

	handled, cmd := m.reduceComposeInputKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected compose reducer to handle enter")
	}
	if cmd != nil {
		t.Fatalf("expected no command for empty message")
	}
	if m.status != "message is required" {
		t.Fatalf("expected validation status, got %q", m.status)
	}
}

func TestMenuReducerEscClosesMenu(t *testing.T) {
	m := NewModel(nil)
	if m.menu == nil {
		t.Fatalf("expected menu controller")
	}
	m.menu.OpenBar()
	if !m.menu.IsActive() {
		t.Fatalf("expected menu to be active")
	}

	handled, _ := m.reduceMenuMode(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected menu reducer to handle esc")
	}
	if m.menu.IsActive() {
		t.Fatalf("expected menu to close on esc")
	}
}

func TestWorkspaceEditReducerRequiresWorkspaceSelection(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModePickWorkspaceRename
	if m.workspacePicker == nil {
		t.Fatalf("expected workspace picker")
	}
	m.workspacePicker.SetOptions(nil)

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle enter")
	}
	if cmd != nil {
		t.Fatalf("expected no command when selection is missing")
	}
	if m.status != "no workspace selected" {
		t.Fatalf("expected missing selection status, got %q", m.status)
	}
}

func TestWorkspaceEditReducerRenameSessionEnterReturnsCommand(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeRenameSession
	m.renameSessionID = "s1"
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.SetValue("Renamed Session")

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle enter")
	}
	if cmd == nil {
		t.Fatalf("expected update session command")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
	if m.status != "renaming session" {
		t.Fatalf("expected renaming status, got %q", m.status)
	}
	if m.renameSessionID != "" {
		t.Fatalf("expected rename session id to clear")
	}
}

func TestWorkspaceEditReducerRenameSessionRequiresSelection(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeRenameSession
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.SetValue("Renamed Session")

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle enter")
	}
	if cmd != nil {
		t.Fatalf("expected no command without session id")
	}
	if m.status != "no session selected" {
		t.Fatalf("expected missing selection status, got %q", m.status)
	}
}

func TestAddWorkspaceReducerHandlesNilController(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeAddWorkspace
	m.addWorkspace = nil

	handled, cmd := m.reduceAddWorkspaceMode(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected add workspace reducer to handle mode")
	}
	if cmd != nil {
		t.Fatalf("expected no command with nil controller")
	}
}

func TestAddWorktreeReducerHandlesStreamMsg(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeAddWorktree

	handled, cmd := m.reduceAddWorktreeMode(streamMsg{})
	if !handled {
		t.Fatalf("expected add worktree reducer to handle stream messages")
	}
	if cmd != nil {
		t.Fatalf("expected no command for stream message")
	}
}

func TestPickProviderReducerEscExits(t *testing.T) {
	m := NewModel(nil)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()

	handled, cmd := m.reducePickProviderMode(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected pick provider reducer to handle esc")
	}
	if cmd != nil {
		t.Fatalf("expected no command on esc")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected normal mode after esc, got %v", m.mode)
	}
	if m.newSession != nil {
		t.Fatalf("expected new session target to clear")
	}
	if m.status != "new session canceled" {
		t.Fatalf("expected cancel status, got %q", m.status)
	}
}

func TestPickProviderReducerEnterSelectsProvider(t *testing.T) {
	m := NewModel(nil)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()

	handled, cmd := m.reducePickProviderMode(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected pick provider reducer to handle enter")
	}
	if cmd == nil {
		t.Fatalf("expected provider options fetch command after selection")
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode after selection, got %v", m.mode)
	}
	if m.newSession == nil || m.newSession.provider == "" {
		t.Fatalf("expected provider to be selected, got %#v", m.newSession)
	}
}
