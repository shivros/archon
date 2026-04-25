package transcriptadapters

import (
	"encoding/json"
	"strings"
	"testing"

	"control/internal/daemon/acp"
	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

func newHermesMappingContext() MappingContext {
	return MappingContext{
		SessionID:    "sess-1",
		Revision:     transcriptdomain.MustParseRevisionToken("1"),
		ActiveTurnID: "turn-1",
	}
}

func mapHermes(t *testing.T, event types.CodexEvent) []transcriptdomain.TranscriptEvent {
	t.Helper()
	adapter := NewHermesTranscriptAdapter("hermes")
	return adapter.MapEvent(newHermesMappingContext(), event)
}

func sessionUpdateEvent(t *testing.T, update map[string]any) types.CodexEvent {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"sessionId": "sess-acp",
		"update":    update,
	})
	if err != nil {
		t.Fatalf("marshal session/update: %v", err)
	}
	return types.CodexEvent{
		Method: acp.MethodSessionUpdate,
		Params: raw,
	}
}

func TestHermesAdapterProvider(t *testing.T) {
	adapter := NewHermesTranscriptAdapter("HERMES")
	if got := adapter.Provider(); got != "hermes" {
		t.Fatalf("expected provider hermes, got %q", got)
	}
	adapter = NewHermesTranscriptAdapter("")
	if got := adapter.Provider(); got != "hermes" {
		t.Fatalf("expected provider hermes for empty name, got %q", got)
	}
}

func TestHermesAdapterMapsAgentMessageChunk(t *testing.T) {
	event := sessionUpdateEvent(t, map[string]any{
		"sessionUpdate": acp.SessionUpdateAgentMessageChunk,
		"content":       map[string]any{"type": "text", "text": "hello world"},
	})
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	got := events[0]
	if got.Kind != transcriptdomain.TranscriptEventDelta {
		t.Fatalf("expected delta kind, got %q", got.Kind)
	}
	if len(got.Delta) != 1 {
		t.Fatalf("expected one delta block, got %d", len(got.Delta))
	}
	block := got.Delta[0]
	if block.Kind != "message" || block.Role != "assistant" {
		t.Fatalf("unexpected block kind/role: %#v", block)
	}
	if block.Text != "hello world" {
		t.Fatalf("expected text 'hello world', got %q", block.Text)
	}
}

func TestHermesAdapterMapsUserMessageChunk(t *testing.T) {
	event := sessionUpdateEvent(t, map[string]any{
		"sessionUpdate": acp.SessionUpdateUserMessageChunk,
		"content":       map[string]any{"type": "text", "text": "echoed"},
	})
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	block := events[0].Delta[0]
	if block.Role != "user" || block.Text != "echoed" {
		t.Fatalf("unexpected block: %#v", block)
	}
}

func TestHermesAdapterMapsAgentThoughtChunk(t *testing.T) {
	event := sessionUpdateEvent(t, map[string]any{
		"sessionUpdate": acp.SessionUpdateAgentThoughtChunk,
		"content":       map[string]any{"type": "text", "text": "thinking..."},
	})
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	block := events[0].Delta[0]
	if block.Kind != "thinking" || block.Variant != "thinking" {
		t.Fatalf("expected thinking block, got %#v", block)
	}
	if block.Text != "thinking..." {
		t.Fatalf("expected thinking text, got %q", block.Text)
	}
}

func TestHermesAdapterMapsToolCall(t *testing.T) {
	event := sessionUpdateEvent(t, map[string]any{
		"sessionUpdate": acp.SessionUpdateToolCall,
		"toolCallId":    "tc-1",
		"title":         "Run tests",
		"kind":          "execute",
		"status":        "in_progress",
		"content": []map[string]any{
			{
				"type":    "content",
				"content": map[string]any{"type": "text", "text": "go test ./..."},
			},
		},
	})
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	block := events[0].Delta[0]
	if block.Kind != "tool_call" || block.Variant != "started" {
		t.Fatalf("unexpected tool_call block: %#v", block)
	}
	if block.Meta["tool_call_id"] != "tc-1" || block.Meta["tool_kind"] != "execute" {
		t.Fatalf("unexpected meta: %#v", block.Meta)
	}
	if !strings.Contains(block.Text, "go test") {
		t.Fatalf("expected content text, got %q", block.Text)
	}
}

func TestHermesAdapterMapsToolCallUpdate(t *testing.T) {
	event := sessionUpdateEvent(t, map[string]any{
		"sessionUpdate": acp.SessionUpdateToolCallUpdate,
		"toolCallId":    "tc-1",
		"status":        "completed",
		"content": []map[string]any{
			{"type": "content", "content": map[string]any{"type": "text", "text": "ok"}},
		},
	})
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	block := events[0].Delta[0]
	if block.Kind != "tool_call" || block.Variant != "completed" {
		t.Fatalf("unexpected tool_call_update block: %#v", block)
	}
	if block.Meta["status"] != "completed" {
		t.Fatalf("expected status=completed meta, got %#v", block.Meta)
	}
}

func TestHermesAdapterMapsToolCallUpdateDefaultsVariant(t *testing.T) {
	event := sessionUpdateEvent(t, map[string]any{
		"sessionUpdate": acp.SessionUpdateToolCallUpdate,
		"toolCallId":    "tc-2",
	})
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	if events[0].Delta[0].Variant != "update" {
		t.Fatalf("expected default variant 'update', got %q", events[0].Delta[0].Variant)
	}
}

func TestHermesAdapterMapsPlan(t *testing.T) {
	event := sessionUpdateEvent(t, map[string]any{
		"sessionUpdate": acp.SessionUpdatePlan,
		"entries": []map[string]any{
			{"content": "step 1", "priority": "high", "status": "pending"},
			{"content": "step 2", "priority": "low", "status": "in_progress"},
		},
	})
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	block := events[0].Delta[0]
	if block.Kind != "plan" {
		t.Fatalf("expected plan block, got %#v", block)
	}
	if !strings.Contains(block.Text, "step 1") {
		t.Fatalf("expected plan text to carry entries, got %q", block.Text)
	}
	if block.Meta["entries"] != 2 {
		t.Fatalf("expected entries=2 meta, got %#v", block.Meta)
	}
}

func TestHermesAdapterPreservesUnknownSessionUpdate(t *testing.T) {
	event := sessionUpdateEvent(t, map[string]any{
		"sessionUpdate": "future_update_variant",
		"payload":       "opaque",
	})
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected passthrough event for unknown variant, got %d", len(events))
	}
	block := events[0].Delta[0]
	if block.Kind != "event" || block.Variant != "raw" {
		t.Fatalf("expected raw passthrough block, got %#v", block)
	}
	if !strings.Contains(block.Text, "future_update_variant") {
		t.Fatalf("expected raw text to carry unknown payload, got %q", block.Text)
	}
}

func TestHermesAdapterMapsTurnStarted(t *testing.T) {
	event := types.CodexEvent{
		Method: "turn/started",
		Params: json.RawMessage(`{"turnId":"t1"}`),
	}
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventTurnStarted {
		t.Fatalf("expected turn started, got %q", events[0].Kind)
	}
	if events[0].Turn == nil || events[0].Turn.TurnID != "t1" {
		t.Fatalf("unexpected turn payload: %#v", events[0].Turn)
	}
}

func TestHermesAdapterMapsTurnCompletedStopReasons(t *testing.T) {
	// Archon's provider always surfaces stop reasons through turn/completed or
	// turn/failed; the adapter doesn't gate on the stop reason text itself.
	reasons := []string{
		acp.StopReasonEndTurn,
		acp.StopReasonMaxTokens,
		acp.StopReasonMaxTurnRequests,
		acp.StopReasonRefusal,
	}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			event := types.CodexEvent{
				Method: "turn/completed",
				Params: json.RawMessage(`{"turnId":"t1","stopReason":"` + reason + `","status":"completed"}`),
			}
			events := mapHermes(t, event)
			if len(events) != 1 {
				t.Fatalf("expected single event, got %d", len(events))
			}
			if events[0].Kind != transcriptdomain.TranscriptEventTurnCompleted {
				t.Fatalf("expected turn completed, got %q", events[0].Kind)
			}
			if events[0].Turn == nil || events[0].Turn.State != transcriptdomain.TurnStateCompleted {
				t.Fatalf("unexpected turn state: %#v", events[0].Turn)
			}
		})
	}
}

func TestHermesAdapterMapsTurnFailedCancelled(t *testing.T) {
	event := types.CodexEvent{
		Method: "turn/failed",
		Params: json.RawMessage(`{"turnId":"t1","status":"failed","error":"turn cancelled"}`),
	}
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventTurnFailed {
		t.Fatalf("expected turn failed, got %q", events[0].Kind)
	}
	if events[0].Turn == nil || events[0].Turn.Error == "" {
		t.Fatalf("expected error on turn state, got %#v", events[0].Turn)
	}
}

func TestHermesAdapterMapsApprovalPending(t *testing.T) {
	id := 42
	event := types.CodexEvent{
		ID:     &id,
		Method: acp.MethodRequestPermission,
		Params: json.RawMessage(`{"sessionId":"s","toolCall":{"toolCallId":"tc"},"options":[{"optionId":"allow_once"}]}`),
	}
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	got := events[0]
	if got.Kind != transcriptdomain.TranscriptEventApprovalPending {
		t.Fatalf("expected approval pending, got %q", got.Kind)
	}
	if got.Approval == nil || got.Approval.RequestID != 42 {
		t.Fatalf("unexpected approval payload: %#v", got.Approval)
	}
	if got.Approval.Method != acp.MethodRequestPermission {
		t.Fatalf("expected method to carry ACP method, got %q", got.Approval.Method)
	}
}

func TestHermesAdapterMapsApprovalResolved(t *testing.T) {
	id := 7
	event := types.CodexEvent{
		ID:     &id,
		Method: "permission/replied",
		Params: json.RawMessage(`{"outcome":{"outcome":"selected","optionId":"allow_once"}}`),
	}
	events := mapHermes(t, event)
	if len(events) != 1 {
		t.Fatalf("expected single event, got %d", len(events))
	}
	got := events[0]
	if got.Kind != transcriptdomain.TranscriptEventApprovalResolved {
		t.Fatalf("expected approval resolved, got %q", got.Kind)
	}
	if got.Approval == nil || got.Approval.RequestID != 7 || got.Approval.State != "resolved" {
		t.Fatalf("unexpected approval payload: %#v", got.Approval)
	}
}

func TestHermesAdapterIgnoresUnknownMethod(t *testing.T) {
	event := types.CodexEvent{Method: "unknown/event"}
	if got := mapHermes(t, event); len(got) != 0 {
		t.Fatalf("expected no events for unknown method, got %#v", got)
	}
}
