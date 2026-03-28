package app

import (
	"testing"

	"control/internal/daemon/transcriptdomain"
)

func TestTranscriptSnapshotHistoryParityPreservesUserTurnsAcrossProviders(t *testing.T) {
	testCases := []struct {
		name     string
		provider string
	}{
		{name: "codex", provider: "codex"},
		{name: "claude", provider: "claude"},
		{name: "opencode", provider: "opencode"},
		{name: "kilocode", provider: "kilocode"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			historyModel := newPhase0ModelWithSession(tc.provider)
			historyModel.enterCompose("s1")
			historyModel.pendingSessionKey = "sess:s1"

			historyItems := []map[string]any{
				{
					"type":    "userMessage",
					"content": []any{map[string]any{"type": "text", "text": "User asks for status"}},
				},
				{
					"type": "assistant",
					"message": map[string]any{
						"content": []any{map[string]any{"type": "text", "text": "Assistant provides status"}},
					},
				},
			}
			handled, cmd := historyModel.reduceStateMessages(historyMsg{
				id:             "s1",
				key:            "sess:s1",
				requestedLines: 200,
				items:          historyItems,
			})
			if !handled {
				t.Fatalf("expected history message to be handled")
			}
			applyProjectedSessionCmd(t, &historyModel, cmd)

			snapshotModel := newPhase0ModelWithSession(tc.provider)
			snapshotModel.enterCompose("s1")
			snapshotModel.pendingSessionKey = "sess:s1"

			handled, cmd = snapshotModel.reduceStateMessages(transcriptSnapshotMsg{
				id:             "s1",
				key:            "sess:s1",
				requestedLines: 200,
				snapshot: &transcriptdomain.TranscriptSnapshot{
					SessionID: "s1",
					Provider:  tc.provider,
					Revision:  transcriptdomain.MustParseRevisionToken("1"),
					Blocks: []transcriptdomain.Block{
						{ID: "u1", Kind: "user_message", Role: "user", Text: "User asks for status"},
						{ID: "a1", Kind: "assistant_message", Role: "assistant", Text: "Assistant provides status"},
					},
				},
			})
			if !handled {
				t.Fatalf("expected transcript snapshot to be handled")
			}
			if cmd != nil {
				t.Fatalf("expected no follow-up command for snapshot parity")
			}

			historyBlocks := parityRoleTextPairs(historyModel.currentBlocks())
			snapshotBlocks := parityRoleTextPairs(snapshotModel.currentBlocks())
			if len(historyBlocks) == 0 || len(snapshotBlocks) == 0 {
				t.Fatalf("expected both history and snapshot projections to produce blocks, history=%#v snapshot=%#v", historyBlocks, snapshotBlocks)
			}
			if !containsRole(historyModel.currentBlocks(), ChatRoleUser) {
				t.Fatalf("expected history projection to contain user block, got %#v", historyModel.currentBlocks())
			}
			if !containsRole(snapshotModel.currentBlocks(), ChatRoleUser) {
				t.Fatalf("expected snapshot projection to contain user block, got %#v", snapshotModel.currentBlocks())
			}
			if len(historyBlocks) != len(snapshotBlocks) {
				t.Fatalf("expected parity block count, history=%#v snapshot=%#v", historyBlocks, snapshotBlocks)
			}
			for i := range historyBlocks {
				if historyBlocks[i] != snapshotBlocks[i] {
					t.Fatalf("expected parity at index %d, history=%#v snapshot=%#v", i, historyBlocks, snapshotBlocks)
				}
			}
		})
	}
}

func parityRoleTextPairs(blocks []ChatBlock) []string {
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Role == ChatRoleSystem || block.Role == ChatRoleApproval || block.Role == ChatRoleApprovalResolved {
			continue
		}
		out = append(out, string(block.Role)+"|"+block.Text)
	}
	return out
}

func containsRole(blocks []ChatBlock, role ChatRole) bool {
	for _, block := range blocks {
		if block.Role == role {
			return true
		}
	}
	return false
}
