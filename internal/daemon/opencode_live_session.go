package daemon

import (
	"context"
	"sync"

	"control/internal/logging"
	"control/internal/types"
)

type openCodeLiveSession struct {
	mu            sync.Mutex
	sessionID     string
	providerName  string
	providerID    string
	directory     string
	client        *openCodeClient
	events        <-chan types.CodexEvent
	cancelEvents  func()
	hub           *codexSubscriberHub
	turnNotifier  TurnCompletionNotifier
	approvalStore ApprovalStorage
	activeTurn    string
	closed        bool
}

var (
	_ LiveSession        = (*openCodeLiveSession)(nil)
	_ TurnCapableSession = (*openCodeLiveSession)(nil)
	_ NotifiableSession  = (*openCodeLiveSession)(nil)
)

func (s *openCodeLiveSession) Events() (<-chan types.CodexEvent, func()) {
	return s.hub.Add()
}

func (s *openCodeLiveSession) SessionID() string {
	return s.sessionID
}

func (s *openCodeLiveSession) ActiveTurnID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeTurn
}

func (s *openCodeLiveSession) StartTurn(ctx context.Context, input []map[string]any, opts *types.SessionRuntimeOptions) (string, error) {
	text := extractTextInput(input)
	if text == "" {
		return "", invalidError("text input is required", nil)
	}

	turnID := generateTurnID()
	s.mu.Lock()
	s.activeTurn = turnID
	s.mu.Unlock()

	_, err := s.client.Prompt(ctx, s.providerID, text, opts, s.directory)
	if err != nil {
		s.mu.Lock()
		s.activeTurn = ""
		s.mu.Unlock()
		return "", err
	}

	return turnID, nil
}

func (s *openCodeLiveSession) Interrupt(ctx context.Context) error {
	return s.client.AbortSession(ctx, s.providerID, s.directory)
}

func (s *openCodeLiveSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	if s.cancelEvents != nil {
		s.cancelEvents()
	}
}

func (s *openCodeLiveSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *openCodeLiveSession) SetNotificationPublisher(notifier NotificationPublisher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turnNotifier != nil {
		if dn, ok := s.turnNotifier.(*DefaultTurnCompletionNotifier); ok {
			dn.notifier = notifier
		}
	}
}

func (s *openCodeLiveSession) start() {
	go func() {
		defer s.Close()
		for event := range s.events {
			s.hub.Broadcast(event)

			if event.Method == "turn/completed" || event.Method == "session.idle" {
				turnID := parseTurnIDFromEventParams(event.Params)
				if turnID == "" {
					s.mu.Lock()
					turnID = s.activeTurn
					s.activeTurn = ""
					s.mu.Unlock()
				} else {
					s.mu.Lock()
					s.activeTurn = ""
					s.mu.Unlock()
				}
				s.publishTurnCompleted(turnID)
			}

			if isApprovalMethod(event.Method) && event.ID != nil {
				s.storeApproval(event)
			}
		}
	}()
}

func (s *openCodeLiveSession) publishTurnCompleted(turnID string) {
	if s.turnNotifier == nil {
		return
	}
	s.turnNotifier.NotifyTurnCompleted(context.Background(), s.sessionID, turnID, s.providerName, nil)
}

func (s *openCodeLiveSession) storeApproval(event types.CodexEvent) {
	if s.approvalStore == nil || event.ID == nil {
		return
	}
	_ = s.approvalStore.StoreApproval(context.Background(), s.sessionID, *event.ID, event.Method, event.Params)
}

type openCodeLiveSessionFactory struct {
	providerName  string
	turnNotifier  TurnCompletionNotifier
	approvalStore ApprovalStorage
	logger        logging.Logger
}

func newOpenCodeLiveSessionFactory(providerName string, turnNotifier TurnCompletionNotifier, approvalStore ApprovalStorage, logger logging.Logger) *openCodeLiveSessionFactory {
	if logger == nil {
		logger = logging.Nop()
	}
	if turnNotifier == nil {
		turnNotifier = NopTurnCompletionNotifier{}
	}
	if approvalStore == nil {
		approvalStore = NopApprovalStorage{}
	}
	return &openCodeLiveSessionFactory{
		providerName:  providerName,
		turnNotifier:  turnNotifier,
		approvalStore: approvalStore,
		logger:        logger,
	}
}

func (f *openCodeLiveSessionFactory) ProviderName() string {
	return f.providerName
}

func (f *openCodeLiveSessionFactory) CreateTurnCapable(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}

	providerID := ""
	if meta != nil {
		providerID = meta.ProviderSessionID
	}
	if providerID == "" {
		return nil, invalidError("provider session id not available", nil)
	}

	client, err := newOpenCodeClient(resolveOpenCodeClientConfig(f.providerName, loadCoreConfigOrDefault()))
	if err != nil {
		return nil, err
	}

	events, cancel, err := client.SubscribeSessionEvents(ctx, providerID, session.Cwd)
	if err != nil && session.Cwd != "" {
		events, cancel, err = client.SubscribeSessionEvents(ctx, providerID, "")
	}
	if err != nil {
		return nil, err
	}

	ls := &openCodeLiveSession{
		sessionID:     session.ID,
		providerName:  f.providerName,
		providerID:    providerID,
		directory:     session.Cwd,
		client:        client,
		events:        events,
		cancelEvents:  cancel,
		hub:           newCodexSubscriberHub(),
		turnNotifier:  f.turnNotifier,
		approvalStore: f.approvalStore,
	}
	ls.start()

	return ls, nil
}

func generateTurnID() string {
	return "turn_" + logging.NewRequestID()
}
