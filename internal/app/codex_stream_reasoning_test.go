package app

import (
	"encoding/json"
	"strings"
	"testing"

	"control/internal/types"
)

func TestCodexStreamAggregatesMultipleReasoningIDsIntoOneBlock(t *testing.T) {
	stream := NewCodexStreamController(0, 16)
	stream.AppendUserMessage("prompt")

	if !stream.applyEvent(codexReasoningEvent("item/started", "r1", "first")) {
		t.Fatalf("expected first reasoning event to apply")
	}
	if !stream.applyEvent(codexReasoningEvent("item/updated", "r2", "second")) {
		t.Fatalf("expected second reasoning event to apply")
	}
	if !stream.applyEvent(codexReasoningEvent("item/updated", "r1", "first updated")) {
		t.Fatalf("expected reasoning update event to apply")
	}

	blocks := stream.Blocks()
	reasoning := extractReasoningBlocks(blocks)
	if len(reasoning) != 1 {
		t.Fatalf("expected one aggregated reasoning block, got %d (%#v)", len(reasoning), blocks)
	}
	if !strings.HasPrefix(reasoning[0].ID, "reasoning:codex-group-") {
		t.Fatalf("expected aggregated reasoning block id, got %q", reasoning[0].ID)
	}
	want := "- first updated\n\n- second"
	if reasoning[0].Text != want {
		t.Fatalf("expected aggregate text %q, got %q", want, reasoning[0].Text)
	}
}

func TestCodexStreamStartsNewReasoningAggregatePerUserTurn(t *testing.T) {
	stream := NewCodexStreamController(0, 16)
	stream.AppendUserMessage("prompt one")
	stream.applyEvent(codexReasoningEvent("item/started", "r1", "first"))

	stream.AppendUserMessage("prompt two")
	stream.applyEvent(codexReasoningEvent("item/started", "r2", "second"))

	blocks := stream.Blocks()
	reasoning := extractReasoningBlocks(blocks)
	if len(reasoning) != 2 {
		t.Fatalf("expected two reasoning blocks across turns, got %d (%#v)", len(reasoning), blocks)
	}
	if reasoning[0].ID == reasoning[1].ID {
		t.Fatalf("expected unique aggregate ids per turn, got %q", reasoning[0].ID)
	}
}

func codexReasoningEvent(method, id, summary string) types.CodexEvent {
	item := map[string]any{
		"type":    "reasoning",
		"id":      id,
		"summary": []any{summary},
	}
	return types.CodexEvent{
		Method: method,
		Params: mustRawJSONMap(map[string]any{"item": item}),
	}
}

func mustRawJSONMap(payload map[string]any) json.RawMessage {
	data, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return data
}

func extractReasoningBlocks(blocks []ChatBlock) []ChatBlock {
	out := make([]ChatBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Role == ChatRoleReasoning {
			out = append(out, block)
		}
	}
	return out
}
