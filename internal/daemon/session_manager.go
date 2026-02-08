package daemon

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"control/internal/providers"
	"control/internal/types"
)

var ErrSessionNotFound = errors.New("session not found")

type StartSessionConfig struct {
	Provider            string
	Cmd                 string
	Cwd                 string
	Args                []string
	Env                 []string
	CodexHome           string
	Title               string
	Tags                []string
	WorkspaceID         string
	WorktreeID          string
	InitialInput        string
	InitialText         string
	Resume              bool
	ProviderSessionID   string
	OnProviderSessionID func(string)
}

type SessionManager struct {
	mu           sync.Mutex
	baseDir      string
	sessions     map[string]*sessionRuntime
	metaStore    SessionMetaStore
	sessionStore SessionIndexStore
}

type sessionRuntime struct {
	session   *types.Session
	process   *os.Process
	done      chan struct{}
	killed    bool
	stdout    *logBuffer
	stderr    *logBuffer
	hub       *subscriberHub
	sink      *logSink
	items     *itemSink
	itemsHub  *itemHub
	send      func([]byte) error
	interrupt func() error
}

func NewSessionManager(baseDir string) (*SessionManager, error) {
	if baseDir == "" {
		return nil, errors.New("sessions base dir is required")
	}
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, err
	}
	return &SessionManager{
		baseDir:  baseDir,
		sessions: make(map[string]*sessionRuntime),
	}, nil
}

func (m *SessionManager) SetMetaStore(store SessionMetaStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metaStore = store
}

func (m *SessionManager) SetSessionStore(store SessionIndexStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionStore = store
}

func (m *SessionManager) SessionsBaseDir() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.baseDir
}

func (m *SessionManager) StartSession(cfg StartSessionConfig) (*types.Session, error) {
	provider, err := ResolveProvider(cfg.Provider, cfg.Cmd)
	if err != nil {
		return nil, err
	}

	sessionID, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	sessionDir := filepath.Join(m.baseDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return nil, err
	}

	stdoutPath := filepath.Join(sessionDir, "stdout.log")
	stderrPath := filepath.Join(sessionDir, "stderr.log")
	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = stdoutFile.Close()
		return nil, err
	}
	var items *itemSink
	var itemsHub *itemHub
	if providerUsesItems(cfg.Provider) {
		itemsPath := filepath.Join(sessionDir, "items.jsonl")
		itemsHub = newItemHub()
		items, err = newItemSink(itemsPath, itemsHub)
		if err != nil {
			_ = stdoutFile.Close()
			_ = stderrFile.Close()
			return nil, err
		}
	}

	now := time.Now().UTC()
	session := &types.Session{
		ID:        sessionID,
		Provider:  cfg.Provider,
		Cwd:       cfg.Cwd,
		Cmd:       provider.Command(),
		Args:      append([]string{}, cfg.Args...),
		Env:       append([]string{}, cfg.Env...),
		Status:    types.SessionStatusCreated,
		CreatedAt: now,
		Title:     cfg.Title,
		Tags:      append([]string{}, cfg.Tags...),
	}

	runtimeState := &sessionRuntime{
		session:  session,
		done:     make(chan struct{}),
		stdout:   newLogBuffer(logBufferMaxBytes),
		stderr:   newLogBuffer(logBufferMaxBytes),
		hub:      newSubscriberHub(),
		items:    items,
		itemsHub: itemsHub,
	}
	runtimeState.sink = newLogSink(stdoutFile, stderrFile, runtimeState.stdout, runtimeState.stderr)

	m.mu.Lock()
	m.sessions[sessionID] = runtimeState
	session.Status = types.SessionStatusStarting
	m.mu.Unlock()

	cfg.OnProviderSessionID = func(providerID string) {
		m.upsertSessionProviderID(cfg.Provider, sessionID, providerID)
	}
	caps := providers.CapabilitiesFor(cfg.Provider)
	proc, err := provider.Start(cfg, runtimeState.sink, runtimeState.items)
	if err != nil {
		m.mu.Lock()
		session.Status = types.SessionStatusFailed
		session.ExitedAt = ptrTime(time.Now().UTC())
		m.mu.Unlock()
		runtimeState.sink.Close()
		if runtimeState.items != nil {
			runtimeState.items.Close()
		}
		return nil, err
	}

	startedAt := time.Now().UTC()
	m.mu.Lock()
	runtimeState.process = proc.Process
	runtimeState.interrupt = proc.Interrupt
	runtimeState.send = proc.Send
	if proc.Process != nil {
		session.PID = proc.Process.Pid
	}
	if proc.Process != nil {
		session.Status = types.SessionStatusRunning
		session.StartedAt = &startedAt
	} else if caps.NoProcess {
		session.Status = types.SessionStatusInactive
		session.StartedAt = &startedAt
	} else {
		session.Status = types.SessionStatusRunning
		session.StartedAt = &startedAt
	}
	m.mu.Unlock()

	m.upsertSessionMeta(cfg, sessionID, session.Status)
	m.upsertSessionThreadID(sessionID, proc.ThreadID)
	m.upsertSessionProviderID(cfg.Provider, sessionID, proc.ThreadID)
	m.upsertSessionRecord(session, sessionSourceInternal)

	if proc.Process != nil || !caps.NoProcess {
		go m.flushLoop(runtimeState)
		go func() {
			err := proc.Wait()
			exitCode := exitCodeFromError(err)

			var finalStatus types.SessionStatus
			m.mu.Lock()
			if runtimeState.killed {
				session.Status = types.SessionStatusKilled
			} else if err == nil {
				session.Status = types.SessionStatusExited
			} else if isExitSignal(err) {
				session.Status = types.SessionStatusExited
			} else {
				session.Status = types.SessionStatusFailed
			}
			finalStatus = session.Status
			if exitCode != nil {
				session.ExitCode = exitCode
			}
			session.ExitedAt = ptrTime(time.Now().UTC())
			m.mu.Unlock()

			m.upsertSessionMeta(cfg, sessionID, finalStatus)
			m.upsertSessionRecord(session, sessionSourceInternal)
			runtimeState.sink.Close()
			if runtimeState.items != nil {
				runtimeState.items.Close()
			}
			close(runtimeState.done)
		}()
	} else {
		close(runtimeState.done)
	}

	return cloneSession(session), nil
}

func providerUsesItems(provider string) bool {
	return providers.CapabilitiesFor(provider).UsesItems
}

func (m *SessionManager) SendInput(id string, payload []byte) error {
	if len(payload) == 0 {
		return errors.New("payload is required")
	}
	m.mu.Lock()
	state, ok := m.sessions[id]
	send := (*sessionRuntime)(nil)
	if ok {
		send = state
	}
	m.mu.Unlock()
	if send == nil || send.send == nil {
		return ErrSessionNotFound
	}
	return send.send(payload)
}

func (m *SessionManager) ResumeSession(cfg StartSessionConfig, session *types.Session) (*types.Session, error) {
	if session == nil || strings.TrimSpace(session.ID) == "" {
		return nil, errors.New("session is required")
	}
	m.mu.Lock()
	if _, ok := m.sessions[session.ID]; ok {
		m.mu.Unlock()
		return cloneSession(session), nil
	}
	m.mu.Unlock()

	provider, err := ResolveProvider(cfg.Provider, cfg.Cmd)
	if err != nil {
		return nil, err
	}

	sessionDir := filepath.Join(m.baseDir, session.ID)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return nil, err
	}

	stdoutPath := filepath.Join(sessionDir, "stdout.log")
	stderrPath := filepath.Join(sessionDir, "stderr.log")
	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = stdoutFile.Close()
		return nil, err
	}
	var items *itemSink
	var itemsHub *itemHub
	if providerUsesItems(cfg.Provider) {
		itemsPath := filepath.Join(sessionDir, "items.jsonl")
		itemsHub = newItemHub()
		items, err = newItemSink(itemsPath, itemsHub)
		if err != nil {
			_ = stdoutFile.Close()
			_ = stderrFile.Close()
			return nil, err
		}
	}

	runtimeState := &sessionRuntime{
		session:  session,
		done:     make(chan struct{}),
		stdout:   newLogBuffer(logBufferMaxBytes),
		stderr:   newLogBuffer(logBufferMaxBytes),
		hub:      newSubscriberHub(),
		items:    items,
		itemsHub: itemsHub,
	}
	runtimeState.sink = newLogSink(stdoutFile, stderrFile, runtimeState.stdout, runtimeState.stderr)

	m.mu.Lock()
	m.sessions[session.ID] = runtimeState
	session.Status = types.SessionStatusStarting
	m.mu.Unlock()

	caps := providers.CapabilitiesFor(cfg.Provider)
	proc, err := provider.Start(cfg, runtimeState.sink, runtimeState.items)
	if err != nil {
		m.mu.Lock()
		session.Status = types.SessionStatusFailed
		session.ExitedAt = ptrTime(time.Now().UTC())
		m.mu.Unlock()
		runtimeState.sink.Close()
		if runtimeState.items != nil {
			runtimeState.items.Close()
		}
		return nil, err
	}

	startedAt := time.Now().UTC()
	m.mu.Lock()
	runtimeState.process = proc.Process
	runtimeState.interrupt = proc.Interrupt
	runtimeState.send = proc.Send
	if proc.Process != nil {
		session.PID = proc.Process.Pid
	}
	if proc.Process != nil {
		session.Status = types.SessionStatusRunning
		session.StartedAt = &startedAt
	} else if caps.NoProcess {
		session.Status = types.SessionStatusInactive
		session.StartedAt = &startedAt
	} else {
		session.Status = types.SessionStatusRunning
		session.StartedAt = &startedAt
	}
	session.ExitedAt = nil
	m.mu.Unlock()

	m.upsertSessionMeta(cfg, session.ID, session.Status)
	m.upsertSessionThreadID(session.ID, proc.ThreadID)
	m.upsertSessionProviderID(cfg.Provider, session.ID, proc.ThreadID)
	m.upsertSessionRecord(session, sessionSourceInternal)

	if proc.Process != nil || !caps.NoProcess {
		go m.flushLoop(runtimeState)
		go func() {
			err := proc.Wait()
			exitCode := exitCodeFromError(err)

			var finalStatus types.SessionStatus
			m.mu.Lock()
			if runtimeState.killed {
				session.Status = types.SessionStatusKilled
			} else if err == nil {
				session.Status = types.SessionStatusExited
			} else if isExitSignal(err) {
				session.Status = types.SessionStatusExited
			} else {
				session.Status = types.SessionStatusFailed
			}
			finalStatus = session.Status
			if exitCode != nil {
				session.ExitCode = exitCode
			}
			session.ExitedAt = ptrTime(time.Now().UTC())
			m.mu.Unlock()

			m.upsertSessionMeta(cfg, session.ID, finalStatus)
			m.upsertSessionRecord(session, sessionSourceInternal)
			runtimeState.sink.Close()
			if runtimeState.items != nil {
				runtimeState.items.Close()
			}
			close(runtimeState.done)
		}()
	} else {
		close(runtimeState.done)
	}

	return cloneSession(session), nil
}

func (m *SessionManager) Subscribe(id, stream string) (<-chan types.LogEvent, func(), error) {
	m.mu.Lock()
	state, ok := m.sessions[id]
	store := m.metaStore
	m.mu.Unlock()
	if !ok {
		return nil, nil, ErrSessionNotFound
	}
	if store != nil {
		now := time.Now().UTC()
		_, _ = store.Upsert(context.Background(), &types.SessionMeta{
			SessionID:    id,
			LastActiveAt: &now,
		})
	}
	ch, cancel, err := state.hub.Add(stream)
	if err != nil {
		return nil, nil, err
	}
	wrappedCancel := func() {
		cancel()
		if store == nil {
			return
		}
		now := time.Now().UTC()
		_, _ = store.Upsert(context.Background(), &types.SessionMeta{
			SessionID:    id,
			LastActiveAt: &now,
		})
	}
	return ch, wrappedCancel, nil
}

func (m *SessionManager) SubscribeItems(id string) (<-chan map[string]any, func(), error) {
	m.mu.Lock()
	state, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok || state == nil || state.itemsHub == nil {
		return nil, nil, ErrSessionNotFound
	}
	ch, cancel := state.itemsHub.Add()
	return ch, cancel, nil
}

func (m *SessionManager) KillSession(id string) error {
	m.mu.Lock()
	state, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return ErrSessionNotFound
	}
	if state.session.Status == types.SessionStatusExited || state.session.Status == types.SessionStatusFailed || state.session.Status == types.SessionStatusKilled {
		m.mu.Unlock()
		return nil
	}
	state.killed = true
	process := state.process
	interrupt := state.interrupt
	done := state.done
	m.mu.Unlock()

	if interrupt != nil {
		_ = interrupt()
	}

	if process == nil {
		select {
		case <-done:
			return nil
		case <-time.After(1 * time.Second):
			return nil
		}
	}

	_ = signalTerminate(process)
	select {
	case <-done:
		return nil
	case <-time.After(3 * time.Second):
	}
	_ = signalKill(process)
	return nil
}

func (m *SessionManager) MarkExited(id string) error {
	m.mu.Lock()
	state, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return ErrSessionNotFound
	}
	if isActiveStatus(state.session.Status) {
		if !providers.CapabilitiesFor(state.session.Provider).NoProcess {
			m.mu.Unlock()
			return errors.New("session is active; kill it first")
		}
	}
	now := time.Now().UTC()
	state.session.Status = types.SessionStatusExited
	state.session.ExitedAt = &now
	m.mu.Unlock()
	m.upsertSessionRecord(state.session, sessionSourceInternal)
	return nil
}

func (m *SessionManager) GetSession(id string) (*types.Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	return cloneSession(state.session), true
}

func (m *SessionManager) ListSessions() []*types.Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*types.Session, 0, len(m.sessions))
	for _, state := range m.sessions {
		out = append(out, cloneSession(state.session))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (m *SessionManager) TailSession(id, stream string, lines int) ([]string, bool, string, error) {
	if lines <= 0 {
		lines = 200
	}

	m.mu.Lock()
	_, ok := m.sessions[id]
	store := m.metaStore
	m.mu.Unlock()
	if !ok {
		return nil, false, "", ErrSessionNotFound
	}
	if store != nil {
		now := time.Now().UTC()
		_, _ = store.Upsert(context.Background(), &types.SessionMeta{
			SessionID:    id,
			LastActiveAt: &now,
		})
	}

	sessionDir := filepath.Join(m.baseDir, id)
	stdoutPath := filepath.Join(sessionDir, "stdout.log")
	stderrPath := filepath.Join(sessionDir, "stderr.log")

	switch stream {
	case "", "combined":
		stdoutLines, stdoutTrunc, err := tailLines(stdoutPath, lines)
		if err != nil {
			return nil, false, "", err
		}
		stderrLines, stderrTrunc, err := tailLines(stderrPath, lines)
		if err != nil {
			return nil, false, "", err
		}
		combined := append(stdoutLines, stderrLines...)
		return combined, stdoutTrunc || stderrTrunc, "stdout_then_stderr", nil
	case "stdout":
		linesOut, truncated, err := tailLines(stdoutPath, lines)
		return linesOut, truncated, "", err
	case "stderr":
		linesOut, truncated, err := tailLines(stderrPath, lines)
		return linesOut, truncated, "", err
	default:
		return nil, false, "", errors.New("invalid stream")
	}
}

func tailLines(path string, maxLines int) ([]string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, false, nil
		}
		return nil, false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	buffer := make([]string, 0, maxLines)
	truncated := false

	for scanner.Scan() {
		line := scanner.Text()
		if len(buffer) < maxLines {
			buffer = append(buffer, line)
			continue
		}
		truncated = true
		copy(buffer, buffer[1:])
		buffer[maxLines-1] = line
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}
	return buffer, truncated, nil
}

func cloneSession(s *types.Session) *types.Session {
	if s == nil {
		return nil
	}
	out := *s
	if s.Args != nil {
		out.Args = append([]string{}, s.Args...)
	}
	if s.Env != nil {
		out.Env = append([]string{}, s.Env...)
	}
	if s.Tags != nil {
		out.Tags = append([]string{}, s.Tags...)
	}
	return &out
}

func generateSessionID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func exitCodeFromError(err error) *int {
	if err == nil {
		code := 0
		return &code
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		return &code
	}
	return nil
}

func (m *SessionManager) UpdateSessionTitle(id, title string) error {
	m.mu.Lock()
	store := m.metaStore
	sessionStore := m.sessionStore
	m.mu.Unlock()
	if store == nil {
		if sessionStore == nil {
			return nil
		}
	}
	meta := &types.SessionMeta{
		SessionID: id,
		Title:     sanitizeTitle(title),
	}
	if store != nil {
		if _, err := store.Upsert(context.Background(), meta); err != nil {
			return err
		}
	}
	if sessionStore != nil {
		record, ok, err := sessionStore.GetRecord(context.Background(), id)
		if err != nil {
			return err
		}
		if ok && record.Session != nil {
			copy := *record.Session
			copy.Title = strings.TrimSpace(title)
			record.Session = &copy
			if _, err := sessionStore.UpsertRecord(context.Background(), record); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *SessionManager) upsertSessionMeta(cfg StartSessionConfig, sessionID string, status types.SessionStatus) {
	m.mu.Lock()
	store := m.metaStore
	m.mu.Unlock()
	if store == nil {
		return
	}
	now := time.Now().UTC()
	meta := &types.SessionMeta{
		SessionID:    sessionID,
		WorkspaceID:  cfg.WorkspaceID,
		WorktreeID:   cfg.WorktreeID,
		Title:        sanitizeTitle(cfg.Title),
		InitialInput: sanitizeTitle(cfg.InitialInput),
		LastActiveAt: &now,
	}
	_, _ = store.Upsert(context.Background(), meta)
}

func (m *SessionManager) upsertSessionThreadID(sessionID, threadID string) {
	if strings.TrimSpace(threadID) == "" {
		return
	}
	m.mu.Lock()
	store := m.metaStore
	m.mu.Unlock()
	if store == nil {
		return
	}
	meta := &types.SessionMeta{
		SessionID: sessionID,
		ThreadID:  threadID,
	}
	_, _ = store.Upsert(context.Background(), meta)
}

func (m *SessionManager) upsertSessionProviderID(provider, sessionID, providerID string) {
	if strings.TrimSpace(providerID) == "" {
		return
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || provider == "codex" {
		return
	}
	m.mu.Lock()
	store := m.metaStore
	m.mu.Unlock()
	if store == nil {
		return
	}
	meta := &types.SessionMeta{
		SessionID:         sessionID,
		ProviderSessionID: providerID,
	}
	_, _ = store.Upsert(context.Background(), meta)
}

func (m *SessionManager) upsertSessionRecord(session *types.Session, source string) {
	m.mu.Lock()
	store := m.sessionStore
	m.mu.Unlock()
	if store == nil || session == nil {
		return
	}
	copy := cloneSession(session)
	_, _ = store.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: copy,
		Source:  source,
	})
}

func (m *SessionManager) flushLoop(state *sessionRuntime) {
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()

	for range ticker.C {
		subscribers := state.hub.Count()
		if subscribers == 0 {
			state.stdout.Clear()
			state.stderr.Clear()
			if isDone(state) {
				return
			}
			continue
		}

		flushBuffer(state.stdout, "stdout", state.hub)
		flushBuffer(state.stderr, "stderr", state.hub)
	}
}

func flushBuffer(buffer *logBuffer, stream string, hub *subscriberHub) {
	if buffer == nil {
		return
	}
	for i := 0; i < maxChunksPerFlush; i++ {
		chunk := buffer.Drain(logChunkBytes)
		if len(chunk) == 0 {
			return
		}
		event := types.LogEvent{
			Type:   "log",
			Stream: stream,
			Chunk:  string(chunk),
			TS:     time.Now().UTC().Format(time.RFC3339Nano),
		}
		hub.Broadcast(event)
	}
}

func isDone(state *sessionRuntime) bool {
	select {
	case <-state.done:
		return true
	default:
		return false
	}
}

func isActiveStatus(status types.SessionStatus) bool {
	switch status {
	case types.SessionStatusCreated, types.SessionStatusStarting, types.SessionStatusRunning:
		return true
	default:
		return false
	}
}
