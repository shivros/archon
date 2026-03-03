package transcriptadapters

import (
	"testing"

	"control/internal/daemon/transcriptdomain"
)

func TestClaudeAdapterMapsDeltaItem(t *testing.T) {
	adapter := NewClaudeTranscriptAdapter("claude")
	events := adapter.MapItem(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
	}, map[string]any{"type": "agentMessageDelta", "delta": "hello"})
	if len(events) != 1 {
		t.Fatalf("expected one canonical event, got %d", len(events))
	}
	got := events[0]
	if got.Kind != transcriptdomain.TranscriptEventDelta {
		t.Fatalf("expected delta event, got %q", got.Kind)
	}
	if len(got.Delta) != 1 || got.Delta[0].Role != "assistant" || got.Delta[0].Text != "hello" {
		t.Fatalf("unexpected delta payload: %#v", got.Delta)
	}
}

func TestClaudeAdapterMapsReasoningFromContentBlocks(t *testing.T) {
	adapter := NewClaudeTranscriptAdapter("claude")
	events := adapter.MapItem(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
	}, map[string]any{
		"type": "reasoning",
		"content": []any{
			map[string]any{"text": "thought 1"},
			map[string]any{"thinking": "thought 2"},
		},
	})
	if len(events) != 1 {
		t.Fatalf("expected one canonical event, got %d", len(events))
	}
	if events[0].Delta[0].Variant != "reasoning" {
		t.Fatalf("expected reasoning variant, got %#v", events[0].Delta[0])
	}
}

func TestClaudeAdapterMapsAgentMessageEndToTurnCompleted(t *testing.T) {
	adapter := NewClaudeTranscriptAdapter("claude")
	events := adapter.MapItem(MappingContext{
		SessionID:    "s1",
		Revision:     transcriptdomain.MustParseRevisionToken("3"),
		ActiveTurnID: "turn-1",
	}, map[string]any{"type": "agentMessageEnd"})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventTurnCompleted {
		t.Fatalf("expected turn.completed, got %q", events[0].Kind)
	}
	if events[0].Turn == nil || events[0].Turn.TurnID != "turn-1" {
		t.Fatalf("unexpected turn payload: %#v", events[0].Turn)
	}
}

func TestClaudeAdapterDropsAgentMessageEndWithoutTurnID(t *testing.T) {
	adapter := NewClaudeTranscriptAdapter("claude")
	events := adapter.MapItem(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("4"),
	}, map[string]any{"type": "agentMessageEnd"})
	if len(events) != 0 {
		t.Fatalf("expected no event when turn id missing, got %#v", events)
	}
}

func TestClaudeAdapterProviderDefaultsToClaude(t *testing.T) {
	adapter := NewClaudeTranscriptAdapter(" ")
	if adapter.Provider() != "claude" {
		t.Fatalf("expected default provider claude, got %q", adapter.Provider())
	}
}
