package transcriptadapters

import (
	"encoding/json"
	"testing"

	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

func TestCapabilityEnvelopeFromProvider(t *testing.T) {
	caps := CapabilityEnvelopeFromProvider("opencode")
	if !caps.SupportsEvents || !caps.SupportsApprovals || !caps.UsesItems {
		t.Fatalf("unexpected capability mapping: %#v", caps)
	}
}

func TestTranscriptEventFromCodexEventTurnCompleted(t *testing.T) {
	event := types.CodexEvent{
		Method: "turn/completed",
		Params: json.RawMessage(`{"turn_id":"t1","status":"completed"}`),
	}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("5"), event)
	if got.Kind != transcriptdomain.TranscriptEventTurnCompleted {
		t.Fatalf("expected turn completed event, got %q", got.Kind)
	}
	if got.Turn == nil || got.Turn.State != transcriptdomain.TurnStateCompleted || got.Turn.TurnID != "t1" {
		t.Fatalf("unexpected turn payload: %#v", got.Turn)
	}
}

func TestTranscriptEventFromCodexEventTurnStarted(t *testing.T) {
	event := types.CodexEvent{Method: "turn/started", Params: json.RawMessage(`{"turn_id":"t0"}`)}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("1"), event)
	if got.Kind != transcriptdomain.TranscriptEventTurnStarted {
		t.Fatalf("expected turn started event, got %q", got.Kind)
	}
	if got.Turn == nil || got.Turn.State != transcriptdomain.TurnStateRunning {
		t.Fatalf("unexpected turn started payload: %#v", got.Turn)
	}
}

func TestTranscriptEventFromCodexEventSessionIdleMapsToStreamStatus(t *testing.T) {
	event := types.CodexEvent{Method: "session.idle"}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("2"), event)
	if got.Kind != transcriptdomain.TranscriptEventStreamStatus {
		t.Fatalf("expected stream status event, got %q", got.Kind)
	}
	if got.StreamStatus != transcriptdomain.StreamStatusReady {
		t.Fatalf("unexpected stream status: %q", got.StreamStatus)
	}
}

func TestTranscriptEventFromCodexEventErrorBecomesTurnFailed(t *testing.T) {
	event := types.CodexEvent{
		Method: "error",
		Params: json.RawMessage(`{"turn_id":"t2","status":"failed","error":"boom"}`),
	}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("6"), event)
	if got.Kind != transcriptdomain.TranscriptEventTurnFailed {
		t.Fatalf("expected turn failed event, got %q", got.Kind)
	}
	if got.Turn == nil || got.Turn.State != transcriptdomain.TurnStateFailed || got.Turn.Error != "boom" {
		t.Fatalf("unexpected failed turn payload: %#v", got.Turn)
	}
}

func TestTranscriptEventFromCodexEventApprovalPending(t *testing.T) {
	id := 7
	event := types.CodexEvent{ID: &id, Method: "item/fileChange/requestApproval"}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("8"), event)
	if got.Kind != transcriptdomain.TranscriptEventApprovalPending {
		t.Fatalf("expected approval pending event, got %q", got.Kind)
	}
	if got.Approval == nil || got.Approval.RequestID != 7 {
		t.Fatalf("unexpected approval payload: %#v", got.Approval)
	}
}

func TestTranscriptEventFromCodexEventApprovalResolved(t *testing.T) {
	event := types.CodexEvent{Method: "item/replyPermission", Params: json.RawMessage(`{"request_id":42}`)}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("9"), event)
	if got.Kind != transcriptdomain.TranscriptEventApprovalResolved {
		t.Fatalf("expected approval resolved event, got %q", got.Kind)
	}
	if got.Approval == nil || got.Approval.RequestID != 42 {
		t.Fatalf("unexpected approval payload: %#v", got.Approval)
	}
}

func TestTranscriptEventFromCodexEventUnknownDefaultsToDelta(t *testing.T) {
	event := types.CodexEvent{Method: "item/unknown"}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("10"), event)
	if got.Kind != "" {
		t.Fatalf("expected unknown empty event for non-content item, got %#v", got)
	}
}

func TestTranscriptEventFromCodexEventParsesTimestamp(t *testing.T) {
	event := types.CodexEvent{Method: "item/unknown", TS: "2026-03-02T12:00:00Z"}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("11"), event)
	if got.Kind != "" {
		t.Fatalf("expected ignored non-content item event, got %#v", got)
	}
}

func TestTranscriptEventFromCodexEventAgentMessageDelta(t *testing.T) {
	event := types.CodexEvent{
		Method: "item/agentMessage/delta",
		Params: json.RawMessage(`{"itemId":"msg_1","delta":"hello"}`),
	}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("11"), event)
	if got.Kind != transcriptdomain.TranscriptEventDelta {
		t.Fatalf("expected delta event, got %q", got.Kind)
	}
	if len(got.Delta) != 1 {
		t.Fatalf("expected single delta block, got %#v", got.Delta)
	}
	if got.Delta[0].Role != "assistant" || got.Delta[0].Text != "hello" || got.Delta[0].ID != "msg_1" {
		t.Fatalf("unexpected delta payload: %#v", got.Delta[0])
	}
}

func TestTranscriptEventFromCodexEventThreadStatusChangedIdleMapsToReady(t *testing.T) {
	event := types.CodexEvent{
		Method: "thread/status/changed",
		Params: json.RawMessage(`{"status":{"type":"idle"}}`),
	}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("11"), event)
	if got.Kind != transcriptdomain.TranscriptEventStreamStatus || got.StreamStatus != transcriptdomain.StreamStatusReady {
		t.Fatalf("unexpected thread status mapping: %#v", got)
	}
}

func TestTranscriptEventFromCodexEventThreadStatusChangedActiveIgnored(t *testing.T) {
	event := types.CodexEvent{
		Method: "thread/status/changed",
		Params: json.RawMessage(`{"status":{"type":"active"}}`),
	}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("11"), event)
	if got.Kind != "" {
		t.Fatalf("expected active thread status event ignored, got %#v", got)
	}
}

func TestTranscriptEventFromCodexEventMCPStartupIgnored(t *testing.T) {
	event := types.CodexEvent{
		Method: "codex/event/mcp_startup_complete",
		Params: json.RawMessage(`{"msg":{"type":"mcp_startup_complete"}}`),
	}
	got := TranscriptEventFromCodexEvent("s1", "codex", transcriptdomain.MustParseRevisionToken("11"), event)
	if got.Kind != "" {
		t.Fatalf("expected mcp startup event ignored, got %#v", got)
	}
}

func TestBlockFromItem(t *testing.T) {
	block, ok := BlockFromItem(map[string]any{
		"id":   "m1",
		"type": "assistant_message",
		"text": "hello",
	})
	if !ok {
		t.Fatal("expected block conversion")
	}
	if block.ID != "m1" || block.Kind != "assistant_message" || block.Text != "hello" {
		t.Fatalf("unexpected block: %#v", block)
	}
}

func TestBlockFromItemFallsBackRoleAndVariant(t *testing.T) {
	block, ok := BlockFromItem(map[string]any{
		"item_id": "m2",
		"type":    "assistant_tool",
		"content": "tool output",
		"subtype": "tool_result",
	})
	if !ok {
		t.Fatal("expected block conversion")
	}
	if block.Role != "assistant" || block.Variant != "tool_result" || block.ID != "m2" {
		t.Fatalf("unexpected block fallback mapping: %#v", block)
	}
}

func TestBlockFromItemRejectsMissingText(t *testing.T) {
	if _, ok := BlockFromItem(map[string]any{"type": "assistant"}); ok {
		t.Fatal("expected block conversion failure for empty text")
	}
}

func TestDeltaEventFromItem(t *testing.T) {
	event, ok := DeltaEventFromItem("s1", "claude", transcriptdomain.MustParseRevisionToken("2"), map[string]any{
		"type": "assistant",
		"text": "hi",
	})
	if !ok {
		t.Fatal("expected delta event conversion")
	}
	if event.Kind != transcriptdomain.TranscriptEventDelta || len(event.Delta) != 1 {
		t.Fatalf("unexpected event: %#v", event)
	}
}

func TestDeltaEventFromItemRejectsInvalidItem(t *testing.T) {
	if _, ok := DeltaEventFromItem("s1", "claude", transcriptdomain.MustParseRevisionToken("2"), map[string]any{"type": "assistant"}); ok {
		t.Fatal("expected invalid item conversion to fail")
	}
}

func TestTurnStateFromEventParamsFallbacks(t *testing.T) {
	turn := turnStateFromEventParams(json.RawMessage(`{"turnId":"t5","status":"started"}`), transcriptdomain.TurnStateCompleted)
	if turn.State != transcriptdomain.TurnStateRunning || turn.TurnID != "t5" {
		t.Fatalf("unexpected turn mapping: %#v", turn)
	}

	failed := turnStateFromEventParams(json.RawMessage(`{"id":"t6","status":"failed"}`), transcriptdomain.TurnStateCompleted)
	if failed.State != transcriptdomain.TurnStateFailed || failed.Error == "" {
		t.Fatalf("expected failed fallback error, got %#v", failed)
	}
}

func TestApprovalRequestIDVariants(t *testing.T) {
	id := 12
	if got := approvalRequestID(types.CodexEvent{ID: &id}); got != 12 {
		t.Fatalf("expected id from event.ID, got %d", got)
	}
	if got := approvalRequestID(types.CodexEvent{Params: json.RawMessage(`{"request_id":13}`)}); got != 13 {
		t.Fatalf("expected id from request_id, got %d", got)
	}
	if got := approvalRequestID(types.CodexEvent{Params: json.RawMessage(`{"requestId":14}`)}); got != 14 {
		t.Fatalf("expected id from requestId, got %d", got)
	}
	if got := approvalRequestID(types.CodexEvent{Params: json.RawMessage(`{"id":15}`)}); got != 15 {
		t.Fatalf("expected id from id, got %d", got)
	}
	if got := approvalRequestID(types.CodexEvent{Params: json.RawMessage(`{"x":1}`)}); got != 0 {
		t.Fatalf("expected zero when no id present, got %d", got)
	}
}

func TestParseEventTime(t *testing.T) {
	if got := parseEventTime(""); !got.IsZero() {
		t.Fatalf("expected zero time for empty input, got %v", got)
	}
	if got := parseEventTime("not-a-time"); !got.IsZero() {
		t.Fatalf("expected zero time for invalid input, got %v", got)
	}
	if got := parseEventTime("2026-03-02T12:00:00Z"); got.IsZero() {
		t.Fatal("expected parsed RFC3339 time")
	}
}

func TestThreadStatusFromEventParams(t *testing.T) {
	if got := threadStatusFromEventParams(json.RawMessage(`{"status":{"type":"idle"}}`)); got != "idle" {
		t.Fatalf("expected idle status, got %q", got)
	}
	if got := threadStatusFromEventParams(json.RawMessage(`{"status":"active"}`)); got != "active" {
		t.Fatalf("expected active status, got %q", got)
	}
}

func TestDecodeMapAndAsString(t *testing.T) {
	decoded := decodeMap(json.RawMessage(`{"a":"b"}`))
	if decoded["a"] != "b" {
		t.Fatalf("unexpected decodeMap result: %#v", decoded)
	}
	if got := decodeMap(json.RawMessage(`{`)); len(got) != 0 {
		t.Fatalf("expected empty map on malformed json, got %#v", got)
	}
	if got := decodeMap(nil); len(got) != 0 {
		t.Fatalf("expected empty map on nil input, got %#v", got)
	}
	if asString("x") != "x" {
		t.Fatal("expected string passthrough")
	}
	if asString(json.Number("42")) != "42" {
		t.Fatal("expected json.Number conversion")
	}
	if asString(99) != "" {
		t.Fatal("expected unsupported type to convert to empty string")
	}
}

func TestAsIntCoversAllSupportedTypes(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
		ok   bool
	}{
		{name: "int", in: int(1), want: 1, ok: true},
		{name: "int32", in: int32(2), want: 2, ok: true},
		{name: "int64", in: int64(3), want: 3, ok: true},
		{name: "float64", in: float64(4), want: 4, ok: true},
		{name: "json number", in: json.Number("5"), want: 5, ok: true},
		{name: "invalid json number", in: json.Number("nan"), want: 0, ok: false},
		{name: "unsupported", in: "6", want: 0, ok: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := asInt(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("asInt(%#v) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestCodexAdapterMapEventUsesActiveTurnFallback(t *testing.T) {
	adapter := NewCodexTranscriptAdapter("codex")
	events := adapter.MapEvent(MappingContext{
		SessionID:    "s1",
		Revision:     transcriptdomain.MustParseRevisionToken("12"),
		ActiveTurnID: "turn-fallback",
	}, types.CodexEvent{Method: "turn/completed", Params: json.RawMessage(`{"status":"completed"}`)})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Turn == nil || events[0].Turn.TurnID != "turn-fallback" {
		t.Fatalf("expected active turn fallback, got %#v", events[0].Turn)
	}
}

func TestCodexAdapterProviderDefaultsToCodex(t *testing.T) {
	adapter := NewCodexTranscriptAdapter(" ")
	if adapter.Provider() != "codex" {
		t.Fatalf("expected default provider codex, got %q", adapter.Provider())
	}
}

func TestCodexAdapterMapItem(t *testing.T) {
	adapter := NewCodexTranscriptAdapter("codex")
	events := adapter.MapItem(MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("13"),
	}, map[string]any{"type": "assistant", "text": "hello"})
	if len(events) != 1 {
		t.Fatalf("expected one event, got %d", len(events))
	}
	if events[0].Kind != transcriptdomain.TranscriptEventDelta {
		t.Fatalf("expected delta event, got %q", events[0].Kind)
	}
}
