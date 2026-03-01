package daemon

import (
	"context"
	"testing"

	"control/internal/types"
)

func TestDefaultOpenCodeTurnFinalizerNoNotifierNoop(t *testing.T) {
	finalizer := &defaultOpenCodeTurnFinalizer{
		sessionID:    "sess-1",
		providerName: "opencode",
		notifier:     nil,
	}
	finalizer.FinalizeTurn(turnEventParams{TurnID: "turn-1", Status: "completed"}, nil)
}

func TestDefaultOpenCodeTurnFinalizerUsesDefaultPayloadBuilder(t *testing.T) {
	notifier := &captureTurnCompletionNotifier{}
	finalizer := &defaultOpenCodeTurnFinalizer{
		sessionID:    "sess-1",
		providerName: "opencode",
		notifier:     notifier,
		payloads:     nil,
		freshness:    &alwaysFreshTracker{},
	}
	finalizer.FinalizeTurn(turnEventParams{TurnID: "turn-1", Status: "completed", Output: "fallback"}, map[string]any{"k": "v"})
	if len(notifier.events) != 1 {
		t.Fatalf("expected one completion event, got %d", len(notifier.events))
	}
	if got := notifier.events[0].Payload["k"]; got != "v" {
		t.Fatalf("expected additional payload to be merged, got %#v", notifier.events[0].Payload)
	}
}

func TestDefaultOpenCodeTurnFinalizerStaleOutputDropped(t *testing.T) {
	notifier := &captureTurnCompletionNotifier{}
	finalizer := &defaultOpenCodeTurnFinalizer{
		sessionID:    "sess-2",
		providerName: "opencode",
		notifier:     notifier,
		payloads:     defaultTurnCompletionPayloadBuilder{},
		freshness:    &alwaysStaleTracker{},
	}
	finalizer.FinalizeTurn(turnEventParams{TurnID: "turn-2", Status: "completed", Output: "should-drop"}, nil)
	if len(notifier.events) != 1 {
		t.Fatalf("expected one completion event, got %d", len(notifier.events))
	}
	payload := notifier.events[0].Payload
	if _, ok := payload["turn_output"]; ok {
		t.Fatalf("expected stale output to be removed, payload=%#v", payload)
	}
	if stale, _ := payload["stale_turn_output_dropped"].(bool); !stale {
		t.Fatalf("expected stale_turn_output_dropped marker, payload=%#v", payload)
	}
}

type captureTurnCompletionNotifier struct {
	events []TurnCompletionEvent
}

func (c *captureTurnCompletionNotifier) NotifyTurnCompleted(context.Context, string, string, string, *types.SessionMeta) {
}

func (c *captureTurnCompletionNotifier) NotifyTurnCompletedEvent(_ context.Context, event TurnCompletionEvent) {
	c.events = append(c.events, event)
}

type alwaysFreshTracker struct{}

func (alwaysFreshTracker) MarkFresh(string, string, string) bool { return true }

type alwaysStaleTracker struct{}

func (alwaysStaleTracker) MarkFresh(string, string, string) bool { return false }
