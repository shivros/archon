package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestReduceSettingsMenuMouseAndNonKeyBranches(t *testing.T) {
	m := NewModel(nil)
	m.settingsMenu.Open()

	handled, cmd := m.reduceSettingsMenu(tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: 1})
	if !handled {
		t.Fatalf("expected mouse to be handled while settings menu is open")
	}
	if cmd != nil {
		t.Fatalf("expected no command for mouse branch")
	}

	handled, cmd = m.reduceSettingsMenu(tea.WindowSizeMsg{Width: 120, Height: 40})
	if handled {
		t.Fatalf("expected non-key/non-mouse message to fall through")
	}
	if cmd != nil {
		t.Fatalf("expected no command for fallthrough branch")
	}
}

func TestReduceSettingsMenuQuitClosesDebugStream(t *testing.T) {
	m := NewModel(nil)
	stream := &fakeDebugStreamViewModel{}
	m.debugStream = stream
	m.settingsMenu.Open()
	_, _ = m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyDown})
	handled, cmd := m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
	if !stream.closedByClose {
		t.Fatalf("expected debug stream to close on settings quit")
	}
}
