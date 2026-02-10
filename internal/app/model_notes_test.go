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
	}, noteScopeTarget{Scope: types.NoteScopeWorkspace, WorkspaceID: "ws1"})

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
	m.notesScope = noteScopeTarget{Scope: types.NoteScopeSession, SessionID: "s1"}

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
