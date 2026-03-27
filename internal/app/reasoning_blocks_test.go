package app

import (
	"context"
	"testing"
)

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

func TestCoalesceAdjacentReasoningBlocksWithContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	blocks, err := coalesceAdjacentReasoningBlocksWithContext(ctx, []ChatBlock{
		{Role: ChatRoleReasoning, Text: "first"},
		{Role: ChatRoleReasoning, Text: "second"},
	})
	if !isCanceledRequestError(err) {
		t.Fatalf("expected canceled error, got %v", err)
	}
	if blocks != nil {
		t.Fatalf("expected no blocks on canceled coalesce, got %#v", blocks)
	}
}

func TestCoalesceAdjacentReasoningBlocksWithContextMergesAdjacentReasoning(t *testing.T) {
	blocks, err := coalesceAdjacentReasoningBlocksWithContext(context.Background(), []ChatBlock{
		{ID: "r1", Role: ChatRoleReasoning, Text: "first"},
		{ID: "r2", Role: ChatRoleReasoning, Text: "second"},
		{ID: "a1", Role: ChatRoleAgent, Text: "answer"},
		{ID: "r3", Role: ChatRoleReasoning, Text: "third"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks after context-aware coalescing, got %#v", blocks)
	}
	if blocks[0].Role != ChatRoleReasoning || blocks[0].Text != "first\n\nsecond" {
		t.Fatalf("unexpected merged reasoning block %#v", blocks[0])
	}
}
