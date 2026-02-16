package app

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestConfirmDialogWidthCappedByMaxWidth(t *testing.T) {
	c := NewConfirmController()
	longName := strings.Repeat("extremely-long-workspace-name-", 6)
	c.Open("Delete Workspace", fmt.Sprintf("Delete workspace %q?", longName), "Delete", "Cancel")

	_, _, width, _ := c.layout(200, 40)
	if width != confirmMaxWidth {
		t.Fatalf("expected width %d, got %d", confirmMaxWidth, width)
	}
}

func TestConfirmDialogViewWrapsLongMessageWithinMaxWidth(t *testing.T) {
	c := NewConfirmController()
	longName := strings.Repeat("extremely-long-workspace-name-", 6)
	c.Open("Delete Workspace", fmt.Sprintf("Delete workspace %q?", longName), "Delete", "Cancel")

	view, _ := c.View(confirmMaxWidth, 40)
	plain := xansi.Strip(view)
	lines := strings.Split(plain, "\n")
	if len(lines) <= 6 {
		t.Fatalf("expected wrapped dialog lines, got %d lines: %q", len(lines), plain)
	}

	maxWidth := 0
	for _, line := range lines {
		if w := xansi.StringWidth(line); w > maxWidth {
			maxWidth = w
		}
	}
	if maxWidth > confirmMaxWidth {
		t.Fatalf("expected wrapped lines to fit max width %d, got %d", confirmMaxWidth, maxWidth)
	}
}

func TestConfirmDialogMouseButtonsRespectBorderedLayout(t *testing.T) {
	c := NewConfirmController()
	c.Open("Delete Note", "Delete note \"hello\"?", "Delete", "Cancel")

	x, y, width, height := c.layout(120, 40)
	buttonRow := y + height - 2
	borderRow := y + height - 1

	handled, choice := c.HandleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      x + 2,
		Y:      buttonRow,
	}, 120, 40)
	if !handled || choice != confirmChoiceConfirm {
		t.Fatalf("expected confirm click on button row, handled=%v choice=%v", handled, choice)
	}

	handled, choice = c.HandleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      x + width - 3,
		Y:      buttonRow,
	}, 120, 40)
	if !handled || choice != confirmChoiceCancel {
		t.Fatalf("expected cancel click on button row, handled=%v choice=%v", handled, choice)
	}

	handled, choice = c.HandleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      x + 2,
		Y:      borderRow,
	}, 120, 40)
	if !handled || choice != confirmChoiceNone {
		t.Fatalf("expected bordered row click to be ignored, handled=%v choice=%v", handled, choice)
	}
}
