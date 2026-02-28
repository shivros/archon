package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

type claudeLiveSession struct {
	mu              sync.Mutex
	sessionID       string
	session         *types.Session
	meta            *types.SessionMeta
	manager         *SessionManager
	stores          *Stores
	logger          logging.Logger
	orchestrator    claudeSendOrchestrator
	failureReporter ClaudeTurnFailureReporter
	scheduler       ClaudeTurnScheduler
	activeTurn      string
	closed          bool
}

const claudeTurnQueueSize = 256

var (
	_ TurnCapableSession = (*claudeLiveSession)(nil)
	_ closeAwareSession  = (*claudeLiveSession)(nil)
)

func (s *claudeLiveSession) SessionID() string {
	return s.sessionID
}

func (s *claudeLiveSession) ActiveTurnID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeTurn
}

func (s *claudeLiveSession) StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", invalidError("session is closed", nil)
	}
	session := cloneSessionShallow(s.session)
	meta := cloneSessionMeta(s.meta)
	s.mu.Unlock()

	if opts != nil {
		if meta == nil {
			meta = &types.SessionMeta{SessionID: session.ID}
		}
		meta.RuntimeOptions = types.CloneRuntimeOptions(opts)
	}

	prepared, err := s.orchestrator.PrepareTurn(session, meta, input)
	if err != nil {
		return "", err
	}
	if s.orchestrator.stateStore != nil {
		s.orchestrator.stateStore.SaveTurnState(ctx, session.ID, prepared.TurnID)
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", invalidError("session is closed", nil)
	}
	s.ensureSchedulerLocked()
	if s.activeTurn == "" {
		s.activeTurn = prepared.TurnID
	}
	enqueueErr := s.scheduler.Enqueue(claudeTurnJob{
		sendCtx:  claudeSendContext{},
		session:  session,
		meta:     meta,
		prepared: prepared,
	})
	s.mu.Unlock()
	if enqueueErr != nil {
		return "", enqueueErr
	}

	return prepared.TurnID, nil
}

func (s *claudeLiveSession) Interrupt(ctx context.Context) error {
	if s.manager == nil {
		return unavailableError("session manager not available", nil)
	}
	return s.manager.InterruptSession(s.sessionID)
}

func (s *claudeLiveSession) Events() (<-chan types.CodexEvent, func()) {
	ch := make(chan types.CodexEvent)
	close(ch)
	return ch, func() {}
}

func (s *claudeLiveSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	scheduler := s.scheduler
	s.scheduler = nil
	s.mu.Unlock()
	if scheduler != nil {
		scheduler.Close()
	}
}

func (s *claudeLiveSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *claudeLiveSession) SetSessionMeta(meta *types.SessionMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meta = cloneSessionMeta(meta)
}

func (s *claudeLiveSession) ensureSchedulerLocked() {
	if s.scheduler != nil {
		return
	}
	failureReporter := s.failureReporter
	if failureReporter == nil {
		failureReporter = defaultClaudeTurnFailureReporter{
			sessionID:    s.sessionID,
			providerName: "claude",
			logger:       s.logger,
			debugWriter:  s.manager,
		}
	}
	s.scheduler = newClaudeTurnScheduler(
		claudeTurnQueueSize,
		s.orchestrator,
		failureReporter,
		s.setActiveTurn,
		s.clearActiveTurn,
	)
}

func (s *claudeLiveSession) setActiveTurn(turnID string) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.activeTurn = turnID
}

func (s *claudeLiveSession) clearActiveTurn(turnID string) {
	turnID = strings.TrimSpace(turnID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if turnID == "" || s.activeTurn == turnID {
		s.activeTurn = ""
	}
}

type claudeLiveSessionFactory struct {
	manager      *SessionManager
	stores       *Stores
	repository   TurnArtifactRepository
	turnNotifier TurnCompletionNotifier
	logger       logging.Logger
}

func newClaudeLiveSessionFactory(
	manager *SessionManager,
	stores *Stores,
	repository TurnArtifactRepository,
	turnNotifier TurnCompletionNotifier,
	logger logging.Logger,
) *claudeLiveSessionFactory {
	if logger == nil {
		logger = logging.Nop()
	}
	if turnNotifier == nil {
		turnNotifier = NopTurnCompletionNotifier{}
	}
	if repository == nil {
		repository = &fileSessionItemsRepository{}
	}
	return &claudeLiveSessionFactory{
		manager:      manager,
		stores:       stores,
		repository:   repository,
		turnNotifier: turnNotifier,
		logger:       logger,
	}
}

func (f *claudeLiveSessionFactory) ProviderName() string {
	return "claude"
}

func (f *claudeLiveSessionFactory) CreateTurnCapable(_ context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	if f.manager == nil {
		return nil, unavailableError("session manager not available", nil)
	}

	orchestrator := claudeSendOrchestrator{
		validator:           defaultClaudeInputValidator{},
		transport:           claudeLiveSessionTransport{manager: f.manager, stores: f.stores},
		turnIDs:             defaultTurnIDGenerator{},
		stateStore:          claudeLiveSessionStateStore{stores: f.stores},
		completionReader:    claudeLiveSessionCompletionReader{repository: f.repository},
		completionPublisher: claudeLiveSessionCompletionPublisher{notifier: f.turnNotifier},
		completionPolicy:    defaultClaudeCompletionDecisionPolicy{strategy: claudeItemDeltaCompletionStrategy{}},
	}

	ls := &claudeLiveSession{
		sessionID:    session.ID,
		session:      cloneSessionShallow(session),
		meta:         cloneSessionMeta(meta),
		manager:      f.manager,
		stores:       f.stores,
		logger:       f.logger,
		orchestrator: orchestrator,
		failureReporter: defaultClaudeTurnFailureReporter{
			sessionID:    session.ID,
			providerName: "claude",
			logger:       f.logger,
			debugWriter:  f.manager,
			repository:   f.repository,
			notifier:     f.turnNotifier,
		},
	}
	return ls, nil
}

// claudeLiveSessionTransport implements claudeSendTransport using
// SessionManager directly, without requiring a SessionService.
type claudeLiveSessionTransport struct {
	manager *SessionManager
	stores  *Stores
}

func (t claudeLiveSessionTransport) Send(
	_ context.Context,
	_ claudeSendContext,
	session *types.Session,
	meta *types.SessionMeta,
	payload []byte,
	runtimeOptions *types.SessionRuntimeOptions,
) error {
	if t.manager == nil {
		return unavailableError("session manager not available", nil)
	}
	if session == nil {
		return invalidError("session is required", nil)
	}
	if err := t.manager.SendInput(session.ID, payload); err != nil {
		if !errors.Is(err, ErrSessionNotFound) {
			return invalidError(err.Error(), err)
		}
		providerSessionID := ""
		if meta != nil {
			providerSessionID = meta.ProviderSessionID
		}
		if strings.TrimSpace(providerSessionID) == "" {
			return invalidError("provider session id not available", nil)
		}
		if strings.TrimSpace(session.Cwd) == "" {
			return invalidError("session cwd is required", nil)
		}
		_, resumeErr := t.manager.ResumeSession(StartSessionConfig{
			Provider:          session.Provider,
			Cwd:               session.Cwd,
			Env:               session.Env,
			RuntimeOptions:    runtimeOptions,
			Resume:            true,
			ProviderSessionID: providerSessionID,
		}, session)
		if resumeErr != nil {
			return invalidError(resumeErr.Error(), resumeErr)
		}
		if err := t.manager.SendInput(session.ID, payload); err != nil {
			return invalidError(err.Error(), err)
		}
	}
	return nil
}

// claudeLiveSessionStateStore implements claudeTurnStateStore using Stores directly.
type claudeLiveSessionStateStore struct {
	stores *Stores
}

func (s claudeLiveSessionStateStore) SaveTurnState(ctx context.Context, sessionID, turnID string) {
	if s.stores == nil || s.stores.SessionMeta == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()
	_, _ = s.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
		SessionID:    sessionID,
		LastTurnID:   turnID,
		LastActiveAt: &now,
	})
}

// claudeLiveSessionCompletionReader adapts TurnArtifactRepository to the
// claudeCompletionReader interface expected by claudeSendOrchestrator.
type claudeLiveSessionCompletionReader struct {
	repository TurnArtifactRepository
}

func (r claudeLiveSessionCompletionReader) ReadSessionItems(sessionID string, lines int) ([]map[string]any, error) {
	if r.repository == nil || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	return r.repository.ReadItems(sessionID, lines)
}

// claudeLiveSessionCompletionPublisher adapts TurnCompletionNotifier to the
// claudeTurnCompletionPublisher interface expected by claudeSendOrchestrator.
type claudeLiveSessionCompletionPublisher struct {
	notifier TurnCompletionNotifier
}

func (p claudeLiveSessionCompletionPublisher) PublishTurnCompleted(session *types.Session, meta *types.SessionMeta, turnID, source string) {
	if p.notifier == nil || session == nil {
		return
	}
	p.notifier.NotifyTurnCompletedEvent(context.Background(), TurnCompletionEvent{
		SessionID: strings.TrimSpace(session.ID),
		TurnID:    strings.TrimSpace(turnID),
		Provider:  strings.TrimSpace(session.Provider),
		Source:    strings.TrimSpace(source),
	})
}
