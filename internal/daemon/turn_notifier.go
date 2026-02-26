package daemon

import (
	"context"
	"strings"
	"time"

	"control/internal/types"
)

type TurnCompletionEvent struct {
	SessionID   string
	TurnID      string
	Provider    string
	WorkspaceID string
	WorktreeID  string
	Source      string
	Status      string
	Error       string
	Output      string
	Payload     map[string]any
}

type TurnCompletionNotifier interface {
	NotifyTurnCompleted(ctx context.Context, sessionID, turnID, provider string, meta *types.SessionMeta)
	NotifyTurnCompletedEvent(ctx context.Context, event TurnCompletionEvent)
}

type TurnCompletionNotificationPublisherAware interface {
	SetNotificationPublisher(NotificationPublisher)
}

type DefaultTurnCompletionNotifier struct {
	notifier NotificationPublisher
	stores   *Stores
}

func NewTurnCompletionNotifier(notifier NotificationPublisher, stores *Stores) *DefaultTurnCompletionNotifier {
	return &DefaultTurnCompletionNotifier{
		notifier: notifier,
		stores:   stores,
	}
}

func (n *DefaultTurnCompletionNotifier) NotifyTurnCompleted(ctx context.Context, sessionID, turnID, provider string, meta *types.SessionMeta) {
	event := TurnCompletionEvent{
		SessionID: strings.TrimSpace(sessionID),
		TurnID:    strings.TrimSpace(turnID),
		Provider:  strings.TrimSpace(provider),
		Source:    "live_session_event",
	}
	if meta != nil {
		event.WorkspaceID = strings.TrimSpace(meta.WorkspaceID)
		event.WorktreeID = strings.TrimSpace(meta.WorktreeID)
	}
	n.NotifyTurnCompletedEvent(ctx, event)
}

func (n *DefaultTurnCompletionNotifier) NotifyTurnCompletedEvent(ctx context.Context, completion TurnCompletionEvent) {
	if n.notifier == nil {
		return
	}

	event := types.NotificationEvent{
		Trigger:    types.NotificationTriggerTurnCompleted,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  strings.TrimSpace(completion.SessionID),
		TurnID:     strings.TrimSpace(completion.TurnID),
		Provider:   strings.TrimSpace(completion.Provider),
		Source:     strings.TrimSpace(completion.Source),
	}

	if event.Source == "" {
		event.Source = "live_session_event"
	}
	event.WorkspaceID = strings.TrimSpace(completion.WorkspaceID)
	event.WorktreeID = strings.TrimSpace(completion.WorktreeID)
	if event.WorkspaceID == "" || event.WorktreeID == "" {
		if n.stores != nil && n.stores.SessionMeta != nil {
			if m, ok, _ := n.stores.SessionMeta.Get(ctx, event.SessionID); ok && m != nil {
				if event.WorkspaceID == "" {
					event.WorkspaceID = strings.TrimSpace(m.WorkspaceID)
				}
				if event.WorktreeID == "" {
					event.WorktreeID = strings.TrimSpace(m.WorktreeID)
				}
			}
		}
	}
	if strings.TrimSpace(completion.Status) != "" || strings.TrimSpace(completion.Error) != "" {
		event.Payload = map[string]any{
			"turn_status": strings.TrimSpace(completion.Status),
			"turn_error":  strings.TrimSpace(completion.Error),
		}
	}
	if strings.TrimSpace(completion.Output) != "" {
		if event.Payload == nil {
			event.Payload = map[string]any{}
		}
		event.Payload["turn_output"] = strings.TrimSpace(completion.Output)
	}
	for key, value := range cloneNotificationPayload(completion.Payload) {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if event.Payload == nil {
			event.Payload = map[string]any{}
		}
		event.Payload[key] = value
	}

	n.notifier.Publish(event)
}

type NopTurnCompletionNotifier struct{}

func (NopTurnCompletionNotifier) NotifyTurnCompleted(_ context.Context, _, _, _ string, _ *types.SessionMeta) {
}

func (NopTurnCompletionNotifier) NotifyTurnCompletedEvent(context.Context, TurnCompletionEvent) {
}

func (n *DefaultTurnCompletionNotifier) SetNotificationPublisher(notifier NotificationPublisher) {
	if n == nil {
		return
	}
	n.notifier = notifier
}
