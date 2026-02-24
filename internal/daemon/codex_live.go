package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

type CodexLiveManager struct {
	mu        sync.Mutex
	sessions  map[string]*codexLiveSession
	stores    *Stores
	logger    logging.Logger
	notifier  NotificationPublisher
	turnProbe turnActivityProbe
}

func NewCodexLiveManager(stores *Stores, logger logging.Logger) *CodexLiveManager {
	if logger == nil {
		logger = logging.Nop()
	}
	return &CodexLiveManager{
		sessions: make(map[string]*codexLiveSession),
		stores:   stores,
		logger:   logger,
		turnProbe: codexThreadTurnActivityProbe{
			timeout: defaultTurnActivityProbeTimeout,
		},
	}
}

func (m *CodexLiveManager) SetNotificationPublisher(notifier NotificationPublisher) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifier = notifier
}

func (m *CodexLiveManager) StartTurn(
	ctx context.Context,
	session *types.Session,
	meta *types.SessionMeta,
	codexHome string,
	input []map[string]any,
	runtimePatch *types.SessionRuntimeOptions,
) (string, error) {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return "", errors.New("session is required")
	}
	if session.Provider != "codex" {
		return "", errors.New("provider does not support live events")
	}
	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		latestMeta := m.refreshSessionMeta(session.ID, meta)
		if runtimePatch != nil {
			if latestMeta != nil {
				metaCopy := *latestMeta
				latestMeta = &metaCopy
			} else {
				latestMeta = &types.SessionMeta{SessionID: session.ID}
			}
			latestMeta.RuntimeOptions = types.MergeRuntimeOptions(latestMeta.RuntimeOptions, runtimePatch)
		}
		runtimeOptions, model := codexRuntimeConfig(latestMeta)

		ls, err := m.ensure(ctx, session, latestMeta, codexHome)
		if err != nil {
			lastErr = err
			if !isClosedPipeError(lastErr) {
				m.logger.Error("codex_live_ensure_error", logging.F("session_id", session.ID), logging.F("error", err))
				return "", err
			}
			m.dropSession(session.ID)
			if attempt < maxAttempts {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(time.Duration(attempt*100) * time.Millisecond):
				}
				continue
			}
			break
		}
		startTurn := func() (string, error) {
			ls.mu.Lock()
			threadID := ls.threadID
			ls.mu.Unlock()
			return ls.client.StartTurn(ctx, threadID, input, runtimeOptions, model)
		}
		turnID, err := reserveSessionTurn(ctx, ls, m.turnProbe, startTurn)
		if err == nil {
			m.logger.Info("codex_turn_started", logging.F("session_id", session.ID), logging.F("thread_id", ls.threadID), logging.F("turn_id", turnID))
			return turnID, nil
		}
		lastErr = err
		if !isClosedPipeError(lastErr) {
			m.logger.Error("codex_live_start_error", logging.F("session_id", session.ID), logging.F("error", lastErr))
			return "", lastErr
		}
		m.dropSession(session.ID)
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt*100) * time.Millisecond):
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("codex turn start failed")
	}
	m.logger.Error("codex_live_start_error", logging.F("session_id", session.ID), logging.F("error", lastErr))
	return "", lastErr
}

func (m *CodexLiveManager) Subscribe(session *types.Session, meta *types.SessionMeta, codexHome string) (<-chan types.CodexEvent, func(), error) {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return nil, nil, errors.New("session is required")
	}
	if session.Provider != "codex" {
		return nil, nil, errors.New("provider does not support live events")
	}
	ls, err := m.ensure(context.Background(), session, meta, codexHome)
	if err != nil {
		m.logger.Error("codex_live_subscribe_error", logging.F("session_id", session.ID), logging.F("error", err))
		return nil, nil, err
	}
	m.logger.Debug("codex_live_subscribed", logging.F("session_id", session.ID), logging.F("thread_id", ls.threadID))
	ch, cancel := ls.hub.Add()
	wrappedCancel := func() {
		cancel()
		ls.maybeClose()
	}
	return ch, wrappedCancel, nil
}

func (m *CodexLiveManager) Respond(ctx context.Context, session *types.Session, meta *types.SessionMeta, codexHome string, requestID int, result map[string]any) error {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return errors.New("session is required")
	}
	if session.Provider != "codex" {
		return errors.New("provider does not support approvals")
	}
	if requestID < 0 {
		return errors.New("request id is required")
	}
	ls, err := m.ensure(ctx, session, meta, codexHome)
	if err != nil {
		return err
	}
	if err := ls.client.respond(requestID, result); err != nil {
		m.logger.Error("codex_live_respond_error", logging.F("session_id", session.ID), logging.F("request_id", requestID), logging.F("error", err))
		return err
	}
	ls.mu.Lock()
	ls.lastActive = time.Now().UTC()
	ls.mu.Unlock()
	return nil
}

func (m *CodexLiveManager) Interrupt(ctx context.Context, session *types.Session, meta *types.SessionMeta, codexHome string) error {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return errors.New("session is required")
	}
	if session.Provider != "codex" {
		return errors.New("provider does not support live events")
	}
	ls, err := m.ensure(ctx, session, meta, codexHome)
	if err != nil {
		return err
	}
	ls.mu.Lock()
	turnID := ls.activeTurn
	ls.mu.Unlock()
	if turnID == "" && meta != nil {
		turnID = meta.LastTurnID
	}
	if turnID == "" {
		return errors.New("no active turn")
	}
	if err := ls.client.InterruptTurn(ctx, ls.threadID, turnID); err != nil {
		m.logger.Error("codex_live_interrupt_error", logging.F("session_id", session.ID), logging.F("turn_id", turnID), logging.F("error", err))
		return err
	}
	ls.mu.Lock()
	ls.activeTurn = ""
	ls.lastActive = time.Now().UTC()
	ls.mu.Unlock()
	m.logger.Info("codex_turn_interrupted", logging.F("session_id", session.ID), logging.F("turn_id", turnID))
	return nil
}

func (m *CodexLiveManager) ensure(ctx context.Context, session *types.Session, meta *types.SessionMeta, codexHome string) (*codexLiveSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.Lock()
	ls := m.sessions[session.ID]
	if ls != nil && ls.isClosed() {
		delete(m.sessions, session.ID)
		ls = nil
	}
	if ls != nil {
		m.mu.Unlock()
		return ls, nil
	}
	m.mu.Unlock()

	latestMeta := m.refreshSessionMeta(session.ID, meta)
	runtimeOptions, model := codexRuntimeConfig(latestMeta)
	client, err := startCodexAppServer(ctx, session.Cwd, codexHome, m.logger)
	if err != nil {
		m.logger.Error("codex_start_error", logging.F("session_id", session.ID), logging.F("error", err))
		return nil, err
	}
	threadID := strings.TrimSpace(resolveThreadID(session, latestMeta))
	if threadID == "" {
		client.Close()
		return nil, errors.New("thread id not available")
	}
	if err := client.ResumeThread(ctx, threadID); err != nil {
		if !isCodexMissingThreadError(err) {
			m.logger.Error("codex_resume_error", logging.F("session_id", session.ID), logging.F("thread_id", threadID), logging.F("error", err))
			client.Close()
			return nil, err
		}
		m.logger.Warn("codex_resume_missing_thread",
			logging.F("session_id", session.ID),
			logging.F("thread_id", threadID),
			logging.F("error", err),
		)
		if !shouldBootstrapMissingThread(session, latestMeta) {
			client.Close()
			return nil, err
		}
		recoveredThreadID, startErr := client.StartThread(ctx, model, session.Cwd, runtimeOptions)
		if startErr != nil {
			client.Close()
			return nil, startErr
		}
		threadID = strings.TrimSpace(recoveredThreadID)
		if threadID == "" {
			client.Close()
			return nil, errors.New("thread id not available")
		}
		m.logger.Info("codex_missing_thread_bootstrap_recovered",
			logging.F("session_id", session.ID),
			logging.F("thread_id", threadID),
		)
		m.persistSessionThreadID(session.ID, threadID)
	}
	if latestMeta == nil || strings.TrimSpace(latestMeta.ThreadID) == "" {
		m.persistSessionThreadID(session.ID, threadID)
	}

	ls = &codexLiveSession{
		sessionID: session.ID,
		threadID:  threadID,
		client:    client,
		hub:       newCodexSubscriberHub(),
		stores:    m.stores,
		notifier:  m.notifier,
	}
	ls.start()

	m.mu.Lock()
	m.sessions[session.ID] = ls
	m.mu.Unlock()
	return ls, nil
}

func codexRuntimeConfig(meta *types.SessionMeta) (*types.SessionRuntimeOptions, string) {
	if meta == nil || meta.RuntimeOptions == nil {
		return nil, loadCoreConfigOrDefault().CodexDefaultModel()
	}
	runtimeOptions := types.CloneRuntimeOptions(meta.RuntimeOptions)
	model := strings.TrimSpace(runtimeOptions.Model)
	if model == "" {
		model = loadCoreConfigOrDefault().CodexDefaultModel()
	}
	return runtimeOptions, model
}

func (m *CodexLiveManager) refreshSessionMeta(sessionID string, fallback *types.SessionMeta) *types.SessionMeta {
	if m == nil || m.stores == nil || m.stores.SessionMeta == nil {
		return fallback
	}
	meta, ok, err := m.stores.SessionMeta.Get(context.Background(), sessionID)
	if err != nil || !ok || meta == nil {
		return fallback
	}
	return meta
}

func (m *CodexLiveManager) persistSessionThreadID(sessionID, threadID string) {
	if m == nil || m.stores == nil || m.stores.SessionMeta == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	threadID = strings.TrimSpace(threadID)
	if sessionID == "" || threadID == "" {
		return
	}
	now := time.Now().UTC()
	_, _ = m.stores.SessionMeta.Upsert(context.Background(), &types.SessionMeta{
		SessionID:    sessionID,
		ThreadID:     threadID,
		LastActiveAt: &now,
	})
}

func (s *codexLiveSession) StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error) {
	s.mu.Lock()
	threadID := s.threadID
	s.mu.Unlock()
	return s.client.StartTurn(ctx, threadID, input, opts, "")
}

func (s *codexLiveSession) Interrupt(ctx context.Context) error {
	s.mu.Lock()
	turnID := s.activeTurn
	threadID := s.threadID
	s.mu.Unlock()
	if turnID == "" {
		return errors.New("no active turn")
	}
	return s.client.InterruptTurn(ctx, threadID, turnID)
}

func (s *codexLiveSession) Respond(ctx context.Context, requestID int, result map[string]any) error {
	if err := s.client.respond(requestID, result); err != nil {
		return err
	}
	s.mu.Lock()
	s.lastActive = time.Now().UTC()
	s.mu.Unlock()
	return nil
}

type codexLiveSession struct {
	mu         sync.Mutex
	sessionID  string
	threadID   string
	client     *codexAppServer
	hub        *codexSubscriberHub
	stores     *Stores
	notifier   NotificationPublisher
	activeTurn string
	starting   bool
	lastActive time.Time
	closed     bool
}

var (
	_ LiveSession            = (*codexLiveSession)(nil)
	_ TurnCapableSession     = (*codexLiveSession)(nil)
	_ ApprovalCapableSession = (*codexLiveSession)(nil)
	_ NotifiableSession      = (*codexLiveSession)(nil)
)

func (s *codexLiveSession) Events() (<-chan types.CodexEvent, func()) {
	return s.hub.Add()
}

func (s *codexLiveSession) SessionID() string {
	return s.sessionID
}

func (s *codexLiveSession) ActiveTurnID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeTurn
}

func (s *codexLiveSession) Close() {
	s.close()
}

func (s *codexLiveSession) SetNotificationPublisher(notifier NotificationPublisher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifier = notifier
}

func (s *codexLiveSession) start() {
	go func() {
		notes := s.client.Notifications()
		reqs := s.client.Requests()
		errs := s.client.Errors()
		for {
			select {
			case err, ok := <-errs:
				if !ok || err != nil {
					s.close()
					return
				}
			case msg, ok := <-notes:
				if !ok {
					s.close()
					return
				}
				s.handleNote(msg)
			case msg, ok := <-reqs:
				if !ok {
					s.close()
					return
				}
				s.handleRequest(msg)
			}
		}
	}()
}

func (s *codexLiveSession) handleNote(msg rpcMessage) {
	event := types.CodexEvent{
		Method: msg.Method,
		Params: msg.Params,
		TS:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	if s.client != nil && s.client.logger != nil && s.client.logger.Enabled(logging.Debug) {
		if msg.Method == "error" || msg.Method == "codex/event/error" {
			s.client.logger.Debug("codex_event_error",
				logging.F("session_id", s.sessionID),
				logging.F("params", string(msg.Params)),
			)
		}
	}
	s.hub.Broadcast(event)
	if msg.Method == "turn/completed" {
		var payload struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		}
		if len(msg.Params) > 0 && json.Unmarshal(msg.Params, &payload) == nil {
			s.mu.Lock()
			if s.activeTurn == "" || payload.Turn.ID == s.activeTurn {
				s.activeTurn = ""
			}
			s.mu.Unlock()
		} else {
			s.mu.Lock()
			s.activeTurn = ""
			s.mu.Unlock()
		}
		s.publishTurnCompleted(parseTurnIDFromEventParams(msg.Params))
		s.maybeClose()
	}
}

func (s *codexLiveSession) publishTurnCompleted(turnID string) {
	if s == nil || s.notifier == nil {
		return
	}
	event := types.NotificationEvent{
		Trigger:    types.NotificationTriggerTurnCompleted,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  strings.TrimSpace(s.sessionID),
		TurnID:     strings.TrimSpace(turnID),
		Source:     "codex_live_event",
	}
	if s.stores != nil && s.stores.Sessions != nil {
		if record, ok, err := s.stores.Sessions.GetRecord(context.Background(), s.sessionID); err == nil && ok && record != nil && record.Session != nil {
			event.Provider = strings.TrimSpace(record.Session.Provider)
			event.Title = strings.TrimSpace(record.Session.Title)
			event.Cwd = strings.TrimSpace(record.Session.Cwd)
			event.Status = strings.TrimSpace(string(record.Session.Status))
		}
	}
	if s.stores != nil && s.stores.SessionMeta != nil {
		if meta, ok, err := s.stores.SessionMeta.Get(context.Background(), s.sessionID); err == nil && ok && meta != nil {
			event.WorkspaceID = strings.TrimSpace(meta.WorkspaceID)
			event.WorktreeID = strings.TrimSpace(meta.WorktreeID)
		}
	}
	s.notifier.Publish(event)
}

func (s *codexLiveSession) handleRequest(msg rpcMessage) {
	event := types.CodexEvent{
		ID:     msg.ID,
		Method: msg.Method,
		Params: msg.Params,
		TS:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	s.hub.Broadcast(event)
	if msg.ID != nil && s.stores != nil && s.stores.Approvals != nil && isApprovalMethod(msg.Method) {
		approval := &types.Approval{
			SessionID: s.sessionID,
			RequestID: *msg.ID,
			Method:    msg.Method,
			Params:    msg.Params,
			CreatedAt: time.Now().UTC(),
		}
		_, _ = s.stores.Approvals.Upsert(context.Background(), approval)
	}
	s.publishApprovalRequiredNotification(msg)
}

func (s *codexLiveSession) publishApprovalRequiredNotification(msg rpcMessage) {
	if s == nil || s.notifier == nil || msg.ID == nil || !isApprovalMethod(msg.Method) {
		return
	}
	requestID := *msg.ID
	source := "approval_request:" + strings.TrimSpace(s.sessionID) + ":" + strconv.Itoa(requestID)
	event := types.NotificationEvent{
		Trigger:    types.NotificationTriggerTurnCompleted,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:  strings.TrimSpace(s.sessionID),
		Status:     "approval_required",
		Source:     source,
		Payload: map[string]any{
			"kind":       "approval_required",
			"request_id": requestID,
			"method":     strings.TrimSpace(msg.Method),
		},
	}
	if s.stores != nil && s.stores.Sessions != nil {
		if record, ok, err := s.stores.Sessions.GetRecord(context.Background(), s.sessionID); err == nil && ok && record != nil && record.Session != nil {
			event.Provider = strings.TrimSpace(record.Session.Provider)
			event.Title = strings.TrimSpace(record.Session.Title)
			event.Cwd = strings.TrimSpace(record.Session.Cwd)
		}
	}
	if s.stores != nil && s.stores.SessionMeta != nil {
		if meta, ok, err := s.stores.SessionMeta.Get(context.Background(), s.sessionID); err == nil && ok && meta != nil {
			event.WorkspaceID = strings.TrimSpace(meta.WorkspaceID)
			event.WorktreeID = strings.TrimSpace(meta.WorktreeID)
		}
	}
	s.notifier.Publish(event)
}

func (s *codexLiveSession) maybeClose() {
	s.mu.Lock()
	active := s.activeTurn != ""
	closed := s.closed
	s.mu.Unlock()
	if closed || active {
		return
	}
	if s.hub.Count() == 0 {
		s.close()
	}
}

func (s *codexLiveSession) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.activeTurn = ""
	s.mu.Unlock()
	if s.client != nil {
		s.client.Close()
	}
}

func (s *codexLiveSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (m *CodexLiveManager) dropSession(sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	m.mu.Lock()
	ls := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	if ls != nil {
		ls.close()
	}
}

func isApprovalMethod(method string) bool {
	switch method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "tool/requestUserInput":
		return true
	default:
		return false
	}
}

func isClosedPipeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "file already closed") || strings.Contains(msg, "broken pipe") || strings.Contains(msg, "closed pipe")
}

func isCodexMissingThreadError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "thread not found") ||
		strings.Contains(text, "thread not loaded") ||
		strings.Contains(text, "no rollout found for thread id")
}

func shouldBootstrapMissingThread(session *types.Session, meta *types.SessionMeta) bool {
	if session == nil {
		return false
	}
	if meta == nil || strings.TrimSpace(meta.ThreadID) == "" {
		return false
	}
	if strings.TrimSpace(meta.LastTurnID) != "" {
		return false
	}
	createdAt := session.CreatedAt.UTC()
	if createdAt.IsZero() {
		return true
	}
	return time.Since(createdAt) <= 2*time.Minute
}

func reserveSessionTurn(ctx context.Context, ls *codexLiveSession, probe turnActivityProbe, start func() (string, error)) (string, error) {
	if ls == nil {
		return "", errors.New("live session is required")
	}
	if start == nil {
		return "", errors.New("turn starter is required")
	}
	ls.mu.Lock()
	if ls.activeTurn == "" && !ls.starting {
		ls.starting = true
		ls.mu.Unlock()
		return startReservedSessionTurn(ls, start)
	}
	busyTurnID := strings.TrimSpace(ls.activeTurn)
	threadID := strings.TrimSpace(ls.threadID)
	reader := ls.client
	ls.mu.Unlock()

	if busyTurnID != "" && probe != nil && reader != nil {
		status, err := probe.Probe(ctx, reader, threadID, busyTurnID)
		if err == nil && status == turnActivityInactive {
			ls.mu.Lock()
			if !ls.starting && strings.TrimSpace(ls.activeTurn) == busyTurnID {
				ls.activeTurn = ""
			}
			if ls.activeTurn == "" && !ls.starting {
				ls.starting = true
				ls.mu.Unlock()
				return startReservedSessionTurn(ls, start)
			}
			ls.mu.Unlock()
		}
	}

	return "", errors.New("turn already in progress")
}

func startReservedSessionTurn(ls *codexLiveSession, start func() (string, error)) (string, error) {
	turnID, err := start()
	now := time.Now().UTC()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.starting = false
	if err != nil {
		return "", err
	}
	ls.activeTurn = turnID
	ls.lastActive = now
	return turnID, nil
}

type codexSubscriber struct {
	id int
	ch chan types.CodexEvent
}

type codexSubscriberHub struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]*codexSubscriber
}

func newCodexSubscriberHub() *codexSubscriberHub {
	return &codexSubscriberHub{subs: make(map[int]*codexSubscriber)}
}

func (h *codexSubscriberHub) Add() (<-chan types.CodexEvent, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	id := h.nextID
	ch := make(chan types.CodexEvent, 256)
	h.subs[id] = &codexSubscriber{id: id, ch: ch}
	cancel := func() {
		h.mu.Lock()
		sub, ok := h.subs[id]
		if ok {
			delete(h.subs, id)
		}
		h.mu.Unlock()
		if ok {
			close(sub.ch)
		}
	}
	return ch, cancel
}

func (h *codexSubscriberHub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

func (h *codexSubscriberHub) Broadcast(event types.CodexEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sub := range h.subs {
		select {
		case sub.ch <- event:
		default:
		}
	}
}
