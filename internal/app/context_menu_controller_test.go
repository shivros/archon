package app

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func findContextMenuLabel(c *ContextMenuController, action ContextMenuAction) string {
	for _, item := range c.items {
		if item.Action == action {
			return item.Label
		}
	}
	return ""
}

func TestContextMenuViewBlockMatchesLayoutGeometry(t *testing.T) {
	c := NewContextMenuController()
	c.OpenWorkspace("ws-1", "Workspace One", 12, 5)

	x, y, width, height := c.layout(120, 40)
	block, bx, by := c.ViewBlock(120, 40)
	if block == "" {
		t.Fatalf("expected non-empty context menu block")
	}
	if bx != x || by != y {
		t.Fatalf("expected view block coordinates to match layout, got (%d,%d) want (%d,%d)", bx, by, x, y)
	}
	plain := xansi.Strip(block)
	lines := strings.Split(plain, "\n")
	if len(lines) != height {
		t.Fatalf("expected context menu height %d, got %d", height, len(lines))
	}
	maxWidth := 0
	for _, line := range lines {
		if w := xansi.StringWidth(line); w > maxWidth {
			maxWidth = w
		}
	}
	if maxWidth != width {
		t.Fatalf("expected context menu width %d, got %d", width, maxWidth)
	}
}

func TestContextMenuContainsUsesComputedBounds(t *testing.T) {
	c := NewContextMenuController()
	c.OpenWorkspace("ws-1", "Workspace One", 12, 5)

	x, y, width, height := c.layout(120, 40)
	if !c.Contains(x, y, 120, 40) {
		t.Fatalf("expected top-left point to be inside context menu")
	}
	if !c.Contains(x+width-1, y+height-1, 120, 40) {
		t.Fatalf("expected bottom-right point to be inside context menu")
	}
	if c.Contains(x-1, y, 120, 40) {
		t.Fatalf("expected left-outside point to be outside context menu")
	}
	if c.Contains(x, y+height, 120, 40) {
		t.Fatalf("expected below-outside point to be outside context menu")
	}
}

func TestContextMenuWorkspaceSelectionCopyUsesSelectedIDsLabel(t *testing.T) {
	c := NewContextMenuController()
	c.OpenWorkspaceSelectionCopy("ws-1", "Workspace One", 12, 5)

	if got := findContextMenuLabel(c, ContextMenuWorkspaceCopyPath); got != "Copy Selected IDs" {
		t.Fatalf("expected selection copy label, got %q", got)
	}
}

func TestContextMenuSessionSelectionCopyUsesSelectedIDsLabel(t *testing.T) {
	c := NewContextMenuController()
	c.OpenSessionSelectionCopy("s-1", "ws-1", "wt-1", "Session One", 12, 5)

	if got := findContextMenuLabel(c, ContextMenuSessionCopyID); got != "Copy Selected IDs" {
		t.Fatalf("expected selection copy label, got %q", got)
	}
}

func TestContextMenuWorkflowSelectionCopyUsesSelectedIDsLabel(t *testing.T) {
	c := NewContextMenuController()
	c.OpenWorkflowSelectionCopy("gwf-1", "Workflow One", "running", false, 12, 5)

	if got := findContextMenuLabel(c, ContextMenuWorkflowCopyID); got != "Copy Selected IDs" {
		t.Fatalf("expected selection copy label, got %q", got)
	}
}
