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
	state.ComposeHistory = map[string][]string{
		"s1": []string{"hello", "world"},
	}
	state.ProviderBadges = map[string]*types.ProviderBadgeConfig{
		"codex": {Prefix: "[GPT]", Color: "231"},
	}

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
	if len(loaded.ComposeHistory["s1"]) != 2 || loaded.ComposeHistory["s1"][0] != "hello" || loaded.ComposeHistory["s1"][1] != "world" {
		t.Fatalf("expected compose history to round-trip")
	}
	if loaded.ProviderBadges["codex"] == nil || loaded.ProviderBadges["codex"].Prefix != "[GPT]" || loaded.ProviderBadges["codex"].Color != "231" {
		t.Fatalf("expected provider badge overrides to round-trip")
	}
}
