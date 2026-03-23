package daemon

import (
	"context"
	"sync"
	"time"

	"control/internal/types"
)

// NotificationDispatchProbe captures sink delivery for notification integration tests.
type NotificationDispatchProbe interface {
	WaitForMatch(target NotificationMatchTarget, policy NotificationMatchPolicy, timeout time.Duration) (types.NotificationEvent, bool)
	MatchingEvents(target NotificationMatchTarget, policy NotificationMatchPolicy) []types.NotificationEvent
	Snapshot() []types.NotificationEvent
}

type capturingNotificationDispatchProbe struct {
	mu     sync.Mutex
	events []types.NotificationEvent
	signal chan struct{}
}

func newCapturingNotificationDispatchProbe() *capturingNotificationDispatchProbe {
	return &capturingNotificationDispatchProbe{
		signal: make(chan struct{}, 1),
	}
}

func (p *capturingNotificationDispatchProbe) Record(event types.NotificationEvent) {
	if p == nil {
		return
	}
	normalized := normalizeNotificationEvent(event)
	normalized.Payload = cloneNotificationPayload(normalized.Payload)

	p.mu.Lock()
	p.events = append(p.events, normalized)
	p.mu.Unlock()

	select {
	case p.signal <- struct{}{}:
	default:
	}
}

func (p *capturingNotificationDispatchProbe) WaitForMatch(target NotificationMatchTarget, policy NotificationMatchPolicy, timeout time.Duration) (types.NotificationEvent, bool) {
	if p == nil {
		return types.NotificationEvent{}, false
	}
	if policy == nil {
		policy = newProviderNotificationMatchPolicy()
	}
	if event, ok := firstMatchingNotificationEvent(p.Snapshot(), target, policy); ok {
		return event, true
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		waitWindow := providerNotificationPollInterval
		if remaining < waitWindow {
			waitWindow = remaining
		}
		if waitWindow <= 0 {
			break
		}
		select {
		case <-p.signal:
		case <-time.After(waitWindow):
		}
		if event, ok := firstMatchingNotificationEvent(p.Snapshot(), target, policy); ok {
			return event, true
		}
	}
	return types.NotificationEvent{}, false
}

func (p *capturingNotificationDispatchProbe) MatchingEvents(target NotificationMatchTarget, policy NotificationMatchPolicy) []types.NotificationEvent {
	if p == nil {
		return nil
	}
	if policy == nil {
		policy = newProviderNotificationMatchPolicy()
	}
	snapshot := p.Snapshot()
	out := make([]types.NotificationEvent, 0, len(snapshot))
	for _, event := range snapshot {
		if policy.Matches(event, target) {
			out = append(out, event)
		}
	}
	return out
}

func (p *capturingNotificationDispatchProbe) Snapshot() []types.NotificationEvent {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]types.NotificationEvent, len(p.events))
	copy(out, p.events)
	for i := range out {
		out[i].Payload = cloneNotificationPayload(out[i].Payload)
	}
	return out
}

type notificationDispatchProbeSink struct {
	probe *capturingNotificationDispatchProbe
}

func newNotificationDispatchProbeSink(probe *capturingNotificationDispatchProbe) NotificationSink {
	return notificationDispatchProbeSink{probe: probe}
}

func (notificationDispatchProbeSink) Method() types.NotificationMethod {
	return types.NotificationMethodBell
}

func (s notificationDispatchProbeSink) Notify(ctx context.Context, event types.NotificationEvent, settings types.NotificationSettings) error {
	if s.probe == nil {
		return nil
	}
	s.probe.Record(event)
	return nil
}
