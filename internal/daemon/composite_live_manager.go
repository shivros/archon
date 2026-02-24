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

	ch := ls.Events()
	return ch, func() { ls.Close() }, nil
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
		return ls, nil
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

	ls, err := f.manager.ensure(ctx, session, meta, codexHome)
	if err != nil {
		return nil, err
	}

	return ls, nil
}

func resolveWorkspacePathFromMeta(meta *types.SessionMeta) string {
	if meta == nil {
		return ""
	}
	return strings.TrimSpace(meta.WorkspaceID)
}
