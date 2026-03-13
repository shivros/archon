package app

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestApplyThemeSetsPaneBackgroundStyles(t *testing.T) {
	previous := CurrentThemeID()
	t.Cleanup(func() {
		ApplyTheme(previous)
	})

	ApplyTheme("default")
	defaultMain := mainPaneStyle.Render(" ")
	defaultSidebar := sidebarPaneStyle.Render(" ")
	defaultMainFg := mainPaneStyle.GetForeground()
	defaultSidebarFg := sidebarPaneStyle.GetForeground()
	if mainPaneStyle.GetBackground() == nil {
		t.Fatalf("expected main pane background style to be configured")
	}
	if sidebarPaneStyle.GetBackground() == nil {
		t.Fatalf("expected sidebar pane background style to be configured")
	}
	if mainPaneStyle.GetForeground() == nil {
		t.Fatalf("expected main pane foreground style to be configured")
	}
	if sidebarPaneStyle.GetForeground() == nil {
		t.Fatalf("expected sidebar pane foreground style to be configured")
	}

	ApplyTheme("monokai")
	monokaiMain := mainPaneStyle.Render(" ")
	monokaiSidebar := sidebarPaneStyle.Render(" ")
	monokaiMainFg := mainPaneStyle.GetForeground()
	monokaiSidebarFg := sidebarPaneStyle.GetForeground()
	if defaultMain == monokaiMain {
		t.Fatalf("expected main pane background rendering to change across themes")
	}
	if defaultSidebar == monokaiSidebar {
		t.Fatalf("expected sidebar pane background rendering to change across themes")
	}
	if defaultMainFg == monokaiMainFg {
		t.Fatalf("expected main pane foreground rendering to change across themes")
	}
	if defaultSidebarFg == monokaiSidebarFg {
		t.Fatalf("expected sidebar pane foreground rendering to change across themes")
	}
}

func TestFillBlockWithBackgroundStylesTrailingPadding(t *testing.T) {
	previous := CurrentThemeID()
	t.Cleanup(func() {
		ApplyTheme(previous)
	})
	ApplyTheme("nordic")

	rendered := fillBlockWithBackground("abc", 8, 1, mainPaneStyle)
	prefix, _, ok := backgroundEnvelope(mainPaneStyle)
	if !ok || prefix == "" {
		t.Fatalf("expected pane style to provide ANSI envelope")
	}
	if !strings.Contains(rendered, prefix) {
		t.Fatalf("expected background escape codes in padded render")
	}
	if got := xansi.StringWidth(rendered); got != 8 {
		t.Fatalf("expected padded width 8, got %d", got)
	}
}

func TestFillBlockWithBackgroundReappliesPaneBackgroundAfterInnerResets(t *testing.T) {
	previous := CurrentThemeID()
	t.Cleanup(func() {
		ApplyTheme(previous)
	})
	ApplyTheme("nordic")

	line := lipgloss.PlaceHorizontal(12, lipgloss.Left, statusStyle.Render("meta"))
	rendered := fillBlockWithBackground(line, 12, 1, mainPaneStyle)
	prefix, _, ok := backgroundEnvelope(mainPaneStyle)
	if !ok || prefix == "" {
		t.Fatalf("expected pane style to provide ANSI envelope")
	}
	if !strings.Contains(rendered, "\x1b[m"+prefix) && !strings.Contains(rendered, "\x1b[0m"+prefix) {
		t.Fatalf("expected pane background to be re-applied after inner reset, got %q", rendered)
	}
	if got := xansi.StringWidth(rendered); got != 12 {
		t.Fatalf("expected rendered width 12, got %d", got)
	}
}

func TestFillBlockWithBackgroundReappliesPaneBackgroundAcrossMultipleStyledSegments(t *testing.T) {
	previous := CurrentThemeID()
	t.Cleanup(func() {
		ApplyTheme(previous)
	})
	ApplyTheme("nordic")

	line := statusStyle.Render("A") + headerStyle.Render("B") + "    "
	rendered := fillBlockWithBackground(line, 6, 1, mainPaneStyle)
	prefix, _, ok := backgroundEnvelope(mainPaneStyle)
	if !ok || prefix == "" {
		t.Fatalf("expected pane style to provide ANSI envelope")
	}
	reapplied := strings.Count(rendered, "\x1b[m"+prefix) + strings.Count(rendered, "\x1b[0m"+prefix)
	if reapplied < 2 {
		t.Fatalf("expected pane background to be re-applied across multiple styled segments, got %q", rendered)
	}
	if got := xansi.StringWidth(rendered); got != 6 {
		t.Fatalf("expected rendered width 6, got %d", got)
	}
}

func TestFillBlockWithBackgroundReappliesAfterResetBackgroundSGR(t *testing.T) {
	previous := CurrentThemeID()
	t.Cleanup(func() {
		ApplyTheme(previous)
	})
	ApplyTheme("nordic")

	line := "\x1b[38;5;10mabc\x1b[39;49mdef"
	rendered := fillBlockWithBackground(line, 8, 1, mainPaneStyle)
	prefix, _, ok := backgroundEnvelope(mainPaneStyle)
	if !ok || prefix == "" {
		t.Fatalf("expected pane style to provide ANSI envelope")
	}
	if !strings.Contains(rendered, "\x1b[39;49m"+prefix) {
		t.Fatalf("expected pane background re-apply after 39;49 reset, got %q", rendered)
	}
	if got := xansi.StringWidth(rendered); got != 8 {
		t.Fatalf("expected rendered width 8, got %d", got)
	}
}
