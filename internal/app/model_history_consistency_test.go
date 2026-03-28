package app

import (
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

func TestHistoryMsgCodexSkipsSnapshotWhileLiveEventsFlow(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"
	m.startRequestActivity("s1", "codex")
	m.requestActivity.eventCount = 3

	streamBlocks := []ChatBlock{
		{ID: "reasoning:r1", Role: ChatRoleReasoning, Text: "Reasoning\nlive stream"},
	}
	_, _ = m.transcriptStream.SetSnapshot(transcriptdomain.TranscriptSnapshot{
		Revision: transcriptdomain.MustParseRevisionToken("1"),
		Blocks: []transcriptdomain.Block{
			{Kind: "reasoning", Role: "reasoning", Text: "Reasoning\nlive stream", Meta: map[string]any{"id": "reasoning:r1"}},
		},
	})
	m.setSnapshotBlocks(streamBlocks)

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
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected live stream blocks to remain unchanged, got %d", len(blocks))
	}
	if blocks[0].Text != "Reasoning\nlive stream" {
		t.Fatalf("expected live reasoning block to remain visible, got %q", blocks[0].Text)
	}
}

func TestHistoryMsgCodexUsesProjectedSnapshotWhenApplyingHistory(t *testing.T) {
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
	applyProjectedSessionCmd(t, &m, cmd)

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

	cached := m.transcriptCache["sess:s1"]
	if len(cached) != 1 {
		t.Fatalf("expected transcript cache snapshot to stay aligned, got %d blocks", len(cached))
	}
	if cached[0].Text != blocks[0].Text {
		t.Fatalf("expected model and cached snapshots to match")
	}
}

func TestHistoryMsgSplitsAdjacentAgentBlocksForCodex(t *testing.T) {
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
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected split assistant blocks, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent || blocks[1].Role != ChatRoleAgent {
		t.Fatalf("expected agent roles, got %#v", blocks)
	}
	if blocks[0].Text != "First sentence." || blocks[1].Text != "Second sentence." {
		t.Fatalf("unexpected split text %#v", blocks)
	}
}

func TestHistoryMsgSplitsAdjacentAgentBlocksForItemsProvider(t *testing.T) {
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
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected split assistant blocks, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent || blocks[1].Role != ChatRoleAgent {
		t.Fatalf("expected agent roles, got %#v", blocks)
	}
	if blocks[0].Text != "First sentence." || blocks[1].Text != "Second sentence." {
		t.Fatalf("unexpected split text %#v", blocks)
	}
}

func TestHistoryReplayDoesNotDuplicateExistingClaudeTranscript(t *testing.T) {
	m := newPhase0ModelWithSession("claude")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"

	items := []map[string]any{
		{
			"type":       "userMessage",
			"created_at": "2026-02-27T05:11:57.000000000Z",
			"text":       "What's the current git status?",
		},
		{
			"type":       "assistant",
			"created_at": "2026-02-27T05:12:02.000000000Z",
			"message": map[string]any{
				"content": []any{
					map[string]any{"type": "text", "text": "On branch main."},
				},
			},
		},
	}

	handled, cmd := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: items,
	})
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	applyProjectedSessionCmd(t, &m, cmd)

	handled, cmd = m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: items,
	})
	if !handled {
		t.Fatalf("expected replayed history message to be handled")
	}
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected replayed stream snapshot to remain deduped, got %#v", blocks)
	}
	if blocks[0].Role != ChatRoleUser || blocks[1].Role != ChatRoleAgent {
		t.Fatalf("unexpected role order after replay: %#v", blocks)
	}
}

func TestHistoryMsgCodexCoalescesAdjacentReasoningIDs(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"

	msg := historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "reasoning", "id": "r1", "summary": []any{"first"}},
			{"type": "reasoning", "id": "r2", "summary": []any{"second"}},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one coalesced reasoning block, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleReasoning {
		t.Fatalf("expected reasoning role, got %s", blocks[0].Role)
	}
	if blocks[0].Text != "- first\n\n- second" {
		t.Fatalf("unexpected coalesced reasoning text %q", blocks[0].Text)
	}
}

func TestHistoryMsgCodexKeepsApprovalsInRelativeOrder(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"

	m.setApprovalsForSession("s1", []*ApprovalRequest{
		{
			RequestID: 10,
			SessionID: "s1",
			Method:    "item/commandExecution/requestApproval",
			Summary:   "command",
			Detail:    "first",
			CreatedAt: time.Date(2026, 2, 10, 10, 0, 0, 0, time.UTC),
		},
		{
			RequestID: 11,
			SessionID: "s1",
			Method:    "item/commandExecution/requestApproval",
			Summary:   "command",
			Detail:    "second",
			CreatedAt: time.Date(2026, 2, 10, 10, 1, 0, 0, time.UTC),
		},
	})
	m.setSnapshotBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "user one"},
		{Role: ChatRoleAgent, Text: "agent one"},
		{Role: ChatRoleApproval, ID: approvalBlockID(10), RequestID: 10, SessionID: "s1", Text: "Approval required: command"},
		{Role: ChatRoleApproval, ID: approvalBlockID(11), RequestID: 11, SessionID: "s1", Text: "Approval required: command"},
		{Role: ChatRoleUser, Text: "user two"},
	})

	msg := historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "userMessage", "text": "user one"},
			{"type": "agentMessage", "text": "agent one"},
			{"type": "userMessage", "text": "user two"},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	expectedRoles := []ChatRole{ChatRoleUser, ChatRoleAgent, ChatRoleApproval, ChatRoleApproval, ChatRoleUser}
	if len(blocks) != len(expectedRoles) {
		t.Fatalf("expected %d blocks, got %#v", len(expectedRoles), blocks)
	}
	for i, want := range expectedRoles {
		if blocks[i].Role != want {
			t.Fatalf("unexpected role at %d: got %s want %s (blocks=%#v)", i, blocks[i].Role, want, blocks)
		}
	}
	if blocks[2].RequestID != 10 || blocks[3].RequestID != 11 {
		t.Fatalf("unexpected approval order: %#v", blocks)
	}
}

func TestTailMsgCodexKeepsApprovalsInRelativeOrder(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"

	m.setApprovalsForSession("s1", []*ApprovalRequest{
		{
			RequestID: 20,
			SessionID: "s1",
			Method:    "item/commandExecution/requestApproval",
			Summary:   "command",
			Detail:    "first",
			CreatedAt: time.Date(2026, 2, 10, 11, 0, 0, 0, time.UTC),
		},
		{
			RequestID: 21,
			SessionID: "s1",
			Method:    "item/commandExecution/requestApproval",
			Summary:   "command",
			Detail:    "second",
			CreatedAt: time.Date(2026, 2, 10, 11, 1, 0, 0, time.UTC),
		},
	})
	m.setSnapshotBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "user one"},
		{Role: ChatRoleAgent, Text: "agent one"},
		{Role: ChatRoleApproval, ID: approvalBlockID(20), RequestID: 20, SessionID: "s1", Text: "Approval required: command"},
		{Role: ChatRoleApproval, ID: approvalBlockID(21), RequestID: 21, SessionID: "s1", Text: "Approval required: command"},
		{Role: ChatRoleUser, Text: "user two"},
	})

	msg := tailMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "userMessage", "text": "user one"},
			{"type": "agentMessage", "text": "agent one"},
			{"type": "userMessage", "text": "user two"},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected tail message to be handled")
	}
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	expectedRoles := []ChatRole{ChatRoleUser, ChatRoleAgent, ChatRoleApproval, ChatRoleApproval, ChatRoleUser}
	if len(blocks) != len(expectedRoles) {
		t.Fatalf("expected %d blocks, got %#v", len(expectedRoles), blocks)
	}
	for i, want := range expectedRoles {
		if blocks[i].Role != want {
			t.Fatalf("unexpected role at %d: got %s want %s (blocks=%#v)", i, blocks[i].Role, want, blocks)
		}
	}
	if blocks[2].RequestID != 20 || blocks[3].RequestID != 21 {
		t.Fatalf("unexpected approval order: %#v", blocks)
	}
}

func TestHistoryMsgItemsProviderMergesPendingApprovals(t *testing.T) {
	m := newPhase0ModelWithSession("kilocode")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"

	m.setApprovalsForSession("s1", []*ApprovalRequest{
		{
			RequestID: 31,
			SessionID: "s1",
			Method:    "tool/requestUserInput",
			Summary:   "user input",
			Detail:    "confirm deployment target",
			CreatedAt: time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC),
		},
	})

	msg := historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "assistant", "content": []any{map[string]any{"type": "text", "text": "I need one confirmation."}}},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected assistant + approval blocks, got %#v", blocks)
	}
	if blocks[0].Role != ChatRoleAgent {
		t.Fatalf("expected assistant block first, got %#v", blocks[0])
	}
	if blocks[1].Role != ChatRoleApproval || blocks[1].RequestID != 31 {
		t.Fatalf("expected approval block to be preserved, got %#v", blocks[1])
	}
}

func TestHistoryMsgClaudeDoesNotMergeApprovals(t *testing.T) {
	m := newPhase0ModelWithSession("claude")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"

	// Claude does not expose approval events through this pipeline; ensure no
	// cross-provider approval blocks leak into Claude history rendering.
	m.setApprovalsForSession("s1", []*ApprovalRequest{
		{
			RequestID: 41,
			SessionID: "s1",
			Method:    "tool/requestUserInput",
			Summary:   "user input",
			Detail:    "this should not render for claude",
			CreatedAt: time.Date(2026, 2, 14, 12, 1, 0, 0, time.UTC),
		},
	})

	msg := historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "assistant", "content": []any{map[string]any{"type": "text", "text": "Claude reply."}}},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	if len(blocks) != 1 {
		t.Fatalf("expected only assistant block, got %#v", blocks)
	}
	if blocks[0].Role != ChatRoleAgent {
		t.Fatalf("expected assistant role, got %#v", blocks[0])
	}
}

func TestHistoryMsgClaudeMergesExitPlanApproval(t *testing.T) {
	m := newPhase0ModelWithSession("claude")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"

	m.setApprovalsForSession("s1", []*ApprovalRequest{
		{
			RequestID: 42,
			SessionID: "s1",
			Method:    types.ApprovalMethodClaudeExitPlanMode,
			Summary:   "plan",
			Detail:    "Review implementation plan",
			CreatedAt: time.Date(2026, 2, 14, 12, 1, 0, 0, time.UTC),
		},
	})

	msg := historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "assistant", "content": []any{map[string]any{"type": "text", "text": "Claude reply."}}},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected assistant + approval blocks, got %#v", blocks)
	}
	if blocks[0].Role != ChatRoleAgent {
		t.Fatalf("expected assistant role first, got %#v", blocks[0])
	}
	if blocks[1].Role != ChatRoleApproval || blocks[1].RequestID != 42 {
		t.Fatalf("expected Claude plan approval block, got %#v", blocks[1])
	}
}

func TestHistoryMsgKiloCodeSplitsBackfillAssistantItemsByProviderMessageID(t *testing.T) {
	m := newPhase0ModelWithSession("kilocode")
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"

	msg := historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{
				"type":                "assistant",
				"provider_message_id": "msg-1",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Backfill one."}},
				},
			},
			{
				"type":                "assistant",
				"provider_message_id": "msg-2",
				"message": map[string]any{
					"content": []any{map[string]any{"type": "text", "text": "Backfill two."}},
				},
			},
		},
	}

	handled, cmd := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	applyProjectedSessionCmd(t, &m, cmd)

	blocks := m.currentBlocks()
	if len(blocks) != 2 {
		t.Fatalf("expected split assistant blocks for backfill, got %#v", blocks)
	}
	if blocks[0].Text != "Backfill one." || blocks[1].Text != "Backfill two." {
		t.Fatalf("unexpected backfill block order %#v", blocks)
	}
}
