package app

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestComposeDraftRestoresPerSession(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.SetValue("draft one")

	m.enterCompose("s2")
	if got := m.chatInput.Value(); got != "" {
		t.Fatalf("expected empty draft for new session, got %q", got)
	}
	m.chatInput.SetValue("draft two")

	m.enterCompose("s1")
	if got := m.chatInput.Value(); got != "draft one" {
		t.Fatalf("expected s1 draft restore, got %q", got)
	}
	m.enterCompose("s2")
	if got := m.chatInput.Value(); got != "draft two" {
		t.Fatalf("expected s2 draft restore, got %q", got)
	}
}

func TestComposeDraftClearsAfterSubmit(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.SetValue("hello draft")
	m.enterCompose("s2")
	m.enterCompose("s1")
	if got := m.chatInput.Value(); got != "hello draft" {
		t.Fatalf("expected restore before send, got %q", got)
	}

	cmd := m.submitComposeInput("hello draft")
	if cmd == nil {
		t.Fatalf("expected send command")
	}
	m.enterCompose("s1")
	if got := m.chatInput.Value(); got != "" {
		t.Fatalf("expected draft cleared after send, got %q", got)
	}
}

func TestNoteDraftRestoresPerScope(t *testing.T) {
	m := NewModel(nil)
	workspaceScope := noteScopeTarget{Scope: types.NoteScopeWorkspace, WorkspaceID: "ws1"}
	sessionScope := noteScopeTarget{Scope: types.NoteScopeSession, SessionID: "s1"}

	_ = m.openNotesScope(workspaceScope)
	m.enterAddNote()
	if m.noteInput == nil {
		t.Fatalf("expected note input")
	}
	m.noteInput.SetValue("workspace draft")

	_ = m.openNotesScope(sessionScope)
	m.enterAddNote()
	if got := m.noteInput.Value(); got != "" {
		t.Fatalf("expected empty draft for new note scope, got %q", got)
	}
	m.noteInput.SetValue("session draft")

	_ = m.openNotesScope(workspaceScope)
	m.enterAddNote()
	if got := m.noteInput.Value(); got != "workspace draft" {
		t.Fatalf("expected workspace draft restore, got %q", got)
	}

	_ = m.openNotesScope(sessionScope)
	m.enterAddNote()
	if got := m.noteInput.Value(); got != "session draft" {
		t.Fatalf("expected session draft restore, got %q", got)
	}
}

func TestNoteDraftClearsAfterSubmit(t *testing.T) {
	m := NewModel(nil)
	scope := noteScopeTarget{Scope: types.NoteScopeWorkspace, WorkspaceID: "ws1"}
	_ = m.openNotesScope(scope)
	m.enterAddNote()
	if m.noteInput == nil {
		t.Fatalf("expected note input")
	}
	m.noteInput.SetValue("note draft")

	cmd := m.submitAddNoteInput("note draft")
	if cmd == nil {
		t.Fatalf("expected create note command")
	}
	m.enterAddNote()
	if got := m.noteInput.Value(); got != "" {
		t.Fatalf("expected note draft cleared after submit, got %q", got)
	}
}

func TestInputDraftsPersistViaAppState(t *testing.T) {
	m := NewModel(nil)
	noteScope := noteScopeTarget{Scope: types.NoteScopeWorkspace, WorkspaceID: "ws1"}

	if changed := m.setComposeDraft("s1", "persisted compose draft"); !changed {
		t.Fatalf("expected compose draft set")
	}
	if changed := m.setNoteDraft(noteScope, "persisted note draft"); !changed {
		t.Fatalf("expected note draft set")
	}
	m.syncAppStateInputDrafts()
	state := m.appState

	n := NewModel(nil)
	n.applyAppState(&state)

	n.enterCompose("s1")
	if got := n.chatInput.Value(); got != "persisted compose draft" {
		t.Fatalf("expected persisted compose draft, got %q", got)
	}

	_ = n.openNotesScope(noteScope)
	n.enterAddNote()
	if got := n.noteInput.Value(); got != "persisted note draft" {
		t.Fatalf("expected persisted note draft, got %q", got)
	}
}

func TestComposeSelectionChangeSavesPreviousDraftAndRestoresTarget(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now, Title: "Session One"},
		{ID: "s2", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now.Add(-time.Minute), Title: "Session Two"},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	if m.sidebar == nil {
		t.Fatalf("expected sidebar")
	}
	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected to select s1")
	}
	m.enterCompose("s1")
	m.chatInput.SetValue("draft one")

	if !m.sidebar.SelectBySessionID("s2") {
		t.Fatalf("expected to select s2")
	}
	_ = m.onSelectionChangedImmediate()
	if got := m.chatInput.Value(); got != "" {
		t.Fatalf("expected empty s2 draft initially, got %q", got)
	}
	if got := m.composeDrafts["s1"]; got != "draft one" {
		t.Fatalf("expected s1 draft to be persisted on selection change, got %q", got)
	}

	m.chatInput.SetValue("draft two")
	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected to reselect s1")
	}
	_ = m.onSelectionChangedImmediate()
	if got := m.chatInput.Value(); got != "draft one" {
		t.Fatalf("expected s1 draft restore after switching back, got %q", got)
	}
	if got := m.composeDrafts["s2"]; got != "draft two" {
		t.Fatalf("expected s2 draft to persist when leaving s2, got %q", got)
	}
}
