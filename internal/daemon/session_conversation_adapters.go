package daemon

import (
	"context"
	"time"

	"control/internal/logging"
	"control/internal/providers"
	"control/internal/types"
)

type conversationProvider interface {
	Provider() string
}

type sendDeps struct {
	liveManager      LiveManager
	sessionMetaStore SessionMetaStore
}

type historyDeps struct {
	manager            *SessionManager
	logger             logging.Logger
	readSessionItems   func(sessionID string, lines int) ([]map[string]any, error)
	readSessionLogs    func(sessionID string, lines int) ([]string, error)
	appendSessionItems func(sessionID string, items []map[string]any) error
	tailCodexThread    func(ctx context.Context, session *types.Session, threadID string, lines int) ([]map[string]any, error)
}

type eventDeps struct {
	liveManager LiveManager
}

type approvalDeps struct {
	liveManager      LiveManager
	approvalStore    ApprovalStore
	sessionMetaStore SessionMetaStore
}

type interruptDeps struct {
	liveManager LiveManager
}

type conversationSender interface {
	conversationProvider
	SendMessage(ctx context.Context, deps sendDeps, session *types.Session, meta *types.SessionMeta, input []map[string]any) (string, error)
}

type conversationHistoryReader interface {
	conversationProvider
	History(ctx context.Context, deps historyDeps, session *types.Session, meta *types.SessionMeta, lines int) ([]map[string]any, error)
}

type conversationEventSubscriber interface {
	conversationProvider
	SubscribeEvents(ctx context.Context, deps eventDeps, session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error)
}

type conversationApprover interface {
	conversationProvider
	Approve(ctx context.Context, deps approvalDeps, session *types.Session, meta *types.SessionMeta, requestID int, decision string, responses []string, acceptSettings map[string]any) error
}

type conversationInterrupter interface {
	conversationProvider
	Interrupt(ctx context.Context, deps interruptDeps, session *types.Session, meta *types.SessionMeta) error
}

type conversationAdapterRegistry struct {
	fallbackSender      conversationSender
	fallbackHistory     conversationHistoryReader
	fallbackSubscriber  conversationEventSubscriber
	fallbackApprover    conversationApprover
	fallbackInterrupter conversationInterrupter

	senders      map[string]conversationSender
	history      map[string]conversationHistoryReader
	subscribers  map[string]conversationEventSubscriber
	approvers    map[string]conversationApprover
	interrupters map[string]conversationInterrupter
}

func newConversationAdapterRegistry(extra ...conversationProvider) *conversationAdapterRegistry {
	fallbackSender := unsupportedConversationSender{}
	fallbackHistory := defaultHistoryReader{}
	fallbackSubscriber := unsupportedConversationEventSubscriber{}
	fallbackApprover := unsupportedConversationApprover{}
	fallbackInterrupter := unsupportedConversationInterrupter{}
	registry := &conversationAdapterRegistry{
		fallbackSender:      fallbackSender,
		fallbackHistory:     fallbackHistory,
		fallbackSubscriber:  fallbackSubscriber,
		fallbackApprover:    fallbackApprover,
		fallbackInterrupter: fallbackInterrupter,
		senders:             map[string]conversationSender{},
		history:             map[string]conversationHistoryReader{},
		subscribers:         map[string]conversationEventSubscriber{},
		approvers:           map[string]conversationApprover{},
		interrupters:        map[string]conversationInterrupter{},
	}

	for _, def := range providers.All() {
		name := providers.Normalize(def.Name)
		if name == "" {
			continue
		}
		sender, historyReader, subscriber, approver, interrupter := defaultConversationPortsFor(def, fallbackHistory)
		if sender != nil {
			registry.senders[name] = sender
		}
		if historyReader != nil {
			registry.history[name] = historyReader
		}
		if subscriber != nil {
			registry.subscribers[name] = subscriber
		}
		if approver != nil {
			registry.approvers[name] = approver
		}
		if interrupter != nil {
			registry.interrupters[name] = interrupter
		}
	}

	for _, provider := range extra {
		if provider == nil {
			continue
		}
		name := providers.Normalize(provider.Provider())
		if name == "" {
			continue
		}
		if sender, ok := provider.(conversationSender); ok {
			registry.senders[name] = sender
		}
		if historyReader, ok := provider.(conversationHistoryReader); ok {
			registry.history[name] = historyReader
		}
		if subscriber, ok := provider.(conversationEventSubscriber); ok {
			registry.subscribers[name] = subscriber
		}
		if approver, ok := provider.(conversationApprover); ok {
			registry.approvers[name] = approver
		}
		if interrupter, ok := provider.(conversationInterrupter); ok {
			registry.interrupters[name] = interrupter
		}
	}

	return registry
}

func defaultConversationPortsFor(
	def providers.Definition,
	fallbackHistory defaultHistoryReader,
) (conversationSender, conversationHistoryReader, conversationEventSubscriber, conversationApprover, conversationInterrupter) {
	switch def.Runtime {
	case providers.RuntimeCodex:
		name := providers.Normalize(def.Name)
		live := liveManagerConversationSender{providerName: name}
		return live, codexHistoryReader{providerName: name, fallback: fallbackHistory}, liveManagerConversationEventSubscriber{providerName: name}, liveManagerConversationApprover{providerName: name}, liveManagerConversationInterrupter{providerName: name}
	case providers.RuntimeClaude:
		name := providers.Normalize(def.Name)
		live := liveManagerConversationSender{providerName: name}
		return live, fallbackHistory, liveManagerConversationEventSubscriber{providerName: name}, liveManagerConversationApprover{providerName: name}, liveManagerConversationInterrupter{providerName: name}
	case providers.RuntimeOpenCodeServer:
		name := providers.Normalize(def.Name)
		live := liveManagerConversationSender{providerName: name}
		return live, openCodeHistoryReader{providerName: name, fallback: fallbackHistory}, liveManagerConversationEventSubscriber{providerName: name}, liveManagerConversationApprover{providerName: name}, liveManagerConversationInterrupter{providerName: name}
	default:
		return nil, nil, nil, nil, nil
	}
}

func (r *conversationAdapterRegistry) senderFor(provider string) conversationSender {
	if r == nil {
		return unsupportedConversationSender{}
	}
	if sender, ok := r.senders[providers.Normalize(provider)]; ok && sender != nil {
		return sender
	}
	if r.fallbackSender != nil {
		return r.fallbackSender
	}
	return unsupportedConversationSender{}
}

func (r *conversationAdapterRegistry) historyFor(provider string) conversationHistoryReader {
	if r == nil {
		return defaultHistoryReader{}
	}
	if reader, ok := r.history[providers.Normalize(provider)]; ok && reader != nil {
		return reader
	}
	if r.fallbackHistory != nil {
		return r.fallbackHistory
	}
	return defaultHistoryReader{}
}

func (r *conversationAdapterRegistry) eventsFor(provider string) conversationEventSubscriber {
	if r == nil {
		return unsupportedConversationEventSubscriber{}
	}
	if sub, ok := r.subscribers[providers.Normalize(provider)]; ok && sub != nil {
		return sub
	}
	if r.fallbackSubscriber != nil {
		return r.fallbackSubscriber
	}
	return unsupportedConversationEventSubscriber{}
}

func (r *conversationAdapterRegistry) approverFor(provider string) conversationApprover {
	if r == nil {
		return unsupportedConversationApprover{}
	}
	if approver, ok := r.approvers[providers.Normalize(provider)]; ok && approver != nil {
		return approver
	}
	if r.fallbackApprover != nil {
		return r.fallbackApprover
	}
	return unsupportedConversationApprover{}
}

func (r *conversationAdapterRegistry) interrupterFor(provider string) conversationInterrupter {
	if r == nil {
		return unsupportedConversationInterrupter{}
	}
	if interrupter, ok := r.interrupters[providers.Normalize(provider)]; ok && interrupter != nil {
		return interrupter
	}
	if r.fallbackInterrupter != nil {
		return r.fallbackInterrupter
	}
	return unsupportedConversationInterrupter{}
}

type unsupportedConversationSender struct{}

func (unsupportedConversationSender) Provider() string { return "*" }

func (unsupportedConversationSender) SendMessage(context.Context, sendDeps, *types.Session, *types.SessionMeta, []map[string]any) (string, error) {
	return "", invalidError("provider does not support messaging", nil)
}

type unsupportedConversationEventSubscriber struct{}

func (unsupportedConversationEventSubscriber) Provider() string { return "*" }

func (unsupportedConversationEventSubscriber) SubscribeEvents(context.Context, eventDeps, *types.Session, *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	return nil, nil, invalidError("provider does not support events", nil)
}

type unsupportedConversationApprover struct{}

func (unsupportedConversationApprover) Provider() string { return "*" }

func (unsupportedConversationApprover) Approve(context.Context, approvalDeps, *types.Session, *types.SessionMeta, int, string, []string, map[string]any) error {
	return invalidError("provider does not support approvals", nil)
}

type unsupportedConversationInterrupter struct{}

func (unsupportedConversationInterrupter) Provider() string { return "*" }

func (unsupportedConversationInterrupter) Interrupt(context.Context, interruptDeps, *types.Session, *types.SessionMeta) error {
	return invalidError("provider does not support interrupt", nil)
}

type liveManagerConversationSender struct {
	providerName string
}

func (a liveManagerConversationSender) Provider() string {
	return a.providerName
}

func (a liveManagerConversationSender) SendMessage(ctx context.Context, deps sendDeps, session *types.Session, meta *types.SessionMeta, input []map[string]any) (string, error) {
	if session == nil {
		return "", invalidError("session is required", nil)
	}
	if deps.liveManager == nil {
		return "", unavailableError("live manager not available", nil)
	}
	runtimeOptions := (*types.SessionRuntimeOptions)(nil)
	if meta != nil {
		runtimeOptions = types.CloneRuntimeOptions(meta.RuntimeOptions)
	}
	turnID, err := deps.liveManager.StartTurn(ctx, session, meta, input, runtimeOptions)
	if err != nil {
		return "", invalidError(err.Error(), err)
	}
	if deps.sessionMetaStore != nil {
		now := time.Now().UTC()
		_, _ = deps.sessionMetaStore.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			LastTurnID:   turnID,
			LastActiveAt: &now,
		})
	}
	return turnID, nil
}

type liveManagerConversationEventSubscriber struct {
	providerName string
}

func (a liveManagerConversationEventSubscriber) Provider() string {
	return a.providerName
}

func (a liveManagerConversationEventSubscriber) SubscribeEvents(ctx context.Context, deps eventDeps, session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	if session == nil {
		return nil, nil, invalidError("session is required", nil)
	}
	if deps.liveManager == nil {
		return nil, nil, unavailableError("live manager not available", nil)
	}
	ch, cancel, err := deps.liveManager.Subscribe(session, meta)
	if err != nil {
		return nil, nil, invalidError(err.Error(), err)
	}
	return ch, cancel, nil
}

type liveManagerConversationApprover struct {
	providerName string
}

func (a liveManagerConversationApprover) Provider() string {
	return a.providerName
}

func (a liveManagerConversationApprover) Approve(
	ctx context.Context,
	deps approvalDeps,
	session *types.Session,
	meta *types.SessionMeta,
	requestID int,
	decision string,
	responses []string,
	acceptSettings map[string]any,
) error {
	if session == nil {
		return invalidError("session is required", nil)
	}
	if deps.liveManager == nil {
		return unavailableError("live manager not available", nil)
	}
	result := map[string]any{
		"decision": decision,
	}
	if len(responses) > 0 {
		result["responses"] = responses
	}
	if len(acceptSettings) > 0 {
		result["acceptSettings"] = acceptSettings
	}
	if err := deps.liveManager.Respond(ctx, session, meta, requestID, result); err != nil {
		return invalidError(err.Error(), err)
	}
	if deps.approvalStore != nil {
		_ = deps.approvalStore.Delete(ctx, session.ID, requestID)
	}
	if deps.sessionMetaStore != nil {
		now := time.Now().UTC()
		_, _ = deps.sessionMetaStore.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			LastActiveAt: &now,
		})
	}
	return nil
}

type liveManagerConversationInterrupter struct {
	providerName string
}

func (a liveManagerConversationInterrupter) Provider() string {
	return a.providerName
}

func (a liveManagerConversationInterrupter) Interrupt(ctx context.Context, deps interruptDeps, session *types.Session, meta *types.SessionMeta) error {
	if session == nil {
		return invalidError("session is required", nil)
	}
	if deps.liveManager == nil {
		return unavailableError("live manager not available", nil)
	}
	if err := deps.liveManager.Interrupt(ctx, session, meta); err != nil {
		return invalidError(err.Error(), err)
	}
	return nil
}

type defaultHistoryReader struct{}

func (defaultHistoryReader) Provider() string { return "*" }

func (defaultHistoryReader) History(_ context.Context, deps historyDeps, session *types.Session, _ *types.SessionMeta, lines int) ([]map[string]any, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	if providerUsesItems(session.Provider) && deps.readSessionItems != nil {
		if items, err := deps.readSessionItems(session.ID, lines); err == nil && items != nil {
			return items, nil
		}
	}
	if deps.manager != nil {
		if _, ok := deps.manager.GetSession(session.ID); ok {
			out, _, _, err := deps.manager.TailSession(session.ID, "combined", lines)
			if err == nil {
				return logLinesToItems(out), nil
			}
		}
	}
	if deps.readSessionLogs == nil {
		return nil, unavailableError("session history not available", nil)
	}
	out, err := deps.readSessionLogs(session.ID, lines)
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
	return logLinesToItems(out), nil
}

type codexHistoryReader struct {
	providerName string
	fallback     defaultHistoryReader
}

func (a codexHistoryReader) Provider() string { return a.providerName }

func (a codexHistoryReader) History(ctx context.Context, deps historyDeps, session *types.Session, meta *types.SessionMeta, lines int) ([]map[string]any, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	threadID := resolveThreadID(session, meta)
	if threadID != "" {
		if deps.tailCodexThread == nil {
			return nil, unavailableError("codex history not available", nil)
		}
		return deps.tailCodexThread(ctx, session, threadID, lines)
	}
	return a.fallback.History(ctx, deps, session, meta, lines)
}

type openCodeHistoryReader struct {
	providerName string
	fallback     defaultHistoryReader
}

func (a openCodeHistoryReader) Provider() string { return a.providerName }

func (a openCodeHistoryReader) History(ctx context.Context, deps historyDeps, session *types.Session, meta *types.SessionMeta, lines int) ([]map[string]any, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	opID := logging.NewRequestID()
	baseFields := append(openCodeSessionLogFields(session, meta), logging.F("op_id", opID), logging.F("lines", lines))
	if deps.logger != nil && deps.logger.Enabled(logging.Debug) {
		deps.logger.Debug("opencode_history_request", baseFields...)
	}
	reconciler := newOpenCodeHistoryReconciler(session, meta, openCodeHistoryReconcilerStore{
		readSessionItems:   deps.readSessionItems,
		appendSessionItems: deps.appendSessionItems,
	}, deps.logger)
	if syncResult, err := reconciler.Sync(ctx, lines); err == nil {
		if len(syncResult.items) > 0 {
			if deps.logger != nil && deps.logger.Enabled(logging.Debug) {
				deps.logger.Debug("opencode_history_remote_ok",
					append(baseFields, logging.F("items", len(syncResult.items)))...,
				)
			}
			return syncResult.items, nil
		}
		if deps.logger != nil && deps.logger.Enabled(logging.Debug) {
			deps.logger.Debug("opencode_history_remote_empty", baseFields...)
		}
	} else if deps.logger != nil {
		deps.logger.Warn("opencode_history_remote_failed",
			append(append(baseFields, logging.F("fallback", "local_history")), openCodeErrorLogFields(err)...)...,
		)
	}
	return a.fallback.History(ctx, deps, session, meta, lines)
}
