package app

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestOverlayTransientViewsRendersSettingsMenu(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.settingsMenu.Open()

	body := strings.Repeat("base body\n", 40)
	out := m.overlayTransientViews(body)
	plain := xansi.Strip(out)
	if !strings.Contains(plain, "SETTINGS") {
		t.Fatalf("expected settings overlay in rendered body")
	}
}

func TestOverlayTransientViewsUsesDefaultPresenterWhenNil(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.settingsMenu.Open()
	m.settingsMenuPresenter = nil

	body := strings.Repeat("base body\n", 40)
	out := m.overlayTransientViews(body)
	plain := xansi.Strip(out)
	if !strings.Contains(plain, "SETTINGS") {
		t.Fatalf("expected settings overlay with nil presenter fallback")
	}
}
