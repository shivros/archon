package app

import "testing"

func TestModelHighlightAdapterNilSafeDefaults(t *testing.T) {
	var adapter *modelHighlightAdapter

	if mode := adapter.CurrentUIMode(); mode != uiModeNormal {
		t.Fatalf("expected normal mode fallback, got %v", mode)
	}
	if h := adapter.ViewportHeight(); h != 0 {
		t.Fatalf("expected viewport height 0, got %d", h)
	}
	if adapter.MouseOverInput(1) {
		t.Fatalf("expected input hover false for nil adapter")
	}
	if idx := adapter.BlockIndexByViewportPoint(0, 0); idx != -1 {
		t.Fatalf("expected transcript index -1 for nil adapter, got %d", idx)
	}
	if adapter.NotesPanelOpen() {
		t.Fatalf("expected notes panel open false for nil adapter")
	}
	if adapter.NotesPanelVisible() {
		t.Fatalf("expected notes panel visible false for nil adapter")
	}
	if h := adapter.NotesPanelViewportHeight(); h != 0 {
		t.Fatalf("expected panel viewport height 0, got %d", h)
	}
	if idx := adapter.NotePanelBlockIndexByViewportPoint(0, 0); idx != -1 {
		t.Fatalf("expected panel index -1 for nil adapter, got %d", idx)
	}
	if w := adapter.SidebarWidth(); w != 0 {
		t.Fatalf("expected sidebar width 0, got %d", w)
	}
	if key := adapter.SidebarItemKeyAtRow(0); key != "" {
		t.Fatalf("expected empty sidebar key, got %q", key)
	}
	if keys := adapter.SidebarHighlightedKeysBetweenRows(0, 2); keys != nil {
		t.Fatalf("expected nil highlighted keys for nil adapter, got %#v", keys)
	}
}

func TestModelHighlightAdapterModelBackedSidebarAndPanel(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	m.notesPanelOpen = true
	m.notesPanelVisible = true
	m.notesPanelViewport.SetHeight(9)

	adapter := newModelHighlightAdapter(&m)
	if adapter.CurrentUIMode() != uiModeNormal {
		t.Fatalf("expected normal mode")
	}
	if adapter.ViewportHeight() <= 0 {
		t.Fatalf("expected positive viewport height")
	}
	if !adapter.NotesPanelOpen() || !adapter.NotesPanelVisible() {
		t.Fatalf("expected notes panel open and visible from model")
	}
	if h := adapter.NotesPanelViewportHeight(); h != 9 {
		t.Fatalf("expected panel viewport height 9, got %d", h)
	}
	if adapter.SidebarWidth() <= 0 {
		t.Fatalf("expected positive sidebar width")
	}

	row := -1
	for y := 0; y < 20; y++ {
		if key := adapter.SidebarItemKeyAtRow(y); key != "" {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected at least one sidebar row with key")
	}

	keys := adapter.SidebarHighlightedKeysBetweenRows(row, row)
	if len(keys) != 1 {
		t.Fatalf("expected one highlighted key in row range, got %#v", keys)
	}
}
