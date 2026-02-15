package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

func TestModelLatencyRecordsUpdateAndViewSpans(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := NewModel(nil, WithUILatencySink(sink))

	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	_ = m.View()

	metrics := sink.Snapshot()
	if !hasLatencyMetric(metrics, uiLatencySpanModelUpdate, UILatencyCategorySpan, "") {
		t.Fatalf("missing update latency metric: %#v", metrics)
	}
	if !hasLatencyMetric(metrics, uiLatencySpanModelView, UILatencyCategorySpan, "") {
		t.Fatalf("missing view latency metric: %#v", metrics)
	}
}

func TestModelLatencyRecordsCriticalActionProbes(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := newLatencyProbeModel(sink)

	m.toggleSidebar()
	_ = m.toggleNotesPanel()
	m.enterCompose("s1")
	m.exitCompose("compose closed")
	if !m.enterNewSession() {
		t.Fatalf("expected enterNewSession to succeed")
	}
	item := m.selectedItem()
	if item == nil || item.session == nil {
		t.Fatalf("expected selected session")
	}
	_ = m.loadSelectedSession(item)
	handled, _ := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   item.key(),
		items: []map[string]any{},
	})
	if !handled {
		t.Fatalf("expected history message to be handled")
	}

	metrics := sink.Snapshot()
	required := []string{
		uiLatencyActionToggleSessionsSidebar,
		uiLatencyActionToggleNotesSidebar,
		uiLatencyActionExitCompose,
		uiLatencyActionOpenNewSession,
		uiLatencyActionSwitchSession,
	}
	for _, action := range required {
		if !hasLatencyMetric(metrics, action, UILatencyCategoryAction, uiLatencyOutcomeOK) && !hasLatencyMetric(metrics, action, UILatencyCategoryAction, uiLatencyOutcomeCacheHit) {
			t.Fatalf("missing action latency metric for %s: %#v", action, metrics)
		}
	}
}

func hasLatencyMetric(metrics []UILatencyMetric, name string, category UILatencyCategory, outcome string) bool {
	for _, metric := range metrics {
		if metric.Name != name {
			continue
		}
		if metric.Category != category {
			continue
		}
		if outcome != "" && metric.Outcome != outcome {
			continue
		}
		return true
	}
	return false
}

func newLatencyProbeModel(sink UILatencySink) Model {
	m := NewModel(nil, WithUILatencySink(sink))
	now := time.Now().UTC()
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	m.sessions = []*types.Session{
		{
			ID:        "s1",
			Provider:  "codex",
			Status:    types.SessionStatusRunning,
			CreatedAt: now,
			Title:     "Session",
		},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	if m.sidebar != nil {
		_ = m.sidebar.SelectBySessionID("s1")
	}
	return m
}
