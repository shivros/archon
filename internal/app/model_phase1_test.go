package app

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

type phase1SelectionPolicy struct {
	delay time.Duration
}

func (p phase1SelectionPolicy) SelectionLoadDelay(base time.Duration) time.Duration {
	if p.delay > 0 {
		return p.delay
	}
	return base
}

func (phase1SelectionPolicy) ShouldReloadOnSessionsUpdate(previous, next sessionSelectionSnapshot) bool {
	if !next.isSession {
		return false
	}
	if !previous.isSession {
		return true
	}
	return previous.revision != next.revision || previous.sessionID != next.sessionID
}

type phase1AppStateSyncStub struct {
	calls int
}

func (s *phase1AppStateSyncStub) GetAppState(ctx context.Context) (*types.AppState, error) {
	return nil, nil
}

func (s *phase1AppStateSyncStub) UpdateAppState(ctx context.Context, state *types.AppState) (*types.AppState, error) {
	s.calls++
	if state == nil {
		return nil, nil
	}
	cloned := *state
	return &cloned, nil
}

func TestPhase1SelectionChangeLoadsImmediatelyByDefault(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}

	cmd := m.onSelectionChanged()
	if cmd == nil {
		t.Fatalf("expected load command")
	}
	if m.pendingSessionKey != "sess:s1" {
		t.Fatalf("expected immediate session load, got pending key %q", m.pendingSessionKey)
	}
	if m.status != "loading s1" {
		t.Fatalf("expected immediate loading status, got %q", m.status)
	}
}

func TestPhase1SelectionPolicyCanInjectDelay(t *testing.T) {
	m := NewModel(nil, WithSessionSelectionLoadPolicy(phase1SelectionPolicy{delay: 15 * time.Millisecond}))
	now := time.Now().UTC()
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	m.sessions = []*types.Session{
		{
			ID:        "s1",
			Provider:  "codex",
			Status:    types.SessionStatusRunning,
			CreatedAt: now,
		},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.appState.ActiveWorkspaceID = "ws1"
	m.applySidebarItems()
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}

	cmd := m.onSelectionChanged()
	if cmd == nil {
		t.Fatalf("expected debounce command")
	}
	if m.pendingSessionKey != "" {
		t.Fatalf("expected delayed load to not set pending key yet, got %q", m.pendingSessionKey)
	}
	msg := cmd()
	if _, ok := msg.(selectDebounceMsg); ok {
		return
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("expected debounce message, got %T", msg)
	}
	if _, ok := batch[0]().(selectDebounceMsg); !ok {
		t.Fatalf("expected selectDebounceMsg, got %T", batch[0]())
	}
}

func TestPhase1SelectionChangeDoesNotFetchProviderOptions(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	cmd := m.onSelectionChanged()
	if cmd == nil {
		t.Fatalf("expected load command")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected batch message, got %T", msg)
	}
	if len(batch) == 0 {
		t.Fatalf("expected non-empty selection command batch")
	}
	loadMsg := batch[0]()
	loadBatch, ok := loadMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected nested load batch, got %T", loadMsg)
	}
	// history + backfill + approvals + stream/items + events; provider options fetch is intentionally excluded.
	if len(loadBatch) != 5 {
		t.Fatalf("expected 5 selection-load commands, got %d", len(loadBatch))
	}
}

func TestPhase1SessionsWithMetaReloadsOnlyWhenSelectedRevisionChanges(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.pendingSessionKey = ""
	m.status = "steady"
	currentSession := m.sessions[0]
	currentMeta := m.sessionMeta["s1"]
	sameSessions := []*types.Session{
		{
			ID:        currentSession.ID,
			Provider:  currentSession.Provider,
			Status:    currentSession.Status,
			CreatedAt: currentSession.CreatedAt,
			Title:     currentSession.Title,
		},
	}
	sameMeta := []*types.SessionMeta{
		{
			SessionID:   currentMeta.SessionID,
			WorkspaceID: currentMeta.WorkspaceID,
			WorktreeID:  currentMeta.WorktreeID,
			LastTurnID:  currentMeta.LastTurnID,
		},
	}
	handled, cmd := m.reduceStateMessages(sessionsWithMetaMsg{sessions: sameSessions, meta: sameMeta})
	if !handled {
		t.Fatalf("expected message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no reload command when revision unchanged")
	}

	changedMeta := []*types.SessionMeta{
		{SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-2"},
	}
	handled, cmd = m.reduceStateMessages(sessionsWithMetaMsg{sessions: sameSessions, meta: changedMeta})
	if !handled {
		t.Fatalf("expected changed message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected reload command when revision changes")
	}
}

func TestPhase1AppStateSaveQueueDebouncesAndFlushes(t *testing.T) {
	stub := &phase1AppStateSyncStub{}
	m := NewModel(nil)
	m.stateAPI = stub
	m.hasAppState = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}

	first := m.requestAppStateSaveCmd()
	if first == nil {
		t.Fatalf("expected first queued save command")
	}
	if !m.appStateSaveDirty || !m.appStateSaveScheduled {
		t.Fatalf("expected queued save flags to be set")
	}
	second := m.requestAppStateSaveCmd()
	if second != nil {
		t.Fatalf("expected duplicate save request to coalesce")
	}

	flushMsg, ok := first().(appStateSaveFlushMsg)
	if !ok {
		t.Fatalf("expected appStateSaveFlushMsg, got %T", first())
	}
	nextModel, saveCmd := m.Update(flushMsg)
	next := asModel(t, nextModel)
	if saveCmd == nil {
		t.Fatalf("expected queued save to flush into save command")
	}
	if !next.appStateSaveInFlight {
		t.Fatalf("expected save to be marked in-flight")
	}
	saved, ok := saveCmd().(appStateSavedMsg)
	if !ok {
		t.Fatalf("expected appStateSavedMsg, got %T", saveCmd())
	}
	finalModel, follow := next.Update(saved)
	final := asModel(t, finalModel)
	if follow != nil {
		t.Fatalf("expected no follow-up when queue is clean")
	}
	if final.appStateSaveInFlight {
		t.Fatalf("expected in-flight flag to clear after save completion")
	}
	if stub.calls != 1 {
		t.Fatalf("expected exactly one write-behind save call, got %d", stub.calls)
	}
}
