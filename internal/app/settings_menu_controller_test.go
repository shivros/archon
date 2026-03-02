package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSettingsMenuControllerRootActions(t *testing.T) {
	c := NewSettingsMenuController()
	c.Open()
	if !c.IsOpen() {
		t.Fatalf("expected menu to open")
	}
	handled, action := c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if action != SettingsMenuActionNone {
		t.Fatalf("expected no root action for help selection, got %v", action)
	}
	handled, action = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected esc to be handled")
	}
	if action != SettingsMenuActionNone {
		t.Fatalf("expected no action when returning from help, got %v", action)
	}
	handled, action = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !handled {
		t.Fatalf("expected down to be handled")
	}
	handled, action = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if action != SettingsMenuActionQuit {
		t.Fatalf("expected quit action, got %v", action)
	}
}

func TestSettingsMenuControllerDataDrivenMenuSupportsAdditionalItems(t *testing.T) {
	items := []SettingsMenuItem{
		{ID: "help", Title: "HELP", Screen: settingsMenuScreenHelp},
		{ID: "about", Title: "ABOUT", Screen: settingsMenuScreenRoot},
		{ID: "quit", Title: "QUIT", Action: SettingsMenuActionQuit},
	}
	c := NewSettingsMenuController(items...)
	c.Open()
	_, _ = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	handled, action := c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if action != SettingsMenuActionQuit {
		t.Fatalf("expected quit action from third item, got %v", action)
	}
}

func TestSettingsMenuControllerCloseAndHelpScrollBranches(t *testing.T) {
	c := NewSettingsMenuController()
	c.Open()
	c.Close()
	if c.IsOpen() {
		t.Fatalf("expected menu to be closed")
	}
	c.Open()
	_, _ = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	_, _ = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
	_, _ = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	_, _ = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	_, _ = c.HandleKey(tea.KeyPressMsg{Code: tea.KeyHome})
	if c.helpOffset != 0 {
		t.Fatalf("expected home to reset help offset, got %d", c.helpOffset)
	}
}

func TestNewSettingsMenuControllerFallsBackWhenItemsInvalid(t *testing.T) {
	c := NewSettingsMenuController(SettingsMenuItem{ID: "", Title: "ignored"})
	if c == nil {
		t.Fatalf("expected controller")
	}
	if len(c.items) != len(DefaultSettingsMenuItems()) {
		t.Fatalf("expected fallback default items")
	}
}

func TestSettingsMenuReducerQuitReturnsQuitCommand(t *testing.T) {
	m := NewModel(nil)
	m.settingsMenu.Open()
	_, _ = m.settingsMenu.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	handled, cmd := m.reduceSettingsMenu(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected settings reducer to handle enter")
	}
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
}

func TestSettingsMenuEscPolicyUsesNarrowContext(t *testing.T) {
	policy := defaultSettingsMenuEscPolicy{}
	ctx := fakeSettingsMenuOpenContext{mode: uiModeNormal}
	if !policy.CanOpen(ctx) {
		t.Fatalf("expected policy to allow open in normal mode when overlays are closed")
	}
	ctx.confirmOpen = true
	if policy.CanOpen(ctx) {
		t.Fatalf("expected policy to block when confirm is open")
	}
	ctx.confirmOpen = false
	ctx.statusOpen = true
	if policy.CanOpen(ctx) {
		t.Fatalf("expected policy to block when status history is open")
	}
}

func TestSettingsHotkeyCatalogUsesSourceAndOverrides(t *testing.T) {
	catalog := defaultSettingsMenuHotkeyCatalog{}
	source := fakeSettingsHotkeySource{hotkeys: []Hotkey{{Context: HotkeyGlobal, Key: "ctrl+q", Label: "quit", Priority: 1}}}
	mappings := catalog.Mappings(source)
	if len(mappings) != 1 {
		t.Fatalf("expected one mapping, got %d", len(mappings))
	}
	if mappings[0].Label != "quit" || mappings[0].Key != "ctrl+q" {
		t.Fatalf("expected quit mapping with override key, got %#v", mappings[0])
	}
}

type fakeSettingsMenuOpenContext struct {
	mode            uiMode
	confirmOpen     bool
	contextMenuOpen bool
	topMenuActive   bool
	settingsOpen    bool
	statusOpen      bool
}

func (f fakeSettingsMenuOpenContext) Mode() uiMode             { return f.mode }
func (f fakeSettingsMenuOpenContext) IsConfirmOpen() bool      { return f.confirmOpen }
func (f fakeSettingsMenuOpenContext) IsContextMenuOpen() bool  { return f.contextMenuOpen }
func (f fakeSettingsMenuOpenContext) IsTopMenuActive() bool    { return f.topMenuActive }
func (f fakeSettingsMenuOpenContext) IsSettingsMenuOpen() bool { return f.settingsOpen }
func (f fakeSettingsMenuOpenContext) IsStatusHistoryOpen() bool {
	return f.statusOpen
}

type fakeSettingsHotkeySource struct {
	hotkeys []Hotkey
}

func (f fakeSettingsHotkeySource) ResolvedHotkeys() []Hotkey {
	return f.hotkeys
}
