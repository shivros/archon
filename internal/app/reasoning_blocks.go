package app

import (
	"context"
	"strings"
)

func coalesceAdjacentReasoningBlocks(blocks []ChatBlock) []ChatBlock {
	coalesced, _ := coalesceAdjacentReasoningBlocksWithContext(context.Background(), blocks)
	return coalesced
}

func coalesceAdjacentReasoningBlocksWithContext(ctx context.Context, blocks []ChatBlock) ([]ChatBlock, error) {
	if err := projectionContextError(ctx); err != nil {
		return nil, err
	}
	if len(blocks) <= 1 {
		return blocks, nil
	}
	result := make([]ChatBlock, 0, len(blocks))
	for i, block := range blocks {
		if projectionContextCheckNeeded(i) {
			if err := projectionContextError(ctx); err != nil {
				return nil, err
			}
		}
		text := strings.TrimSpace(block.Text)
		if len(result) == 0 || block.Role != ChatRoleReasoning || result[len(result)-1].Role != ChatRoleReasoning {
			result = append(result, block)
			continue
		}
		if text == "" {
			continue
		}
		last := &result[len(result)-1]
		lastText := strings.TrimSpace(last.Text)
		if lastText == "" {
			last.Text = text
			if strings.TrimSpace(last.ID) == "" {
				last.ID = block.ID
			}
			continue
		}
		if lastText == text {
			continue
		}
		last.Text = lastText + "\n\n" + text
	}
	if err := projectionContextError(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
