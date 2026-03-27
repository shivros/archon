package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/daemon/transcriptdomain"
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

func TestSessionLoadLatencyFinishesOnceAfterAsyncCachedRender(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := newLatencyProbeModel(sink)
	WithAsyncViewportRendering(true)(&m)
	WithRenderPipeline(delayedRenderPipeline{delay: 100 * time.Millisecond})(&m)
	m.sessionTranscriptAPI = nil
	m.resize(120, 40)

	item := m.selectedItem()
	if item == nil || item.session == nil {
		t.Fatalf("expected selected session")
	}
	m.transcriptCache[item.key()] = []ChatBlock{{Role: ChatRoleAgent, Text: "cached reply"}}

	_ = m.loadSelectedSession(item)

	if got := countLatencyMetricsByName(sink.Snapshot(), uiLatencyActionSwitchSession); got != 0 {
		t.Fatalf("expected no switch-session latency metric before async render settles, got %d", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.consumeCompletedViewportRender()
		metrics := sink.Snapshot()
		if countLatencyMetricsByName(metrics, uiLatencyActionSwitchSession) > 0 {
			if got := countLatencyMetricsByName(metrics, uiLatencyActionSwitchSession); got != 1 {
				t.Fatalf("expected one switch-session latency metric after async render, got %d", got)
			}
			if !hasLatencyMetric(metrics, uiLatencyActionSwitchSession, UILatencyCategoryAction, uiLatencyOutcomeCacheHit) {
				t.Fatalf("expected cache-hit switch-session latency metric, got %#v", metrics)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for async cached render to finish switch-session latency")
}

func TestSessionLoadLatencyStaysOpenForCachedTranscriptWhileBootstrapContinues(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := newLatencyProbeModel(sink)
	m.resize(120, 40)

	item := m.selectedItem()
	if item == nil || item.session == nil {
		t.Fatalf("expected selected session")
	}
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	m.transcriptCache[item.key()] = []ChatBlock{{Role: ChatRoleAgent, Text: "cached reply"}}

	_ = m.loadSelectedSession(item)

	if got := countLatencyMetricsByName(sink.Snapshot(), uiLatencyActionSwitchSession); got != 0 {
		t.Fatalf("expected no switch-session latency metric while transcript bootstrap is still pending, got %d", got)
	}
	if !m.loading || m.loadingKey != item.key() {
		t.Fatalf("expected cached transcript with bootstrap API to remain in loading state")
	}
}

func TestSessionLoadLatencyFinishesOnceAfterAsyncSnapshotRender(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := newPhase0ModelWithSession("codex")
	WithUILatencySink(sink)(&m)
	WithAsyncViewportRendering(true)(&m)
	WithRenderPipeline(delayedRenderPipeline{delay: 100 * time.Millisecond})(&m)
	m.resize(120, 40)
	m.pendingSessionKey = "sess:s1"
	m.loading = true
	m.loadingKey = "sess:s1"
	m.startUILatencyAction(uiLatencyActionSwitchSession, "sess:s1")

	handled, _ := m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("1"),
			Blocks: []transcriptdomain.Block{
				{Kind: "assistant_message", Role: "assistant", Text: "snapshot reply"},
			},
		},
	})
	if !handled {
		t.Fatalf("expected transcript snapshot to be handled")
	}
	if got := countLatencyMetricsByName(sink.Snapshot(), uiLatencyActionSwitchSession); got != 0 {
		t.Fatalf("expected no switch-session latency metric before async snapshot render settles, got %d", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.consumeCompletedViewportRender()
		metrics := sink.Snapshot()
		if countLatencyMetricsByName(metrics, uiLatencyActionSwitchSession) > 0 {
			if got := countLatencyMetricsByName(metrics, uiLatencyActionSwitchSession); got != 1 {
				t.Fatalf("expected one switch-session latency metric after async snapshot render, got %d", got)
			}
			if !hasLatencyMetric(metrics, uiLatencyActionSwitchSession, UILatencyCategoryAction, uiLatencyOutcomeOK) {
				t.Fatalf("expected ok switch-session latency metric, got %#v", metrics)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for async snapshot render to finish switch-session latency")
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

func countLatencyMetricsByName(metrics []UILatencyMetric, name string) int {
	count := 0
	for _, metric := range metrics {
		if metric.Name == name {
			count++
		}
	}
	return count
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
