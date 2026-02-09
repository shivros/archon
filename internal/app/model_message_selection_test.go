package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestMessageSelectionEnterWithV(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "one"},
		{Role: ChatRoleAgent, Text: "two"},
	})

	handled, cmd := m.reduceViewToggleKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
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

	handled, cmd := m.reduceMessageSelectionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if !handled || cmd != nil {
		t.Fatalf("expected j to be handled without command")
	}
	if m.messageSelectIndex != 1 {
		t.Fatalf("expected selected index to move to 1, got %d", m.messageSelectIndex)
	}

	handled, cmd = m.reduceMessageSelectionKey(tea.KeyMsg{Type: tea.KeyEsc})
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

	handled, cmd := m.reduceMessageSelectionKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
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
