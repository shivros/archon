package daemon

import (
	"context"

	"control/internal/types"
)

type LiveSession interface {
	Events() <-chan types.CodexEvent
	Close()
	SessionID() string
}

type TurnCapableSession interface {
	LiveSession
	StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error)
	Interrupt(ctx context.Context) error
	ActiveTurnID() string
}

type ApprovalCapableSession interface {
	LiveSession
	Respond(ctx context.Context, requestID int, result map[string]any) error
}

type NotifiableSession interface {
	LiveSession
	SetNotificationPublisher(notifier NotificationPublisher)
}
