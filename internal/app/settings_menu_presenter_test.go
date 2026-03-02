package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestSettingsMenuPresenterViewHandlesNilAndClosedController(t *testing.T) {
	p := defaultSettingsMenuPresenter{}
	block, row := p.View(nil, 80, 20, nil)
	if block != "" || row != 0 {
		t.Fatalf("expected empty view for nil controller")
	}
	c := NewSettingsMenuController()
	block, row = p.View(c, 80, 20, nil)
	if block != "" || row != 0 {
		t.Fatalf("expected empty view for closed controller")
	}
}

func TestSettingsMenuPresenterRootViewIncludesSettingsAndItems(t *testing.T) {
	p := defaultSettingsMenuPresenter{}
	c := NewSettingsMenuController()
	c.Open()
	block, _ := p.View(c, 120, 40, nil)
	plain := xansi.Strip(block)
	if !strings.Contains(plain, "SETTINGS") {
		t.Fatalf("expected settings title in root view, got %q", plain)
	}
	if !strings.Contains(plain, "_____") {
		t.Fatalf("expected ascii word art in root view")
	}
}

func TestSettingsMenuPresenterHelpViewRendersMappingsAndClampsOffset(t *testing.T) {
	p := defaultSettingsMenuPresenter{}
	c := NewSettingsMenuController()
	c.Open()
	_, _ = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	c.helpOffset = 1 << 20
	mappings := []SettingsHotkeyMapping{
		{Context: "global", Key: "ctrl+q", Label: "quit", Priority: 1},
	}
	block, _ := p.View(c, 80, 12, mappings)
	plain := xansi.Strip(block)
	if !strings.Contains(plain, "HOTKEY HELP") {
		t.Fatalf("expected help header, got %q", plain)
	}
	if !strings.Contains(plain, "ctrl+q") {
		t.Fatalf("expected mapping key in help view")
	}
	if c.helpOffset < 0 {
		t.Fatalf("expected clamped non-negative offset")
	}
}

func TestSettingsMenuWordArtAndLayoutHelpers(t *testing.T) {
	if got := settingsMenuWordArt("help"); !strings.Contains(got, "_____") {
		t.Fatalf("expected HELP ascii art, got %q", got)
	}
	if got := settingsMenuWordArt("custom"); got != "CUSTOM" {
		t.Fatalf("expected uppercase fallback, got %q", got)
	}
	if got, row := centerOverlayBlock("", 80, 20); got != "" || row != 0 {
		t.Fatalf("expected empty block for empty input")
	}
	if got := longestLine("a\nbbb\ncc"); got != "bbb" {
		t.Fatalf("expected longest line bbb, got %q", got)
	}
}
