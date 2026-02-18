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
	state.SidebarWorkspaceExpanded = map[string]bool{
		"ws_1": false,
	}
	state.SidebarWorktreeExpanded = map[string]bool{
		"wt_1": true,
	}
	state.ComposeHistory = map[string][]string{
		"s1": []string{"hello", "world"},
	}
	state.ProviderBadges = map[string]*types.ProviderBadgeConfig{
		"codex": {Prefix: "[GPT]", Color: "231"},
	}
	state.Recents = &types.AppStateRecents{
		Version: 1,
		Running: map[string]types.AppStateRecentRun{
			"s-running": {SessionID: "s-running", BaselineTurnID: "turn-u1", StartedAtUnix: 1710000000},
		},
		Ready: map[string]types.AppStateReadyItem{
			"s-ready": {SessionID: "s-ready", CompletionTurn: "turn-a1", CompletedAtUnix: 1710000300, LastKnownTurnID: "turn-a1"},
		},
		ReadyQueue:    []types.AppStateReadyQueueEntry{{SessionID: "s-ready", Seq: 1}},
		DismissedTurn: map[string]string{"s-dismissed": "turn-a0"},
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
	if got, ok := loaded.SidebarWorkspaceExpanded["ws_1"]; !ok || got {
		t.Fatalf("expected workspace expansion override ws_1=false, got=%v ok=%v", got, ok)
	}
	if got, ok := loaded.SidebarWorktreeExpanded["wt_1"]; !ok || !got {
		t.Fatalf("expected worktree expansion override wt_1=true, got=%v ok=%v", got, ok)
	}
	if len(loaded.ComposeHistory["s1"]) != 2 || loaded.ComposeHistory["s1"][0] != "hello" || loaded.ComposeHistory["s1"][1] != "world" {
		t.Fatalf("expected compose history to round-trip")
	}
	if loaded.ProviderBadges["codex"] == nil || loaded.ProviderBadges["codex"].Prefix != "[GPT]" || loaded.ProviderBadges["codex"].Color != "231" {
		t.Fatalf("expected provider badge overrides to round-trip")
	}
	if loaded.Recents == nil {
		t.Fatalf("expected recents state to round-trip")
	}
	if run, ok := loaded.Recents.Running["s-running"]; !ok || run.BaselineTurnID != "turn-u1" {
		t.Fatalf("expected running recents state to round-trip, got %#v", loaded.Recents.Running)
	}
	if item, ok := loaded.Recents.Ready["s-ready"]; !ok || item.CompletionTurn != "turn-a1" {
		t.Fatalf("expected ready recents state to round-trip, got %#v", loaded.Recents.Ready)
	}
	if len(loaded.Recents.ReadyQueue) != 1 || loaded.Recents.ReadyQueue[0].SessionID != "s-ready" {
		t.Fatalf("expected ready queue to round-trip, got %#v", loaded.Recents.ReadyQueue)
	}
	if got := loaded.Recents.DismissedTurn["s-dismissed"]; got != "turn-a0" {
		t.Fatalf("expected dismissed turn to round-trip, got %q", got)
	}
}
