package app

import (
	"testing"
	"time"
)

type alwaysMergeAssistantPolicy struct{}

func (alwaysMergeAssistantPolicy) ShouldMerge(last ChatBlock, nextText string, ctx assistantAppendContext) bool {
	return last.Role == ChatRoleAgent
}

type fixedAssistantMetadataExtractor struct {
	turnID            string
	providerMessageID string
}

func (f fixedAssistantMetadataExtractor) Extract(item map[string]any, _ time.Time, itemType string) assistantItemMetadata {
	return assistantItemMetadata(f)
}

func TestChatTranscriptAcceptsInjectedMergePolicy(t *testing.T) {
	tp := NewChatTranscriptWithDependencies(
		0,
		nil,
		nil,
		WithAssistantMergePolicy(alwaysMergeAssistantPolicy{}),
	)
	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "A."})
	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "B."})

	blocks := tp.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected injected merge policy to merge blocks, got %#v", blocks)
	}
	if blocks[0].Text != "A.B." {
		t.Fatalf("unexpected merged text %q", blocks[0].Text)
	}
}

func TestChatTranscriptAcceptsInjectedMetadataExtractor(t *testing.T) {
	tp := NewChatTranscriptWithDependencies(
		0,
		nil,
		nil,
		WithAssistantMetadataExtractor(fixedAssistantMetadataExtractor{
			turnID:            "turn-custom",
			providerMessageID: "msg-custom",
		}),
	)

	tp.AppendItem(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "text", "text": "Hello"}}}})

	blocks := tp.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one assistant block, got %#v", blocks)
	}
	if blocks[0].TurnID != "turn-custom" || blocks[0].ProviderMessageID != "msg-custom" {
		t.Fatalf("expected injected metadata to populate block, got %#v", blocks[0])
	}
}

func TestChatTranscriptOptionGuards(t *testing.T) {
	var nilTranscript *ChatTranscript
	WithAssistantMergePolicy(alwaysMergeAssistantPolicy{})(nilTranscript)
	WithAssistantMetadataExtractor(fixedAssistantMetadataExtractor{})(nilTranscript)

	tp := NewChatTranscriptWithDependencies(
		0,
		nil,
		nil,
		WithAssistantMergePolicy(nil),
		WithAssistantMetadataExtractor(nil),
	)
	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "A."})
	tp.AppendItem(map[string]any{"type": "agentMessage", "text": "B."})

	blocks := tp.Blocks()
	if len(blocks) != 2 {
		t.Fatalf("expected default behavior when nil options are provided, got %#v", blocks)
	}
}
