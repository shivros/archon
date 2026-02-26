package daemon

import (
	"testing"
	"time"

	"control/internal/types"
)

type captureSessionServiceNotificationPublisher struct {
	events []types.NotificationEvent
}

func (p *captureSessionServiceNotificationPublisher) Publish(event types.NotificationEvent) {
	p.events = append(p.events, event)
}

func TestSessionServicePublishTurnCompletedWithPayload(t *testing.T) {
	publisher := &captureSessionServiceNotificationPublisher{}
	service := &SessionService{notifier: publisher}
	session := &types.Session{
		ID:        "sess-1",
		Provider:  "opencode",
		Status:    types.SessionStatusRunning,
		CreatedAt: time.Now().UTC(),
	}
	meta := &types.SessionMeta{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}
	service.publishTurnCompletedWithPayload(session, meta, "turn-1", "test_source", map[string]any{
		"turn_status": "failed",
		"turn_error":  "unsupported model",
	})
	if len(publisher.events) != 1 {
		t.Fatalf("expected one event, got %d", len(publisher.events))
	}
	event := publisher.events[0]
	if event.Trigger != types.NotificationTriggerTurnCompleted {
		t.Fatalf("unexpected trigger: %q", event.Trigger)
	}
	if event.Source != "test_source" {
		t.Fatalf("unexpected source: %q", event.Source)
	}
	if event.WorkspaceID != "ws-1" || event.WorktreeID != "wt-1" {
		t.Fatalf("expected workspace/worktree from meta, got %#v", event)
	}
	if event.Payload["turn_status"] != "failed" || event.Payload["turn_error"] != "unsupported model" {
		t.Fatalf("unexpected payload: %#v", event.Payload)
	}
}

func TestSessionServicePublishTurnCompletedWithoutPayload(t *testing.T) {
	publisher := &captureSessionServiceNotificationPublisher{}
	service := &SessionService{notifier: publisher}
	session := &types.Session{
		ID:       "sess-2",
		Provider: "codex",
		Status:   types.SessionStatusRunning,
	}
	service.publishTurnCompleted(session, nil, "turn-2", "test_source")
	if len(publisher.events) != 1 {
		t.Fatalf("expected one event, got %d", len(publisher.events))
	}
	if publisher.events[0].Payload != nil {
		t.Fatalf("expected nil payload for publishTurnCompleted wrapper, got %#v", publisher.events[0].Payload)
	}
}
