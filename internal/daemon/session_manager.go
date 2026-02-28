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

	"control/internal/logging"
	"control/internal/providers"
	"control/internal/types"
)

var ErrSessionNotFound = errors.New("session not found")

type StartSessionConfig struct {
	Provider              string
	Cmd                   string
	Cwd                   string
	AdditionalDirectories []string
	Args                  []string
	Env                   []string
	CodexHome             string
	Title                 string
	Tags                  []string
	RuntimeOptions        *types.SessionRuntimeOptions
	WorkspaceID           string
	WorktreeID            string
	InitialInput          string
	InitialText           string
	Resume                bool
	ProviderSessionID     string
	NotificationOverrides *types.NotificationSettingsPatch
	OnProviderSessionID   func(string)
}

type SessionManager struct {
	mu           sync.Mutex
	baseDir      string
	sessions     map[string]*sessionRuntime
	metaStore    SessionMetaStore
	sessionStore SessionIndexStore
	notifier     NotificationPublisher
	emitter      SessionLifecycleEmitter
	defaultEmit  bool
	logger       logging.Logger
}

type sessionRuntime struct {
	session   *types.Session
	process   *os.Process
	done      chan struct{}
	killed    bool
	stdout    *logBuffer
	stderr    *logBuffer
	hub       *subscriberHub
	debugHub  *debugHub
	debugBuf  *debugBuffer
	sink      *logSink
	debug     debugChunkSink
	items     *itemSink
	itemsHub  *itemHub
	send      func([]byte) error
	interrupt func() error
}

func (m *SessionManager) buildSessionRuntime(sessionID, provider string) (*sessionRuntime, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	sessionDir := filepath.Join(m.baseDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return nil, err
	}

	stdoutPath := filepath.Join(sessionDir, "stdout.log")
	stderrPath := filepath.Join(sessionDir, "stderr.log")
	debugPath := filepath.Join(sessionDir, "debug.jsonl")

	stdoutFile, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		_ = stdoutFile.Close()
		return nil, err
	}
	debugHub := newDebugHub()
	debugBuf := newDebugBufferWithPolicy(defaultDebugRetentionPolicy())
	debugSink, err := newDebugSink(debugPath, sessionID, provider, debugHub, debugBuf)
	if err != nil {
		_ = stdoutFile.Close()
		_ = stderrFile.Close()
		return nil, err
	}

	var (
		items    *itemSink
		itemsHub *itemHub
	)
	if providerUsesItems(provider) {
		itemsPath := filepath.Join(sessionDir, "items.jsonl")
		itemsHub = newItemHub()
		items, err = newItemSink(itemsPath, itemsHub, m.newItemTimestampMetrics(provider, sessionID), debugSink)
		if err != nil {
			_ = stdoutFile.Close()
			_ = stderrFile.Close()
			debugSink.Close()
			return nil, err
		}
	}

	state := &sessionRuntime{
		done:     make(chan struct{}),
		stdout:   newLogBuffer(logBufferMaxBytes),
		stderr:   newLogBuffer(logBufferMaxBytes),
		hub:      newSubscriberHub(),
		debugHub: debugHub,
		debugBuf: debugBuf,
		debug:    debugSink,
		items:    items,
		itemsHub: itemsHub,
	}
	state.sink = newLogSink(stdoutFile, stderrFile, state.stdout, state.stderr, debugSink)
	return state, nil
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
		logger:   logging.Nop(),
	}, nil
}

func (m *SessionManager) SetLogger(logger logging.Logger) {
	if m == nil {
		return
	}
	if logger == nil {
		logger = logging.Nop()
	}
	m.mu.Lock()
	m.logger = logger
	m.mu.Unlock()
}

func (m *SessionManager) newItemTimestampMetrics(provider, sessionID string) itemTimestampMetricsSink {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	logger := m.logger
	m.mu.Unlock()
	if logger == nil {
		logger = logging.Nop()
	}
	return newItemTimestampLogMetricsSink(logger.With(
		logging.F("provider", strings.TrimSpace(provider)),
		logging.F("session_id", strings.TrimSpace(sessionID)),
	))
}

func (m *SessionManager) SetMetaStore(store SessionMetaStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metaStore = store
	if m.defaultEmit && m.notifier != nil {
		m.emitter = NewSessionLifecycleEmitter(m.notifier, m.metaStore, nil)
	}
}

func (m *SessionManager) SetSessionStore(store SessionIndexStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionStore = store
}

func (m *SessionManager) SetNotificationPublisher(notifier NotificationPublisher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifier = notifier
	m.emitter = NewSessionLifecycleEmitter(notifier, m.metaStore, nil)
	m.defaultEmit = true
}

func (m *SessionManager) SetSessionLifecycleEmitter(emitter SessionLifecycleEmitter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitter = emitter
	m.defaultEmit = false
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

	runtimeState, err := m.buildSessionRuntime(sessionID, cfg.Provider)
	if err != nil {
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

	runtimeState.session = session

	m.mu.Lock()
	m.sessions[sessionID] = runtimeState
	session.Status = types.SessionStatusStarting
	m.mu.Unlock()

	cfg.OnProviderSessionID = func(providerID string) {
		// Use session.ID rather than the original sessionID because the
		// session may have been re-keyed to a codex thread ID.
		m.upsertSessionProviderID(cfg.Provider, session.ID, providerID)
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
	// Re-key the session to the codex thread ID so there is exactly one
	// entry per conversation thread. session.ID is mutated in place.
	if threadID := strings.TrimSpace(proc.ThreadID); threadID != "" && threadID != sessionID {
		m.rekeySession(sessionID, threadID, runtimeState)
	}
	startedSession := cloneSession(session)
	m.mu.Unlock()

	// After re-key, session.ID holds the effective (possibly new) ID.
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

			// Use session.ID (not the original sessionID) since it may
			// have been re-keyed to the codex thread ID.
			m.upsertSessionMeta(cfg, session.ID, finalStatus)
			m.upsertSessionRecord(session, sessionSourceInternal)
			m.publishSessionLifecycleEvent(session, cfg, finalStatus, "session_manager_wait")
			runtimeState.sink.Close()
			if runtimeState.items != nil {
				runtimeState.items.Close()
			}
			close(runtimeState.done)
		}()
	} else {
		close(runtimeState.done)
	}

	return startedSession, nil
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

	runtimeState, err := m.buildSessionRuntime(session.ID, cfg.Provider)
	if err != nil {
		return nil, err
	}
	runtimeState.session = session

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

	oldID := session.ID
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
	// Re-key old-format sessions that still use a random internal ID.
	if threadID := strings.TrimSpace(proc.ThreadID); threadID != "" && threadID != oldID {
		m.rekeySession(oldID, threadID, runtimeState)
	}
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
			m.publishSessionLifecycleEvent(session, cfg, finalStatus, "session_manager_resume_wait")
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

func (m *SessionManager) BroadcastItems(id string, items []map[string]any) {
	if m == nil || strings.TrimSpace(id) == "" || len(items) == 0 {
		return
	}
	m.mu.Lock()
	state, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok || state == nil || state.itemsHub == nil {
		return
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		state.itemsHub.Broadcast(item)
	}
}

func (m *SessionManager) SubscribeDebug(id string) (<-chan types.DebugEvent, func(), error) {
	m.mu.Lock()
	state, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok || state == nil || state.debugHub == nil {
		return nil, nil, ErrSessionNotFound
	}
	ch, cancel := state.debugHub.Add()
	return ch, cancel, nil
}

func (m *SessionManager) DebugSnapshot(id string, lines int) ([]types.DebugEvent, error) {
	m.mu.Lock()
	state, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok || state == nil || state.debugBuf == nil {
		return nil, ErrSessionNotFound
	}
	return state.debugBuf.Snapshot(lines), nil
}

func (m *SessionManager) WriteSessionDebug(id, stream string, data []byte) error {
	if m == nil || strings.TrimSpace(id) == "" {
		return ErrSessionNotFound
	}
	if len(data) == 0 {
		return nil
	}
	m.mu.Lock()
	state, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok || state == nil || state.sink == nil {
		return ErrSessionNotFound
	}
	state.sink.WriteDebug(stream, data)
	return nil
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

func (m *SessionManager) InterruptSession(id string) error {
	m.mu.Lock()
	state, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return ErrSessionNotFound
	}
	interrupt := state.interrupt
	m.mu.Unlock()
	if interrupt == nil {
		return errors.New("session does not support interrupt")
	}
	return interrupt()
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
	sessionCopy := cloneSession(state.session)
	m.mu.Unlock()
	m.upsertSessionRecord(state.session, sessionSourceInternal)
	m.publishSessionLifecycleEvent(sessionCopy, StartSessionConfig{}, types.SessionStatusExited, "session_manager_mark_exited")
	return nil
}

func (m *SessionManager) DismissSession(id string) error {
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
	m.mu.Unlock()
	now := time.Now().UTC()
	m.upsertSessionDismissedAt(id, &now)
	return nil
}

func (m *SessionManager) UndismissSession(id string) error {
	m.mu.Lock()
	_, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return ErrSessionNotFound
	}
	m.mu.Unlock()
	clear := time.Time{}
	m.upsertSessionDismissedAt(id, &clear)
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

// rekeySession changes a session's ID from oldID to newID. This is used to
// reconcile codex sessions: they start with a random internal ID, but once
// the codex thread ID is known we re-key so that every codex conversation
// has exactly one entry keyed by its thread ID.
//
// Must be called while m.mu is held.
func (m *SessionManager) rekeySession(oldID, newID string, state *sessionRuntime) {
	// Re-key the in-memory sessions map and session object.
	state.session.ID = newID
	m.sessions[newID] = state
	delete(m.sessions, oldID)

	// Rename the log directory. On Linux this is atomic and existing open
	// file descriptors (stdout/stderr writers) remain valid.
	oldDir := filepath.Join(m.baseDir, oldID)
	newDir := filepath.Join(m.baseDir, newID)
	if _, err := os.Stat(newDir); err != nil {
		// Target doesn't exist; safe to rename.
		_ = os.Rename(oldDir, newDir)
	}
	// If newDir already exists we leave files in oldDir. The open handles
	// are still valid and codex history comes from the thread server anyway.

	// Migrate the meta store entry.
	if m.metaStore != nil {
		ctx := context.Background()
		if existing, ok, err := m.metaStore.Get(ctx, oldID); err == nil && ok && existing != nil {
			migrated := *existing
			migrated.SessionID = newID
			if migrated.ThreadID == "" {
				migrated.ThreadID = newID
			}
			_, _ = m.metaStore.Upsert(ctx, &migrated)
		}
		_ = m.metaStore.Delete(ctx, oldID)
	}

	// Migrate the session index entry.
	if m.sessionStore != nil {
		ctx := context.Background()
		if record, ok, err := m.sessionStore.GetRecord(ctx, oldID); err == nil && ok && record != nil && record.Session != nil {
			clone := *record.Session
			clone.ID = newID
			_, _ = m.sessionStore.UpsertRecord(ctx, &types.SessionRecord{
				Session: &clone,
				Source:  record.Source,
			})
		}
		_ = m.sessionStore.DeleteRecord(ctx, oldID)
	}
}

func (m *SessionManager) UpdateSessionTitle(id, title string) error {
	m.mu.Lock()
	store := m.metaStore
	sessionStore := m.sessionStore
	if state, ok := m.sessions[id]; ok && state != nil && state.session != nil {
		state.session.Title = strings.TrimSpace(title)
	}
	m.mu.Unlock()
	if store == nil {
		if sessionStore == nil {
			return nil
		}
	}
	sanitized := sanitizeTitle(title)
	meta := &types.SessionMeta{
		SessionID:   id,
		Title:       sanitized,
		TitleLocked: true,
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
	title := sanitizeTitle(cfg.Title)
	titleLocked := false
	if existing, ok, err := store.Get(context.Background(), sessionID); err == nil && ok && existing != nil {
		if existing.TitleLocked {
			// Preserve explicit user renames across lifecycle metadata refreshes.
			title = ""
			titleLocked = true
		} else if existingTitle := strings.TrimSpace(existing.Title); existingTitle != "" && title != "" && existingTitle != title {
			// Migrate legacy custom titles written before title locking existed.
			title = ""
			titleLocked = true
		}
	}
	now := time.Now().UTC()
	meta := &types.SessionMeta{
		SessionID:             sessionID,
		WorkspaceID:           cfg.WorkspaceID,
		WorktreeID:            cfg.WorktreeID,
		Title:                 title,
		TitleLocked:           titleLocked,
		InitialInput:          sanitizeTitle(cfg.InitialInput),
		RuntimeOptions:        types.CloneRuntimeOptions(cfg.RuntimeOptions),
		NotificationOverrides: types.CloneNotificationSettingsPatch(cfg.NotificationOverrides),
		LastActiveAt:          &now,
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

func (m *SessionManager) upsertSessionDismissedAt(sessionID string, dismissedAt *time.Time) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	m.mu.Lock()
	store := m.metaStore
	m.mu.Unlock()
	if store == nil {
		return
	}
	meta := &types.SessionMeta{
		SessionID:   sessionID,
		DismissedAt: dismissedAt,
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

func (m *SessionManager) publishSessionLifecycleEvent(session *types.Session, cfg StartSessionConfig, status types.SessionStatus, source string) {
	if session == nil {
		return
	}
	m.mu.Lock()
	emitter := m.emitter
	m.mu.Unlock()
	if emitter == nil {
		return
	}
	emitter.EmitSessionLifecycleEvent(context.Background(), session, cfg, status, source)
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
