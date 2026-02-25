package daemon

import "testing"

func TestBuildSessionRuntimeWithoutItemsProvider(t *testing.T) {
	manager, err := NewSessionManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	state, err := manager.buildSessionRuntime("sess-no-items", "gemini")
	if err != nil {
		t.Fatalf("buildSessionRuntime: %v", err)
	}
	if state == nil {
		t.Fatalf("expected runtime state")
	}
	if state.sink == nil {
		t.Fatalf("expected log sink")
	}
	if state.debug == nil || state.debugHub == nil || state.debugBuf == nil {
		t.Fatalf("expected debug streaming components to be initialized")
	}
	if state.items != nil || state.itemsHub != nil {
		t.Fatalf("expected no item streaming setup for gemini provider")
	}
}

func TestBuildSessionRuntimeWithItemsProvider(t *testing.T) {
	manager, err := NewSessionManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	state, err := manager.buildSessionRuntime("sess-items", "claude")
	if err != nil {
		t.Fatalf("buildSessionRuntime: %v", err)
	}
	if state == nil {
		t.Fatalf("expected runtime state")
	}
	if state.items == nil || state.itemsHub == nil {
		t.Fatalf("expected item streaming setup for claude provider")
	}
	if state.debug == nil {
		t.Fatalf("expected debug sink to be initialized")
	}
}
