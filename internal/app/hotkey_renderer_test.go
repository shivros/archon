package app

import "testing"

func TestFilterHotkeys(t *testing.T) {
	keys := []Hotkey{
		{Key: "b", Label: "beta", Context: HotkeySidebar, Priority: 2},
		{Key: "a", Label: "alpha", Context: HotkeySidebar, Priority: 1},
		{Key: "z", Label: "zeta", Context: HotkeyChatInput, Priority: 0},
	}
	got := FilterHotkeys(keys, []HotkeyContext{HotkeySidebar})
	if len(got) != 2 {
		t.Fatalf("expected 2 hotkeys, got %d", len(got))
	}
	if got[0].Key != "a" || got[1].Key != "b" {
		t.Fatalf("unexpected order: %s, %s", got[0].Key, got[1].Key)
	}
}

func TestDefaultHotkeyResolver(t *testing.T) {
	model := &Model{mode: uiModeAddWorktree}
	resolver := DefaultHotkeyResolver{}
	contexts := resolver.ActiveContexts(model)
	found := false
	for _, ctx := range contexts {
		if ctx == HotkeyAddWorktree {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected HotkeyAddWorktree context")
	}
}

func TestDefaultHotkeysUseCanonicalInputClearCommand(t *testing.T) {
	hotkeys := DefaultHotkeys()
	foundCanonical := false
	for _, hotkey := range hotkeys {
		if hotkey.Command == KeyCommandInputClear {
			foundCanonical = true
		}
		if hotkey.Command == KeyCommandComposeClearInput {
			t.Fatalf("did not expect legacy compose clear command in hotkey metadata")
		}
	}
	if !foundCanonical {
		t.Fatalf("expected canonical input clear command in default hotkeys")
	}
}

func TestResolveHotkeysAppliesInputClearOverride(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandInputClear: "f7",
	})
	hotkeys := ResolveHotkeys(DefaultHotkeys(), bindings)
	found := false
	for _, hotkey := range hotkeys {
		if hotkey.Command != KeyCommandInputClear {
			continue
		}
		found = true
		if hotkey.Key != "f7" {
			t.Fatalf("expected overridden input clear hotkey f7, got %q", hotkey.Key)
		}
	}
	if !found {
		t.Fatalf("expected input clear hotkey to be present")
	}
}

func TestDefaultHotkeysUseStartGuidedWorkflowCommand(t *testing.T) {
	hotkeys := DefaultHotkeys()
	found := false
	for _, hotkey := range hotkeys {
		if hotkey.Command == KeyCommandStartGuidedWorkflow {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected start guided workflow command in default hotkeys")
	}
}

func TestResolveHotkeysAppliesStartGuidedWorkflowOverride(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandStartGuidedWorkflow: "ctrl+w",
	})
	hotkeys := ResolveHotkeys(DefaultHotkeys(), bindings)
	found := false
	for _, hotkey := range hotkeys {
		if hotkey.Command != KeyCommandStartGuidedWorkflow {
			continue
		}
		found = true
		if hotkey.Key != "ctrl+w" {
			t.Fatalf("expected overridden start guided workflow key ctrl+w, got %q", hotkey.Key)
		}
	}
	if !found {
		t.Fatalf("expected start guided workflow hotkey to be present")
	}
}

func TestDefaultHotkeysUseOpenSettingsCommand(t *testing.T) {
	hotkeys := DefaultHotkeys()
	found := false
	for _, hotkey := range hotkeys {
		if hotkey.Command == KeyCommandOpenSettings {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected open settings command in default hotkeys")
	}
}

func TestResolveHotkeysAppliesOpenSettingsOverride(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandOpenSettings: "f12",
	})
	hotkeys := ResolveHotkeys(DefaultHotkeys(), bindings)
	found := false
	for _, hotkey := range hotkeys {
		if hotkey.Command != KeyCommandOpenSettings {
			continue
		}
		found = true
		if hotkey.Key != "f12" {
			t.Fatalf("expected overridden open settings key f12, got %q", hotkey.Key)
		}
	}
	if !found {
		t.Fatalf("expected open settings hotkey to be present")
	}
}

func TestDefaultHotkeysUseToggleContextPanelCommand(t *testing.T) {
	hotkeys := DefaultHotkeys()
	found := false
	for _, hotkey := range hotkeys {
		if hotkey.Command == KeyCommandToggleContextPanel {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected toggle context panel command in default hotkeys")
	}
}

func TestResolveHotkeysAppliesToggleContextPanelOverride(t *testing.T) {
	bindings := NewKeybindings(map[string]string{
		KeyCommandToggleContextPanel: "f6",
	})
	hotkeys := ResolveHotkeys(DefaultHotkeys(), bindings)
	found := false
	for _, hotkey := range hotkeys {
		if hotkey.Command != KeyCommandToggleContextPanel {
			continue
		}
		found = true
		if hotkey.Key != "f6" {
			t.Fatalf("expected overridden context panel key f6, got %q", hotkey.Key)
		}
	}
	if !found {
		t.Fatalf("expected toggle context panel hotkey to be present")
	}
}

func TestDefaultHotkeysUseCanonicalCopySelectionIDsCommand(t *testing.T) {
	hotkeys := DefaultHotkeys()
	sidebarFound := false
	chatFound := false
	for _, hotkey := range hotkeys {
		if hotkey.Command == KeyCommandCopySessionID {
			t.Fatalf("did not expect legacy copy-session command in default hotkeys")
		}
		if hotkey.Command != KeyCommandCopySelectionIDs {
			continue
		}
		if hotkey.Context == HotkeySidebar {
			sidebarFound = true
		}
		if hotkey.Context == HotkeyChatInput {
			chatFound = true
		}
	}
	if !sidebarFound {
		t.Fatalf("expected canonical copy-selection hotkey in sidebar context")
	}
	if !chatFound {
		t.Fatalf("expected canonical copy-selection hotkey in chat context")
	}
}
