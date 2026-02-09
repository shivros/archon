package app

import "testing"

func TestHistoryMsgCodexSkipsSnapshotWhileLiveEventsFlow(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"
	m.startRequestActivity("s1", "codex")
	m.requestActivity.eventCount = 3

	streamBlocks := []ChatBlock{
		{ID: "reasoning:r1", Role: ChatRoleReasoning, Text: "Reasoning\n\nlive stream"},
	}
	m.codexStream.SetSnapshotBlocks(streamBlocks)
	m.setSnapshotBlocks(m.codexStream.Blocks())

	msg := historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "reasoning", "id": "r1", "summary": []any{"older summary"}},
			{"type": "reasoning", "id": "r2", "summary": []any{"another summary"}},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command for history message")
	}

	blocks := m.currentBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected live stream blocks to remain unchanged, got %d", len(blocks))
	}
	if blocks[0].Text != "Reasoning\n\nlive stream" {
		t.Fatalf("expected live reasoning block to remain visible, got %q", blocks[0].Text)
	}
}

func TestHistoryMsgCodexUsesCodexStreamSnapshotWhenApplyingHistory(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"
	m.startRequestActivity("s1", "codex")

	msg := historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "reasoning", "id": "r1", "summary": []any{"first"}},
			{"type": "reasoning", "id": "r1", "summary": []any{"second"}},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command for history message")
	}

	blocks := m.currentBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected consolidated reasoning block, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleReasoning {
		t.Fatalf("expected reasoning role, got %s", blocks[0].Role)
	}
	if blocks[0].Collapsed {
		t.Fatalf("expected newest reasoning to be expanded while in compose")
	}
	if got := blocks[0].Text; got == "" || got == "Reasoning (summary)\n\n- first" {
		t.Fatalf("expected latest reasoning update text, got %q", got)
	}

	streamBlocks := m.codexStream.Blocks()
	if len(streamBlocks) != 1 {
		t.Fatalf("expected codex stream snapshot to stay aligned, got %d blocks", len(streamBlocks))
	}
	if streamBlocks[0].Text != blocks[0].Text {
		t.Fatalf("expected model and codex stream snapshots to match")
	}
}

func TestHistoryMsgCoalescesAdjacentAgentBlocksForCodex(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"

	msg := historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "agentMessage", "text": "First sentence."},
			{"type": "assistant", "content": []any{map[string]any{"type": "text", "text": "Second sentence."}}},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command for history message")
	}

	blocks := m.currentBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one coalesced assistant block, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent {
		t.Fatalf("expected agent role, got %s", blocks[0].Role)
	}
	if blocks[0].Text != "First sentence.\n\nSecond sentence." {
		t.Fatalf("unexpected coalesced text %q", blocks[0].Text)
	}
}

func TestHistoryMsgCoalescesAdjacentAgentBlocksForItemsProvider(t *testing.T) {
	m := newPhase0ModelWithSession("claude")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"

	msg := historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "assistant", "content": []any{map[string]any{"type": "text", "text": "First sentence."}}},
			{"type": "result", "result": "Second sentence."},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command for history message")
	}

	blocks := m.currentBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one coalesced assistant block, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent {
		t.Fatalf("expected agent role, got %s", blocks[0].Role)
	}
	if blocks[0].Text != "First sentence.\n\nSecond sentence." {
		t.Fatalf("unexpected coalesced text %q", blocks[0].Text)
	}
}
