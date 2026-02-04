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
