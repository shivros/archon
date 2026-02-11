package app

import "testing"

func TestCoalesceAdjacentReasoningBlocksMergesOnlyAdjacentReasoning(t *testing.T) {
	blocks := []ChatBlock{
		{ID: "a1", Role: ChatRoleAgent, Text: "answer one"},
		{ID: "reasoning:r1", Role: ChatRoleReasoning, Text: "- first"},
		{ID: "reasoning:r2", Role: ChatRoleReasoning, Text: "- second"},
		{ID: "a2", Role: ChatRoleAgent, Text: "answer two"},
		{ID: "reasoning:r3", Role: ChatRoleReasoning, Text: "- third"},
	}

	got := coalesceAdjacentReasoningBlocks(blocks)
	if len(got) != 4 {
		t.Fatalf("expected 4 blocks after coalescing, got %d", len(got))
	}
	if got[1].Role != ChatRoleReasoning {
		t.Fatalf("expected second block to remain reasoning, got %s", got[1].Role)
	}
	if got[1].Text != "- first\n\n- second" {
		t.Fatalf("unexpected merged reasoning text %q", got[1].Text)
	}
	if got[3].Text != "- third" {
		t.Fatalf("expected non-adjacent reasoning block to remain separate, got %q", got[3].Text)
	}
}
