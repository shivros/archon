package daemon

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

type stubTurnHistoryFetcher struct {
	mu        sync.Mutex
	snapshots []openCodeAssistantSnapshot
	err       error
	calls     int
}

func (s *stubTurnHistoryFetcher) FetchLatestAssistant(context.Context) (openCodeAssistantSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return openCodeAssistantSnapshot{}, s.err
	}
	if len(s.snapshots) == 0 {
		return openCodeAssistantSnapshot{}, nil
	}
	if len(s.snapshots) == 1 {
		return s.snapshots[0], nil
	}
	next := s.snapshots[0]
	s.snapshots = s.snapshots[1:]
	return next, nil
}

type captureTurnPublisher struct {
	mu      sync.Mutex
	results []openCodeTerminalResult
}

func (c *captureTurnPublisher) PublishTurnTerminal(result openCodeTerminalResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results = append(c.results, result)
}

func (c *captureTurnPublisher) waitCount(n int, timeout time.Duration) []openCodeTerminalResult {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		if len(c.results) >= n {
			out := append([]openCodeTerminalResult(nil), c.results...)
			c.mu.Unlock()
			return out
		}
		c.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]openCodeTerminalResult(nil), c.results...)
}

func TestOpenCodeTurnLifecycleSynthesizesCompletionFromHistory(t *testing.T) {
	fetcher := &stubTurnHistoryFetcher{
		snapshots: []openCodeAssistantSnapshot{
			{MessageID: "msg-1", Text: "assistant done"},
		},
	}
	publisher := &captureTurnPublisher{}
	engine := newOpenCodeTurnLifecycleEngine(
		"sess-1",
		"opencode",
		fetcher,
		openCodeDefaultTurnStateResolver{abandonTimeout: 2 * time.Second},
		publisher,
		logging.Nop(),
		openCodeTurnLifecycleConfig{
			reconcileInterval: 20 * time.Millisecond,
			historyTimeout:    100 * time.Millisecond,
			abandonTimeout:    2 * time.Second,
		},
	)
	engine.RegisterTurn("turn-1", openCodeAssistantSnapshot{}, time.Now().UTC())
	engine.Start()
	defer engine.Close()

	results := publisher.waitCount(1, 500*time.Millisecond)
	if len(results) != 1 {
		t.Fatalf("expected one terminal result, got %d", len(results))
	}
	got := results[0]
	if got.Status != turnTerminalCompleted {
		t.Fatalf("expected completed, got %s", got.Status)
	}
	if got.Reason != "history_progression_detected" {
		t.Fatalf("expected history reason, got %q", got.Reason)
	}
	if got.Source != "history_reconcile" {
		t.Fatalf("expected history source, got %q", got.Source)
	}
}

func TestOpenCodeTurnLifecycleAbandonsStaleTurn(t *testing.T) {
	fetcher := &stubTurnHistoryFetcher{}
	publisher := &captureTurnPublisher{}
	engine := newOpenCodeTurnLifecycleEngine(
		"sess-2",
		"opencode",
		fetcher,
		openCodeDefaultTurnStateResolver{abandonTimeout: 60 * time.Millisecond},
		publisher,
		logging.Nop(),
		openCodeTurnLifecycleConfig{
			reconcileInterval: 15 * time.Millisecond,
			historyTimeout:    50 * time.Millisecond,
			abandonTimeout:    60 * time.Millisecond,
		},
	)
	engine.RegisterTurn("turn-timeout", openCodeAssistantSnapshot{}, time.Now().Add(-time.Second))
	engine.Start()
	defer engine.Close()

	results := publisher.waitCount(1, 500*time.Millisecond)
	if len(results) != 1 {
		t.Fatalf("expected one terminal result, got %d", len(results))
	}
	got := results[0]
	if got.Status != turnTerminalAbandoned {
		t.Fatalf("expected abandoned, got %s", got.Status)
	}
	if got.Reason != "history_stale_timeout" {
		t.Fatalf("expected stale timeout reason, got %q", got.Reason)
	}
}

func TestOpenCodeTurnLifecycleDedupesEventAfterHistoryTerminal(t *testing.T) {
	fetcher := &stubTurnHistoryFetcher{
		snapshots: []openCodeAssistantSnapshot{
			{MessageID: "msg-2", Text: "assistant"},
		},
	}
	publisher := &captureTurnPublisher{}
	engine := newOpenCodeTurnLifecycleEngine(
		"sess-3",
		"opencode",
		fetcher,
		openCodeDefaultTurnStateResolver{abandonTimeout: 3 * time.Second},
		publisher,
		logging.Nop(),
		openCodeTurnLifecycleConfig{
			reconcileInterval: 20 * time.Millisecond,
			historyTimeout:    50 * time.Millisecond,
			abandonTimeout:    3 * time.Second,
		},
	)
	engine.RegisterTurn("turn-dedupe", openCodeAssistantSnapshot{}, time.Now().UTC())
	engine.Start()
	defer engine.Close()

	results := publisher.waitCount(1, 500*time.Millisecond)
	if len(results) != 1 {
		t.Fatalf("expected one terminal result, got %d", len(results))
	}
	engine.ObserveEvent(types.CodexEvent{
		Method: "turn/completed",
		Params: []byte(`{"turn":{"id":"turn-dedupe","status":"completed"}}`),
	})
	time.Sleep(80 * time.Millisecond)
	results = publisher.waitCount(2, 120*time.Millisecond)
	if len(results) != 1 {
		t.Fatalf("expected exactly one terminalization, got %d", len(results))
	}
}

func TestOpenCodeTurnLifecycleEventTerminalizesImmediately(t *testing.T) {
	fetcher := &stubTurnHistoryFetcher{}
	publisher := &captureTurnPublisher{}
	engine := newOpenCodeTurnLifecycleEngine(
		"sess-4",
		"opencode",
		fetcher,
		openCodeDefaultTurnStateResolver{abandonTimeout: 2 * time.Second},
		publisher,
		logging.Nop(),
		openCodeTurnLifecycleConfig{
			reconcileInterval: 100 * time.Millisecond,
			historyTimeout:    50 * time.Millisecond,
			abandonTimeout:    2 * time.Second,
		},
	)
	engine.RegisterTurn("turn-event", openCodeAssistantSnapshot{}, time.Now().UTC())
	engine.ObserveEvent(types.CodexEvent{
		Method: "error",
		Params: []byte(`{"error":{"message":"provider timeout"}}`),
	})
	results := publisher.waitCount(1, 200*time.Millisecond)
	if len(results) != 1 {
		t.Fatalf("expected one terminal result, got %d", len(results))
	}
	if results[0].Status != turnTerminalFailed {
		t.Fatalf("expected failed terminal status, got %s", results[0].Status)
	}
	if results[0].Reason != "event_error" {
		t.Fatalf("expected event_error reason, got %q", results[0].Reason)
	}
}

func TestOpenCodeTurnStateResolverSupportsPluggableEventRule(t *testing.T) {
	resolver := openCodeDefaultTurnStateResolver{
		eventRules: []EventTerminalRule{
			customEventRule{},
		},
	}
	resolution, ok := resolver.ResolveEvent(types.CodexEvent{Method: "provider/custom"}, "turn-custom")
	if !ok {
		t.Fatalf("expected custom event rule to match")
	}
	if resolution.Reason != "custom_event_rule" {
		t.Fatalf("unexpected resolution reason: %q", resolution.Reason)
	}
}

func TestOpenCodeTurnStateResolverSupportsPluggableHistoryRule(t *testing.T) {
	resolver := openCodeDefaultTurnStateResolver{
		historyRules: []HistoryTerminalRule{
			customHistoryRule{},
		},
	}
	resolution, ok := resolver.ResolveHistory(time.Now().UTC(), openCodePendingTurn{TurnID: "turn-history"}, openCodeAssistantSnapshot{})
	if !ok {
		t.Fatalf("expected custom history rule to match")
	}
	if resolution.Reason != "custom_history_rule" {
		t.Fatalf("unexpected resolution reason: %q", resolution.Reason)
	}
}

func TestOpenCodeHistoryFetcherDependsOnAPIInterface(t *testing.T) {
	fetcher := openCodeHistoryFetcher{
		api:         stubOpenCodeHistoryAPI{messages: []openCodeSessionMessage{{Info: map[string]any{"role": "assistant", "id": "m1"}, Parts: []map[string]any{{"type": "text", "text": "done"}}}}},
		providerID:  "provider-session",
		historySize: 10,
	}
	snapshot, err := fetcher.FetchLatestAssistant(context.Background())
	if err != nil {
		t.Fatalf("FetchLatestAssistant: %v", err)
	}
	if snapshot.MessageID != "m1" || snapshot.Text != "done" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

type customEventRule struct{}

func (customEventRule) ApplyEvent(event types.CodexEvent, fallbackTurnID string) (openCodeTerminalResolution, bool) {
	if event.Method != "provider/custom" {
		return openCodeTerminalResolution{}, false
	}
	return openCodeTerminalResolution{
		TurnID: fallbackTurnID,
		Status: turnTerminalCompleted,
		Reason: "custom_event_rule",
	}, true
}

type customHistoryRule struct{}

func (customHistoryRule) ApplyHistory(_ time.Time, pending openCodePendingTurn, _ openCodeAssistantSnapshot) (openCodeTerminalResolution, bool) {
	if pending.TurnID == "" {
		return openCodeTerminalResolution{}, false
	}
	return openCodeTerminalResolution{
		TurnID: pending.TurnID,
		Status: turnTerminalCompleted,
		Reason: "custom_history_rule",
	}, true
}

type stubOpenCodeHistoryAPI struct {
	messages []openCodeSessionMessage
	err      error
}

func (s stubOpenCodeHistoryAPI) ListSessionMessages(context.Context, string, string, int) ([]openCodeSessionMessage, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]openCodeSessionMessage(nil), s.messages...), nil
}

func TestOpenCodeHistoryFetcherRequiresProviderID(t *testing.T) {
	fetcher := openCodeHistoryFetcher{api: stubOpenCodeHistoryAPI{}}
	_, err := fetcher.FetchLatestAssistant(context.Background())
	if err == nil {
		t.Fatalf("expected provider session id error")
	}
}

func TestOpenCodeHistoryFetcherRequiresAPI(t *testing.T) {
	fetcher := openCodeHistoryFetcher{providerID: "provider-session"}
	_, err := fetcher.FetchLatestAssistant(context.Background())
	if err == nil {
		t.Fatalf("expected non-nil API requirement error")
	}
}

func TestNewOpenCodeTurnLifecycleEngineAppliesDefaults(t *testing.T) {
	engine := newOpenCodeTurnLifecycleEngine("sess-defaults", "opencode", &stubTurnHistoryFetcher{}, nil, &captureTurnPublisher{}, nil, openCodeTurnLifecycleConfig{})
	if engine == nil {
		t.Fatalf("expected engine")
	}
	if engine.cfg.reconcileInterval <= 0 || engine.cfg.historyTimeout <= 0 || engine.cfg.abandonTimeout <= 0 {
		t.Fatalf("expected default config values, got %#v", engine.cfg)
	}
	if engine.resolver == nil {
		t.Fatalf("expected default resolver")
	}
}

func TestOpenCodeTurnLifecycleEngineCloseIsIdempotent(t *testing.T) {
	engine := newOpenCodeTurnLifecycleEngine("sess-close", "opencode", &stubTurnHistoryFetcher{}, nil, &captureTurnPublisher{}, logging.Nop(), openCodeTurnLifecycleConfig{})
	engine.Close()
	engine.Close()
}

func TestOpenCodeTurnLifecycleEngineRegisterAfterCloseNoop(t *testing.T) {
	engine := newOpenCodeTurnLifecycleEngine("sess-closed", "opencode", &stubTurnHistoryFetcher{}, nil, &captureTurnPublisher{}, logging.Nop(), openCodeTurnLifecycleConfig{})
	engine.Close()
	engine.RegisterTurn("turn-closed", openCodeAssistantSnapshot{}, time.Now().UTC())
	if got := engine.ActiveTurnID(); got != "" {
		t.Fatalf("expected no active turn after closed register, got %q", got)
	}
}

func TestOpenCodeTurnLifecycleEngineLogsDebugOnReconcileError(t *testing.T) {
	var out bytes.Buffer
	logger := logging.New(&out, logging.Debug)
	fetcher := &stubTurnHistoryFetcher{err: context.DeadlineExceeded}
	engine := newOpenCodeTurnLifecycleEngine(
		"sess-debug",
		"opencode",
		fetcher,
		openCodeDefaultTurnStateResolver{abandonTimeout: 3 * time.Second},
		&captureTurnPublisher{},
		logger,
		openCodeTurnLifecycleConfig{
			reconcileInterval: 10 * time.Millisecond,
			historyTimeout:    10 * time.Millisecond,
			abandonTimeout:    3 * time.Second,
		},
	)
	engine.RegisterTurn("turn-debug", openCodeAssistantSnapshot{}, time.Now().UTC())
	engine.reconcileOnce(time.Now().UTC())
	if out.String() == "" || !contains(out.String(), "opencode_turn_reconcile_attempt") {
		t.Fatalf("expected debug reconcile log, got %q", out.String())
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || bytes.Contains([]byte(haystack), []byte(needle)))
}
