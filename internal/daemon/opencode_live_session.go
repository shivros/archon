package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

type openCodeLiveSession struct {
	mu            sync.Mutex
	sessionID     string
	providerName  string
	providerID    string
	directory     string
	client        *openCodeClient
	connector     openCodeEventStreamConnector
	reconnect     openCodeEventReconnectPolicy
	logger        logging.Logger
	repository    TurnArtifactRepository
	events        <-chan types.CodexEvent
	cancelEvents  func()
	streamCtx     context.Context
	cancelStream  context.CancelFunc
	hub           *codexSubscriberHub
	turnNotifier  TurnCompletionNotifier
	approvalStore ApprovalStorage
	artifactSync  TurnArtifactSynchronizer
	payloads      TurnCompletionPayloadBuilder
	freshness     TurnEvidenceFreshnessTracker
	finalizer     openCodeTurnFinalizer
	activeTurn    string
	lastTurnID    string
	cancelTurn    context.CancelFunc
	lifecycle     *openCodeTurnLifecycleEngine
	retryAttempt  int
	closed        bool
}

var (
	_ LiveSession            = (*openCodeLiveSession)(nil)
	_ TurnCapableSession     = (*openCodeLiveSession)(nil)
	_ ApprovalCapableSession = (*openCodeLiveSession)(nil)
	_ NotifiableSession      = (*openCodeLiveSession)(nil)
)

func (s *openCodeLiveSession) Events() (<-chan types.CodexEvent, func()) {
	return s.hub.Add()
}

func (s *openCodeLiveSession) SessionID() string {
	return s.sessionID
}

func (s *openCodeLiveSession) ActiveTurnID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeTurn
}

func (s *openCodeLiveSession) SetSessionMeta(meta *types.SessionMeta) {
	if s == nil {
		return
	}
	lastTurnID := ""
	if meta != nil {
		lastTurnID = strings.TrimSpace(meta.LastTurnID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastTurnID = lastTurnID
}

func (s *openCodeLiveSession) StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error) {
	text := extractTextInput(input)
	if text == "" {
		return "", invalidError("text input is required", nil)
	}

	turnID := generateTurnID()
	baseline := s.fetchLatestAssistantSnapshot(ctx)
	startedAt := time.Now().UTC()
	turnCtx, cancelTurn := context.WithCancel(ctx)
	s.mu.Lock()
	s.activeTurn = turnID
	s.lastTurnID = turnID
	s.cancelTurn = cancelTurn
	s.mu.Unlock()
	if s.lifecycle != nil {
		s.lifecycle.RegisterTurn(turnID, baseline, startedAt)
	}

	s.persistItems([]map[string]any{
		{
			"type": "userMessage",
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
		},
	})

	err := s.client.PromptAsync(turnCtx, s.providerID, text, opts, s.directory)
	s.mu.Lock()
	if s.activeTurn == turnID {
		s.cancelTurn = nil
	}
	s.mu.Unlock()
	if err != nil {
		if errors.Is(err, errOpenCodePromptPending) {
			// The upstream may continue processing after client timeout; keep
			// the active turn open so completion can arrive through events/recovery.
			return turnID, nil
		}
		if s.lifecycle != nil {
			s.lifecycle.UnregisterTurn(turnID)
		}
		s.onTurnTerminal(openCodeTerminalResult{
			TurnID:         turnID,
			Status:         turnTerminalFailed,
			Error:          strings.TrimSpace(err.Error()),
			Reason:         "prompt_error",
			Source:         "prompt_send",
			StartedAt:      startedAt,
			TerminalizedAt: time.Now().UTC(),
		})
		return "", err
	}

	return turnID, nil
}

func (s *openCodeLiveSession) Interrupt(ctx context.Context) error {
	turnID := strings.TrimSpace(s.ActiveTurnID())
	if turnID == "" && s.lifecycle != nil {
		turnID = strings.TrimSpace(s.lifecycle.ActiveTurnID())
	}
	if turnID == "" {
		s.mu.Lock()
		turnID = strings.TrimSpace(s.lastTurnID)
		cancelTurn := s.cancelTurn
		s.mu.Unlock()
		if cancelTurn != nil {
			cancelTurn()
		}
	} else {
		s.mu.Lock()
		cancelTurn := s.cancelTurn
		s.mu.Unlock()
		if cancelTurn != nil {
			cancelTurn()
		}
	}
	if turnID != "" {
		if s.lifecycle != nil {
			s.lifecycle.UnregisterTurn(turnID)
		}
		now := time.Now().UTC()
		s.onTurnTerminal(openCodeTerminalResult{
			TurnID:         turnID,
			Status:         turnTerminalInterrupted,
			Error:          "turn interrupted",
			Reason:         "interrupt_requested",
			Source:         "interrupt_request",
			StartedAt:      now,
			TerminalizedAt: now,
		})
	}
	return s.client.AbortSession(ctx, s.providerID, s.directory)
}

func (s *openCodeLiveSession) Respond(ctx context.Context, requestID int, result map[string]any) error {
	if s.approvalStore == nil {
		return invalidError("approval store not available", nil)
	}
	record, ok, err := s.approvalStore.GetApproval(ctx, s.sessionID, requestID)
	if err != nil {
		return unavailableError(err.Error(), err)
	}
	if !ok || record == nil {
		return notFoundError("approval not found", nil)
	}
	params := map[string]any{}
	if len(record.Params) > 0 {
		_ = json.Unmarshal(record.Params, &params)
	}
	permissionID := strings.TrimSpace(asString(params["permission_id"]))
	if permissionID == "" {
		permissionID = strings.TrimSpace(asString(params["permissionID"]))
	}
	if permissionID == "" {
		return invalidError("provider permission id not available", nil)
	}
	decision := asString(result["decision"])
	var responses []string
	if raw, ok := result["responses"]; ok {
		if arr, ok := raw.([]string); ok {
			responses = arr
		}
	}
	if err := s.client.ReplyPermission(ctx, s.providerID, permissionID, decision, responses, s.directory); err != nil {
		return invalidError(err.Error(), err)
	}
	_ = s.approvalStore.DeleteApproval(ctx, s.sessionID, requestID)
	return nil
}

func (s *openCodeLiveSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	cancelEvents := s.cancelEvents
	s.cancelEvents = nil
	s.events = nil
	cancelStream := s.cancelStream
	s.cancelStream = nil
	s.streamCtx = nil
	lifecycle := s.lifecycle
	s.mu.Unlock()

	if cancelEvents != nil {
		cancelEvents()
	}
	if cancelStream != nil {
		cancelStream()
	}
	if lifecycle != nil {
		lifecycle.Close()
	}
}

func (s *openCodeLiveSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *openCodeLiveSession) SetNotificationPublisher(notifier NotificationPublisher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if aware, ok := s.turnNotifier.(TurnCompletionNotificationPublisherAware); ok {
		aware.SetNotificationPublisher(notifier)
	}
}

func (s *openCodeLiveSession) start() {
	if s.lifecycle != nil {
		s.lifecycle.Start()
	}
	go func() {
		for {
			if s.isClosed() {
				return
			}
			events := s.currentEvents()
			if events == nil {
				if !s.reconnectEventStream() && !s.waitForEventReconnectRetry() {
					return
				}
				continue
			}
			event, ok := <-events
			if !ok {
				if s.isClosed() {
					return
				}
				if !s.reconnectEventStream() && !s.waitForEventReconnectRetry() {
					return
				}
				continue
			}
			s.handleProviderEvent(event)
		}
	}()
}

func (s *openCodeLiveSession) currentEvents() <-chan types.CodexEvent {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.events
}

func (s *openCodeLiveSession) handleProviderEvent(event types.CodexEvent) {
	if s == nil {
		return
	}
	s.hub.Broadcast(event)
	s.persistEventItems(event)

	if s.lifecycle != nil {
		s.lifecycle.ObserveEvent(event)
	} else if event.Method == "turn/completed" || event.Method == "session.idle" || event.Method == "error" {
		turn := parseTurnEventFromParams(event.Params)
		turnID := turn.TurnID
		if turnID == "" {
			s.mu.Lock()
			turnID = s.activeTurn
			s.activeTurn = ""
			s.mu.Unlock()
			turn.TurnID = turnID
		} else {
			s.mu.Lock()
			s.activeTurn = ""
			s.mu.Unlock()
		}
		s.publishTurnCompleted(turn)
	}

	if isApprovalMethod(event.Method) && event.ID != nil {
		s.storeApproval(event)
	}
}

func (s *openCodeLiveSession) reconnectEventStream() bool {
	if s == nil {
		return false
	}
	connector := s.connectorOrDefault()
	if connector == nil {
		return false
	}
	providerID := strings.TrimSpace(s.providerID)
	if providerID == "" {
		return false
	}
	streamCtx := s.streamContext()
	if streamCtx == nil || s.isClosed() {
		return false
	}
	events, cancel, err := connector.SubscribeSessionEvents(streamCtx, providerID, s.directory)
	if err != nil && strings.TrimSpace(s.directory) != "" {
		events, cancel, err = connector.SubscribeSessionEvents(streamCtx, providerID, "")
	}
	if err != nil {
		attempt := s.markReconnectFailure()
		if s.logger != nil && s.reconnectPolicyOrDefault().ShouldLogFailure(attempt) {
			s.logger.Warn("opencode_event_stream_resubscribe_failed",
				logging.F("session_id", strings.TrimSpace(s.sessionID)),
				logging.F("provider", strings.TrimSpace(s.providerName)),
				logging.F("provider_session_id", providerID),
				logging.F("attempt", attempt),
				logging.F("error", err),
			)
		}
		return false
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return false
	}
	previousCancel := s.cancelEvents
	s.events = events
	s.cancelEvents = cancel
	s.retryAttempt = 0
	s.mu.Unlock()
	if previousCancel != nil {
		previousCancel()
	}
	if s.logger != nil {
		s.logger.Info("opencode_event_stream_resubscribed",
			logging.F("session_id", strings.TrimSpace(s.sessionID)),
			logging.F("provider", strings.TrimSpace(s.providerName)),
			logging.F("provider_session_id", providerID),
		)
	}
	return true
}

func (s *openCodeLiveSession) waitForEventReconnectRetry() bool {
	attempt := s.currentReconnectAttempt()
	delay := s.reconnectPolicyOrDefault().RetryDelay(attempt)
	if delay <= 0 {
		delay = 300 * time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	<-timer.C
	return !s.isClosed() && s.streamContext() != nil
}

func (s *openCodeLiveSession) streamContext() context.Context {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streamCtx
}

func (s *openCodeLiveSession) connectorOrDefault() openCodeEventStreamConnector {
	if s == nil {
		return nil
	}
	if s.connector != nil {
		return s.connector
	}
	return s.client
}

func (s *openCodeLiveSession) reconnectPolicyOrDefault() openCodeEventReconnectPolicy {
	if s == nil || s.reconnect == nil {
		return defaultOpenCodeEventReconnectPolicy{}
	}
	return s.reconnect
}

func (s *openCodeLiveSession) markReconnectFailure() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retryAttempt++
	return s.retryAttempt
}

func (s *openCodeLiveSession) currentReconnectAttempt() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retryAttempt
}

func (s *openCodeLiveSession) persistEventItems(event types.CodexEvent) {
	items := openCodeEventItems(event)
	if len(items) == 0 {
		return
	}
	s.persistItems(items)
}

func (s *openCodeLiveSession) persistItems(items []map[string]any) {
	if s == nil || s.repository == nil || len(items) == 0 {
		return
	}
	if err := s.repository.AppendItems(s.sessionID, items); err != nil {
		if s.logger != nil {
			s.logger.Warn("opencode_live_item_persist_failed",
				logging.F("session_id", strings.TrimSpace(s.sessionID)),
				logging.F("provider", strings.TrimSpace(s.providerName)),
				logging.F("items_count", len(items)),
				logging.F("error", err),
			)
		}
	}
}

func (s *openCodeLiveSession) fetchLatestAssistantSnapshot(ctx context.Context) openCodeAssistantSnapshot {
	if s == nil || s.client == nil || strings.TrimSpace(s.providerID) == "" {
		return openCodeAssistantSnapshot{}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
	defer cancel()
	messages, err := s.client.ListSessionMessages(callCtx, s.providerID, s.directory, 40)
	if err != nil && strings.TrimSpace(s.directory) != "" {
		messages, err = s.client.ListSessionMessages(callCtx, s.providerID, "", 40)
	}
	if err != nil {
		return openCodeAssistantSnapshot{}
	}
	return openCodeLatestAssistantSnapshot(messages)
}

func (s *openCodeLiveSession) onTurnTerminal(result openCodeTerminalResult) {
	if s == nil {
		return
	}
	turnID := strings.TrimSpace(result.TurnID)
	s.mu.Lock()
	if strings.TrimSpace(s.activeTurn) == turnID {
		s.activeTurn = ""
		s.cancelTurn = nil
	}
	s.mu.Unlock()

	payload := map[string]any{
		"terminalization_reason": strings.TrimSpace(result.Reason),
		"terminalization_source": strings.TrimSpace(result.Source),
		"turn_age_ms":            result.TerminalizedAt.Sub(result.StartedAt).Milliseconds(),
		"reconcile_attempts":     result.AttemptCount,
	}
	s.persistItems([]map[string]any{
		{
			"type":                   "turnCompletion",
			"turn_id":                turnID,
			"turn_status":            string(result.Status),
			"turn_error":             strings.TrimSpace(result.Error),
			"turn_output":            strings.TrimSpace(result.Output),
			"terminalization_reason": strings.TrimSpace(result.Reason),
			"terminalization_source": strings.TrimSpace(result.Source),
			"completed_at":           result.TerminalizedAt.Format(time.RFC3339Nano),
		},
	})
	turn := turnEventParams{
		TurnID: turnID,
		Status: string(result.Status),
		Error:  strings.TrimSpace(result.Error),
		Output: strings.TrimSpace(result.Output),
	}
	s.publishTurnCompletedWithPayload(turn, payload)

	if strings.TrimSpace(result.Source) != "live_event" && s.hub != nil {
		s.hub.Broadcast(types.CodexEvent{
			Method: "turn/completed",
			Params: encodeTurnCompletedEventParams(result),
			TS:     result.TerminalizedAt.Format(time.RFC3339Nano),
		})
	}
}

func openCodeEventItems(event types.CodexEvent) []map[string]any {
	switch strings.TrimSpace(strings.ToLower(event.Method)) {
	case "item/agentmessage/delta":
		var payload struct {
			Delta string `json:"delta"`
			Text  string `json:"text"`
		}
		_ = json.Unmarshal(event.Params, &payload)
		text := strings.TrimSpace(payload.Delta)
		if text == "" {
			text = strings.TrimSpace(payload.Text)
		}
		if text == "" {
			return nil
		}
		item := newAgentMessageDeltaItem(text)
		if item == nil {
			return nil
		}
		return []map[string]any{item}
	default:
		return nil
	}
}

func (s *openCodeLiveSession) publishTurnCompleted(turn turnEventParams) {
	s.publishTurnCompletedWithPayload(turn, nil)
}

func (s *openCodeLiveSession) publishTurnCompletedWithPayload(turn turnEventParams, additionalPayload map[string]any) {
	finalizer := s.turnFinalizerOrDefault()
	if finalizer == nil {
		return
	}
	finalizer.FinalizeTurn(turn, additionalPayload)
}

func (s *openCodeLiveSession) storeApproval(event types.CodexEvent) {
	if s.approvalStore == nil || event.ID == nil {
		return
	}
	_ = s.approvalStore.StoreApproval(context.Background(), s.sessionID, *event.ID, event.Method, event.Params)
}

type openCodeLiveSessionFactory struct {
	providerName  string
	turnNotifier  TurnCompletionNotifier
	approvalStore ApprovalStorage
	repository    TurnArtifactRepository
	payloads      TurnCompletionPayloadBuilder
	freshness     TurnEvidenceFreshnessTracker
	reconnect     openCodeEventReconnectPolicy
	logger        logging.Logger
}

type openCodeLiveSessionFactoryOption func(*openCodeLiveSessionFactory)

func withOpenCodeEventReconnectPolicy(policy openCodeEventReconnectPolicy) openCodeLiveSessionFactoryOption {
	return func(factory *openCodeLiveSessionFactory) {
		if factory == nil || policy == nil {
			return
		}
		factory.reconnect = policy
	}
}

func (f *openCodeLiveSessionFactory) ValidateLifecycleWiring() error {
	if f == nil {
		return errors.New("factory is required")
	}
	if f.turnNotifier == nil {
		return errors.New("turn notifier is not wired")
	}
	if f.repository == nil {
		return errors.New("turn artifact repository is not wired")
	}
	if f.payloads == nil {
		return errors.New("turn payload builder is not wired")
	}
	if f.freshness == nil {
		return errors.New("turn freshness tracker is not wired")
	}
	return nil
}

func newOpenCodeLiveSessionFactory(
	providerName string,
	turnNotifier TurnCompletionNotifier,
	approvalStore ApprovalStorage,
	repository TurnArtifactRepository,
	payloads TurnCompletionPayloadBuilder,
	freshness TurnEvidenceFreshnessTracker,
	logger logging.Logger,
	opts ...openCodeLiveSessionFactoryOption,
) *openCodeLiveSessionFactory {
	if logger == nil {
		logger = logging.Nop()
	}
	if turnNotifier == nil {
		turnNotifier = NopTurnCompletionNotifier{}
	}
	if approvalStore == nil {
		approvalStore = NopApprovalStorage{}
	}
	if repository == nil {
		repository = &fileSessionItemsRepository{}
	}
	if payloads == nil {
		payloads = defaultTurnCompletionPayloadBuilder{}
	}
	if freshness == nil {
		freshness = NewTurnEvidenceFreshnessTracker()
	}
	factory := &openCodeLiveSessionFactory{
		providerName:  providerName,
		turnNotifier:  turnNotifier,
		approvalStore: approvalStore,
		repository:    repository,
		payloads:      payloads,
		freshness:     freshness,
		reconnect:     defaultOpenCodeEventReconnectPolicy{},
		logger:        logger,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(factory)
		}
	}
	return factory
}

func (f *openCodeLiveSessionFactory) ProviderName() string {
	return f.providerName
}

func (f *openCodeLiveSessionFactory) CreateTurnCapable(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}

	providerID := ""
	if meta != nil {
		providerID = meta.ProviderSessionID
	}
	if providerID == "" {
		return nil, invalidError("provider session id not available", nil)
	}

	client, err := newOpenCodeClient(resolveOpenCodeClientConfig(f.providerName, loadCoreConfigOrDefault()))
	if err != nil {
		return nil, err
	}

	streamCtx, cancelStream := context.WithCancel(context.Background())
	events, cancel, err := client.SubscribeSessionEvents(streamCtx, providerID, session.Cwd)
	if err != nil && session.Cwd != "" {
		events, cancel, err = client.SubscribeSessionEvents(streamCtx, providerID, "")
	}
	if err != nil {
		cancelStream()
		return nil, err
	}

	ls := &openCodeLiveSession{
		sessionID:     session.ID,
		providerName:  f.providerName,
		providerID:    providerID,
		directory:     session.Cwd,
		client:        client,
		connector:     client,
		reconnect:     f.reconnect,
		logger:        f.logger,
		repository:    f.repository,
		events:        events,
		cancelEvents:  cancel,
		streamCtx:     streamCtx,
		cancelStream:  cancelStream,
		hub:           newCodexSubscriberHub(),
		turnNotifier:  f.turnNotifier,
		approvalStore: f.approvalStore,
		artifactSync: newOpenCodeTurnArtifactSynchronizer(
			session.ID,
			providerID,
			session.Cwd,
			openCodeTurnArtifactRemoteSource{client: client},
			f.repository,
		),
		payloads:  f.payloads,
		freshness: f.freshness,
	}
	ls.finalizer = &defaultOpenCodeTurnFinalizer{
		sessionID:    session.ID,
		providerName: f.providerName,
		notifier:     f.turnNotifier,
		artifactSync: ls.artifactSync,
		payloads:     f.payloads,
		freshness:    f.freshness,
	}
	ls.lifecycle = newOpenCodeTurnLifecycleEngine(
		session.ID,
		f.providerName,
		openCodeHistoryFetcher{
			api:         client,
			providerID:  providerID,
			directory:   session.Cwd,
			historySize: 40,
		},
		openCodeDefaultTurnStateResolver{abandonTimeout: defaultOpenCodeTurnAbandonTimeout},
		openCodeLiveTurnPublisher{session: ls},
		f.logger,
		openCodeTurnLifecycleConfig{
			reconcileInterval: defaultOpenCodeTurnReconcileInterval,
			historyTimeout:    defaultOpenCodeTurnHistoryTimeout,
			abandonTimeout:    defaultOpenCodeTurnAbandonTimeout,
		},
	)
	ls.start()

	return ls, nil
}

func generateTurnID() string {
	return "turn_" + logging.NewRequestID()
}

func (s *openCodeLiveSession) turnFinalizerOrDefault() openCodeTurnFinalizer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finalizer == nil {
		s.finalizer = &defaultOpenCodeTurnFinalizer{
			sessionID:    strings.TrimSpace(s.sessionID),
			providerName: strings.TrimSpace(s.providerName),
			notifier:     s.turnNotifier,
			artifactSync: s.artifactSync,
			payloads:     s.payloads,
			freshness:    s.freshness,
		}
	}
	return s.finalizer
}
