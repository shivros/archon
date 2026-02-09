package app

import "testing"

func TestChatTranscriptUpsertReasoningUpdatesExistingBlock(t *testing.T) {
	tp := NewChatTranscript(0)
	if tp == nil {
		t.Fatalf("expected transcript")
	}

	if changed := tp.UpsertReasoning("r-1", "Reasoning\n\nfirst"); !changed {
		t.Fatalf("expected first reasoning upsert to change transcript")
	}
	blocks := tp.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one block, got %d", len(blocks))
	}
	if blocks[0].ID != "reasoning:r-1" || blocks[0].Role != ChatRoleReasoning {
		t.Fatalf("unexpected first block: %#v", blocks[0])
	}
	if blocks[0].Text != "Reasoning\n\nfirst" {
		t.Fatalf("unexpected first block text %q", blocks[0].Text)
	}

	if changed := tp.UpsertReasoning("r-1", "Reasoning\n\nfirst"); changed {
		t.Fatalf("expected unchanged reasoning text to be ignored")
	}
	if changed := tp.UpsertReasoning("r-1", "Reasoning\n\nupdated"); !changed {
		t.Fatalf("expected reasoning update to change transcript")
	}
	blocks = tp.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one block after update, got %d", len(blocks))
	}
	if blocks[0].Text != "Reasoning\n\nupdated" {
		t.Fatalf("expected updated reasoning text, got %q", blocks[0].Text)
	}
}
