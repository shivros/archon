package daemon

import (
	"context"
	"testing"
	"time"

	"control/internal/types"
)

func TestNewNotificationIntegrationServerWiresRecorder(t *testing.T) {
	t.Parallel()
	env := newNotificationIntegrationServer(t)
	defer env.Close()

	if env == nil || env.server == nil || env.manager == nil || env.stores == nil || env.live == nil || env.recorder == nil || env.dispatchProbe == nil || env.notificationService == nil {
		t.Fatalf("expected non-nil server wiring components")
	}
	if got := listSessions(t, env.server); len(got.Sessions) != 0 {
		t.Fatalf("expected empty sessions list on new integration server")
	}

	env.manager.mu.Lock()
	notifier := env.manager.notifier
	env.manager.mu.Unlock()
	if notifier == nil {
		t.Fatalf("expected manager notifier to be wired")
	}

	target := NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-smoke",
		Provider:  "codex",
		TurnID:    "turn-smoke",
	}
	notifier.Publish(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-smoke",
		Provider:  "codex",
		TurnID:    "turn-smoke",
	})
	if _, ok := env.recorder.WaitForMatch(target, newProviderNotificationMatchPolicy(), time.Second); !ok {
		t.Fatalf("expected published event to reach integration recorder")
	}
	if _, ok := env.dispatchProbe.WaitForMatch(target, newProviderNotificationMatchPolicy(), time.Second); !ok {
		t.Fatalf("expected published event to reach integration dispatch sink")
	}
}

type stubNotificationEventProcessor struct {
	calls  int
	last   types.NotificationEvent
	lastOK bool
}

func (s *stubNotificationEventProcessor) Process(ctx context.Context, event types.NotificationEvent) {
	s.calls++
	s.last = event
	s.lastOK = ctx != nil
}

func TestSynchronousNotificationServicePublisherUsesProcessor(t *testing.T) {
	t.Parallel()
	processor := &stubNotificationEventProcessor{}
	publisher := newSynchronousNotificationServicePublisher(processor)
	if publisher == nil {
		t.Fatalf("expected publisher")
	}
	event := types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-sync",
		TurnID:    "turn-sync",
	}
	publisher.Publish(event)
	if processor.calls != 1 {
		t.Fatalf("expected one processor call, got %d", processor.calls)
	}
	if !processor.lastOK {
		t.Fatalf("expected non-nil context")
	}
	if processor.last.SessionID != event.SessionID || processor.last.TurnID != event.TurnID {
		t.Fatalf("unexpected forwarded event: %#v", processor.last)
	}
}
