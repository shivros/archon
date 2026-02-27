package daemon

import (
	"context"
	"errors"
	"strings"
	"time"

	"control/internal/types"
)

type claudeInputValidator interface {
	TextFromInput(input []map[string]any) (string, error)
}

type defaultClaudeInputValidator struct{}

func (defaultClaudeInputValidator) TextFromInput(input []map[string]any) (string, error) {
	text := extractTextInput(input)
	if strings.TrimSpace(text) == "" {
		return "", invalidError("text input is required", nil)
	}
	return text, nil
}

type claudeSendTransport interface {
	Send(
		ctx context.Context,
		sendCtx claudeSendContext,
		session *types.Session,
		meta *types.SessionMeta,
		payload []byte,
		runtimeOptions *types.SessionRuntimeOptions,
	) error
}

type claudeSessionManager interface {
	SendInput(sessionID string, payload []byte) error
	ResumeSession(cfg StartSessionConfig, existing *types.Session) (*types.Session, error)
}

type claudeSendContext struct {
	Manager                      claudeSessionManager
	ResolveAdditionalDirectories func(ctx context.Context, session *types.Session, meta *types.SessionMeta) ([]string, error)
}

type defaultClaudeSendTransport struct{}

func (defaultClaudeSendTransport) Send(
	ctx context.Context,
	sendCtx claudeSendContext,
	session *types.Session,
	meta *types.SessionMeta,
	payload []byte,
	runtimeOptions *types.SessionRuntimeOptions,
) error {
	if sendCtx.Manager == nil {
		return unavailableError("session manager not available", nil)
	}
	if session == nil {
		return invalidError("session is required", nil)
	}
	if err := sendCtx.Manager.SendInput(session.ID, payload); err != nil {
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
		additionalDirectories := []string{}
		if sendCtx.ResolveAdditionalDirectories != nil {
			var dirsErr error
			additionalDirectories, dirsErr = sendCtx.ResolveAdditionalDirectories(ctx, session, meta)
			if dirsErr != nil {
				return invalidError(dirsErr.Error(), dirsErr)
			}
		}
		_, resumeErr := sendCtx.Manager.ResumeSession(StartSessionConfig{
			Provider:              session.Provider,
			Cwd:                   session.Cwd,
			AdditionalDirectories: additionalDirectories,
			Env:                   session.Env,
			RuntimeOptions:        runtimeOptions,
			Resume:                true,
			ProviderSessionID:     providerSessionID,
		}, session)
		if resumeErr != nil {
			return invalidError(resumeErr.Error(), resumeErr)
		}
		if err := sendCtx.Manager.SendInput(session.ID, payload); err != nil {
			return invalidError(err.Error(), err)
		}
	}
	return nil
}

type claudeTurnStateStore interface {
	SaveTurnState(ctx context.Context, sessionID, turnID string)
}

type sessionServiceClaudeTurnStateStore struct {
	service *SessionService
}

func (s sessionServiceClaudeTurnStateStore) SaveTurnState(ctx context.Context, sessionID, turnID string) {
	if s.service == nil || s.service.stores == nil || s.service.stores.SessionMeta == nil {
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
	_, _ = s.service.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
		SessionID:    sessionID,
		LastTurnID:   turnID,
		LastActiveAt: &now,
	})
}

type claudeCompletionDecisionPolicy interface {
	Decide(beforeCount int, items []map[string]any, sendErr error) (publish bool, source string)
}

type defaultClaudeCompletionDecisionPolicy struct {
	strategy turnCompletionStrategy
}

func (p defaultClaudeCompletionDecisionPolicy) Decide(beforeCount int, items []map[string]any, sendErr error) (bool, string) {
	if sendErr != nil {
		return false, ""
	}
	strategy := p.strategy
	if strategy == nil {
		strategy = claudeItemDeltaCompletionStrategy{}
	}
	if strategy.ShouldPublishCompletion(beforeCount, items) {
		return true, strategy.Source()
	}
	return true, "claude_sync_send_completed"
}

type claudeSendOrchestrator struct {
	validator  claudeInputValidator
	transport  claudeSendTransport
	turnIDs    turnIDGenerator
	stateStore claudeTurnStateStore

	completionReader    claudeCompletionReader
	completionPublisher claudeTurnCompletionPublisher
	completionPolicy    claudeCompletionDecisionPolicy
}

type claudeCompletionReader interface {
	ReadSessionItems(sessionID string, lines int) ([]map[string]any, error)
}

type claudeTurnCompletionPublisher interface {
	PublishTurnCompleted(session *types.Session, meta *types.SessionMeta, turnID, source string)
}

func (o claudeSendOrchestrator) Send(
	ctx context.Context,
	sendCtx claudeSendContext,
	session *types.Session,
	meta *types.SessionMeta,
	input []map[string]any,
) (string, error) {
	if session == nil {
		return "", invalidError("session is required", nil)
	}
	if o.transport == nil && sendCtx.Manager == nil {
		return "", unavailableError("session manager not available", nil)
	}
	validator := o.validator
	if validator == nil {
		validator = defaultClaudeInputValidator{}
	}
	text, err := validator.TextFromInput(input)
	if err != nil {
		return "", err
	}
	turnIDGen := o.turnIDs
	if turnIDGen == nil {
		turnIDGen = defaultTurnIDGenerator{}
	}
	turnID := strings.TrimSpace(turnIDGen.NewTurnID(session.Provider))
	if turnID == "" {
		return "", unavailableError("turn id generator failed", nil)
	}
	runtimeOptions := (*types.SessionRuntimeOptions)(nil)
	if meta != nil {
		runtimeOptions = types.CloneRuntimeOptions(meta.RuntimeOptions)
	}
	payload := buildClaudeUserPayloadWithRuntime(text, runtimeOptions)
	preSendCount := claudeCompletionProbeItemCount(o.completionReader, session.ID)
	transport := o.transport
	if transport == nil {
		transport = defaultClaudeSendTransport{}
	}
	if err := transport.Send(ctx, sendCtx, session, meta, payload, runtimeOptions); err != nil {
		return "", err
	}
	if o.stateStore != nil {
		o.stateStore.SaveTurnState(ctx, session.ID, turnID)
	}
	items, _ := readClaudeCompletionItems(o.completionReader, session.ID)
	policy := o.completionPolicy
	if policy == nil {
		policy = defaultClaudeCompletionDecisionPolicy{strategy: claudeItemDeltaCompletionStrategy{}}
	}
	publish, source := policy.Decide(preSendCount, items, nil)
	if publish && o.completionPublisher != nil {
		o.completionPublisher.PublishTurnCompleted(session, meta, turnID, strings.TrimSpace(source))
	}
	return turnID, nil
}

func readClaudeCompletionItems(reader claudeCompletionReader, sessionID string) ([]map[string]any, error) {
	if reader == nil || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}
	items, err := reader.ReadSessionItems(sessionID, 10_000)
	if err != nil {
		return nil, err
	}
	return items, nil
}

type turnCompletionStrategy interface {
	ShouldPublishCompletion(beforeCount int, items []map[string]any) bool
	Source() string
}

type claudeItemDeltaCompletionStrategy struct{}

func (claudeItemDeltaCompletionStrategy) Source() string {
	return "claude_items_post_send"
}

func (claudeItemDeltaCompletionStrategy) ShouldPublishCompletion(beforeCount int, items []map[string]any) bool {
	if beforeCount < 0 {
		beforeCount = 0
	}
	if beforeCount > len(items) {
		beforeCount = len(items)
	}
	for _, item := range items[beforeCount:] {
		if claudeCompletionItemSignalsTurnCompletion(item) {
			return true
		}
	}
	return false
}

func claudeCompletionItemSignalsTurnCompletion(item map[string]any) bool {
	if item == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(asString(item["type"]))) {
	case "agentmessage", "agentmessagedelta", "agentmessageend", "assistant", "reasoning", "result":
		return true
	default:
		return false
	}
}

func claudeCompletionProbeItemCount(io claudeCompletionReader, sessionID string) int {
	if io == nil || strings.TrimSpace(sessionID) == "" {
		return 0
	}
	items, err := io.ReadSessionItems(sessionID, 10_000)
	if err != nil {
		return 0
	}
	return len(items)
}

func claudeCompletionProbeHasTerminalOutput(io claudeCompletionReader, strategy turnCompletionStrategy, sessionID string, baselineCount int) bool {
	if io == nil || strategy == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	if baselineCount < 0 {
		baselineCount = 0
	}
	items, err := io.ReadSessionItems(sessionID, 10_000)
	if err != nil || len(items) == 0 {
		return false
	}
	return strategy.ShouldPublishCompletion(baselineCount, items)
}
