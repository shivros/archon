package app

import (
	"os"
	"path/filepath"
	"testing"
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
