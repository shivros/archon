package app

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestLoadKeybindingsDefaultsWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	bindings, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if got := bindings.KeyFor(KeyCommandToggleSidebar, ""); got != "ctrl+b" {
		t.Fatalf("unexpected default binding: %q", got)
	}
}

func TestLoadKeybindingsDefaultsWhenPathEmpty(t *testing.T) {
	bindings, err := LoadKeybindings("   ")
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if got := bindings.KeyFor(KeyCommandToggleSidebar, ""); got != "ctrl+b" {
		t.Fatalf("unexpected default binding: %q", got)
	}
}

func TestLoadKeybindingsDefaultsWhenFileBlank(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keybindings.json")
	if err := os.WriteFile(path, []byte(" \n\t "), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bindings, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if got := bindings.KeyFor(KeyCommandRefresh, ""); got != "r" {
		t.Fatalf("unexpected default refresh binding: %q", got)
	}
}

func TestLoadKeybindingsArrayOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keybindings.json")
	data := []byte(`[
  {"command":"ui.toggleSidebar","key":"alt+b"},
  {"command":"ui.refresh","key":"F5"}
]`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bindings, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if got := bindings.KeyFor(KeyCommandToggleSidebar, ""); got != "alt+b" {
		t.Fatalf("unexpected sidebar binding: %q", got)
	}
	if got := bindings.KeyFor(KeyCommandRefresh, ""); got != "F5" {
		t.Fatalf("unexpected refresh binding: %q", got)
	}
	if got := bindings.Remap("alt+b"); got != "ctrl+b" {
		t.Fatalf("expected remap to canonical key, got %q", got)
	}
}

func TestLoadKeybindingsMapOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keybindings.json")
	data := []byte(`{"ui.toggleSidebar":"alt+b","ui.copySessionID":"alt+y"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bindings, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if got := bindings.KeyFor(KeyCommandToggleSidebar, ""); got != "alt+b" {
		t.Fatalf("unexpected sidebar binding: %q", got)
	}
	if got := bindings.KeyFor(KeyCommandCopySessionID, ""); got != "alt+y" {
		t.Fatalf("unexpected copy id binding: %q", got)
	}
}

func TestLoadKeybindingsLegacyDismissAliasMapsToSelectionCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keybindings.json")
	data := []byte(`{"ui.dismissSession":"alt+d"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bindings, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if got := bindings.KeyFor(KeyCommandDismissSelection, ""); got != "alt+d" {
		t.Fatalf("unexpected dismiss selection binding: %q", got)
	}
	if got := bindings.KeyFor(KeyCommandDismissSession, ""); got != "alt+d" {
		t.Fatalf("unexpected dismiss legacy alias binding: %q", got)
	}
	if got := bindings.Remap("alt+d"); got != "d" {
		t.Fatalf("expected alias override remap to canonical d, got %q", got)
	}
}

func TestLoadKeybindingsLegacyDismissAliasArrayFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keybindings.json")
	data := []byte(`[
  {"command":"ui.dismissSession","key":"ctrl+d"}
]`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bindings, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if got := bindings.KeyFor(KeyCommandDismissSelection, ""); got != "ctrl+d" {
		t.Fatalf("unexpected dismiss selection binding: %q", got)
	}
}

func TestLoadKeybindingsLegacyComposeClearAliasMapsToInputClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keybindings.json")
	data := []byte(`{"ui.composeClearInput":"f7"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bindings, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if got := bindings.KeyFor(KeyCommandInputClear, ""); got != "f7" {
		t.Fatalf("unexpected input clear binding: %q", got)
	}
	if got := bindings.KeyFor(KeyCommandComposeClearInput, ""); got != "f7" {
		t.Fatalf("unexpected compose clear legacy alias binding: %q", got)
	}
	if got := bindings.Remap("f7"); got != "ctrl+c" {
		t.Fatalf("expected legacy alias remap to canonical ctrl+c, got %q", got)
	}
}

func TestLoadKeybindingsLegacyComposeClearAliasArrayFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keybindings.json")
	data := []byte(`[
  {"command":"ui.composeClearInput","key":"f8"}
]`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bindings, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if got := bindings.KeyFor(KeyCommandInputClear, ""); got != "f8" {
		t.Fatalf("unexpected input clear binding from array alias: %q", got)
	}
	if got := bindings.KeyFor(KeyCommandComposeClearInput, ""); got != "f8" {
		t.Fatalf("unexpected compose clear alias binding from array alias: %q", got)
	}
}

func TestResolveHotkeysUsesBindings(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandToggleSidebar: "alt+b",
	})
	hotkeys := ResolveHotkeys([]Hotkey{
		{Key: "ctrl+b", Command: KeyCommandToggleSidebar, Label: "sidebar"},
		{Key: "q", Label: "quit"},
	}, bindings)
	if hotkeys[0].Key != "alt+b" {
		t.Fatalf("expected overridden hotkey, got %q", hotkeys[0].Key)
	}
	if hotkeys[1].Key != "q" {
		t.Fatalf("expected unchanged hotkey, got %q", hotkeys[1].Key)
	}
}

func TestKeybindingsBindingsIncludesOverrides(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandToggleSidebar: "alt+b",
	})
	m := bindings.Bindings()
	if m[KeyCommandToggleSidebar] != "alt+b" {
		t.Fatalf("expected override to be present")
	}
	if m[KeyCommandRefresh] == "" {
		t.Fatalf("expected defaults to be present")
	}
}

func TestApplyKeybindingsSetsRenderer(t *testing.T) {
	model := &Model{}
	bindings := NewKeybindings(map[string]string{
		KeyCommandToggleSidebar: "alt+b",
	})
	model.applyKeybindings(bindings)
	if model.keybindings == nil {
		t.Fatalf("expected keybindings to be set")
	}
	if model.hotkeys == nil {
		t.Fatalf("expected hotkey renderer to be set")
	}
}

func TestLoadKeybindingsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keybindings.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := LoadKeybindings(path); err == nil {
		t.Fatalf("expected invalid JSON error")
	}
}

func TestLoadKeybindingsSkipsUnknownAndBlankValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keybindings.json")
	data := []byte(`[
  {"command":"ui.toggleSidebar","key":" alt+b "},
  {"command":"ui.unknown","key":"k"},
  {"command":"ui.refresh","key":"   "},
  {"command":"","key":"q"}
]`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	bindings, err := LoadKeybindings(path)
	if err != nil {
		t.Fatalf("LoadKeybindings: %v", err)
	}
	if got := bindings.KeyFor(KeyCommandToggleSidebar, ""); got != "alt+b" {
		t.Fatalf("unexpected sidebar binding: %q", got)
	}
	if got := bindings.KeyFor(KeyCommandRefresh, ""); got != "r" {
		t.Fatalf("expected refresh default to remain, got %q", got)
	}
}

func TestNewKeybindingsIgnoresUnknownCommands(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		"ui.unknown":            "u",
		KeyCommandToggleSidebar: "",
	})
	if got := bindings.KeyFor(KeyCommandToggleSidebar, ""); got != "ctrl+b" {
		t.Fatalf("expected default sidebar key, got %q", got)
	}
}

func TestKeyForFallsBackForUnknownCommand(t *testing.T) {
	bindings := DefaultKeybindings()
	if got := bindings.KeyFor("ui.unknown", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback key, got %q", got)
	}
}

func TestKeyForEmptyCommandFallsBack(t *testing.T) {
	bindings := DefaultKeybindings()
	if got := bindings.KeyFor("   ", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback key for empty command, got %q", got)
	}
}

func TestApplyKeybindingsWithNilUsesDefaults(t *testing.T) {
	model := &Model{}
	model.applyKeybindings(nil)
	if model.keybindings == nil {
		t.Fatalf("expected default keybindings")
	}
	if got := model.keybindings.KeyFor(KeyCommandToggleSidebar, ""); got != "ctrl+b" {
		t.Fatalf("unexpected default sidebar key: %q", got)
	}
}

func TestModelKeyStringUsesRemap(t *testing.T) {
	model := &Model{}
	model.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandToggleSidebar: "alt+b",
	}))
	key := model.keyString(tea.KeyPressMsg{Code: 'b', Mod: tea.ModAlt})
	if key != "ctrl+b" {
		t.Fatalf("expected remapped key ctrl+b, got %q", key)
	}
}

func TestModelKeyStringWithNilModelReturnsRaw(t *testing.T) {
	var model *Model
	key := model.keyString(tea.KeyPressMsg{Text: "q"})
	if key != "q" {
		t.Fatalf("expected raw key q, got %q", key)
	}
}

func TestRemapEmptyKeyReturnsEmpty(t *testing.T) {
	bindings := DefaultKeybindings()
	if got := bindings.Remap("   "); got != "" {
		t.Fatalf("expected empty remap result, got %q", got)
	}
}

func TestDefaultKeybindingsMenuAndRename(t *testing.T) {
	bindings := DefaultKeybindings()
	if got := bindings.KeyFor(KeyCommandMenu, ""); got != "ctrl+m" {
		t.Fatalf("expected default menu key ctrl+m, got %q", got)
	}
	if got := bindings.KeyFor(KeyCommandRename, ""); got != "m" {
		t.Fatalf("expected default rename key m, got %q", got)
	}
	if got := bindings.KeyFor(KeyCommandCopySessionID, ""); got != "ctrl+g" {
		t.Fatalf("expected default copy session key ctrl+g, got %q", got)
	}
	if got := bindings.KeyFor(KeyCommandInputRedo, ""); got != "ctrl+y" {
		t.Fatalf("expected default input redo key ctrl+y, got %q", got)
	}
	if got := bindings.KeyFor(KeyCommandInputClear, ""); got != "ctrl+c" {
		t.Fatalf("expected default input clear key ctrl+c, got %q", got)
	}
	if got := bindings.KeyFor(KeyCommandComposeClearInput, ""); got != "ctrl+c" {
		t.Fatalf("expected legacy compose clear alias key ctrl+c, got %q", got)
	}
	if got := bindings.KeyFor(KeyCommandHistoryBack, ""); got != "alt+left" {
		t.Fatalf("expected default history back key alt+left, got %q", got)
	}
	if got := bindings.KeyFor(KeyCommandHistoryForward, ""); got != "alt+right" {
		t.Fatalf("expected default history forward key alt+right, got %q", got)
	}
}

func TestMenuOverrideRemapsToCanonicalCtrlM(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandMenu: "m",
	})
	if got := bindings.Remap("m"); got != "ctrl+m" {
		t.Fatalf("expected menu override to map to ctrl+m, got %q", got)
	}
}

func TestNewKeybindingsAmbiguousOverrideDoesNotRemap(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandToggleSidebar: "alt+b",
		KeyCommandRefresh:       "alt+b",
	})
	if got := bindings.KeyFor(KeyCommandToggleSidebar, ""); got != "alt+b" {
		t.Fatalf("unexpected sidebar binding: %q", got)
	}
	if got := bindings.KeyFor(KeyCommandRefresh, ""); got != "alt+b" {
		t.Fatalf("unexpected refresh binding: %q", got)
	}
	if got := bindings.Remap("alt+b"); got != "alt+b" {
		t.Fatalf("expected ambiguous key to remain raw, got %q", got)
	}
}

func TestKeyMatchesOverriddenCommandRequiresOverride(t *testing.T) {
	model := &Model{}
	model.applyKeybindings(DefaultKeybindings())

	if model.keyMatchesOverriddenCommand(tea.KeyPressMsg{Text: "n"}, KeyCommandNotesNew, "n") {
		t.Fatalf("expected default binding to not count as an override")
	}
}

func TestKeyMatchesOverriddenCommandMatchesRawKey(t *testing.T) {
	model := &Model{}
	model.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandNotesNew: "ctrl+n",
	}))

	if !model.keyMatchesOverriddenCommand(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}, KeyCommandNotesNew, "n") {
		t.Fatalf("expected overridden notes key to match raw event")
	}
}
