package app

import (
	"encoding/json"
	"testing"

	"control/internal/types"
)

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

func TestChatTranscriptSplitsCodexAgentStreamsAcrossCompletedItems(t *testing.T) {
	stream := NewCodexStreamController(0, 32)
	events := make(chan types.CodexEvent, 8)
	stream.SetStream(events, nil)

	events <- codexItemStartedEvent("a1")
	events <- codexAgentDeltaEvent("First streamed answer.")
	events <- codexItemCompletedEvent("a1")
	events <- codexItemStartedEvent("a2")
	events <- codexAgentDeltaEvent("Second streamed answer.")
	events <- codexItemCompletedEvent("a2")
	close(events)

	for {
		_, closed, _ := stream.ConsumeTick()
		if closed {
			break
		}
	}

	blocks := stream.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("expected split stream blocks, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent || blocks[1].Role != ChatRoleAgent {
		t.Fatalf("expected agent roles, got %#v", blocks)
	}
	if blocks[0].Text != "First streamed answer." || blocks[1].Text != "Second streamed answer." {
		t.Fatalf("unexpected stream text %#v", blocks)
	}
}

func TestChatTranscriptCodexStreamCarriesItemIdentityMetadata(t *testing.T) {
	stream := NewCodexStreamController(0, 32)
	events := make(chan types.CodexEvent, 8)
	stream.SetStream(events, nil)

	events <- codexItemStartedEventWithTurn("a1", "turn-1")
	events <- codexAgentDeltaEvent("First streamed answer.")
	events <- codexItemCompletedEvent("a1")
	events <- codexItemStartedEventWithTurn("a2", "turn-2")
	events <- codexAgentDeltaEvent("Second streamed answer.")
	events <- codexItemCompletedEvent("a2")
	close(events)

	for {
		_, closed, _ := stream.ConsumeTick()
		if closed {
			break
		}
	}

	blocks := stream.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("expected two blocks, got %#v", blocks)
	}
	if blocks[0].ProviderMessageID != "a1" || blocks[1].ProviderMessageID != "a2" {
		t.Fatalf("expected provider message ids to track item ids, got %#v", blocks)
	}
	if blocks[0].TurnID != "turn-1" || blocks[1].TurnID != "turn-2" {
		t.Fatalf("expected turn ids to propagate from stream items, got %#v", blocks)
	}
}

func TestChatTranscriptSplitsAdjacentItemStreamAssistantMessagesByDefault(t *testing.T) {
	stream := NewItemStreamController(0, 32)
	items := make(chan map[string]any, 4)
	stream.SetStream(items, nil)

	items <- map[string]any{
		"type": "assistant",
		"content": []any{
			map[string]any{"type": "text", "text": "First streamed answer."},
		},
	}
	items <- map[string]any{
		"type":   "result",
		"result": "Second streamed answer.",
	}
	close(items)

	for {
		_, closed := stream.ConsumeTick()
		if closed {
			break
		}
	}

	blocks := stream.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("expected split stream blocks, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent || blocks[1].Role != ChatRoleAgent {
		t.Fatalf("expected agent roles, got %#v", blocks)
	}
	if blocks[0].Text != "First streamed answer." || blocks[1].Text != "Second streamed answer." {
		t.Fatalf("unexpected stream text %#v", blocks)
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

func codexItemStartedEvent(id string) types.CodexEvent {
	return codexItemStartedEventWithTurn(id, "")
}

func codexItemStartedEventWithTurn(id, turnID string) types.CodexEvent {
	item := map[string]any{
		"type": "agentMessage",
		"id":   id,
	}
	if turnID != "" {
		item["turn_id"] = turnID
	}
	return types.CodexEvent{
		Method: "item/started",
		Params: mustRawJSON(map[string]any{
			"item": item,
		}),
	}
}

func codexItemCompletedEvent(id string) types.CodexEvent {
	return types.CodexEvent{
		Method: "item/completed",
		Params: mustRawJSON(map[string]any{
			"item": map[string]any{
				"type": "agentMessage",
				"id":   id,
			},
		}),
	}
}

func codexAgentDeltaEvent(delta string) types.CodexEvent {
	return types.CodexEvent{
		Method: "item/agentMessage/delta",
		Params: mustRawJSON(map[string]any{
			"delta": delta,
		}),
	}
}

func mustRawJSON(payload map[string]any) json.RawMessage {
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return data
}
