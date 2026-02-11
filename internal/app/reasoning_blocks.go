package app

import "strings"

func coalesceAdjacentReasoningBlocks(blocks []ChatBlock) []ChatBlock {
	if len(blocks) <= 1 {
		return blocks
	}
	result := make([]ChatBlock, 0, len(blocks))
	for _, block := range blocks {
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
	return result
}
