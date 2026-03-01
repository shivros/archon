package daemon

import (
	"context"
	"errors"
	"strings"
	"sync"

	"control/internal/logging"
	"control/internal/providers"
	"control/internal/types"
)

type CompositeLiveManager struct {
	mu        sync.Mutex
	factories map[string]TurnCapableSessionFactory
	sessions  map[string]TurnCapableSession
	stores    *Stores
	logger    logging.Logger
	notifier  NotificationPublisher
}

type managedTurnStarter interface {
	StartTurnForSession(ctx context.Context, session *types.Session, meta *types.SessionMeta, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error)
}

type closeAwareSession interface {
	IsClosed() bool
}

var _ LiveManager = (*CompositeLiveManager)(nil)

func NewCompositeLiveManager(stores *Stores, logger logging.Logger, factories ...TurnCapableSessionFactory) *CompositeLiveManager {
	if logger == nil {
		logger = logging.Nop()
	}
	factoryMap := make(map[string]TurnCapableSessionFactory)
	for _, f := range factories {
		if f != nil {
			factoryMap[providers.Normalize(f.ProviderName())] = f
		}
	}
	return &CompositeLiveManager{
		factories: factoryMap,
		sessions:  make(map[string]TurnCapableSession),
		stores:    stores,
		logger:    logger,
	}
}

func (m *CompositeLiveManager) SetNotificationPublisher(notifier NotificationPublisher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifier = notifier
	for _, s := range m.sessions {
		if ns, ok := s.(NotifiableSession); ok {
			ns.SetNotificationPublisher(notifier)
		}
	}
}

func (m *CompositeLiveManager) StartTurn(ctx context.Context, session *types.Session, meta *types.SessionMeta, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error) {
	if session == nil {
		return "", errors.New("session is required")
	}

	ls, err := m.ensure(ctx, session, meta)
	if err != nil {
		return "", err
	}
	if managed, ok := ls.(managedTurnStarter); ok {
		return managed.StartTurnForSession(ctx, session, meta, input, opts)
	}

	return ls.StartTurn(ctx, input, opts)
}

func (m *CompositeLiveManager) Subscribe(session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	if session == nil {
		return nil, nil, errors.New("session is required")
	}

	ls, err := m.ensure(context.Background(), session, meta)
	if err != nil {
		return nil, nil, err
	}

	ch, cancel := ls.Events()
	return ch, cancel, nil
}

func (m *CompositeLiveManager) Respond(ctx context.Context, session *types.Session, meta *types.SessionMeta, requestID int, result map[string]any) error {
	if session == nil {
		return errors.New("session is required")
	}

	ls, err := m.ensure(ctx, session, meta)
	if err != nil {
		return err
	}

	if als, ok := ls.(ApprovalCapableSession); ok {
		return als.Respond(ctx, requestID, result)
	}

	return errors.New("provider does not support approval responses via live manager")
}

func (m *CompositeLiveManager) Interrupt(ctx context.Context, session *types.Session, meta *types.SessionMeta) error {
	if session == nil {
		return errors.New("session is required")
	}

	ls, err := m.ensure(ctx, session, meta)
	if err != nil {
		return err
	}

	return ls.Interrupt(ctx)
}

func (m *CompositeLiveManager) ensure(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ls := m.sessions[session.ID]; ls != nil {
		if closeAware, ok := ls.(closeAwareSession); ok && closeAware.IsClosed() {
			delete(m.sessions, session.ID)
		} else {
			if withMeta, ok := ls.(interface{ SetSessionMeta(*types.SessionMeta) }); ok {
				withMeta.SetSessionMeta(meta)
			}
			return ls, nil
		}
	}

	provider := providers.Normalize(session.Provider)
	factory := m.factories[provider]
	if factory == nil {
		return nil, errors.New("provider does not support live sessions")
	}

	ls, err := factory.CreateTurnCapable(ctx, session, meta)
	if err != nil {
		return nil, err
	}
	if withMeta, ok := ls.(interface{ SetSessionMeta(*types.SessionMeta) }); ok {
		withMeta.SetSessionMeta(meta)
	}

	if ns, ok := ls.(NotifiableSession); ok && m.notifier != nil {
		ns.SetNotificationPublisher(m.notifier)
	}

	m.sessions[session.ID] = ls
	return ls, nil
}

func (m *CompositeLiveManager) Drop(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	m.mu.Lock()
	ls := m.sessions[sessionID]
	delete(m.sessions, sessionID)
	m.mu.Unlock()
	if ls != nil {
		ls.Close()
	}
}

type codexLiveSessionFactory struct {
	manager *CodexLiveManager
}

func newCodexLiveSessionFactory(manager *CodexLiveManager) *codexLiveSessionFactory {
	return &codexLiveSessionFactory{manager: manager}
}

func (f *codexLiveSessionFactory) ProviderName() string {
	return "codex"
}

func (f *codexLiveSessionFactory) CreateTurnCapable(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
	if f.manager == nil {
		return nil, errors.New("codex live manager not initialized")
	}
	if session == nil {
		return nil, errors.New("session is required")
	}
	if session.Provider != "codex" {
		return nil, errors.New("provider does not support codex live sessions")
	}

	workspacePath := resolveWorkspacePathFromMeta(meta)
	codexHome := resolveCodexHome(session.Cwd, workspacePath)

	wrapped := &codexManagedSession{
		manager:   f.manager,
		session:   cloneSessionShallow(session),
		codexHome: codexHome,
	}
	wrapped.SetSessionMeta(meta)
	if _, err := wrapped.ensureLive(ctx); err != nil {
		return nil, err
	}
	return wrapped, nil
}

func resolveWorkspacePathFromMeta(meta *types.SessionMeta) string {
	if meta == nil {
		return ""
	}
	return strings.TrimSpace(meta.WorkspaceID)
}

type codexManagedSession struct {
	mu        sync.Mutex
	manager   *CodexLiveManager
	session   *types.Session
	meta      *types.SessionMeta
	codexHome string
	live      *codexLiveSession
}

var (
	_ TurnCapableSession     = (*codexManagedSession)(nil)
	_ ApprovalCapableSession = (*codexManagedSession)(nil)
	_ managedTurnStarter     = (*codexManagedSession)(nil)
	_ NotifiableSession      = (*codexManagedSession)(nil)
	_ closeAwareSession      = (*codexManagedSession)(nil)
)

func (s *codexManagedSession) SetSessionMeta(meta *types.SessionMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.meta = cloneSessionMeta(meta)
}

func (s *codexManagedSession) StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error) {
	if s == nil || s.session == nil || s.manager == nil {
		return "", errors.New("codex session is not initialized")
	}
	meta := s.currentMeta()
	return s.manager.StartTurn(ctx, s.session, meta, s.codexHome, input, opts)
}

func (s *codexManagedSession) StartTurnForSession(ctx context.Context, session *types.Session, meta *types.SessionMeta, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error) {
	if s == nil || s.manager == nil {
		return "", errors.New("codex session is not initialized")
	}
	if session != nil {
		s.mu.Lock()
		s.session = cloneSessionShallow(session)
		s.mu.Unlock()
	}
	s.SetSessionMeta(meta)
	return s.StartTurn(ctx, input, opts)
}

func (s *codexManagedSession) Interrupt(ctx context.Context) error {
	if s == nil || s.session == nil || s.manager == nil {
		return errors.New("codex session is not initialized")
	}
	meta := s.currentMeta()
	return s.manager.Interrupt(ctx, s.session, meta, s.codexHome)
}

func (s *codexManagedSession) Respond(ctx context.Context, requestID int, result map[string]any) error {
	if s == nil || s.session == nil || s.manager == nil {
		return errors.New("codex session is not initialized")
	}
	meta := s.currentMeta()
	return s.manager.Respond(ctx, s.session, meta, s.codexHome, requestID, result)
}

func (s *codexManagedSession) ActiveTurnID() string {
	ls, err := s.ensureLive(context.Background())
	if err != nil || ls == nil {
		return ""
	}
	return ls.ActiveTurnID()
}

func (s *codexManagedSession) Events() (<-chan types.CodexEvent, func()) {
	ls, err := s.ensureLive(context.Background())
	if err != nil || ls == nil {
		ch := make(chan types.CodexEvent)
		close(ch)
		return ch, func() {}
	}
	ch, cancel := ls.Events()
	wrappedCancel := func() {
		cancel()
		ls.maybeClose()
	}
	return ch, wrappedCancel
}

func (s *codexManagedSession) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	sessionID := ""
	if s.session != nil {
		sessionID = strings.TrimSpace(s.session.ID)
	}
	manager := s.manager
	s.mu.Unlock()
	if manager != nil && sessionID != "" {
		manager.dropSession(sessionID)
	}
}

func (s *codexManagedSession) SessionID() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil {
		return ""
	}
	return s.session.ID
}

func (s *codexManagedSession) SetNotificationPublisher(notifier NotificationPublisher) {
	if s == nil {
		return
	}
	s.mu.Lock()
	sess := s.session
	meta := cloneSessionMeta(s.meta)
	codexHome := s.codexHome
	manager := s.manager
	s.mu.Unlock()
	if manager == nil || sess == nil {
		return
	}
	manager.SetNotificationPublisher(notifier)
	ls, err := manager.ensure(context.Background(), sess, meta, codexHome, false)
	if err != nil || ls == nil {
		return
	}
	ls.SetNotificationPublisher(notifier)
	s.mu.Lock()
	s.live = ls
	s.mu.Unlock()
}

func (s *codexManagedSession) IsClosed() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	live := s.live
	s.mu.Unlock()
	if live == nil {
		return false
	}
	return live.isClosed()
}

func (s *codexManagedSession) ensureLive(ctx context.Context) (*codexLiveSession, error) {
	if s == nil || s.manager == nil || s.session == nil {
		return nil, errors.New("codex session is not initialized")
	}
	meta := s.currentMeta()
	ls, err := s.manager.ensure(ctx, s.session, meta, s.codexHome, true)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.live = ls
	s.mu.Unlock()
	return ls, nil
}

func (s *codexManagedSession) currentMeta() *types.SessionMeta {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSessionMeta(s.meta)
}

func cloneSessionMeta(meta *types.SessionMeta) *types.SessionMeta {
	if meta == nil {
		return nil
	}
	copy := *meta
	copy.RuntimeOptions = types.CloneRuntimeOptions(meta.RuntimeOptions)
	return &copy
}

func cloneSessionShallow(session *types.Session) *types.Session {
	if session == nil {
		return nil
	}
	copy := *session
	return &copy
}

// NewIntegrationLiveManager creates a CompositeLiveManager with factories for
// all built-in providers (codex, claude, opencode, kilocode). This is exported
// for use by integration tests in other packages (e.g. internal/app).
func NewIntegrationLiveManager(stores *Stores, manager *SessionManager, codex *CodexLiveManager, logger logging.Logger) *CompositeLiveManager {
	artifactRepository := newFileSessionItemsRepository(manager)
	return NewCompositeLiveManager(stores, logger,
		newCodexLiveSessionFactory(codex),
		newClaudeLiveSessionFactory(manager, stores, nil, nil, logger),
		newOpenCodeLiveSessionFactory("opencode", nil, nil, artifactRepository, nil, nil, logger),
		newOpenCodeLiveSessionFactory("kilocode", nil, nil, artifactRepository, nil, nil, logger),
	)
}
