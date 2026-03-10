package app

import (
	"fmt"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestOverlayTransientViewsSettingsMenuPreservesColumnsOutsideBounds(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.menu = nil
	m.settingsMenu.Open()

	body := testOverlayBody(m.width, 40)
	bodyHeight := len(strings.Split(body, "\n"))
	block, x, y := m.settingsMenuPresenter.View(m.settingsMenu, m.width, bodyHeight, nil)
	if block == "" {
		t.Fatalf("expected settings menu block")
	}

	out := m.overlayTransientViews(body)
	assertOverlayPreservesOutsideBounds(t, body, out, block, x, y)
}

func TestOverlayTransientViewsConfirmDialogPreservesColumnsOutsideBounds(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.menu = nil
	m.confirm.Open("Delete Note", "Delete note?", "Delete", "Cancel")

	body := testOverlayBody(m.width, 40)
	bodyHeight := len(strings.Split(body, "\n"))
	block, x, y := m.confirm.ViewBlock(m.width, bodyHeight)
	if block == "" {
		t.Fatalf("expected confirm dialog block")
	}

	out := m.overlayTransientViews(body)
	assertOverlayPreservesOutsideBounds(t, body, out, block, x, y)
}

func TestOverlayTransientViewsContextMenuPreservesColumnsOutsideBounds(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.menu = nil
	m.contextMenu.OpenWorkspace("ws-1", "Workspace One", 26, 8)

	body := testOverlayBody(m.width, 40)
	bodyHeight := len(strings.Split(body, "\n"))
	block, x, y := m.contextMenu.ViewBlock(m.width, bodyHeight)
	if block == "" {
		t.Fatalf("expected context menu block")
	}

	out := m.overlayTransientViews(body)
	assertOverlayPreservesOutsideBounds(t, body, out, block, x, y)
}

func TestOverlayTransientViewsComposeOptionPopupPreservesColumnsOutsideBounds(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.menu = nil
	m.mode = uiModeCompose
	m.appState.SidebarCollapsed = false
	m.newSession = &newSessionTarget{provider: "codex"}
	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected compose option picker to open")
	}
	block, x, y := m.composeOptionPopupPlacement()
	if block == "" {
		t.Fatalf("expected compose option popup block")
	}
	height := max(40, y+len(strings.Split(block, "\n"))+2)
	body := testOverlayBody(m.width, height)

	out := m.overlayTransientViews(body)
	assertOverlayPreservesOutsideBounds(t, body, out, block, x, y)
}

func TestOverlayTransientViewsStatusHistoryPreservesColumnsOutsideBounds(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.menu = nil
	m.statusHistory.Append("first status")
	m.statusHistory.Append("second status")
	m.statusHistoryOverlay.Open()

	body := testOverlayBody(m.width, 40)
	bodyHeight := len(strings.Split(body, "\n"))
	block, x, y, ok := m.statusHistoryOverlayView(bodyHeight)
	if !ok || block == "" {
		t.Fatalf("expected status history overlay block")
	}

	out := m.overlayTransientViews(body)
	assertOverlayPreservesOutsideBounds(t, body, out, block, x, y)
}

func testOverlayBody(width, height int) string {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	chunk := "abcdefghijklmnopqrstuvwxyz0123456789"
	lines := make([]string, height)
	for i := 0; i < height; i++ {
		line := fmt.Sprintf("row%02d-", i)
		for len(line) < width {
			line += chunk
		}
		lines[i] = line[:width]
	}
	return strings.Join(lines, "\n")
}

func assertOverlayPreservesOutsideBounds(t *testing.T, body, rendered, block string, x, y int) {
	t.Helper()
	baseLines := strings.Split(body, "\n")
	renderedLines := strings.Split(xansi.Strip(rendered), "\n")
	overlayLines := strings.Split(xansi.Strip(block), "\n")
	if len(baseLines) == 0 || len(renderedLines) == 0 || len(overlayLines) == 0 {
		t.Fatalf("expected non-empty lines for overlay assertion")
	}

	sample := -1
	for i, line := range overlayLines {
		if xansi.StringWidth(line) <= 0 {
			continue
		}
		row := y + i
		if row < 0 || row >= len(baseLines) || row >= len(renderedLines) {
			continue
		}
		sample = i
		break
	}
	if sample < 0 {
		t.Fatalf("could not find overlay row inside canvas")
	}

	row := y + sample
	overlayWidth := xansi.StringWidth(overlayLines[sample])
	if overlayWidth <= 0 {
		t.Fatalf("expected positive overlay width")
	}

	baseLine := baseLines[row]
	renderedLine := renderedLines[row]
	baseWidth := xansi.StringWidth(baseLine)
	if x < 0 {
		x = 0
	}
	if x > baseWidth {
		x = baseWidth
	}

	leftBase := xansi.Cut(baseLine, 0, x)
	leftRendered := xansi.Cut(renderedLine, 0, x)
	if leftBase != leftRendered {
		t.Fatalf("expected left side outside overlay to remain unchanged at row %d:\nbase=%q\nrendered=%q", row, leftBase, leftRendered)
	}

	end := x + overlayWidth
	if end < 0 {
		end = 0
	}
	if end > baseWidth {
		end = baseWidth
	}
	rightBase := xansi.Cut(baseLine, end, baseWidth)
	rightRendered := xansi.Cut(renderedLine, end, baseWidth)
	if rightBase != rightRendered {
		t.Fatalf("expected right side outside overlay to remain unchanged at row %d:\nbase=%q\nrendered=%q", row, rightBase, rightRendered)
	}
}
