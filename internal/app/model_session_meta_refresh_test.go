package app

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestSendMsgUpdatesSessionMetaImmediately(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	before := time.Now().UTC().Add(-1 * time.Hour)
	m.sessionMeta["s1"].LastTurnID = "turn-old"
	m.sessionMeta["s1"].LastActiveAt = &before

	handled, cmd := m.reduceStateMessages(sendMsg{id: "s1", turnID: "turn-new"})
	if !handled {
		t.Fatalf("expected sendMsg to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected follow-up commands after send")
	}
	meta := m.sessionMeta["s1"]
	if meta == nil {
		t.Fatalf("expected session meta for s1")
	}
	if meta.LastTurnID != "turn-new" {
		t.Fatalf("expected last turn id to update, got %q", meta.LastTurnID)
	}
	if meta.LastActiveAt == nil || !meta.LastActiveAt.After(before) {
		t.Fatalf("expected last active timestamp to update, got %#v", meta.LastActiveAt)
	}
	if !m.sessionMetaRefreshPending {
		t.Fatalf("expected session meta refresh to be pending")
	}
}

func TestSendMsgMarksNonSelectedSessionUnread(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "s2", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now.Add(-1 * time.Minute)},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", LastTurnID: "turn-2"},
	}
	m.applySidebarItems()
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected s1 to be selected")
	}
	m.applySidebarItems() // establish unread baseline for both sessions

	handled, _ := m.reduceStateMessages(sendMsg{id: "s2", turnID: "turn-3"})
	if !handled {
		t.Fatalf("expected sendMsg to be handled")
	}
	if m.sidebar == nil || m.sidebar.delegate == nil {
		t.Fatalf("expected sidebar delegate")
	}
	if !m.sidebar.delegate.isUnread("s2") {
		t.Fatalf("expected non-selected session s2 to be marked unread")
	}
}

func TestSendMsgRunStaysRunningOnMetaAdvanceForEventProvider(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.showRecents = true
	m.sessionMeta["s1"].LastTurnID = "turn-old"

	handled, _ := m.reduceStateMessages(sendMsg{id: "s1", turnID: "turn-new"})
	if !handled {
		t.Fatalf("expected sendMsg to be handled")
	}
	if m.recents == nil || !m.recents.IsRunning("s1") {
		t.Fatalf("expected s1 to be tracked as running after send")
	}

	handled, _ = m.reduceStateMessages(sessionsWithMetaMsg{
		sessions: m.sessions,
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-new"},
		},
	})
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if !m.recents.IsRunning("s1") {
		t.Fatalf("expected s1 to remain running; metadata should not complete event-capable runs")
	}
	if m.recents.IsReady("s1") {
		t.Fatalf("expected s1 not to enter ready queue from metadata-only updates")
	}
}

func TestSendMsgRunTransitionsToReadyOnMetaAdvanceForNonEventProvider(t *testing.T) {
	m := newPhase0ModelWithSession("claude")
	m.showRecents = true
	m.sessionMeta["s1"].LastTurnID = "turn-old"

	handled, _ := m.reduceStateMessages(sendMsg{id: "s1"})
	if !handled {
		t.Fatalf("expected sendMsg to be handled")
	}
	if m.recents == nil || !m.recents.IsRunning("s1") {
		t.Fatalf("expected s1 to be tracked as running after send")
	}

	handled, _ = m.reduceStateMessages(sessionsWithMetaMsg{
		sessions: m.sessions,
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-new"},
		},
	})
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if !m.recents.IsReady("s1") {
		t.Fatalf("expected non-event provider run to move into ready from metadata advance")
	}
	if m.recents.IsRunning("s1") {
		t.Fatalf("expected s1 to leave running after metadata completion fallback")
	}
}

func TestSendMsgUsesSendTurnIDAsFallbackBaseline(t *testing.T) {
	m := newPhase0ModelWithSession("claude")
	m.showRecents = true
	m.sessionMeta["s1"].LastTurnID = "turn-old"

	handled, _ := m.reduceStateMessages(sendMsg{id: "s1", turnID: "turn-new"})
	if !handled {
		t.Fatalf("expected sendMsg to be handled")
	}
	if m.recents == nil || !m.recents.IsRunning("s1") {
		t.Fatalf("expected s1 to be tracked as running after send")
	}

	handled, _ = m.reduceStateMessages(sessionsWithMetaMsg{
		sessions: m.sessions,
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-new"},
		},
	})
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if !m.recents.IsRunning("s1") {
		t.Fatalf("expected run to stay running when metadata equals send turn baseline")
	}
	if m.recents.IsReady("s1") {
		t.Fatalf("expected no ready transition when metadata does not advance past send turn")
	}
}

func TestSendMsgRunStaysRunningOnMetaAdvanceForUnknownProvider(t *testing.T) {
	m := newPhase0ModelWithSession("my-provider")
	m.showRecents = true
	m.sessionMeta["s1"].LastTurnID = "turn-old"

	handled, _ := m.reduceStateMessages(sendMsg{id: "s1"})
	if !handled {
		t.Fatalf("expected sendMsg to be handled")
	}
	if m.recents == nil || !m.recents.IsRunning("s1") {
		t.Fatalf("expected s1 to be tracked as running after send")
	}

	handled, _ = m.reduceStateMessages(sessionsWithMetaMsg{
		sessions: m.sessions,
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-new"},
		},
	})
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if !m.recents.IsRunning("s1") {
		t.Fatalf("expected unknown-provider run to remain running without explicit completion signal")
	}
	if m.recents.IsReady("s1") {
		t.Fatalf("expected unknown-provider run not to enter ready from metadata-only updates")
	}
}

func TestMaybeAutoRefreshSessionMetaPrefersSyncWhenDue(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.lastSessionMetaRefreshAt = now.Add(-sessionMetaRefreshDelay - time.Second)
	m.lastSessionMetaSyncAt = now.Add(-sessionMetaSyncDelay - time.Second)

	cmd := m.maybeAutoRefreshSessionMeta(now)
	if cmd == nil {
		t.Fatalf("expected automatic refresh command when sync is due")
	}
	if !m.sessionMetaSyncPending {
		t.Fatalf("expected sync refresh to be marked pending")
	}
	if m.sessionMetaRefreshPending {
		t.Fatalf("did not expect lightweight refresh pending when sync is selected")
	}
}

func TestSessionsWithMetaMsgClearsAutoRefreshPending(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.sessionMetaRefreshPending = true
	m.sessionMetaSyncPending = true

	handled, _ := m.reduceStateMessages(sessionsWithMetaMsg{
		sessions: m.sessions,
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1"},
		},
	})
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if m.sessionMetaRefreshPending || m.sessionMetaSyncPending {
		t.Fatalf("expected auto refresh pending flags to clear")
	}
}

func TestSessionsWithMetaMsgSkipsReloadWhileViewingNotes(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected s1 to be selected")
	}
	m.mode = uiModeNotes

	current := m.sessions[0]
	handled, cmd := m.reduceStateMessages(sessionsWithMetaMsg{
		sessions: []*types.Session{
			{
				ID:        current.ID,
				Provider:  current.Provider,
				Status:    current.Status,
				CreatedAt: current.CreatedAt,
				Title:     current.Title,
			},
		},
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-2"},
		},
	})
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no session reload command while in notes mode")
	}
	if m.mode != uiModeNotes {
		t.Fatalf("expected to stay in notes mode, got %v", m.mode)
	}
}

func TestSessionsWithMetaMsgSkipsReloadWhenFollowPaused(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected s1 to be selected")
	}
	m.follow = false

	current := m.sessions[0]
	handled, cmd := m.reduceStateMessages(sessionsWithMetaMsg{
		sessions: []*types.Session{
			{
				ID:        current.ID,
				Provider:  current.Provider,
				Status:    current.Status,
				CreatedAt: current.CreatedAt,
				Title:     current.Title,
			},
		},
		meta: []*types.SessionMeta{
			{SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-2"},
		},
	})
	if !handled {
		t.Fatalf("expected sessionsWithMetaMsg to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no session reload command while follow is paused")
	}
	if m.follow {
		t.Fatalf("expected follow to remain paused")
	}
}
