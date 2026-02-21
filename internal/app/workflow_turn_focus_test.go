package app

import "testing"

func TestFirstUserBlockIndexForTurnPrefersUserMessage(t *testing.T) {
	items := []map[string]any{
		{
			"type":    "userMessage",
			"turn_id": "turn-1",
			"content": []any{
				map[string]any{"type": "text", "text": "request"},
			},
		},
		{
			"type":    "assistant",
			"turn_id": "turn-1",
			"message": map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "response"},
				},
			},
		},
	}
	blocks := itemsToBlocks(items)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %#v", blocks)
	}
	if blocks[0].Role != ChatRoleUser || blocks[0].TurnID != "turn-1" {
		t.Fatalf("expected first block to be user turn-1, got %#v", blocks[0])
	}
	if blocks[1].Role != ChatRoleAgent || blocks[1].TurnID != "turn-1" {
		t.Fatalf("expected second block to be agent turn-1, got %#v", blocks[1])
	}
	if idx := firstUserBlockIndexForTurn(blocks, "turn-1"); idx != 0 {
		t.Fatalf("expected user block index 0 for turn-1, got %d", idx)
	}
}

func TestSetPendingWorkflowTurnFocusGuardsAndClearing(t *testing.T) {
	var nilModel *Model
	nilModel.setPendingWorkflowTurnFocus("s1", "turn-1")

	m := NewModel(nil)
	m.setPendingWorkflowTurnFocus("s1", "turn-1")
	if m.pendingWorkflowTurnFocus == nil {
		t.Fatalf("expected pending turn focus to be set")
	}
	m.setPendingWorkflowTurnFocus(" ", "turn-1")
	if m.pendingWorkflowTurnFocus != nil {
		t.Fatalf("expected missing session id to clear pending turn focus")
	}
	m.setPendingWorkflowTurnFocus("s1", "turn-1")
	m.setPendingWorkflowTurnFocus("s1", " ")
	if m.pendingWorkflowTurnFocus != nil {
		t.Fatalf("expected missing turn id to clear pending turn focus")
	}
}

func TestApplyPendingWorkflowTurnFocusNoopWhenMissingRequestOrSessionMismatch(t *testing.T) {
	m := NewModel(nil)
	m.status = "ready"
	m.applyPendingWorkflowTurnFocus(sessionProjectionSourceHistory, "s1", []ChatBlock{{Role: ChatRoleUser, TurnID: "turn-1"}})
	if m.status != "ready" {
		t.Fatalf("expected no-op without pending request, got status %q", m.status)
	}

	m.setPendingWorkflowTurnFocus("s1", "turn-1")
	m.applyPendingWorkflowTurnFocus(sessionProjectionSourceHistory, "s2", []ChatBlock{{Role: ChatRoleUser, TurnID: "turn-1"}})
	if m.pendingWorkflowTurnFocus == nil {
		t.Fatalf("expected pending request to remain on session mismatch")
	}
}

func TestApplyPendingWorkflowTurnFocusNotFoundBehaviorBySource(t *testing.T) {
	m := NewModel(nil)
	m.setPendingWorkflowTurnFocus("s1", "turn-1")
	m.applyPendingWorkflowTurnFocus(sessionProjectionSourceTail, "s1", []ChatBlock{{Role: ChatRoleUser, TurnID: "turn-2"}})
	if m.pendingWorkflowTurnFocus == nil {
		t.Fatalf("expected pending request to remain for non-history source")
	}

	m.applyPendingWorkflowTurnFocus(sessionProjectionSourceHistory, "s1", []ChatBlock{{Role: ChatRoleUser, TurnID: "turn-2"}})
	if m.pendingWorkflowTurnFocus != nil {
		t.Fatalf("expected history source miss to clear pending request")
	}
	if m.status != "linked workflow turn not found in loaded history" {
		t.Fatalf("unexpected warning status: %q", m.status)
	}
}

func TestApplyPendingWorkflowTurnFocusMatchWithHiddenViewportClearsWithoutSelecting(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNotes
	m.setPendingWorkflowTurnFocus("s1", "turn-1")
	m.applyPendingWorkflowTurnFocus(sessionProjectionSourceHistory, "s1", []ChatBlock{
		{Role: ChatRoleUser, TurnID: "turn-1", Text: "request"},
	})
	if m.pendingWorkflowTurnFocus != nil {
		t.Fatalf("expected pending request to clear on successful match")
	}
	if m.messageSelectActive {
		t.Fatalf("expected selection to remain inactive when transcript viewport is hidden")
	}
}
