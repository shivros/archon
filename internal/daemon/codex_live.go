package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"control/internal/types"
)

type CodexLiveManager struct {
	mu       sync.Mutex
	sessions map[string]*codexLiveSession
}

func NewCodexLiveManager() *CodexLiveManager {
	return &CodexLiveManager{
		sessions: make(map[string]*codexLiveSession),
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
		return "", err
	}
	ls.mu.Lock()
	ls.activeTurn = turnID
	ls.lastActive = time.Now().UTC()
	ls.mu.Unlock()
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
		return nil, nil, err
	}
	ch, cancel := ls.hub.Add()
	wrappedCancel := func() {
		cancel()
		ls.maybeClose()
	}
	return ch, wrappedCancel, nil
}

func (m *CodexLiveManager) ensure(session *types.Session, meta *types.SessionMeta, codexHome string) (*codexLiveSession, error) {
	m.mu.Lock()
	ls := m.sessions[session.ID]
	if ls != nil {
		m.mu.Unlock()
		return ls, nil
	}
	m.mu.Unlock()

	threadID := resolveThreadID(session, meta)
	if threadID == "" {
		return nil, errors.New("thread id not available")
	}
	client, err := startCodexAppServer(context.Background(), session.Cwd, codexHome)
	if err != nil {
		return nil, err
	}
	if err := client.ResumeThread(context.Background(), threadID); err != nil {
		client.Close()
		return nil, err
	}

	ls = &codexLiveSession{
		sessionID: session.ID,
		threadID:  threadID,
		client:    client,
		hub:       newCodexSubscriberHub(),
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
	activeTurn string
	lastActive time.Time
	closed     bool
}

func (s *codexLiveSession) start() {
	go func() {
		notes := s.client.Notifications()
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
	s.mu.Unlock()
	if s.client != nil {
		s.client.Close()
	}
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
