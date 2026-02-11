package app

import (
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

func TestNotesToBlocksIncludesHeaderAndNoteBlocks(t *testing.T) {
	now := time.Now().UTC()
	blocks := notesToBlocks([]*types.Note{
		{
			ID:        "n1",
			Kind:      types.NoteKindNote,
			Scope:     types.NoteScopeWorkspace,
			Body:      "Remember to split this out",
			Status:    types.NoteStatusTodo,
			Tags:      []string{"refactor"},
			UpdatedAt: now,
		},
		{
			ID:    "n2",
			Kind:  types.NoteKindPin,
			Scope: types.NoteScopeSession,
			Source: &types.NoteSource{
				SessionID: "s1",
				Snippet:   "Keep this for later",
			},
			UpdatedAt: now,
		},
	}, noteScopeTarget{Scope: types.NoteScopeWorkspace, WorkspaceID: "ws1"}, notesFilterState{ShowWorkspace: true})

	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks (header + 2 notes), got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleSystem || !strings.Contains(blocks[0].Text, "Scope: workspace ws1") {
		t.Fatalf("unexpected header block: %#v", blocks[0])
	}
	if blocks[1].Role != ChatRoleWorkspaceNote || !strings.Contains(blocks[1].Text, "Remember to split this out") {
		t.Fatalf("unexpected first note block: %#v", blocks[1])
	}
	if strings.Contains(blocks[1].Text, "#1") {
		t.Fatalf("unexpected redundant summary line in first note: %q", blocks[1].Text)
	}
	if blocks[2].Role != ChatRoleSessionNote || !strings.Contains(blocks[2].Text, "Pinned from conversation") {
		t.Fatalf("unexpected pin block: %#v", blocks[2])
	}
}

func TestReduceStateMessagesNotesMsgRendersBlocks(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeNotes
	m.setNotesRootScope(noteScopeTarget{Scope: types.NoteScopeSession, SessionID: "s1"})
	handled, _ := m.reduceStateMessages(notesMsg{
		scope: noteScopeTarget{Scope: types.NoteScopeSession, SessionID: "s1"},
		notes: []*types.Note{
			{ID: "n1", Kind: types.NoteKindNote, Body: "Hello"},
		},
	})
	if !handled {
		t.Fatalf("expected notes message to be handled")
	}
	if len(m.contentBlocks) < 2 {
		t.Fatalf("expected rendered note blocks, got %#v", m.contentBlocks)
	}
	if m.contentBlocks[1].ID != "n1" {
		t.Fatalf("expected note block id n1, got %q", m.contentBlocks[1].ID)
	}
}

func TestHandleConfirmChoiceDeleteNoteReturnsCommand(t *testing.T) {
	m := NewModel(nil)
	m.confirm.Open("Delete Note", "Delete note?", "Delete", "Cancel")
	m.pendingConfirm = confirmAction{kind: confirmDeleteNote, noteID: "n1"}

	cmd := m.handleConfirmChoice(confirmChoiceConfirm)
	if cmd == nil {
		t.Fatalf("expected delete note command")
	}
	if m.status != "deleting note" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestReduceStateMessagesNoteDeletedRefreshesNotesMode(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNotes
	m.setNotesRootScope(noteScopeTarget{Scope: types.NoteScopeSession, SessionID: "s1"})

	handled, cmd := m.reduceStateMessages(noteDeletedMsg{id: "n1"})
	if !handled {
		t.Fatalf("expected noteDeletedMsg to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected notes refresh command")
	}
	if m.status != "note deleted" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestNoteMoveSessionOptionsExcludeDismissedAndIncludeWorkspaceWide(t *testing.T) {
	m := NewModel(nil)
	m.worktrees["ws1"] = []*types.Worktree{{ID: "wt1", WorkspaceID: "ws1", Name: "feature"}}
	now := time.Now().UTC()
	m.sessions = []*types.Session{
		{ID: "s-workspace", Status: types.SessionStatusRunning, CreatedAt: now},
		{ID: "s-worktree", Status: types.SessionStatusRunning, CreatedAt: now.Add(-time.Minute)},
		{ID: "s-dismissed", Status: types.SessionStatusExited, CreatedAt: now.Add(-2 * time.Minute)},
	}
	m.sessionMeta["s-workspace"] = &types.SessionMeta{SessionID: "s-workspace", WorkspaceID: "ws1"}
	m.sessionMeta["s-worktree"] = &types.SessionMeta{SessionID: "s-worktree", WorkspaceID: "ws1", WorktreeID: "wt1"}
	m.sessionMeta["s-dismissed"] = &types.SessionMeta{SessionID: "s-dismissed", WorkspaceID: "ws1"}

	options := m.noteMoveSessionOptions("ws1")
	if len(options) != 2 {
		t.Fatalf("expected 2 visible workspace sessions, got %d (%#v)", len(options), options)
	}
	ids := []string{options[0].id, options[1].id}
	joined := strings.Join(ids, ",")
	if !strings.Contains(joined, "s-workspace") || !strings.Contains(joined, "s-worktree") {
		t.Fatalf("expected workspace and worktree sessions in options, got %v", ids)
	}
	if strings.Contains(joined, "s-dismissed") {
		t.Fatalf("expected dismissed session to be excluded, got %v", ids)
	}
}

func TestNoteMoveTargetOptionsSessionAllowWorkspaceAndWorktree(t *testing.T) {
	m := NewModel(nil)
	m.worktrees["ws1"] = []*types.Worktree{{ID: "wt1", WorkspaceID: "ws1", Name: "feature"}}
	m.sessionMeta["s1"] = &types.SessionMeta{SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1"}
	note := &types.Note{
		ID:          "n1",
		Scope:       types.NoteScopeSession,
		SessionID:   "s1",
		WorkspaceID: "ws1",
		WorktreeID:  "wt1",
	}

	options := m.noteMoveTargetOptions(note)
	if len(options) != 2 {
		t.Fatalf("expected workspace and worktree targets for session notes, got %d (%#v)", len(options), options)
	}
	ids := options[0].id + "," + options[1].id
	if !strings.Contains(ids, noteMoveTargetWorkspace) || !strings.Contains(ids, noteMoveTargetWorktree) {
		t.Fatalf("expected workspace and worktree targets, got %q", ids)
	}
}

func TestNoteMoveTargetOptionsWorkspaceAllowsSessionWithoutWorktrees(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.sessions = []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta["s1"] = &types.SessionMeta{SessionID: "s1", WorkspaceID: "ws1"}
	note := &types.Note{
		ID:          "n1",
		Scope:       types.NoteScopeWorkspace,
		WorkspaceID: "ws1",
	}

	options := m.noteMoveTargetOptions(note)
	if len(options) != 1 {
		t.Fatalf("expected only session target for workspace note without worktrees, got %d (%#v)", len(options), options)
	}
	if options[0].id != noteMoveTargetSession {
		t.Fatalf("expected session target, got %q", options[0].id)
	}
}

func TestCommitNoteMoveRejectsCrossWorkspaceTargets(t *testing.T) {
	m := NewModel(nil)
	m.worktrees["ws2"] = []*types.Worktree{{ID: "wt2", WorkspaceID: "ws2"}}
	note := &types.Note{
		ID:          "n1",
		Scope:       types.NoteScopeWorkspace,
		WorkspaceID: "ws1",
	}

	cmd := m.commitNoteMove(note, noteScopeTarget{
		Scope:       types.NoteScopeWorktree,
		WorkspaceID: "ws2",
		WorktreeID:  "wt2",
	})
	if cmd != nil {
		t.Fatalf("expected cross-workspace target to be rejected")
	}
	if m.status != "cross-workspace note move is not supported" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestHandleNoteMovePickerSelectionDirectWorktreeToWorkspace(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNotes
	m.notes = []*types.Note{
		{
			ID:          "n1",
			Scope:       types.NoteScopeWorktree,
			WorkspaceID: "ws1",
			WorktreeID:  "wt1",
		},
	}
	m.noteMoveNoteID = "n1"
	m.noteMoveReturnMode = uiModeNotes
	m.mode = uiModePickNoteMoveTarget
	if m.groupSelectPicker != nil {
		m.groupSelectPicker.SetOptions([]selectOption{{id: noteMoveTargetWorkspace, label: "Move to Workspace"}})
	}

	cmd := m.handleNoteMovePickerSelection()
	if cmd == nil {
		t.Fatalf("expected move command")
	}
	if m.mode != uiModeNotes {
		t.Fatalf("expected to return to notes mode, got %v", m.mode)
	}
	if m.noteMoveNoteID != "" {
		t.Fatalf("expected move state to clear, got %q", m.noteMoveNoteID)
	}
	if m.status != "moving note" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestReduceStateMessagesNoteMovedRefreshesNotesMode(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNotes
	m.setNotesRootScope(noteScopeTarget{Scope: types.NoteScopeSession, SessionID: "s1"})

	handled, cmd := m.reduceStateMessages(noteMovedMsg{
		note: &types.Note{
			ID:          "n1",
			Scope:       types.NoteScopeWorkspace,
			WorkspaceID: "ws1",
		},
		previous: &types.Note{
			ID:        "n1",
			Scope:     types.NoteScopeSession,
			SessionID: "s1",
		},
	})
	if !handled {
		t.Fatalf("expected noteMovedMsg to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected notes refresh command")
	}
	if m.status != "note moved" {
		t.Fatalf("unexpected status %q", m.status)
	}
}
