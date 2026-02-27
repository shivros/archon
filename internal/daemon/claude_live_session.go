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
	mu           sync.Mutex
	sessionID    string
	session      *types.Session
	meta         *types.SessionMeta
	manager      *SessionManager
	stores       *Stores
	orchestrator claudeSendOrchestrator
	activeTurn   string
	closed       bool
}

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

	// The orchestrator's Send takes *SessionService, but our custom transport
	// and state store implementations don't use it — pass nil.
	turnID, err := s.orchestrator.Send(ctx, nil, session, meta, input)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.activeTurn = turnID
	s.mu.Unlock()

	return turnID, nil
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
	defer s.mu.Unlock()
	s.closed = true
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
		orchestrator: orchestrator,
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
	_ *SessionService,
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
