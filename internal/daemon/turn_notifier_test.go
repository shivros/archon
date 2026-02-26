package daemon

import (
	"context"
	"testing"

	"control/internal/types"
)

type captureTurnNotifierPublisher struct {
	events []types.NotificationEvent
}

func (p *captureTurnNotifierPublisher) Publish(event types.NotificationEvent) {
	p.events = append(p.events, event)
}

type stubTurnNotifierSessionMetaStore struct {
	meta *types.SessionMeta
}

func (s stubTurnNotifierSessionMetaStore) List(context.Context) ([]*types.SessionMeta, error) {
	if s.meta == nil {
		return []*types.SessionMeta{}, nil
	}
	copy := *s.meta
	return []*types.SessionMeta{&copy}, nil
}

func (s stubTurnNotifierSessionMetaStore) Get(_ context.Context, _ string) (*types.SessionMeta, bool, error) {
	if s.meta == nil {
		return nil, false, nil
	}
	copy := *s.meta
	return &copy, true, nil
}

func (s stubTurnNotifierSessionMetaStore) Upsert(_ context.Context, meta *types.SessionMeta) (*types.SessionMeta, error) {
	if meta == nil {
		return nil, nil
	}
	copy := *meta
	return &copy, nil
}

func (s stubTurnNotifierSessionMetaStore) Delete(context.Context, string) error {
	return nil
}

func TestDefaultTurnCompletionNotifierNotifyTurnCompletedWrapsEvent(t *testing.T) {
	publisher := &captureTurnNotifierPublisher{}
	notifier := NewTurnCompletionNotifier(publisher, nil)
	notifier.NotifyTurnCompleted(context.Background(), "sess-1", "turn-1", "codex", &types.SessionMeta{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if len(publisher.events) != 1 {
		t.Fatalf("expected one event, got %d", len(publisher.events))
	}
	event := publisher.events[0]
	if event.SessionID != "sess-1" || event.TurnID != "turn-1" || event.Provider != "codex" {
		t.Fatalf("unexpected event identity fields: %#v", event)
	}
	if event.WorkspaceID != "ws-1" || event.WorktreeID != "wt-1" {
		t.Fatalf("expected meta workspace/worktree propagation, got %#v", event)
	}
}

func TestDefaultTurnCompletionNotifierNotifyTurnCompletedEventBackfillsMetaAndPayload(t *testing.T) {
	publisher := &captureTurnNotifierPublisher{}
	stores := &Stores{
		SessionMeta: stubTurnNotifierSessionMetaStore{
			meta: &types.SessionMeta{
				SessionID:   "sess-2",
				WorkspaceID: "ws-backfill",
				WorktreeID:  "wt-backfill",
			},
		},
	}
	notifier := NewTurnCompletionNotifier(publisher, stores)
	notifier.NotifyTurnCompletedEvent(context.Background(), TurnCompletionEvent{
		SessionID: "sess-2",
		TurnID:    "turn-2",
		Provider:  "opencode",
		Status:    "failed",
		Error:     "unsupported model",
	})
	if len(publisher.events) != 1 {
		t.Fatalf("expected one event, got %d", len(publisher.events))
	}
	event := publisher.events[0]
	if event.WorkspaceID != "ws-backfill" || event.WorktreeID != "wt-backfill" {
		t.Fatalf("expected store backfill for workspace/worktree, got %#v", event)
	}
	if got := event.Payload["turn_status"]; got != "failed" {
		t.Fatalf("expected turn_status payload, got %#v", got)
	}
	if got := event.Payload["turn_error"]; got != "unsupported model" {
		t.Fatalf("expected turn_error payload, got %#v", got)
	}
}

func TestDefaultTurnCompletionNotifierSetNotificationPublisherSwapsPublisher(t *testing.T) {
	publisherA := &captureTurnNotifierPublisher{}
	publisherB := &captureTurnNotifierPublisher{}
	notifier := NewTurnCompletionNotifier(publisherA, nil)
	notifier.NotifyTurnCompletedEvent(context.Background(), TurnCompletionEvent{SessionID: "sess-a", TurnID: "turn-a"})
	notifier.SetNotificationPublisher(publisherB)
	notifier.NotifyTurnCompletedEvent(context.Background(), TurnCompletionEvent{SessionID: "sess-b", TurnID: "turn-b"})
	if len(publisherA.events) != 1 {
		t.Fatalf("expected first publisher to receive only pre-swap event, got %d", len(publisherA.events))
	}
	if len(publisherB.events) != 1 {
		t.Fatalf("expected swapped publisher to receive one event, got %d", len(publisherB.events))
	}
}

func TestDefaultTurnCompletionNotifierNotifyTurnCompletedEventDefaultsSource(t *testing.T) {
	publisher := &captureTurnNotifierPublisher{}
	notifier := NewTurnCompletionNotifier(publisher, nil)
	notifier.NotifyTurnCompletedEvent(context.Background(), TurnCompletionEvent{
		SessionID: "sess-default-source",
		TurnID:    "turn-default-source",
		Provider:  "codex",
		Source:    "",
	})
	if len(publisher.events) != 1 {
		t.Fatalf("expected one event, got %d", len(publisher.events))
	}
	if publisher.events[0].Source != "live_session_event" {
		t.Fatalf("expected default source fallback, got %q", publisher.events[0].Source)
	}
}

func TestDefaultTurnCompletionNotifierNotifyTurnCompletedEventNoPublisher(t *testing.T) {
	notifier := NewTurnCompletionNotifier(nil, nil)
	notifier.NotifyTurnCompletedEvent(context.Background(), TurnCompletionEvent{
		SessionID: "sess-no-publisher",
		TurnID:    "turn-no-publisher",
	})
}

func TestDefaultTurnCompletionNotifierSetNotificationPublisherNilReceiver(t *testing.T) {
	var notifier *DefaultTurnCompletionNotifier
	notifier.SetNotificationPublisher(nil)
}

func TestNopTurnCompletionNotifierImplementsAllMethods(t *testing.T) {
	var notifier TurnCompletionNotifier = NopTurnCompletionNotifier{}
	notifier.NotifyTurnCompleted(context.Background(), "sess", "turn", "codex", nil)
	notifier.NotifyTurnCompletedEvent(context.Background(), TurnCompletionEvent{SessionID: "sess", TurnID: "turn"})
}
