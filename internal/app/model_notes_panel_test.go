package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/types"
)

func TestToggleNotesPanelOpensAndFetchesCurrentScope(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection")
	}
	m.resize(120, 40)

	cmd := m.toggleNotesPanel()
	if !m.notesPanelOpen {
		t.Fatalf("expected notes panel to open")
	}
	if cmd == nil {
		t.Fatalf("expected fetch command when opening notes panel")
	}
	if m.notesScope.Scope != types.NoteScopeSession || m.notesScope.SessionID != "s1" {
		t.Fatalf("unexpected notes scope: %#v", m.notesScope)
	}
	if !m.notesFilters.ShowWorkspace || !m.notesFilters.ShowWorktree || !m.notesFilters.ShowSession {
		t.Fatalf("expected all available filters on by default, got %#v", m.notesFilters)
	}
}

func TestReduceStateMessagesNotesMsgUpdatesNotesPanel(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(240, 40)
	scope := noteScopeTarget{Scope: types.NoteScopeSession, SessionID: "s1"}
	m.setNotesRootScope(scope)

	handled, _ := m.reduceStateMessages(notesMsg{
		scope: scope,
		notes: []*types.Note{
			{ID: "n1", Scope: types.NoteScopeSession, SessionID: "s1", Body: "panel note"},
		},
	})
	if !handled {
		t.Fatalf("expected notes message to be handled")
	}
	found := false
	for _, block := range m.notesPanelBlocks {
		if block.ID == "n1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected panel blocks to include note n1, got %#v", m.notesPanelBlocks)
	}
}

func TestReduceStateMessagesNotePinnedMsgImmediatelyUpsertsPanel(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(120, 40)
	scope := noteScopeTarget{Scope: types.NoteScopeSession, SessionID: "s1"}
	m.setNotesRootScope(scope)
	now := time.Now()
	m.applyNotesScopeResult(scope, []*types.Note{{
		ID:        "n0",
		Scope:     types.NoteScopeSession,
		SessionID: "s1",
		Body:      "older",
		UpdatedAt: now.Add(-time.Minute),
		CreatedAt: now.Add(-time.Minute),
	}})

	handled, cmd := m.reduceStateMessages(notePinnedMsg{
		note: &types.Note{
			ID:        "n1",
			Scope:     types.NoteScopeSession,
			SessionID: "s1",
			Body:      "newest",
			UpdatedAt: now,
			CreatedAt: now,
		},
		sessionID: "s1",
	})
	if !handled {
		t.Fatalf("expected notePinnedMsg to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected refresh command for open views")
	}
	if len(m.notes) == 0 || m.notes[0] == nil || m.notes[0].ID != "n1" {
		t.Fatalf("expected pinned note to appear first, got %#v", m.notes)
	}
}

func TestMouseReducerNotesPanelCopyClickHandled(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(120, 40)
	m.notesPanelBlocks = []ChatBlock{
		{ID: "n1", Role: ChatRoleSessionNote, Text: "   ", Status: ChatStatusSending},
	}
	m.renderNotesPanel()
	if len(m.notesPanelSpans) != 1 {
		t.Fatalf("expected notes panel spans, got %d", len(m.notesPanelSpans))
	}
	span := m.notesPanelSpans[0]
	if span.CopyLine < 0 || span.CopyStart < 0 {
		t.Fatalf("expected copy hitbox on panel block, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	if !layout.panelVisible {
		t.Fatalf("expected panel to be visible")
	}
	x := layout.panelStart + span.CopyStart
	y := span.CopyLine - m.notesPanelViewport.YOffset + 1
	handled := m.reduceNotesPanelLeftPressMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      y,
	}, layout)
	if !handled {
		t.Fatalf("expected panel copy click to be handled")
	}
	if m.status != "nothing to copy" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerNotesPanelMoveClickOpensMovePicker(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(120, 40)
	m.worktrees["ws1"] = []*types.Worktree{{ID: "wt1", WorkspaceID: "ws1", Name: "wt-one"}}
	m.notes = []*types.Note{
		{
			ID:          "n1",
			Scope:       types.NoteScopeWorkspace,
			WorkspaceID: "ws1",
		},
	}
	m.notesPanelBlocks = []ChatBlock{
		{ID: "n1", Role: ChatRoleWorkspaceNote, Text: "move me"},
	}
	m.renderNotesPanel()
	if len(m.notesPanelSpans) != 1 {
		t.Fatalf("expected notes panel spans, got %d", len(m.notesPanelSpans))
	}
	span := m.notesPanelSpans[0]
	if span.MoveLine < 0 || span.MoveStart < 0 {
		t.Fatalf("expected move hitbox on panel block, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	if !layout.panelVisible {
		t.Fatalf("expected panel to be visible")
	}
	x := layout.panelStart + span.MoveStart
	y := span.MoveLine - m.notesPanelViewport.YOffset + 1
	handled := m.reduceNotesPanelLeftPressMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      y,
	}, layout)
	if !handled {
		t.Fatalf("expected panel move click to be handled")
	}
	if m.mode != uiModePickNoteMoveWorktree {
		t.Fatalf("expected worktree picker mode, got %v", m.mode)
	}
}

func TestMouseReducerNotesPanelDeleteClickOpensConfirm(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(120, 40)
	m.notes = []*types.Note{
		{
			ID:        "n1",
			Scope:     types.NoteScopeSession,
			SessionID: "s1",
			Title:     "delete me",
		},
	}
	m.notesPanelBlocks = []ChatBlock{
		{ID: "n1", Role: ChatRoleSessionNote, Text: "delete me"},
	}
	m.notesPanelViewport.Width = 80
	m.notesPanelWidth = 80
	m.renderNotesPanel()
	if len(m.notesPanelSpans) != 1 {
		t.Fatalf("expected notes panel spans, got %d", len(m.notesPanelSpans))
	}
	span := m.notesPanelSpans[0]
	if span.DeleteLine < 0 || span.DeleteStart < 0 {
		t.Fatalf("expected delete hitbox on panel block, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	if !layout.panelVisible {
		t.Fatalf("expected panel to be visible")
	}
	x := layout.panelStart + span.DeleteStart
	y := span.DeleteLine - m.notesPanelViewport.YOffset + 1
	handled := m.reduceNotesPanelLeftPressMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      y,
	}, layout)
	if !handled {
		t.Fatalf("expected panel delete click to be handled")
	}
	if m.pendingConfirm.kind != confirmDeleteNote || m.pendingConfirm.noteID != "n1" {
		t.Fatalf("unexpected pending confirm: %#v", m.pendingConfirm)
	}
	if m.confirm == nil || !m.confirm.IsOpen() {
		t.Fatalf("expected confirm dialog to open")
	}
}

func TestNotesModeKeyboardFilterToggle(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNotes
	m.setNotesRootScope(noteScopeTarget{
		Scope:       types.NoteScopeSession,
		WorkspaceID: "ws1",
		WorktreeID:  "wt1",
		SessionID:   "s1",
	})
	if !m.notesFilters.ShowWorkspace {
		t.Fatalf("expected workspace filter enabled by default")
	}
	handled, _ := m.reduceNotesModeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if !handled {
		t.Fatalf("expected key to be handled")
	}
	if m.notesFilters.ShowWorkspace {
		t.Fatalf("expected workspace filter to toggle off")
	}
}

func TestMouseReducerNotesPanelFilterToggleHandled(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(120, 40)
	m.setNotesRootScope(noteScopeTarget{
		Scope:       types.NoteScopeSession,
		WorkspaceID: "ws1",
		WorktreeID:  "wt1",
		SessionID:   "s1",
	})
	m.renderNotesViewsFromState()
	if !m.notesFilters.ShowWorkspace {
		t.Fatalf("expected workspace filter enabled by default")
	}
	layout := m.resolveMouseLayout()
	if !layout.panelVisible {
		t.Fatalf("expected panel visible")
	}
	lines := strings.Split(xansi.Strip(m.notesPanelViewport.View()), "\n")
	filterRow := -1
	filterCol := -1
	for i, line := range lines {
		idx := strings.Index(line, "Workspace")
		if idx >= 0 {
			filterRow = i + 1
			filterCol = idx
			break
		}
	}
	if filterRow < 0 || filterCol < 0 {
		t.Fatalf("expected filter token in panel view, got %q", xansi.Strip(m.notesPanelViewport.View()))
	}
	handled := m.reduceNotesPanelLeftPressMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      layout.panelStart + filterCol,
		Y:      filterRow,
	}, layout)
	if !handled {
		t.Fatalf("expected panel filter click handled")
	}
	if m.notesFilters.ShowWorkspace {
		t.Fatalf("expected workspace filter to toggle off")
	}
}

func TestUpdateNotesMsgWhileAddNoteModeUpdatesNotesPanel(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelOpen = true
	m.resize(240, 40)
	m.mode = uiModeAddNote
	scope := noteScopeTarget{
		Scope:     types.NoteScopeSession,
		SessionID: "s1",
	}
	m.setNotesRootScope(scope)

	_, _ = m.Update(notesMsg{
		scope: scope,
		notes: []*types.Note{
			{ID: "n1", Scope: types.NoteScopeSession, SessionID: "s1", Body: "panel while composing"},
		},
	})

	found := false
	for _, block := range m.notesPanelBlocks {
		if block.ID == "n1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected notes panel blocks to include n1 in add note mode, got %#v", m.notesPanelBlocks)
	}
}
