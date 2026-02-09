package app

import (
	"encoding/json"
	"testing"

	"control/internal/types"
)

func TestChatTranscriptCoalescesAdjacentAgentItems(t *testing.T) {
	tp := NewChatTranscript(0)
	if tp == nil {
		t.Fatalf("expected transcript")
	}

	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "First answer."})
	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "Second answer."})

	blocks := tp.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one coalesced block, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent {
		t.Fatalf("expected agent role, got %s", blocks[0].Role)
	}
	if blocks[0].Text != "First answer.\n\nSecond answer." {
		t.Fatalf("unexpected coalesced text %q", blocks[0].Text)
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

func TestChatTranscriptCoalescesAdjacentCodexAgentStreams(t *testing.T) {
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
	if len(blocks) != 1 {
		t.Fatalf("expected one coalesced stream block, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent {
		t.Fatalf("expected agent role, got %s", blocks[0].Role)
	}
	if blocks[0].Text != "First streamed answer.\n\nSecond streamed answer." {
		t.Fatalf("unexpected stream text %q", blocks[0].Text)
	}
}

func TestChatTranscriptCoalescesAdjacentItemStreamAssistantMessages(t *testing.T) {
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
	if len(blocks) != 1 {
		t.Fatalf("expected one coalesced stream block, got %d", len(blocks))
	}
	if blocks[0].Role != ChatRoleAgent {
		t.Fatalf("expected agent role, got %s", blocks[0].Role)
	}
	if blocks[0].Text != "First streamed answer.\n\nSecond streamed answer." {
		t.Fatalf("unexpected stream text %q", blocks[0].Text)
	}
}

func codexItemStartedEvent(id string) types.CodexEvent {
	return types.CodexEvent{
		Method: "item/started",
		Params: mustRawJSON(map[string]any{
			"item": map[string]any{
				"type": "agentMessage",
				"id":   id,
			},
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
