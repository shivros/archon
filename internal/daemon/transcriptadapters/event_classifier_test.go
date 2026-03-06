package transcriptadapters

import (
	"encoding/json"
	"testing"

	"control/internal/types"
)

func TestCodexEventClassifierDefaultsAndProvider(t *testing.T) {
	classifier := NewCodexEventClassifier(" ")
	if classifier.Provider() != "codex" {
		t.Fatalf("expected default provider codex, got %q", classifier.Provider())
	}
}

func TestCodexEventClassifierClassifiesTranscriptAndNonTranscriptEvents(t *testing.T) {
	classifier := NewCodexEventClassifier("codex")
	tests := []struct {
		name        string
		event       types.CodexEvent
		wantChannel EventChannel
		wantIntent  EventIntent
	}{
		{
			name:        "assistant delta",
			event:       types.CodexEvent{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"delta":"hello"}`)},
			wantChannel: EventChannelTranscript,
			wantIntent:  EventIntentAssistantDelta,
		},
		{
			name:        "approval pending",
			event:       types.CodexEvent{Method: "item/fileChange/requestApproval"},
			wantChannel: EventChannelTranscript,
			wantIntent:  EventIntentApprovalPending,
		},
		{
			name:        "turn completed",
			event:       types.CodexEvent{Method: "turn/completed"},
			wantChannel: EventChannelControl,
			wantIntent:  EventIntentTurnCompleted,
		},
		{
			name:        "rate limit update",
			event:       types.CodexEvent{Method: "account/rateLimits/updated"},
			wantChannel: EventChannelMetadata,
			wantIntent:  EventIntentRateLimitUpdate,
		},
		{
			name:        "debug trace",
			event:       types.CodexEvent{Method: "codex/event/token_count"},
			wantChannel: EventChannelDebug,
			wantIntent:  EventIntentDebugTrace,
		},
		{
			name:        "unknown ignored",
			event:       types.CodexEvent{Method: "item/unknown"},
			wantChannel: EventChannelIgnore,
			wantIntent:  EventIntentUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifier.ClassifyEvent(tc.event)
			if got.Channel != tc.wantChannel || got.Intent != tc.wantIntent {
				t.Fatalf("unexpected classification: got=%#v want channel=%q intent=%q", got, tc.wantChannel, tc.wantIntent)
			}
		})
	}
}

func TestCodexEventClassifierThreadStatusIdleMapsToReady(t *testing.T) {
	classifier := NewCodexEventClassifier("codex")
	got := classifier.ClassifyEvent(types.CodexEvent{
		Method: "thread/status/changed",
		Params: json.RawMessage(`{"status":{"type":"idle"}}`),
	})
	if got.Channel != EventChannelControl || got.Intent != EventIntentStreamReady {
		t.Fatalf("unexpected idle thread classification: %#v", got)
	}
}

func TestCodexEventClassifierControlNoiseBranches(t *testing.T) {
	classifier := NewCodexEventClassifier("codex")
	tests := []struct {
		name        string
		event       types.CodexEvent
		wantChannel EventChannel
		wantIntent  EventIntent
	}{
		{
			name:        "thread active control only",
			event:       types.CodexEvent{Method: "thread/status/changed", Params: json.RawMessage(`{"status":{"type":"active"}}`)},
			wantChannel: EventChannelControl,
			wantIntent:  EventIntentUnknown,
		},
		{
			name:        "thread started control only",
			event:       types.CodexEvent{Method: "thread/started"},
			wantChannel: EventChannelControl,
			wantIntent:  EventIntentUnknown,
		},
		{
			name:        "thread updated control only",
			event:       types.CodexEvent{Method: "thread/updated"},
			wantChannel: EventChannelControl,
			wantIntent:  EventIntentUnknown,
		},
		{
			name:        "thread completed control only",
			event:       types.CodexEvent{Method: "thread/completed"},
			wantChannel: EventChannelControl,
			wantIntent:  EventIntentUnknown,
		},
		{
			name:        "mcp startup control only",
			event:       types.CodexEvent{Method: "codex/event/mcp_startup_complete"},
			wantChannel: EventChannelControl,
			wantIntent:  EventIntentUnknown,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifier.ClassifyEvent(tc.event)
			if got.Channel != tc.wantChannel || got.Intent != tc.wantIntent {
				t.Fatalf("unexpected classification: got=%#v want channel=%q intent=%q", got, tc.wantChannel, tc.wantIntent)
			}
		})
	}
}

func TestOpenCodeEventClassifierDefaultsAndProvider(t *testing.T) {
	classifier := NewOpenCodeEventClassifier(" ")
	if classifier.Provider() != "opencode" {
		t.Fatalf("expected default provider opencode, got %q", classifier.Provider())
	}
}

func TestOpenCodeEventClassifierClassifiesTranscriptAndNonTranscriptEvents(t *testing.T) {
	classifier := NewOpenCodeEventClassifier("opencode")
	tests := []struct {
		name        string
		event       types.CodexEvent
		wantChannel EventChannel
		wantIntent  EventIntent
	}{
		{
			name:        "approval pending",
			event:       types.CodexEvent{Method: "item/commandExecution/requestApproval"},
			wantChannel: EventChannelTranscript,
			wantIntent:  EventIntentApprovalPending,
		},
		{
			name:        "turn failed",
			event:       types.CodexEvent{Method: "error"},
			wantChannel: EventChannelControl,
			wantIntent:  EventIntentTurnFailed,
		},
		{
			name:        "token usage update",
			event:       types.CodexEvent{Method: "thread/tokenUsage/updated"},
			wantChannel: EventChannelMetadata,
			wantIntent:  EventIntentTokenUsageUpdate,
		},
		{
			name:        "debug trace",
			event:       types.CodexEvent{Method: "codex/event/turn_diff"},
			wantChannel: EventChannelDebug,
			wantIntent:  EventIntentDebugTrace,
		},
		{
			name:        "unknown ignored",
			event:       types.CodexEvent{Method: "item/unknown"},
			wantChannel: EventChannelIgnore,
			wantIntent:  EventIntentUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifier.ClassifyEvent(tc.event)
			if got.Channel != tc.wantChannel || got.Intent != tc.wantIntent {
				t.Fatalf("unexpected classification: got=%#v want channel=%q intent=%q", got, tc.wantChannel, tc.wantIntent)
			}
		})
	}
}

func TestOpenCodeEventClassifierAdditionalBranches(t *testing.T) {
	classifier := NewOpenCodeEventClassifier("opencode")
	tests := []struct {
		name        string
		event       types.CodexEvent
		wantChannel EventChannel
		wantIntent  EventIntent
	}{
		{
			name:        "session idle ready",
			event:       types.CodexEvent{Method: "session.idle"},
			wantChannel: EventChannelControl,
			wantIntent:  EventIntentStreamReady,
		},
		{
			name:        "turn started",
			event:       types.CodexEvent{Method: "turn/started"},
			wantChannel: EventChannelControl,
			wantIntent:  EventIntentTurnStarted,
		},
		{
			name:        "turn completed",
			event:       types.CodexEvent{Method: "turn/completed"},
			wantChannel: EventChannelControl,
			wantIntent:  EventIntentTurnCompleted,
		},
		{
			name:        "rate limit update",
			event:       types.CodexEvent{Method: "account/rateLimits/updated"},
			wantChannel: EventChannelMetadata,
			wantIntent:  EventIntentRateLimitUpdate,
		},
		{
			name:        "thread status debug trace",
			event:       types.CodexEvent{Method: "thread/status/changed"},
			wantChannel: EventChannelDebug,
			wantIntent:  EventIntentDebugTrace,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifier.ClassifyEvent(tc.event)
			if got.Channel != tc.wantChannel || got.Intent != tc.wantIntent {
				t.Fatalf("unexpected classification: got=%#v want channel=%q intent=%q", got, tc.wantChannel, tc.wantIntent)
			}
		})
	}
}
