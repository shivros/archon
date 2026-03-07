package daemon

import (
	"context"
	"time"

	"control/internal/types"
)

type openCodeEventStreamConnector interface {
	SubscribeSessionEvents(ctx context.Context, sessionID, directory string) (<-chan types.CodexEvent, func(), error)
}

type openCodeEventReconnectPolicy interface {
	RetryDelay(attempt int) time.Duration
	ShouldLogFailure(attempt int) bool
}

type defaultOpenCodeEventReconnectPolicy struct{}

func (defaultOpenCodeEventReconnectPolicy) RetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 300 * time.Millisecond
	for step := 1; step < attempt; step++ {
		delay *= 2
		if delay >= 3*time.Second {
			return 3 * time.Second
		}
	}
	return delay
}

func (defaultOpenCodeEventReconnectPolicy) ShouldLogFailure(attempt int) bool {
	if attempt <= 0 {
		return false
	}
	if attempt <= 3 {
		return true
	}
	return attempt%10 == 0
}
