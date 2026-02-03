package store

import (
	"context"
	"path/filepath"
	"testing"

	"control/internal/types"
)

func TestAppStateStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewFileAppStateStore(path)

	state, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if state.ActiveWorkspaceID != "" {
		t.Fatalf("expected empty state")
	}

	state.ActiveWorkspaceID = "ws_1"
	state.ActiveWorktreeID = "wt_1"
	state.SidebarCollapsed = true

	if err := store.Save(ctx, state); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.ActiveWorkspaceID != "ws_1" || loaded.ActiveWorktreeID != "wt_1" || !loaded.SidebarCollapsed {
		t.Fatalf("unexpected reload state")
	}
}

func TestKeymapStoreDefaults(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "keymap.json")
	store := NewFileKeymapStore(path)

	keymap, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if keymap.Bindings[types.KeyActionToggleSidebar] != "ctrl+b" {
		t.Fatalf("expected default toggle binding")
	}
}

func TestKeymapStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "keymap.json")
	store := NewFileKeymapStore(path)

	custom := &types.Keymap{Bindings: map[string]string{types.KeyActionToggleSidebar: "alt+b"}}
	if err := store.Save(ctx, custom); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Bindings[types.KeyActionToggleSidebar] != "alt+b" {
		t.Fatalf("expected custom binding")
	}
}
