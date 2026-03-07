package app

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestReloadPolicyHardeningCodexIgnoresMetadataChurnDuringActiveTurn(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	current := m.sessions[0]
	now := time.Now().UTC()
	handled, cmd := m.reduceStateMessages(sessionsWithMetaMsg{
		sessions: []*types.Session{
			{ID: current.ID, Provider: current.Provider, Status: current.Status, CreatedAt: current.CreatedAt, Title: current.Title},
		},
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-active", LastActiveAt: &now},
		},
	})
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no reload command for codex metadata churn")
	}
}

func TestReloadPolicyHardeningClaudeDoesNotRequireReselectForMetaRefresh(t *testing.T) {
	m := newPhase0ModelWithSession("claude")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "visible"}})
	current := m.sessions[0]
	now := time.Now().UTC()
	handled, cmd := m.reduceStateMessages(sessionsWithMetaMsg{
		sessions: []*types.Session{
			{ID: current.ID, Provider: current.Provider, Status: current.Status, CreatedAt: current.CreatedAt, Title: current.Title},
		},
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-2", LastActiveAt: &now},
		},
	})
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no selection reload for claude metadata refresh")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "visible" {
		t.Fatalf("expected transcript content to remain visible, got %q", got)
	}
}

func TestReloadPolicyHardeningKiloMetadataBurstsCoalesce(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("kilocode")
	WithTranscriptBoundaryMetricsSink(sink)(&m)
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	current := m.sessions[0]
	now := time.Now().UTC()
	first := sessionsWithMetaMsg{
		sessions: []*types.Session{
			{ID: current.ID, Provider: current.Provider, Status: current.Status, CreatedAt: current.CreatedAt, Title: current.Title},
		},
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastActiveAt: &now, LastTurnID: "turn-1"},
		},
	}
	second := sessionsWithMetaMsg{
		sessions: []*types.Session{
			{ID: current.ID, Provider: current.Provider, Status: current.Status, CreatedAt: current.CreatedAt, Title: current.Title},
		},
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastActiveAt: &now, LastTurnID: "turn-2"},
		},
	}
	handled, cmd := m.reduceStateMessages(first)
	if !handled || cmd != nil {
		t.Fatalf("expected first metadata burst update to be handled without reload")
	}
	handled, cmd = m.reduceStateMessages(second)
	if !handled || cmd != nil {
		t.Fatalf("expected second metadata burst update to be handled without reload")
	}
	metrics := sink.Snapshot()
	if len(metrics) == 0 {
		t.Fatalf("expected session reload metrics for noop/coalesced updates")
	}
	last := metrics[len(metrics)-1]
	if last.Name != transcriptMetricSessionReload || last.Reason != transcriptReasonReloadCoalescedMetadataUpdate || last.Outcome != transcriptOutcomeNoop {
		t.Fatalf("expected coalesced metadata noop metric, got %#v", last)
	}
}

func TestReloadPolicyHardeningSkipReloadResetsCoalescerState(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("kilocode")
	WithTranscriptBoundaryMetricsSink(sink)(&m)
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	current := m.sessions[0]
	now := time.Now().UTC()
	noop := sessionsWithMetaMsg{
		sessions: []*types.Session{
			{ID: current.ID, Provider: current.Provider, Status: current.Status, CreatedAt: current.CreatedAt, Title: current.Title},
		},
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastActiveAt: &now, LastTurnID: "turn-1"},
		},
	}
	_, _ = m.reduceStateMessages(noop)
	_, _ = m.reduceStateMessages(noop)

	m.mode = uiModeNotes
	reloadButSkipped := sessionsWithMetaMsg{
		sessions: []*types.Session{
			{ID: current.ID, Provider: "codex", Status: current.Status, CreatedAt: current.CreatedAt, Title: current.Title},
		},
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1"},
		},
	}
	handled, cmd := m.reduceStateMessages(reloadButSkipped)
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected notes mode to skip reload command")
	}
	m.mode = uiModeNormal
	_, _ = m.reduceStateMessages(noop)
	postSkip := m.sessions[0]
	stableNoop := sessionsWithMetaMsg{
		sessions: []*types.Session{
			{ID: postSkip.ID, Provider: postSkip.Provider, Status: postSkip.Status, CreatedAt: postSkip.CreatedAt, Title: postSkip.Title},
		},
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastActiveAt: &now, LastTurnID: "turn-1"},
		},
	}
	handled, cmd = m.reduceStateMessages(stableNoop)
	if !handled {
		t.Fatalf("expected follow-up metadata update to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected volatile metadata update to remain noop")
	}
	metrics := sink.Snapshot()
	if len(metrics) < 2 {
		t.Fatalf("expected session reload metrics")
	}
	reasons := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		if metric.Name == transcriptMetricSessionReload {
			reasons = append(reasons, metric.Reason)
		}
	}
	if len(reasons) == 0 {
		t.Fatalf("expected session reload reasons")
	}
	last := reasons[len(reasons)-1]
	if last != transcriptReasonReloadVolatileMetadataIgnored {
		t.Fatalf("expected coalescer reset after skipped reload, got last reason %q (all=%#v)", last, reasons)
	}
}
