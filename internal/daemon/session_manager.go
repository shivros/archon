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

	"control/internal/types"
)

var ErrSessionNotFound = errors.New("session not found")

type StartSessionConfig struct {
	Provider     string
	Cmd          string
	Cwd          string
	Args         []string
	Env          []string
	CodexHome    string
	Title        string
	Tags         []string
	WorkspaceID  string
	WorktreeID   string
	InitialInput string
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
		session: session,
		done:    make(chan struct{}),
		stdout:  newLogBuffer(logBufferMaxBytes),
		stderr:  newLogBuffer(logBufferMaxBytes),
		hub:     newSubscriberHub(),
	}
	runtimeState.sink = newLogSink(stdoutFile, stderrFile, runtimeState.stdout, runtimeState.stderr)

	m.mu.Lock()
	m.sessions[sessionID] = runtimeState
	session.Status = types.SessionStatusStarting
	m.mu.Unlock()

	proc, err := provider.Start(cfg, runtimeState.sink)
	if err != nil {
		m.mu.Lock()
		session.Status = types.SessionStatusFailed
		session.ExitedAt = ptrTime(time.Now().UTC())
		m.mu.Unlock()
		runtimeState.sink.Close()
		return nil, err
	}

	startedAt := time.Now().UTC()
	m.mu.Lock()
	runtimeState.process = proc.Process
	runtimeState.interrupt = proc.Interrupt
	if proc.Process != nil {
		session.PID = proc.Process.Pid
	}
	session.Status = types.SessionStatusRunning
	session.StartedAt = &startedAt
	m.mu.Unlock()

	m.upsertSessionMeta(cfg, sessionID, session.Status)
	m.upsertSessionRecord(session, sessionSourceInternal)

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
		close(runtimeState.done)
	}()

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

func (m *SessionManager) KillSession(id string) error {
	m.mu.Lock()
	state, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return ErrSessionNotFound
	}
	if state.process == nil {
		m.mu.Unlock()
		return errors.New("session process not started")
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
		m.mu.Unlock()
		return errors.New("session is active; kill it first")
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
