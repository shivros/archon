package app

import "time"

type MetadataStreamRecoveryDecision struct {
	NextAttempts   int
	ReconnectDelay time.Duration
	RefreshLists   bool
}

type MetadataStreamRecoveryPolicy interface {
	OnError(currentAttempts int) MetadataStreamRecoveryDecision
	OnClosed(currentAttempts int) MetadataStreamRecoveryDecision
	OnConnected() MetadataStreamRecoveryDecision
}

type defaultMetadataStreamRecoveryPolicy struct {
	baseDelay time.Duration
	maxDelay  time.Duration
}

func newDefaultMetadataStreamRecoveryPolicy() MetadataStreamRecoveryPolicy {
	return defaultMetadataStreamRecoveryPolicy{
		baseDelay: metadataStreamRetryBase,
		maxDelay:  metadataStreamRetryMax,
	}
}

func (p defaultMetadataStreamRecoveryPolicy) OnError(currentAttempts int) MetadataStreamRecoveryDecision {
	next := currentAttempts + 1
	if next < 1 {
		next = 1
	}
	return MetadataStreamRecoveryDecision{
		NextAttempts:   next,
		ReconnectDelay: p.reconnectDelay(next),
		RefreshLists:   true,
	}
}

func (p defaultMetadataStreamRecoveryPolicy) OnClosed(currentAttempts int) MetadataStreamRecoveryDecision {
	next := currentAttempts + 1
	if next < 1 {
		next = 1
	}
	return MetadataStreamRecoveryDecision{
		NextAttempts:   next,
		ReconnectDelay: p.reconnectDelay(next),
		RefreshLists:   false,
	}
}

func (p defaultMetadataStreamRecoveryPolicy) OnConnected() MetadataStreamRecoveryDecision {
	return MetadataStreamRecoveryDecision{NextAttempts: 0}
}

func (p defaultMetadataStreamRecoveryPolicy) reconnectDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := p.baseDelay * time.Duration(1<<(attempts-1))
	if delay > p.maxDelay {
		delay = p.maxDelay
	}
	return delay
}
