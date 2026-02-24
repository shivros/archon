package daemon

import (
	"context"
	"time"

	"control/internal/types"
)

type TurnCompletionNotifier interface {
	NotifyTurnCompleted(ctx context.Context, sessionID, turnID, provider string, meta *types.SessionMeta)
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
	if n.notifier == nil {
		return
	}

	event := types.NotificationEvent{
		Trigger:    types.NotificationTriggerTurnCompleted,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  sessionID,
		TurnID:     turnID,
		Provider:   provider,
		Source:     "live_session_event",
	}

	if meta != nil {
		event.WorkspaceID = meta.WorkspaceID
		event.WorktreeID = meta.WorktreeID
	} else if n.stores != nil && n.stores.SessionMeta != nil {
		if m, ok, _ := n.stores.SessionMeta.Get(ctx, sessionID); ok && m != nil {
			event.WorkspaceID = m.WorkspaceID
			event.WorktreeID = m.WorktreeID
		}
	}

	n.notifier.Publish(event)
}

type NopTurnCompletionNotifier struct{}

func (NopTurnCompletionNotifier) NotifyTurnCompleted(_ context.Context, _, _, _ string, _ *types.SessionMeta) {
}
