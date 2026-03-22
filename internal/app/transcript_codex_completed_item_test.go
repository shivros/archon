package app

import (
	"encoding/json"
	"testing"

	"control/internal/daemon/transcriptadapters"
	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

func TestCodexCompletedItemReplacesDivergentLiveDelta(t *testing.T) {
	ingestor := NewDefaultTranscriptIngestor()
	state := TranscriptIngestState{}

	deltaEvent := transcriptadapters.TranscriptEventFromCodexEvent(
		"s1",
		"codex",
		transcriptdomain.MustParseRevisionToken("1"),
		types.CodexEvent{
			Method: "item/agentMessage/delta",
			Params: json.RawMessage(`{"itemId":"msg_1","turnId":"turn-1","delta":"bad/live/path"}`),
		},
	)
	if deltaEvent.Kind != transcriptdomain.TranscriptEventDelta {
		t.Fatalf("expected live delta event, got %#v", deltaEvent)
	}
	first := ingestor.ApplyEvent(state, deltaEvent, true)
	if !first.Changed {
		t.Fatalf("expected live delta to change transcript state")
	}
	state = first.State

	completedEvent := transcriptadapters.TranscriptEventFromCodexEvent(
		"s1",
		"codex",
		transcriptdomain.MustParseRevisionToken("2"),
		types.CodexEvent{
			Method: "item/completed",
			Params: json.RawMessage(`{
				"threadId":"thread-1",
				"turnId":"turn-1",
				"item":{
					"type":"agentMessage",
					"id":"msg_1",
					"text":"good/final/path"
				}
			}`),
		},
	)
	if completedEvent.Kind != transcriptdomain.TranscriptEventDelta {
		t.Fatalf("expected completed item to map as delta event, got %#v", completedEvent)
	}
	second := ingestor.ApplyEvent(state, completedEvent, false)
	if !second.Changed {
		t.Fatalf("expected completed item to replace divergent live text")
	}
	if len(second.State.Blocks) != 1 {
		t.Fatalf("expected single finalized block, got %#v", second.State.Blocks)
	}
	if got := second.State.Blocks[0].Text; got != "good/final/path" {
		t.Fatalf("expected finalized text to win, got %q", got)
	}
}
