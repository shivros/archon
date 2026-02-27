package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
	artifactSync  TurnArtifactSynchronizer
	payloads      TurnCompletionPayloadBuilder
	activeTurn    string
	closed        bool
}

var (
	_ LiveSession            = (*openCodeLiveSession)(nil)
	_ TurnCapableSession     = (*openCodeLiveSession)(nil)
	_ ApprovalCapableSession = (*openCodeLiveSession)(nil)
	_ NotifiableSession      = (*openCodeLiveSession)(nil)
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
		if errors.Is(err, errOpenCodePromptPending) {
			// The upstream may continue processing after client timeout; keep
			// the active turn open so completion can arrive through events/recovery.
			return turnID, nil
		}
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

func (s *openCodeLiveSession) Respond(ctx context.Context, requestID int, result map[string]any) error {
	if s.approvalStore == nil {
		return invalidError("approval store not available", nil)
	}
	record, ok, err := s.approvalStore.GetApproval(ctx, s.sessionID, requestID)
	if err != nil {
		return unavailableError(err.Error(), err)
	}
	if !ok || record == nil {
		return notFoundError("approval not found", nil)
	}
	params := map[string]any{}
	if len(record.Params) > 0 {
		_ = json.Unmarshal(record.Params, &params)
	}
	permissionID := strings.TrimSpace(asString(params["permission_id"]))
	if permissionID == "" {
		permissionID = strings.TrimSpace(asString(params["permissionID"]))
	}
	if permissionID == "" {
		return invalidError("provider permission id not available", nil)
	}
	decision := asString(result["decision"])
	var responses []string
	if raw, ok := result["responses"]; ok {
		if arr, ok := raw.([]string); ok {
			responses = arr
		}
	}
	if err := s.client.ReplyPermission(ctx, s.providerID, permissionID, decision, responses, s.directory); err != nil {
		return invalidError(err.Error(), err)
	}
	_ = s.approvalStore.DeleteApproval(ctx, s.sessionID, requestID)
	return nil
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
	if aware, ok := s.turnNotifier.(TurnCompletionNotificationPublisherAware); ok {
		aware.SetNotificationPublisher(notifier)
	}
}

func (s *openCodeLiveSession) start() {
	go func() {
		defer s.Close()
		for event := range s.events {
			s.hub.Broadcast(event)

			if event.Method == "turn/completed" || event.Method == "session.idle" {
				turn := parseTurnEventFromParams(event.Params)
				turnID := turn.TurnID
				if turnID == "" {
					s.mu.Lock()
					turnID = s.activeTurn
					s.activeTurn = ""
					s.mu.Unlock()
					turn.TurnID = turnID
				} else {
					s.mu.Lock()
					s.activeTurn = ""
					s.mu.Unlock()
				}
				s.publishTurnCompleted(turn)
			}

			if isApprovalMethod(event.Method) && event.ID != nil {
				s.storeApproval(event)
			}
		}
	}()
}

func (s *openCodeLiveSession) publishTurnCompleted(turn turnEventParams) {
	if s.turnNotifier == nil {
		return
	}
	syncResult := TurnArtifactSyncResult{}
	if s.artifactSync != nil {
		syncResult = s.artifactSync.SyncTurnArtifacts(context.Background(), turn)
	}
	output := strings.TrimSpace(syncResult.Output)
	payload := map[string]any{}
	if s.payloads != nil {
		output, payload = s.payloads.Build(turn, syncResult)
	} else {
		output, payload = defaultTurnCompletionPayloadBuilder{}.Build(turn, syncResult)
	}
	s.turnNotifier.NotifyTurnCompletedEvent(context.Background(), TurnCompletionEvent{
		SessionID: strings.TrimSpace(s.sessionID),
		TurnID:    strings.TrimSpace(turn.TurnID),
		Provider:  strings.TrimSpace(s.providerName),
		Source:    "live_session_event",
		Status:    strings.TrimSpace(turn.Status),
		Error:     strings.TrimSpace(turn.Error),
		Output:    strings.TrimSpace(output),
		Payload:   payload,
	})
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
	repository    TurnArtifactRepository
	payloads      TurnCompletionPayloadBuilder
	logger        logging.Logger
}

func newOpenCodeLiveSessionFactory(
	providerName string,
	turnNotifier TurnCompletionNotifier,
	approvalStore ApprovalStorage,
	repository TurnArtifactRepository,
	payloads TurnCompletionPayloadBuilder,
	logger logging.Logger,
) *openCodeLiveSessionFactory {
	if logger == nil {
		logger = logging.Nop()
	}
	if turnNotifier == nil {
		turnNotifier = NopTurnCompletionNotifier{}
	}
	if approvalStore == nil {
		approvalStore = NopApprovalStorage{}
	}
	if repository == nil {
		repository = &fileSessionItemsRepository{}
	}
	if payloads == nil {
		payloads = defaultTurnCompletionPayloadBuilder{}
	}
	return &openCodeLiveSessionFactory{
		providerName:  providerName,
		turnNotifier:  turnNotifier,
		approvalStore: approvalStore,
		repository:    repository,
		payloads:      payloads,
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
		artifactSync: newOpenCodeTurnArtifactSynchronizer(
			session.ID,
			providerID,
			session.Cwd,
			openCodeTurnArtifactRemoteSource{client: client},
			f.repository,
		),
		payloads: f.payloads,
	}
	ls.start()

	return ls, nil
}

func generateTurnID() string {
	return "turn_" + logging.NewRequestID()
}
