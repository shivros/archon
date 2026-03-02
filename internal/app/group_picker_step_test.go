package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestGroupPickerStepSwallowsToggleSidebarHotkey(t *testing.T) {
	picker := NewGroupPicker(40, 5)
	picker.SetGroups(nil, nil)
	status := ""
	canceled := false
	confirmed := false
	h := groupPickerStepHandler{
		picker:    picker,
		keys:      &stubKeyResolver{toggleSidebarMatch: true},
		setStatus: func(s string) { status = s },
		onCancel:  func() { canceled = true },
		onConfirm: func() tea.Cmd {
			confirmed = true
			return nil
		},
	}

	handled, cmd := h.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected toggle sidebar key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for swallowed toggle sidebar key")
	}
	if canceled || confirmed {
		t.Fatalf("expected no cancel/confirm actions for swallowed key")
	}
	if status != "" {
		t.Fatalf("expected no status update, got %q", status)
	}
}

func TestGroupPickerStepHandlesNonKeyNonPasteAsNoOp(t *testing.T) {
	h := groupPickerStepHandler{
		picker:    NewGroupPicker(40, 5),
		keys:      &stubKeyResolver{},
		setStatus: func(string) {},
		onCancel:  func() {},
		onConfirm: func() tea.Cmd { return nil },
	}

	handled, cmd := h.Update(struct{}{})
	if !handled {
		t.Fatalf("expected non-key message to be handled as no-op")
	}
	if cmd != nil {
		t.Fatalf("expected no command for non-key message")
	}
}
