package daemon

import (
	"context"
	"strings"
	"sync"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

type queuedNotification struct {
	ctx   context.Context
	event types.NotificationEvent
}

type NotificationService struct {
	resolver   NotificationPolicyResolver
	dispatcher NotificationDispatcher
	dedupe     NotificationDedupePolicy
	logger     logging.Logger

	mu       sync.RWMutex
	events   chan queuedNotification
	runCtx   context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	started  bool
	stopping bool
	closed   bool
}

func NewNotificationService(resolver NotificationPolicyResolver, dispatcher NotificationDispatcher, logger logging.Logger) *NotificationService {
	if logger == nil {
		logger = logging.Nop()
	}
	svc := &NotificationService{
		resolver:   resolver,
		dispatcher: dispatcher,
		dedupe:     newWindowNotificationDedupePolicy(),
		logger:     logger,
		events:     make(chan queuedNotification, 256),
	}
	svc.Start()
	return svc
}

func (s *NotificationService) Start() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.closed {
		return
	}
	s.runCtx, s.cancel = context.WithCancel(context.Background())
	s.started = true
	s.wg.Add(1)
	go s.run(s.runCtx)
}

func (s *NotificationService) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.stopping = true
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.mu.Lock()
		s.stopping = false
		s.started = false
		s.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *NotificationService) Close() {
	_ = s.Stop(context.Background())
}

func (s *NotificationService) Publish(event types.NotificationEvent) {
	if s == nil {
		return
	}
	s.Start()
	ctx := context.Background()
	s.mu.RLock()
	events := s.events
	if s.stopping || s.closed || !s.started {
		s.mu.RUnlock()
		if s.logger != nil {
			s.logger.Debug("notification_publish_ignored_stopping",
				logging.F("trigger", event.Trigger),
				logging.F("session_id", event.SessionID),
			)
		}
		return
	}
	s.mu.RUnlock()

	select {
	case events <- queuedNotification{ctx: ctx, event: event}:
	default:
		if s.logger != nil {
			s.logger.Warn("notification_queue_full",
				logging.F("trigger", event.Trigger),
				logging.F("session_id", event.SessionID),
			)
		}
	}
}

func (s *NotificationService) run(runCtx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-runCtx.Done():
			return
		case queued := <-s.events:
			s.handle(queued.ctx, queued.event)
		}
	}
}

func (s *NotificationService) handle(ctx context.Context, event types.NotificationEvent) {
	if s == nil || s.resolver == nil || s.dispatcher == nil {
		return
	}
	event = normalizeNotificationEvent(event)
	if event.Trigger == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	settings := s.resolver.Resolve(ctx, event)
	if !settings.Enabled {
		return
	}
	if !types.NotificationTriggerEnabled(settings, event.Trigger) {
		return
	}
	if s.shouldSuppress(event, settings) {
		return
	}

	timeout := time.Duration(settings.ScriptTimeoutSeconds+2) * time.Second
	if timeout < 5*time.Second {
		timeout = 5 * time.Second
	}
	dispatchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := s.dispatcher.Dispatch(dispatchCtx, event, settings); err != nil && s.logger != nil {
		s.logger.Warn("notification_dispatch_failed",
			logging.F("trigger", event.Trigger),
			logging.F("session_id", event.SessionID),
			logging.F("error", err),
		)
	}
}

func (s *NotificationService) shouldSuppress(event types.NotificationEvent, settings types.NotificationSettings) bool {
	if s == nil || s.dedupe == nil {
		return false
	}
	return s.dedupe.ShouldSuppress(event, settings)
}

type windowNotificationDedupePolicy struct {
	mu       sync.Mutex
	lastSent map[string]time.Time
}

func newWindowNotificationDedupePolicy() NotificationDedupePolicy {
	return &windowNotificationDedupePolicy{
		lastSent: map[string]time.Time{},
	}
}

func (p *windowNotificationDedupePolicy) ShouldSuppress(event types.NotificationEvent, settings types.NotificationSettings) bool {
	if p == nil {
		return false
	}
	window := time.Duration(settings.DedupeWindowSeconds) * time.Second
	if window <= 0 {
		return false
	}
	key := notificationDedupeKey(event)
	if strings.TrimSpace(key) == "" {
		return false
	}
	now := time.Now().UTC()

	p.mu.Lock()
	defer p.mu.Unlock()
	if then, ok := p.lastSent[key]; ok && now.Sub(then) < window {
		return true
	}
	p.lastSent[key] = now
	if len(p.lastSent) > 2048 {
		cutoff := now.Add(-2 * window)
		for candidate, ts := range p.lastSent {
			if ts.Before(cutoff) {
				delete(p.lastSent, candidate)
			}
		}
	}
	return false
}
