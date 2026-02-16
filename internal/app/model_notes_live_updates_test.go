package app

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"control/internal/types"
)

func TestHistoryMsgDoesNotOverwriteNotesView(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	scope := enterNotesModeForSession(t, &m, "s1", "ws1")
	m.pendingSessionKey = "sess:s1"
	before := append([]ChatBlock(nil), m.currentBlocks()...)

	handled, cmd := m.reduceStateMessages(historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "assistant", "content": []any{map[string]any{"type": "text", "text": "latest agent reply"}}},
		},
	})
	if !handled {
		t.Fatalf("expected historyMsg to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command for historyMsg")
	}
	if m.mode != uiModeNotes {
		t.Fatalf("expected to remain in notes mode, got %v", m.mode)
	}
	after := m.currentBlocks()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("expected notes blocks to remain unchanged, before=%#v after=%#v", before, after)
	}
	if !noteScopeEqual(m.notesScope, scope) {
		t.Fatalf("expected notes scope to remain unchanged: got %#v want %#v", m.notesScope, scope)
	}
	cached := m.transcriptCache["sess:s1"]
	if len(cached) == 0 {
		t.Fatalf("expected transcript cache to update while notes view is active")
	}
	if cached[len(cached)-1].Role != ChatRoleAgent {
		t.Fatalf("expected cached transcript to contain agent reply, got %#v", cached)
	}
}

func TestApprovalsMsgDoesNotOverwriteNotesView(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	_ = enterNotesModeForSession(t, &m, "s1", "ws1")
	before := append([]ChatBlock(nil), m.currentBlocks()...)

	handled, cmd := m.reduceStateMessages(approvalsMsg{
		id: "s1",
		approvals: []*types.Approval{
			{
				SessionID: "s1",
				RequestID: 42,
				Method:    "tool/requestUserInput",
				Params:    json.RawMessage(`{"question":"Proceed?"}`),
				CreatedAt: time.Now().UTC(),
			},
		},
	})
	if !handled {
		t.Fatalf("expected approvalsMsg to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command for approvalsMsg")
	}
	if m.mode != uiModeNotes {
		t.Fatalf("expected to remain in notes mode, got %v", m.mode)
	}
	after := m.currentBlocks()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("expected notes blocks to remain unchanged, before=%#v after=%#v", before, after)
	}
	if m.pendingApproval == nil || m.pendingApproval.RequestID != 42 {
		t.Fatalf("expected pending approval to still update in notes mode, got %#v", m.pendingApproval)
	}
}

func TestConsumeCodexTickDoesNotOverwriteNotesView(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	_ = enterNotesModeForSession(t, &m, "s1", "ws1")
	before := append([]ChatBlock(nil), m.currentBlocks()...)

	ch := make(chan types.CodexEvent, 1)
	ch <- types.CodexEvent{
		Method: "item/updated",
		Params: json.RawMessage(`{"item":{"type":"agentMessage","delta":"stream update"}}`),
	}
	close(ch)
	m.codexStream.SetStream(ch, nil)

	m.consumeCodexTick()

	after := m.currentBlocks()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("expected notes blocks to remain unchanged, before=%#v after=%#v", before, after)
	}
	key := m.selectedKey()
	cached := m.transcriptCache[key]
	if len(cached) == 0 {
		t.Fatalf("expected transcript cache to update from codex stream while notes view is active")
	}
	if cached[len(cached)-1].Role != ChatRoleAgent {
		t.Fatalf("expected cached stream update to be an agent block, got %#v", cached)
	}
}

func enterNotesModeForSession(t *testing.T, m *Model, sessionID, workspaceID string) noteScopeTarget {
	t.Helper()
	if m.sidebar == nil || !m.sidebar.SelectBySessionID(sessionID) {
		t.Fatalf("expected %s to be selected", sessionID)
	}
	scope := noteScopeTarget{
		Scope:       types.NoteScopeSession,
		SessionID:   sessionID,
		WorkspaceID: workspaceID,
	}
	_ = m.openNotesScope(scope)
	now := time.Now().UTC()
	handled, _ := m.reduceStateMessages(notesMsg{
		scope: scope,
		notes: []*types.Note{
			{
				ID:          "n1",
				Kind:        types.NoteKindNote,
				Scope:       types.NoteScopeSession,
				SessionID:   sessionID,
				WorkspaceID: workspaceID,
				Title:       "Important follow-up",
				Body:        "Read this carefully",
				Status:      types.NoteStatusIdea,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	})
	if !handled {
		t.Fatalf("expected notesMsg to be handled")
	}
	if m.mode != uiModeNotes {
		t.Fatalf("expected notes mode, got %v", m.mode)
	}
	if len(m.currentBlocks()) == 0 {
		t.Fatalf("expected notes blocks to render")
	}
	return scope
}
