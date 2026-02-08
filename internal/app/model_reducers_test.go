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
