package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

const (
	defaultOpenCodeTurnReconcileInterval = 1200 * time.Millisecond
	defaultOpenCodeTurnHistoryTimeout    = 1500 * time.Millisecond
	defaultOpenCodeTurnAbandonTimeout    = 90 * time.Second
)

type turnTerminalState string

const (
	turnTerminalCompleted turnTerminalState = "completed"
	turnTerminalFailed    turnTerminalState = "failed"
	turnTerminalAbandoned turnTerminalState = "abandoned"
)

type EventSubscriber interface {
	ObserveEvent(event types.CodexEvent)
}

type HistoryFetcher interface {
	FetchLatestAssistant(ctx context.Context) (openCodeAssistantSnapshot, error)
}

type TurnStateResolver interface {
	ResolveEvent(event types.CodexEvent, fallbackTurnID string) (openCodeTerminalResolution, bool)
	ResolveHistory(now time.Time, pending openCodePendingTurn, latest openCodeAssistantSnapshot) (openCodeTerminalResolution, bool)
}

type TurnPublisher interface {
	PublishTurnTerminal(result openCodeTerminalResult)
}

type openCodeTerminalResolution struct {
	TurnID string
	Status turnTerminalState
	Error  string
	Output string
	Reason string
}

type openCodePendingTurn struct {
	TurnID       string
	StartedAt    time.Time
	Baseline     openCodeAssistantSnapshot
	LastAttempt  time.Time
	AttemptCount int
}

type openCodeTerminalResult struct {
	TurnID         string
	Status         turnTerminalState
	Error          string
	Output         string
	Reason         string
	Source         string
	StartedAt      time.Time
	TerminalizedAt time.Time
	AttemptCount   int
}

type openCodeDefaultTurnStateResolver struct {
	eventRules     []EventTerminalRule
	historyRules   []HistoryTerminalRule
	abandonTimeout time.Duration
}

type EventTerminalRule interface {
	ApplyEvent(event types.CodexEvent, fallbackTurnID string) (openCodeTerminalResolution, bool)
}

type HistoryTerminalRule interface {
	ApplyHistory(now time.Time, pending openCodePendingTurn, latest openCodeAssistantSnapshot) (openCodeTerminalResolution, bool)
}

func (r openCodeDefaultTurnStateResolver) ResolveEvent(event types.CodexEvent, fallbackTurnID string) (openCodeTerminalResolution, bool) {
	for _, rule := range r.eventRulesOrDefault() {
		if rule == nil {
			continue
		}
		if resolution, ok := rule.ApplyEvent(event, fallbackTurnID); ok {
			return resolution, true
		}
	}
	return openCodeTerminalResolution{}, false
}

func (r openCodeDefaultTurnStateResolver) ResolveHistory(now time.Time, pending openCodePendingTurn, latest openCodeAssistantSnapshot) (openCodeTerminalResolution, bool) {
	for _, rule := range r.historyRulesOrDefault() {
		if rule == nil {
			continue
		}
		if resolution, ok := rule.ApplyHistory(now, pending, latest); ok {
			return resolution, true
		}
	}
	return openCodeTerminalResolution{}, false
}

func (r openCodeDefaultTurnStateResolver) eventRulesOrDefault() []EventTerminalRule {
	if len(r.eventRules) > 0 {
		return r.eventRules
	}
	return []EventTerminalRule{openCodeEventMethodTerminalRule{}}
}

func (r openCodeDefaultTurnStateResolver) historyRulesOrDefault() []HistoryTerminalRule {
	if len(r.historyRules) > 0 {
		return r.historyRules
	}
	timeout := r.abandonTimeout
	if timeout <= 0 {
		timeout = defaultOpenCodeTurnAbandonTimeout
	}
	return []HistoryTerminalRule{
		openCodeHistoryProgressionRule{},
		openCodeHistoryTimeoutRule{timeout: timeout},
	}
}

type openCodeEventMethodTerminalRule struct{}

func (openCodeEventMethodTerminalRule) ApplyEvent(event types.CodexEvent, fallbackTurnID string) (openCodeTerminalResolution, bool) {
	method := strings.ToLower(strings.TrimSpace(event.Method))
	if method != "turn/completed" && method != "session.idle" && method != "error" {
		return openCodeTerminalResolution{}, false
	}
	parsed := parseTurnEventFromParams(event.Params)
	turnID := strings.TrimSpace(parsed.TurnID)
	if turnID == "" {
		turnID = strings.TrimSpace(fallbackTurnID)
	}
	if turnID == "" {
		return openCodeTerminalResolution{}, false
	}
	status := turnTerminalCompleted
	errorText := strings.TrimSpace(parsed.Error)
	if method == "error" {
		status = turnTerminalFailed
		if errorText == "" {
			errorText = "provider emitted error event"
		}
	}
	outcome := classifyTurnOutcome(parsed.Status, errorText)
	if outcome.Failed {
		status = turnTerminalFailed
	}
	if method == "session.idle" && !outcome.Failed {
		status = turnTerminalCompleted
	}
	if method == "turn/completed" && !outcome.Terminal && !outcome.Failed {
		status = turnTerminalCompleted
	}
	reason := "event_" + strings.ReplaceAll(method, "/", "_")
	return openCodeTerminalResolution{
		TurnID: turnID,
		Status: status,
		Error:  strings.TrimSpace(errorText),
		Output: strings.TrimSpace(parsed.Output),
		Reason: reason,
	}, true
}

type openCodeHistoryProgressionRule struct{}

func (openCodeHistoryProgressionRule) ApplyHistory(_ time.Time, pending openCodePendingTurn, latest openCodeAssistantSnapshot) (openCodeTerminalResolution, bool) {
	if strings.TrimSpace(pending.TurnID) == "" {
		return openCodeTerminalResolution{}, false
	}
	if !openCodeAssistantChanged(latest, pending.Baseline) {
		return openCodeTerminalResolution{}, false
	}
	return openCodeTerminalResolution{
		TurnID: pending.TurnID,
		Status: turnTerminalCompleted,
		Output: strings.TrimSpace(latest.Text),
		Reason: "history_progression_detected",
	}, true
}

type openCodeHistoryTimeoutRule struct {
	timeout time.Duration
}

func (r openCodeHistoryTimeoutRule) ApplyHistory(now time.Time, pending openCodePendingTurn, _ openCodeAssistantSnapshot) (openCodeTerminalResolution, bool) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(pending.TurnID) == "" {
		return openCodeTerminalResolution{}, false
	}
	timeout := r.timeout
	if timeout <= 0 {
		timeout = defaultOpenCodeTurnAbandonTimeout
	}
	if now.Sub(pending.StartedAt) < timeout {
		return openCodeTerminalResolution{}, false
	}
	return openCodeTerminalResolution{
		TurnID: pending.TurnID,
		Status: turnTerminalAbandoned,
		Error:  "upstream turn did not emit terminal event before timeout",
		Reason: "history_stale_timeout",
	}, true
}

type openCodeTurnLifecycleConfig struct {
	reconcileInterval time.Duration
	historyTimeout    time.Duration
	abandonTimeout    time.Duration
}

type openCodeTurnLifecycleEngine struct {
	mu       sync.Mutex
	session  string
	provider string
	logger   logging.Logger

	historyFetcher HistoryFetcher
	resolver       TurnStateResolver
	publisher      TurnPublisher

	cfg     openCodeTurnLifecycleConfig
	pending map[string]openCodePendingTurn
	order   []string
	closed  bool
	done    chan struct{}
}

func newOpenCodeTurnLifecycleEngine(
	sessionID string,
	provider string,
	historyFetcher HistoryFetcher,
	resolver TurnStateResolver,
	publisher TurnPublisher,
	logger logging.Logger,
	cfg openCodeTurnLifecycleConfig,
) *openCodeTurnLifecycleEngine {
	if logger == nil {
		logger = logging.Nop()
	}
	if cfg.reconcileInterval <= 0 {
		cfg.reconcileInterval = defaultOpenCodeTurnReconcileInterval
	}
	if cfg.historyTimeout <= 0 {
		cfg.historyTimeout = defaultOpenCodeTurnHistoryTimeout
	}
	if cfg.abandonTimeout <= 0 {
		cfg.abandonTimeout = defaultOpenCodeTurnAbandonTimeout
	}
	if resolver == nil {
		resolver = openCodeDefaultTurnStateResolver{abandonTimeout: cfg.abandonTimeout}
	}
	return &openCodeTurnLifecycleEngine{
		session:        strings.TrimSpace(sessionID),
		provider:       strings.TrimSpace(provider),
		logger:         logger,
		historyFetcher: historyFetcher,
		resolver:       resolver,
		publisher:      publisher,
		cfg:            cfg,
		pending:        map[string]openCodePendingTurn{},
		done:           make(chan struct{}),
	}
}

func (e *openCodeTurnLifecycleEngine) Start() {
	if e == nil {
		return
	}
	go e.run()
}

func (e *openCodeTurnLifecycleEngine) Close() {
	if e == nil {
		return
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	done := e.done
	e.mu.Unlock()
	close(done)
}

func (e *openCodeTurnLifecycleEngine) RegisterTurn(turnID string, baseline openCodeAssistantSnapshot, startedAt time.Time) {
	if e == nil {
		return
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	pending := openCodePendingTurn{
		TurnID:    turnID,
		StartedAt: startedAt,
		Baseline:  baseline,
	}
	e.pending[turnID] = pending
	e.order = append(e.order, turnID)
}

func (e *openCodeTurnLifecycleEngine) ActiveTurnID() string {
	if e == nil {
		return ""
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, turnID := range e.order {
		if _, ok := e.pending[turnID]; ok {
			return turnID
		}
	}
	return ""
}

func (e *openCodeTurnLifecycleEngine) ObserveEvent(event types.CodexEvent) {
	if e == nil {
		return
	}
	fallback := e.ActiveTurnID()
	resolution, ok := e.resolver.ResolveEvent(event, fallback)
	if !ok {
		return
	}
	result := e.resolveTerminal(resolution, "live_event")
	if result == nil {
		return
	}
	e.publish(*result)
}

func (e *openCodeTurnLifecycleEngine) run() {
	ticker := time.NewTicker(e.cfg.reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.done:
			return
		case <-ticker.C:
			e.reconcileOnce(time.Now().UTC())
		}
	}
}

func (e *openCodeTurnLifecycleEngine) reconcileOnce(now time.Time) {
	pending := e.snapshotPending()
	if len(pending) == 0 {
		return
	}
	for _, entry := range pending {
		callCtx, cancel := context.WithTimeout(context.Background(), e.cfg.historyTimeout)
		latest, err := e.historyFetcher.FetchLatestAssistant(callCtx)
		cancel()

		e.markAttempt(entry.TurnID, now)
		if err != nil {
			e.logDebug("opencode_turn_reconcile_attempt",
				logging.F("session_id", e.session),
				logging.F("provider", e.provider),
				logging.F("turn_id", entry.TurnID),
				logging.F("pending_turn_age_ms", now.Sub(entry.StartedAt).Milliseconds()),
				logging.F("reconcile_attempt", entry.AttemptCount+1),
				logging.F("error", err),
			)
			latest = openCodeAssistantSnapshot{}
		}

		resolution, ok := e.resolver.ResolveHistory(now, entry, latest)
		if !ok {
			continue
		}
		result := e.resolveTerminal(resolution, "history_reconcile")
		if result == nil {
			continue
		}
		e.publish(*result)
	}
}

func (e *openCodeTurnLifecycleEngine) snapshotPending() []openCodePendingTurn {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || len(e.pending) == 0 {
		return nil
	}
	out := make([]openCodePendingTurn, 0, len(e.pending))
	for _, turnID := range e.order {
		entry, ok := e.pending[turnID]
		if !ok {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func (e *openCodeTurnLifecycleEngine) markAttempt(turnID string, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	entry, ok := e.pending[strings.TrimSpace(turnID)]
	if !ok {
		return
	}
	entry.LastAttempt = now
	entry.AttemptCount++
	e.pending[entry.TurnID] = entry
}

func (e *openCodeTurnLifecycleEngine) resolveTerminal(resolution openCodeTerminalResolution, source string) *openCodeTerminalResult {
	turnID := strings.TrimSpace(resolution.TurnID)
	if turnID == "" {
		return nil
	}
	e.mu.Lock()
	entry, ok := e.pending[turnID]
	if !ok {
		e.mu.Unlock()
		return nil
	}
	delete(e.pending, turnID)
	terminalizedAt := time.Now().UTC()
	reason := strings.TrimSpace(resolution.Reason)
	if reason == "" {
		reason = "unknown"
	}
	result := openCodeTerminalResult{
		TurnID:         turnID,
		Status:         resolution.Status,
		Error:          strings.TrimSpace(resolution.Error),
		Output:         strings.TrimSpace(resolution.Output),
		Reason:         reason,
		Source:         strings.TrimSpace(source),
		StartedAt:      entry.StartedAt,
		TerminalizedAt: terminalizedAt,
		AttemptCount:   entry.AttemptCount,
	}
	e.mu.Unlock()
	return &result
}

func (e *openCodeTurnLifecycleEngine) publish(result openCodeTerminalResult) {
	e.logInfo("opencode_turn_terminalized",
		logging.F("session_id", e.session),
		logging.F("provider", e.provider),
		logging.F("turn_id", result.TurnID),
		logging.F("terminal_state", result.Status),
		logging.F("terminalization_reason", result.Reason),
		logging.F("source", result.Source),
		logging.F("pending_turn_age_ms", result.TerminalizedAt.Sub(result.StartedAt).Milliseconds()),
		logging.F("reconcile_attempts", result.AttemptCount),
	)
	if e.publisher != nil {
		e.publisher.PublishTurnTerminal(result)
	}
}

func (e *openCodeTurnLifecycleEngine) logInfo(message string, fields ...logging.Field) {
	if e == nil || e.logger == nil {
		return
	}
	e.logger.Info(message, fields...)
}

func (e *openCodeTurnLifecycleEngine) logDebug(message string, fields ...logging.Field) {
	if e == nil || e.logger == nil || !e.logger.Enabled(logging.Debug) {
		return
	}
	e.logger.Debug(message, fields...)
}

type openCodeHistoryFetcher struct {
	api         OpenCodeHistoryAPI
	providerID  string
	directory   string
	historySize int
}

type OpenCodeHistoryAPI interface {
	ListSessionMessages(ctx context.Context, sessionID, directory string, limit int) ([]openCodeSessionMessage, error)
}

func (f openCodeHistoryFetcher) FetchLatestAssistant(ctx context.Context) (openCodeAssistantSnapshot, error) {
	if f.api == nil {
		return openCodeAssistantSnapshot{}, fmt.Errorf("history fetch client is required")
	}
	if strings.TrimSpace(f.providerID) == "" {
		return openCodeAssistantSnapshot{}, fmt.Errorf("provider session id is required")
	}
	limit := f.historySize
	if limit <= 0 {
		limit = 40
	}
	messages, err := f.api.ListSessionMessages(ctx, f.providerID, f.directory, limit)
	if err != nil && strings.TrimSpace(f.directory) != "" {
		messages, err = f.api.ListSessionMessages(ctx, f.providerID, "", limit)
	}
	if err != nil {
		return openCodeAssistantSnapshot{}, err
	}
	return openCodeLatestAssistantSnapshot(messages), nil
}

type openCodeLiveTurnPublisher struct {
	session *openCodeLiveSession
}

func (p openCodeLiveTurnPublisher) PublishTurnTerminal(result openCodeTerminalResult) {
	if p.session == nil {
		return
	}
	p.session.onTurnTerminal(result)
}

func encodeTurnCompletedEventParams(result openCodeTerminalResult) json.RawMessage {
	payload := map[string]any{
		"turn": map[string]any{
			"id":     strings.TrimSpace(result.TurnID),
			"status": strings.TrimSpace(string(result.Status)),
		},
		"status": strings.TrimSpace(string(result.Status)),
	}
	if errMsg := strings.TrimSpace(result.Error); errMsg != "" {
		payload["turn"].(map[string]any)["error"] = map[string]any{"message": errMsg}
		payload["error"] = map[string]any{"message": errMsg}
	}
	if output := strings.TrimSpace(result.Output); output != "" {
		payload["turn"].(map[string]any)["output"] = output
		payload["output"] = output
	}
	raw, _ := json.Marshal(payload)
	return raw
}
