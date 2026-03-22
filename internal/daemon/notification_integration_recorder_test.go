package daemon

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"control/internal/providers"
	"control/internal/types"
)

func notificationTurnStatus(event types.NotificationEvent) string {
	if status := strings.TrimSpace(event.Status); status != "" {
		return status
	}
	return strings.TrimSpace(asString(event.Payload["turn_status"]))
}

// NotificationMatchTarget defines the required event identity for the shared test.
type NotificationMatchTarget struct {
	Trigger   types.NotificationTrigger
	SessionID string
	Provider  string
	TurnID    string
}

// NotificationMatchPolicy decides whether a captured event satisfies a target.
type NotificationMatchPolicy interface {
	Matches(event types.NotificationEvent, target NotificationMatchTarget) bool
}

type providerNotificationMatchPolicy struct{}

func newProviderNotificationMatchPolicy() NotificationMatchPolicy {
	return providerNotificationMatchPolicy{}
}

func (providerNotificationMatchPolicy) Matches(event types.NotificationEvent, target NotificationMatchTarget) bool {
	if normalized, ok := types.NormalizeNotificationTrigger(string(target.Trigger)); ok {
		target.Trigger = normalized
	}
	if normalized, ok := types.NormalizeNotificationTrigger(string(event.Trigger)); ok {
		event.Trigger = normalized
	}
	if target.Trigger != "" && event.Trigger != target.Trigger {
		return false
	}
	if sessionID := strings.TrimSpace(target.SessionID); sessionID != "" && strings.TrimSpace(event.SessionID) != sessionID {
		return false
	}
	if provider := providers.Normalize(target.Provider); provider != "" && providers.Normalize(event.Provider) != provider {
		return false
	}
	if turnID := strings.TrimSpace(target.TurnID); turnID != "" && strings.TrimSpace(event.TurnID) != turnID {
		return false
	}
	return true
}

// NotificationRecorder captures and waits for notification events in integration tests.
type NotificationRecorder interface {
	NotificationPublisher
	WaitForMatch(target NotificationMatchTarget, policy NotificationMatchPolicy, timeout time.Duration) (types.NotificationEvent, bool)
	MatchingEvents(target NotificationMatchTarget, policy NotificationMatchPolicy) []types.NotificationEvent
	Snapshot() []types.NotificationEvent
}

type capturingNotificationRecorder struct {
	mu     sync.Mutex
	events []types.NotificationEvent
	signal chan struct{}
}

func newCapturingNotificationRecorder() *capturingNotificationRecorder {
	return &capturingNotificationRecorder{
		signal: make(chan struct{}, 1),
	}
}

func (r *capturingNotificationRecorder) Publish(event types.NotificationEvent) {
	if r == nil {
		return
	}
	normalized := normalizeNotificationEvent(event)
	normalized.Payload = cloneNotificationPayload(normalized.Payload)

	r.mu.Lock()
	r.events = append(r.events, normalized)
	r.mu.Unlock()

	select {
	case r.signal <- struct{}{}:
	default:
	}
}

func (r *capturingNotificationRecorder) WaitForMatch(target NotificationMatchTarget, policy NotificationMatchPolicy, timeout time.Duration) (types.NotificationEvent, bool) {
	if r == nil {
		return types.NotificationEvent{}, false
	}
	if policy == nil {
		policy = newProviderNotificationMatchPolicy()
	}
	if event, ok := firstMatchingNotificationEvent(r.Snapshot(), target, policy); ok {
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
		case <-r.signal:
		case <-time.After(waitWindow):
		}
		if event, ok := firstMatchingNotificationEvent(r.Snapshot(), target, policy); ok {
			return event, true
		}
	}
	return types.NotificationEvent{}, false
}

func (r *capturingNotificationRecorder) MatchingEvents(target NotificationMatchTarget, policy NotificationMatchPolicy) []types.NotificationEvent {
	if r == nil {
		return nil
	}
	if policy == nil {
		policy = newProviderNotificationMatchPolicy()
	}
	snapshot := r.Snapshot()
	out := make([]types.NotificationEvent, 0, len(snapshot))
	for _, event := range snapshot {
		if policy.Matches(event, target) {
			out = append(out, event)
		}
	}
	return out
}

func (r *capturingNotificationRecorder) Snapshot() []types.NotificationEvent {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]types.NotificationEvent, len(r.events))
	copy(out, r.events)
	for i := range out {
		out[i].Payload = cloneNotificationPayload(out[i].Payload)
	}
	return out
}

func firstMatchingNotificationEvent(events []types.NotificationEvent, target NotificationMatchTarget, policy NotificationMatchPolicy) (types.NotificationEvent, bool) {
	for _, event := range events {
		if policy.Matches(event, target) {
			return event, true
		}
	}
	return types.NotificationEvent{}, false
}

func notificationEventsDebugString(events []types.NotificationEvent) string {
	if len(events) == 0 {
		return "<none>"
	}
	parts := make([]string, 0, len(events))
	for _, event := range events {
		parts = append(parts, fmt.Sprintf("{trigger=%q session=%q provider=%q turn=%q status=%q source=%q}",
			event.Trigger,
			strings.TrimSpace(event.SessionID),
			strings.TrimSpace(event.Provider),
			strings.TrimSpace(event.TurnID),
			strings.TrimSpace(notificationTurnStatus(event)),
			strings.TrimSpace(event.Source),
		))
	}
	return strings.Join(parts, ", ")
}
