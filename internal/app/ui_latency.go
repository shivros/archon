package app

import (
	"strings"
	"sync"
	"time"
)

const (
	uiLatencySpanModelUpdate = "ui.model.update"
	uiLatencySpanModelView   = "ui.model.view"

	uiLatencyActionToggleSessionsSidebar = "ui.action.toggle_sessions_sidebar"
	uiLatencyActionToggleNotesSidebar    = "ui.action.toggle_notes_sidebar"
	uiLatencyActionExitCompose           = "ui.action.exit_compose"
	uiLatencyActionOpenNewSession        = "ui.action.open_new_session"
	uiLatencyActionSwitchSession         = "ui.action.switch_session"
)

const (
	uiLatencyOutcomeOK         = "ok"
	uiLatencyOutcomeSuperseded = "superseded"
	uiLatencyOutcomeError      = "error"
	uiLatencyOutcomeCacheHit   = "cache_hit"
	uiLatencyOutcomeValidation = "validation_failed"
	uiLatencyOutcomeCanceled   = "canceled"
)

type UILatencyCategory string

const (
	UILatencyCategorySpan   UILatencyCategory = "span"
	UILatencyCategoryAction UILatencyCategory = "action"
)

type UILatencyMetric struct {
	Name      string
	Category  UILatencyCategory
	Token     string
	Outcome   string
	StartedAt time.Time
	EndedAt   time.Time
	Duration  time.Duration
}

type UILatencySink interface {
	RecordUILatency(metric UILatencyMetric)
}

type ModelOption func(*Model)

func WithUILatencySink(sink UILatencySink) ModelOption {
	return func(m *Model) {
		if m == nil || m.uiLatency == nil {
			return
		}
		m.uiLatency.setSink(sink)
	}
}

type uiLatencyTracker struct {
	mu      sync.Mutex
	sink    UILatencySink
	nowFn   func() time.Time
	active  map[string]uiLatencyActionState
	enabled bool
}

type uiLatencyActionState struct {
	name      string
	token     string
	startedAt time.Time
}

type noopUILatencySink struct{}

func (noopUILatencySink) RecordUILatency(UILatencyMetric) {}

func newUILatencyTracker(sink UILatencySink) *uiLatencyTracker {
	enabled := sink != nil
	if sink == nil {
		sink = noopUILatencySink{}
	}
	return &uiLatencyTracker{
		sink:    sink,
		nowFn:   time.Now,
		active:  map[string]uiLatencyActionState{},
		enabled: enabled,
	}
}

func (t *uiLatencyTracker) setSink(sink UILatencySink) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if sink == nil {
		t.sink = noopUILatencySink{}
		t.enabled = false
		t.active = map[string]uiLatencyActionState{}
		return
	}
	t.sink = sink
	t.enabled = true
}

func (t *uiLatencyTracker) now() time.Time {
	if t == nil || t.nowFn == nil {
		return time.Now()
	}
	return t.nowFn()
}

func (t *uiLatencyTracker) recordSpan(name string, startedAt time.Time) {
	if t == nil || !t.enabled {
		return
	}
	endedAt := t.now()
	t.record(UILatencyMetric{
		Name:      name,
		Category:  UILatencyCategorySpan,
		StartedAt: startedAt,
		EndedAt:   endedAt,
		Duration:  endedAt.Sub(startedAt),
	})
}

func (t *uiLatencyTracker) startAction(name, token string) {
	if t == nil || !t.enabled {
		return
	}
	key := uiLatencyActionKey(name)
	if key == "" {
		return
	}
	token = strings.TrimSpace(token)
	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()
	if existing, ok := t.active[key]; ok {
		t.recordLocked(UILatencyMetric{
			Name:      existing.name,
			Category:  UILatencyCategoryAction,
			Token:     existing.token,
			Outcome:   uiLatencyOutcomeSuperseded,
			StartedAt: existing.startedAt,
			EndedAt:   now,
			Duration:  now.Sub(existing.startedAt),
		})
	}
	t.active[key] = uiLatencyActionState{name: key, token: token, startedAt: now}
}

func (t *uiLatencyTracker) finishAction(name, token, outcome string) {
	if t == nil || !t.enabled {
		return
	}
	key := uiLatencyActionKey(name)
	if key == "" {
		return
	}
	token = strings.TrimSpace(token)
	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.active[key]
	if !ok {
		return
	}
	if token != "" && token != state.token {
		return
	}
	delete(t.active, key)
	t.recordLocked(UILatencyMetric{
		Name:      state.name,
		Category:  UILatencyCategoryAction,
		Token:     state.token,
		Outcome:   strings.TrimSpace(outcome),
		StartedAt: state.startedAt,
		EndedAt:   now,
		Duration:  now.Sub(state.startedAt),
	})
}

func (t *uiLatencyTracker) cancelAction(name, token string) {
	t.finishAction(name, token, uiLatencyOutcomeCanceled)
}

func (t *uiLatencyTracker) record(metric UILatencyMetric) {
	if t == nil || !t.enabled {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recordLocked(metric)
}

func (t *uiLatencyTracker) recordLocked(metric UILatencyMetric) {
	if t == nil || t.sink == nil {
		return
	}
	t.sink.RecordUILatency(metric)
}

func uiLatencyActionKey(name string) string {
	name = strings.TrimSpace(name)
	return name
}

func (m *Model) startUILatencyAction(name, token string) {
	if m == nil || m.uiLatency == nil {
		return
	}
	m.uiLatency.startAction(name, token)
}

func (m *Model) finishUILatencyAction(name, token, outcome string) {
	if m == nil || m.uiLatency == nil {
		return
	}
	m.uiLatency.finishAction(name, token, outcome)
}

func (m *Model) cancelUILatencyAction(name, token string) {
	if m == nil || m.uiLatency == nil {
		return
	}
	m.uiLatency.cancelAction(name, token)
}

func (m *Model) recordUILatencySpan(name string, startedAt time.Time) {
	if m == nil || m.uiLatency == nil {
		return
	}
	m.uiLatency.recordSpan(name, startedAt)
}

type InMemoryUILatencySink struct {
	mu      sync.Mutex
	metrics []UILatencyMetric
}

func NewInMemoryUILatencySink() *InMemoryUILatencySink {
	return &InMemoryUILatencySink{metrics: []UILatencyMetric{}}
}

func (s *InMemoryUILatencySink) RecordUILatency(metric UILatencyMetric) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = append(s.metrics, metric)
}

func (s *InMemoryUILatencySink) Snapshot() []UILatencyMetric {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]UILatencyMetric, len(s.metrics))
	copy(out, s.metrics)
	return out
}
