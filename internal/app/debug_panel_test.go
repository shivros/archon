package app

import (
	"strings"
	"testing"
)

type testDebugPanelHeaderRenderer struct{}

func (testDebugPanelHeaderRenderer) RenderHeader(title string) string {
	return "[[" + title + "]]"
}

func TestDebugPanelControllerUsesInjectedRenderer(t *testing.T) {
	panel := NewDebugPanelController(24, 4, testDebugPanelHeaderRenderer{})
	panel.SetContent("line")
	view, _ := panel.View()
	if !strings.Contains(view, "[[Debug]]") {
		t.Fatalf("expected injected header renderer output, got %q", view)
	}
}

func TestDebugPanelControllerDefaultRendererAndCaching(t *testing.T) {
	panel := NewDebugPanelController(24, 4, nil)
	panel.SetContent("line one")
	first, firstHeight := panel.View()
	second, secondHeight := panel.View()
	if first != second || firstHeight != secondHeight {
		t.Fatalf("expected stable cached view, got first=%q second=%q", first, second)
	}
	if !strings.Contains(first, "Debug") {
		t.Fatalf("expected default header rendering to include title, got %q", first)
	}
}

func TestDebugPanelControllerNoopBranchesAndNilReceiver(t *testing.T) {
	panel := NewDebugPanelController(24, 4, testDebugPanelHeaderRenderer{})
	panel.SetContent("value")
	before, _ := panel.View()

	panel.SetContent("value")
	panel.Resize(24, 4)
	after, _ := panel.View()
	if before != after {
		t.Fatalf("expected no-op content/resize to keep cached view stable")
	}

	var nilPanel *DebugPanelController
	nilPanel.SetContent("x")
	nilPanel.Resize(10, 2)
	view, height := nilPanel.View()
	if view != "" || height != 0 {
		t.Fatalf("expected nil panel view to be empty, got view=%q height=%d", view, height)
	}
}

func TestDebugPanelControllerNormalizesEmptyContent(t *testing.T) {
	panel := NewDebugPanelController(24, 4, testDebugPanelHeaderRenderer{})
	panel.SetContent("   ")
	view, _ := panel.View()
	if !strings.Contains(view, "Waiting for debug stream") {
		t.Fatalf("expected empty content to normalize to waiting message, got %q", view)
	}
}

func TestDebugPanelControllerScrollsAndWrapsLongLines(t *testing.T) {
	panel := NewDebugPanelController(14, 3, testDebugPanelHeaderRenderer{})
	panel.SetContent("line-1\nline-2\nline-3\nline-4\nline-5")
	if !panel.ScrollDown(2) {
		t.Fatalf("expected vertical scroll down to move viewport")
	}
	if !panel.ScrollUp(1) {
		t.Fatalf("expected vertical scroll up to move viewport")
	}

	panel.SetContent("abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	view, _ := panel.View()
	if !strings.Contains(view, "\n") {
		t.Fatalf("expected long line content to wrap in panel view")
	}
	if panel.ScrollRight(8) {
		t.Fatalf("expected no horizontal scroll movement when soft wrapping is enabled")
	}
	if panel.ScrollLeft(8) {
		t.Fatalf("expected no horizontal scroll movement when soft wrapping is enabled")
	}
}

func TestDebugPanelControllerPageAndGotoNavigation(t *testing.T) {
	panel := NewDebugPanelController(20, 3, testDebugPanelHeaderRenderer{})
	panel.SetContent("a\nb\nc\nd\ne\nf")

	if !panel.PageDown() {
		t.Fatalf("expected page down to move viewport")
	}
	if !panel.PageUp() {
		t.Fatalf("expected page up to move viewport")
	}
	if !panel.GotoBottom() {
		t.Fatalf("expected goto bottom to move viewport")
	}
	if !panel.GotoTop() {
		t.Fatalf("expected goto top to move viewport")
	}
	if panel.Height() != 3 {
		t.Fatalf("expected viewport height 3, got %d", panel.Height())
	}
	if panel.YOffset() != 0 {
		t.Fatalf("expected goto top to reset vertical offset, got %d", panel.YOffset())
	}
}
