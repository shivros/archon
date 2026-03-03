package transcriptadapters

import (
	"encoding/json"
	"testing"

	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

func TestOpenCodeAdapterMapsApprovalEvent(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter("opencode")
	id := 42
	events := adapter.MapEvent(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
	}, types.CodexEvent{ID: &id, Method: "item/commandExecution/requestApproval"})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventApprovalPending {
		t.Fatalf("expected approval.pending, got %q", events[0].Kind)
	}
	if events[0].Approval == nil || events[0].Approval.RequestID != 42 {
		t.Fatalf("unexpected approval payload: %#v", events[0].Approval)
	}
}

func TestOpenCodeAdapterMapsApprovalResolvedEvent(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter("opencode")
	events := adapter.MapEvent(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
	}, types.CodexEvent{Method: "item/replyPermission", Params: json.RawMessage(`{"request_id":11}`)})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventApprovalResolved {
		t.Fatalf("expected approval.resolved, got %q", events[0].Kind)
	}
	if events[0].Approval == nil || events[0].Approval.RequestID != 11 {
		t.Fatalf("unexpected approval payload: %#v", events[0].Approval)
	}
}

func TestOpenCodeAdapterMapsTurnCompletionItemWithFallbackTurnID(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter("opencode")
	events := adapter.MapItem(MappingContext{
		SessionID:    "s1",
		Revision:     transcriptdomain.MustParseRevisionToken("3"),
		ActiveTurnID: "turn-fallback",
	}, map[string]any{"type": "turnCompletion", "turn_status": "completed"})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventTurnCompleted {
		t.Fatalf("expected turn.completed, got %q", events[0].Kind)
	}
	if events[0].Turn == nil || events[0].Turn.TurnID != "turn-fallback" {
		t.Fatalf("unexpected turn payload: %#v", events[0].Turn)
	}
}

func TestOpenCodeAdapterMapsAbandonedTurnCompletionAsFailed(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter("opencode")
	events := adapter.MapItem(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("4"),
	}, map[string]any{"type": "turnCompletion", "turn_id": "turn-2", "turn_status": "abandoned"})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventTurnFailed {
		t.Fatalf("expected turn.failed, got %q", events[0].Kind)
	}
	if events[0].Turn == nil || events[0].Turn.Error == "" {
		t.Fatalf("expected failed turn with error, got %#v", events[0].Turn)
	}
}

func TestOpenCodeAdapterMapsDeltaItem(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter("opencode")
	events := adapter.MapItem(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("5"),
	}, map[string]any{"type": "agentMessageDelta", "delta": "hello"})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventDelta {
		t.Fatalf("expected delta event, got %q", events[0].Kind)
	}
}

func TestKiloCodeUsesOpenCodeAdapterBehavior(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter("kilocode")
	events := adapter.MapEvent(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("6"),
	}, types.CodexEvent{Method: "session.idle"})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Provider != "kilocode" {
		t.Fatalf("expected provider kilocode, got %q", events[0].Provider)
	}
	if events[0].Kind != transcriptdomain.TranscriptEventStreamStatus {
		t.Fatalf("expected stream status event, got %q", events[0].Kind)
	}
}

func TestOpenCodeAdapterProviderDefaultsToOpenCode(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter(" ")
	if adapter.Provider() != "opencode" {
		t.Fatalf("expected default provider opencode, got %q", adapter.Provider())
	}
}

func TestOpenCodeAdapterMapEventTurnStarted(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter("opencode")
	events := adapter.MapEvent(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("7"),
	}, types.CodexEvent{Method: "turn/started", Params: json.RawMessage(`{"turn_id":"turn-1"}`)})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventTurnStarted {
		t.Fatalf("expected turn.started, got %q", events[0].Kind)
	}
	if events[0].Turn == nil || events[0].Turn.TurnID != "turn-1" {
		t.Fatalf("unexpected turn payload: %#v", events[0].Turn)
	}
}

func TestOpenCodeAdapterMapEventTurnCompletedUsesFallbackTurn(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter("opencode")
	events := adapter.MapEvent(MappingContext{
		SessionID:    "s1",
		Revision:     transcriptdomain.MustParseRevisionToken("8"),
		ActiveTurnID: "turn-fallback",
	}, types.CodexEvent{Method: "turn/completed", Params: json.RawMessage(`{"status":"completed"}`)})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Turn == nil || events[0].Turn.TurnID != "turn-fallback" {
		t.Fatalf("expected fallback turn id, got %#v", events[0].Turn)
	}
}

func TestOpenCodeAdapterMapEventErrorFallbackMessage(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter("opencode")
	events := adapter.MapEvent(MappingContext{
		SessionID:    "s1",
		Revision:     transcriptdomain.MustParseRevisionToken("9"),
		ActiveTurnID: "turn-err",
	}, types.CodexEvent{Method: "error", Params: json.RawMessage(`{"status":"failed"}`)})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventTurnFailed {
		t.Fatalf("expected turn.failed, got %q", events[0].Kind)
	}
	if events[0].Turn == nil || events[0].Turn.Error == "" {
		t.Fatalf("expected failed turn with fallback error, got %#v", events[0].Turn)
	}
}

func TestOpenCodeAdapterMapEventUnknownDefaultsToProviderEventDelta(t *testing.T) {
	adapter := NewOpenCodeTranscriptAdapter("opencode")
	events := adapter.MapEvent(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("10"),
	}, types.CodexEvent{Method: "item/unknown"})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventDelta {
		t.Fatalf("expected delta, got %q", events[0].Kind)
	}
	if len(events[0].Delta) != 1 || events[0].Delta[0].Kind != "provider_event" {
		t.Fatalf("unexpected provider-event delta payload: %#v", events[0].Delta)
	}
}
