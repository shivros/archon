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
