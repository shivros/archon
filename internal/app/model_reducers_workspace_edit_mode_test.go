package app

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestReduceWorkspaceEditModesHandlesStreamMessages(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeEditWorkspace

	handled, cmd := m.reduceWorkspaceEditModes(streamMsg{err: errors.New("boom")})
	if !handled {
		t.Fatalf("expected stream message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for stream message")
	}
	if m.status != "stream error: boom" {
		t.Fatalf("unexpected status: %q", m.status)
	}
}

func TestReduceWorkspaceEditModesNoControllerNoop(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeEditWorkspace
	m.editWorkspace = nil

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected edit mode reducer to handle key without controller")
	}
	if cmd != nil {
		t.Fatalf("expected no command when edit controller is missing")
	}
}
