package app

import "testing"

func TestSettingsMenuPolicyModelAdaptersAndDefaults(t *testing.T) {
	var nilModel *Model
	if nilModel.settingsMenuEscPolicyOrDefault() == nil {
		t.Fatalf("expected default esc policy for nil model")
	}
	if nilModel.settingsMenuHotkeyCatalogOrDefault() == nil {
		t.Fatalf("expected default hotkey catalog for nil model")
	}

	m := NewModel(nil)
	m.settingsMenuEscPolicy = nil
	m.settingsMenuHotkeyCatalog = nil
	if m.settingsMenuEscPolicyOrDefault() == nil {
		t.Fatalf("expected esc policy default when nil field")
	}
	if m.settingsMenuHotkeyCatalogOrDefault() == nil {
		t.Fatalf("expected hotkey catalog default when nil field")
	}
}

func TestSettingsMenuHotkeySourceAndCatalogFromModel(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandQuit: "ctrl+q",
	}))
	mappings := m.settingsMenuHotkeyCatalogOrDefault().Mappings(m.settingsMenuHotkeySource())
	found := false
	for _, item := range mappings {
		if item.Label == "quit" && item.Key == "ctrl+q" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected model hotkey source to resolve overridden quit key")
	}
}

func TestSettingsMenuOpenContextAdapterReflectsModelState(t *testing.T) {
	m := NewModel(nil)
	ctx := m.settingsMenuOpenContext()
	if ctx.Mode() != uiModeNormal {
		t.Fatalf("expected normal mode from context adapter")
	}
	if ctx.IsConfirmOpen() || ctx.IsContextMenuOpen() || ctx.IsTopMenuActive() || ctx.IsSettingsMenuOpen() {
		t.Fatalf("expected all overlays closed by default")
	}
	m.confirm.Open("T", "M", "ok", "cancel")
	if !ctx.IsConfirmOpen() {
		t.Fatalf("expected confirm open from adapter")
	}
	m.confirm.Close()
	m.contextMenu.OpenSession("s1", "", "", "Session", 1, 1)
	if !ctx.IsContextMenuOpen() {
		t.Fatalf("expected context menu open from adapter")
	}
	m.contextMenu.Close()
	m.menu.OpenBar()
	if !ctx.IsTopMenuActive() {
		t.Fatalf("expected top menu active from adapter")
	}
	m.menu.CloseAll()
	m.settingsMenu.Open()
	if !ctx.IsSettingsMenuOpen() {
		t.Fatalf("expected settings menu open from adapter")
	}
}

func TestSettingsHotkeyContextLabelAllBranches(t *testing.T) {
	cases := map[HotkeyContext]string{
		HotkeyGlobal:         "global",
		HotkeySidebar:        "sidebar",
		HotkeyChatInput:      "chat-input",
		HotkeyAddWorkspace:   "add-workspace",
		HotkeyAddWorktree:    "add-worktree",
		HotkeyPickProvider:   "pick-provider",
		HotkeySearch:         "search",
		HotkeyContextMenu:    "context-menu",
		HotkeyConfirm:        "confirm",
		HotkeyApproval:       "approval",
		HotkeyGuidedWorkflow: "guided-workflow",
	}
	for ctx, want := range cases {
		if got := settingsHotkeyContextLabel(ctx); got != want {
			t.Fatalf("expected context label %q, got %q", want, got)
		}
	}
	if got := settingsHotkeyContextLabel(HotkeyContext(999)); got != "other" {
		t.Fatalf("expected unknown context label other, got %q", got)
	}
}
