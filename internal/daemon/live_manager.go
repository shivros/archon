package daemon

import (
	"context"

	"control/internal/types"
)

type TurnStarter interface {
	StartTurn(ctx context.Context, session *types.Session, meta *types.SessionMeta, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error)
}

type EventStreamer interface {
	Subscribe(session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error)
}

type ApprovalResponder interface {
	Respond(ctx context.Context, session *types.Session, meta *types.SessionMeta, requestID int, result map[string]any) error
}

type TurnInterrupter interface {
	Interrupt(ctx context.Context, session *types.Session, meta *types.SessionMeta) error
}

type NotificationReceiver interface {
	SetNotificationPublisher(notifier NotificationPublisher)
}

type LiveManager interface {
	TurnStarter
	EventStreamer
	ApprovalResponder
	TurnInterrupter
	NotificationReceiver
}

type LiveSessionFactory interface {
	Create(ctx context.Context, session *types.Session, meta *types.SessionMeta) (LiveSession, error)
	ProviderName() string
}

type TurnCapableSessionFactory interface {
	CreateTurnCapable(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error)
	ProviderName() string
}
