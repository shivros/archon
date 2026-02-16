package daemon

import (
	"context"

	"control/internal/types"
)

// NotificationPublisher emits notification events asynchronously.
type NotificationPublisher interface {
	Publish(event types.NotificationEvent)
}

// NotificationPolicyResolver computes effective notification settings for an event.
type NotificationPolicyResolver interface {
	Resolve(ctx context.Context, event types.NotificationEvent) types.NotificationSettings
}

// NotificationDispatcher sends a notification event through configured channels.
type NotificationDispatcher interface {
	Dispatch(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error
}

// NotificationSink handles one notification method implementation.
type NotificationSink interface {
	Method() types.NotificationMethod
	Notify(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error
}

// NotificationLifecycle controls long-running notification processing resources.
type NotificationLifecycle interface {
	Start()
	Stop(ctx context.Context) error
}

// NotificationDedupePolicy decides whether an event should be suppressed.
type NotificationDedupePolicy interface {
	ShouldSuppress(event types.NotificationEvent, settings types.NotificationSettings) bool
}

// SessionLifecycleEmitter emits terminal session lifecycle notifications.
type SessionLifecycleEmitter interface {
	EmitSessionLifecycleEvent(ctx context.Context, session *types.Session, cfg StartSessionConfig, status types.SessionStatus, source string)
}
