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

type CodexLiveManager struct {
	mu       sync.Mutex
	sessions map[string]*codexLiveSession
	stores   *Stores
	logger   logging.Logger
}

func NewCodexLiveManager(stores *Stores, logger logging.Logger) *CodexLiveManager {
	if logger == nil {
		logger = logging.Nop()
	}
	return &CodexLiveManager{
		sessions: make(map[string]*codexLiveSession),
		stores:   stores,
		logger:   logger,
	}
}

func (m *CodexLiveManager) StartTurn(ctx context.Context, session *types.Session, meta *types.SessionMeta, codexHome string, input []map[string]any) (string, error) {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return "", errors.New("session is required")
	}
	if session.Provider != "codex" {
		return "", errors.New("provider does not support live events")
	}
	ls, err := m.ensure(session, meta, codexHome)
	if err != nil {
		m.logger.Error("codex_live_ensure_error", logging.F("session_id", session.ID), logging.F("error", err))
		return "", err
	}
	ls.mu.Lock()
	if ls.activeTurn != "" {
		ls.mu.Unlock()
		return "", errors.New("turn already in progress")
	}
	ls.mu.Unlock()

	turnID, err := ls.client.StartTurn(ctx, ls.threadID, input)
	if err != nil {
		if isClosedPipeError(err) {
			m.dropSession(session.ID)
			ls, err = m.ensure(session, meta, codexHome)
			if err != nil {
				m.logger.Error("codex_live_restart_error", logging.F("session_id", session.ID), logging.F("error", err))
				return "", err
			}
			turnID, err = ls.client.StartTurn(ctx, ls.threadID, input)
			if err != nil {
				m.logger.Error("codex_live_start_error", logging.F("session_id", session.ID), logging.F("error", err))
				return "", err
			}
		} else {
			m.logger.Error("codex_live_start_error", logging.F("session_id", session.ID), logging.F("error", err))
			return "", err
		}
	}
	ls.mu.Lock()
	ls.activeTurn = turnID
	ls.lastActive = time.Now().UTC()
	ls.mu.Unlock()
	m.logger.Info("codex_turn_started", logging.F("session_id", session.ID), logging.F("thread_id", ls.threadID), logging.F("turn_id", turnID))
	return turnID, nil
}

func (m *CodexLiveManager) Subscribe(session *types.Session, meta *types.SessionMeta, codexHome string) (<-chan types.CodexEvent, func(), error) {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return nil, nil, errors.New("session is required")
	}
	if session.Provider != "codex" {
		return nil, nil, errors.New("provider does not support live events")
	}
	ls, err := m.ensure(session, meta, codexHome)
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
	ls, err := m.ensure(session, meta, codexHome)
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
	ls, err := m.ensure(session, meta, codexHome)
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

func (m *CodexLiveManager) ensure(session *types.Session, meta *types.SessionMeta, codexHome string) (*codexLiveSession, error) {
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

	threadID := resolveThreadID(session, meta)
	if threadID == "" {
		return nil, errors.New("thread id not available")
	}
	client, err := startCodexAppServer(context.Background(), session.Cwd, codexHome, m.logger)
	if err != nil {
		m.logger.Error("codex_start_error", logging.F("session_id", session.ID), logging.F("error", err))
		return nil, err
	}
	if err := client.ResumeThread(context.Background(), threadID); err != nil {
		m.logger.Error("codex_resume_error", logging.F("session_id", session.ID), logging.F("thread_id", threadID), logging.F("error", err))
		client.Close()
		return nil, err
	}

	ls = &codexLiveSession{
		sessionID: session.ID,
		threadID:  threadID,
		client:    client,
		hub:       newCodexSubscriberHub(),
		stores:    m.stores,
	}
	ls.start()

	m.mu.Lock()
	m.sessions[session.ID] = ls
	m.mu.Unlock()
	return ls, nil
}

type codexLiveSession struct {
	mu         sync.Mutex
	sessionID  string
	threadID   string
	client     *codexAppServer
	hub        *codexSubscriberHub
	stores     *Stores
	activeTurn string
	lastActive time.Time
	closed     bool
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
		s.maybeClose()
	}
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
