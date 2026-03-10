package app

import "testing"

func TestChatTranscriptSplitsAdjacentNonDeltaAgentItemsByDefault(t *testing.T) {
	tp := NewChatTranscript(0)
	if tp == nil {
		t.Fatalf("expected transcript")
	}

	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "First answer."})
	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "Second answer."})

	blocks := tp.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("expected two split blocks, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent || blocks[1].Role != ChatRoleAgent {
		t.Fatalf("expected agent roles, got %#v", blocks)
	}
	if blocks[0].Text != "First answer." || blocks[1].Text != "Second answer." {
		t.Fatalf("unexpected split text %#v", blocks)
	}
}

func TestChatTranscriptDoesNotCoalesceAcrossReasoning(t *testing.T) {
	tp := NewChatTranscript(0)
	if tp == nil {
		t.Fatalf("expected transcript")
	}

	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "First answer."})
	tp.AppendItem(map[string]any{"type": "reasoning", "id": "r1", "summary": []any{"thinking"}})
	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "Second answer."})

	blocks := tp.Blocks()
	if len(blocks) != 3 {
		t.Fatalf("expected role boundary to keep three blocks, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent || blocks[0].Text != "First answer." {
		t.Fatalf("unexpected first block %#v", blocks[0])
	}
	if blocks[1].Role != ChatRoleReasoning {
		t.Fatalf("expected reasoning middle block, got %s", blocks[1].Role)
	}
	if blocks[2].Role != ChatRoleAgent || blocks[2].Text != "Second answer." {
		t.Fatalf("unexpected third block %#v", blocks[2])
	}
}

func TestChatTranscriptMergesAssistantItemsWithSameProviderMessageID(t *testing.T) {
	tp := NewChatTranscript(0)

	tp.AppendItem(map[string]any{
		"type":                "assistant",
		"provider_message_id": "msg-1",
		"message": map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Hello"}},
		},
	})
	tp.AppendItem(map[string]any{
		"type":                "assistant",
		"provider_message_id": "msg-1",
		"message": map[string]any{
			"content": []any{map[string]any{"type": "text", "text": " world"}},
		},
	})

	blocks := tp.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one merged block, got %#v", blocks)
	}
	if blocks[0].Text != "Hello world" {
		t.Fatalf("unexpected merged text %q", blocks[0].Text)
	}
}

func TestChatTranscriptSplitsAssistantItemsWithDifferentProviderMessageIDs(t *testing.T) {
	tp := NewChatTranscript(0)

	tp.AppendItem(map[string]any{"type": "assistant", "provider_message_id": "msg-1", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "One"}}}})
	tp.AppendItem(map[string]any{"type": "assistant", "provider_message_id": "msg-2", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "Two"}}}})

	blocks := tp.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("expected split blocks for different message ids, got %#v", blocks)
	}
}

func TestChatTranscriptSplitsAssistantItemsAcrossTurnChange(t *testing.T) {
	tp := NewChatTranscript(0)

	tp.AppendItem(map[string]any{"type": "assistant", "turn_id": "turn-1", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "First"}}}})
	tp.AppendItem(map[string]any{"type": "assistant", "turn_id": "turn-2", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "Second"}}}})

	blocks := tp.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("expected split blocks across turn ids, got %#v", blocks)
	}
}

func TestChatTranscriptCoalescesDeltaLifecycleFragments(t *testing.T) {
	tp := NewChatTranscript(0)
	tp.AppendItem(map[string]any{"type": "agentMessageDelta", "delta": "stream "})
	tp.AppendItem(map[string]any{"type": "agentMessageDelta", "delta": "fragment"})

	blocks := tp.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one streaming block, got %#v", blocks)
	}
	if blocks[0].Text != "stream fragment" {
		t.Fatalf("unexpected delta text %q", blocks[0].Text)
	}
}

func TestChatTranscriptSplitsAfterExplicitEndMarkerByDefault(t *testing.T) {
	tp := NewChatTranscript(0)
	tp.AppendItem(map[string]any{"type": "agentMessageDelta", "delta": "first"})
	tp.AppendItem(map[string]any{"type": "agentMessageEnd"})
	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "Second reply."})

	blocks := tp.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("expected split blocks after explicit end marker, got %#v", blocks)
	}
	if blocks[0].Text != "first" || blocks[1].Text != "Second reply." {
		t.Fatalf("unexpected blocks %#v", blocks)
	}
}

func TestConcatAdjacentAgentTextAvoidsDuplicateReplaySuffix(t *testing.T) {
	if got := concatAdjacentAgentText("shows", "shows: `an "); got != "shows: `an " {
		t.Fatalf("expected cumulative replay to merge cleanly, got %q", got)
	}
}

func TestConcatAdjacentAgentTextUsesSuffixPrefixOverlap(t *testing.T) {
	if got := concatAdjacentAgentText("Upscale button ", "button is disabled"); got != "Upscale button is disabled" {
		t.Fatalf("expected overlap-aware merge, got %q", got)
	}
}

func TestConcatAdjacentAgentTextDropsExactReplayChunk(t *testing.T) {
	if got := concatAdjacentAgentText("Tooltip", "Tooltip"); got != "Tooltip" {
		t.Fatalf("expected exact replay chunk to be ignored, got %q", got)
	}
}
