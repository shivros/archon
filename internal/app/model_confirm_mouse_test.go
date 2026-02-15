package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

func TestConfirmDialogIgnoresMouseReleaseOutside(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	if m.confirm == nil {
		t.Fatalf("expected confirm controller")
	}
	m.pendingConfirm = confirmAction{kind: confirmDeleteNote, noteID: "n1"}
	m.confirm.Open("Delete Note", "Delete note?", "Delete", "Cancel")

	nextModel, cmd := m.Update(tea.MouseReleaseMsg{
		Button: tea.MouseLeft,
		X:      0,
		Y:      0,
	})
	if cmd != nil {
		t.Fatalf("expected no command for release")
	}
	next := asModel(t, nextModel)

	if next.confirm == nil || !next.confirm.IsOpen() {
		t.Fatalf("expected confirm dialog to remain open on release")
	}
	if next.pendingConfirm.kind != confirmDeleteNote || next.pendingConfirm.noteID != "n1" {
		t.Fatalf("expected pending confirmation to remain unchanged, got %#v", next.pendingConfirm)
	}
}

func TestNotesPanelDeleteConfirmPersistsAfterMouseRelease(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(120, 40)
	m.notes = []*types.Note{
		{
			ID:        "n1",
			Scope:     types.NoteScopeSession,
			SessionID: "s1",
			Title:     "delete me",
		},
	}
	m.notesPanelBlocks = []ChatBlock{
		{ID: "n1", Role: ChatRoleSessionNote, Text: "delete me"},
	}
	m.notesPanelViewport.SetWidth(80)
	m.notesPanelWidth = 80
	m.renderNotesPanel()
	if len(m.notesPanelSpans) != 1 {
		t.Fatalf("expected notes panel spans, got %d", len(m.notesPanelSpans))
	}
	span := m.notesPanelSpans[0]
	if span.DeleteLine < 0 || span.DeleteStart < 0 {
		t.Fatalf("expected delete hitbox on panel block, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	if !layout.panelVisible {
		t.Fatalf("expected panel to be visible")
	}
	x := layout.panelStart + span.DeleteStart
	y := span.DeleteLine - m.notesPanelViewport.YOffset() + 1

	nextModel, _ := m.Update(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      x,
		Y:      y,
	})
	next := asModel(t, nextModel)
	if next.confirm == nil || !next.confirm.IsOpen() {
		t.Fatalf("expected confirm dialog to open after delete click")
	}
	if next.pendingConfirm.kind != confirmDeleteNote || next.pendingConfirm.noteID != "n1" {
		t.Fatalf("unexpected pending confirm after click: %#v", next.pendingConfirm)
	}

	nextModel, _ = next.Update(tea.MouseReleaseMsg{
		Button: tea.MouseLeft,
		X:      x,
		Y:      y,
	})
	next = asModel(t, nextModel)
	if next.confirm == nil || !next.confirm.IsOpen() {
		t.Fatalf("expected confirm dialog to stay open after release")
	}
	if next.pendingConfirm.kind != confirmDeleteNote || next.pendingConfirm.noteID != "n1" {
		t.Fatalf("unexpected pending confirm after release: %#v", next.pendingConfirm)
	}
}
