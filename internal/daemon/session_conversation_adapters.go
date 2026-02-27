package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"control/internal/logging"
	"control/internal/providers"
	"control/internal/types"
)

type conversationAdapter interface {
	Provider() string
	History(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, source string, lines int) ([]map[string]any, error)
	SendMessage(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, input []map[string]any) (string, error)
	SubscribeEvents(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error)
	Approve(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, requestID int, decision string, responses []string, acceptSettings map[string]any) error
	Interrupt(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta) error
}

type conversationAdapterRegistry struct {
	fallback conversationAdapter
	byName   map[string]conversationAdapter
}

func newConversationAdapterRegistry(extra ...conversationAdapter) *conversationAdapterRegistry {
	fallback := defaultConversationAdapter{}
	registry := &conversationAdapterRegistry{
		fallback: fallback,
		byName:   map[string]conversationAdapter{},
	}
	for _, def := range providers.All() {
		name := providers.Normalize(def.Name)
		if name == "" {
			continue
		}
		if adapter := defaultConversationAdapterFor(def, fallback); adapter != nil {
			registry.byName[name] = adapter
		}
	}
	for _, adapter := range extra {
		if adapter == nil {
			continue
		}
		name := providers.Normalize(adapter.Provider())
		if name == "" {
			continue
		}
		registry.byName[name] = adapter
	}
	return registry
}

func defaultConversationAdapterFor(def providers.Definition, fallback defaultConversationAdapter) conversationAdapter {
	switch def.Runtime {
	case providers.RuntimeCodex:
		return codexConversationAdapter{fallback: fallback}
	case providers.RuntimeClaude:
		return newClaudeConversationAdapter(fallback)
	case providers.RuntimeOpenCodeServer:
		return openCodeConversationAdapter{providerName: providers.Normalize(def.Name), fallback: fallback}
	default:
		return nil
	}
}

func (r *conversationAdapterRegistry) adapterFor(provider string) conversationAdapter {
	if r == nil {
		return defaultConversationAdapter{}
	}
	if adapter, ok := r.byName[providers.Normalize(provider)]; ok && adapter != nil {
		return adapter
	}
	if r.fallback != nil {
		return r.fallback
	}
	return defaultConversationAdapter{}
}

type defaultConversationAdapter struct{}

func (defaultConversationAdapter) Provider() string {
	return "*"
}

func (defaultConversationAdapter) History(ctx context.Context, service *SessionService, session *types.Session, _ *types.SessionMeta, _ string, lines int) ([]map[string]any, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	if providerUsesItems(session.Provider) {
		if items, _, err := service.readSessionItems(session.ID, lines); err == nil && items != nil {
			return items, nil
		}
	}
	if service.manager != nil {
		if _, ok := service.manager.GetSession(session.ID); ok {
			out, _, _, err := service.manager.TailSession(session.ID, "combined", lines)
			if err == nil {
				return logLinesToItems(out), nil
			}
		}
	}
	out, _, _, err := service.readSessionLogs(session.ID, lines)
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
	return logLinesToItems(out), nil
}

func (defaultConversationAdapter) SendMessage(context.Context, *SessionService, *types.Session, *types.SessionMeta, []map[string]any) (string, error) {
	return "", invalidError("provider does not support messaging", nil)
}

func (defaultConversationAdapter) SubscribeEvents(context.Context, *SessionService, *types.Session, *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	return nil, nil, invalidError("provider does not support events", nil)
}

func (defaultConversationAdapter) Approve(context.Context, *SessionService, *types.Session, *types.SessionMeta, int, string, []string, map[string]any) error {
	return invalidError("provider does not support approvals", nil)
}

func (defaultConversationAdapter) Interrupt(context.Context, *SessionService, *types.Session, *types.SessionMeta) error {
	return invalidError("provider does not support interrupt", nil)
}

type codexConversationAdapter struct {
	fallback defaultConversationAdapter
}

func (codexConversationAdapter) Provider() string {
	return "codex"
}

func (a codexConversationAdapter) History(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, source string, lines int) ([]map[string]any, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	threadID := resolveThreadID(session, meta)
	if source == sessionSourceCodex || threadID != "" {
		return service.tailCodexThread(ctx, session, threadID, lines)
	}
	return a.fallback.History(ctx, service, session, meta, source, lines)
}

func (codexConversationAdapter) SendMessage(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, input []map[string]any) (string, error) {
	if session == nil {
		return "", invalidError("session is required", nil)
	}
	threadID := resolveThreadID(session, meta)
	service.logger.Info("send_resolved",
		logging.F("session_id", session.ID),
		logging.F("provider", session.Provider),
		logging.F("thread_id", threadID),
		logging.F("cwd", session.Cwd),
	)
	if threadID == "" {
		return "", invalidError("thread id not available", nil)
	}
	if strings.TrimSpace(session.Cwd) == "" {
		return "", invalidError("session cwd is required", nil)
	}
	if service.liveManager == nil {
		return "", unavailableError("live manager not available", nil)
	}
	runtimeOptions := (*types.SessionRuntimeOptions)(nil)
	if meta != nil {
		runtimeOptions = types.CloneRuntimeOptions(meta.RuntimeOptions)
	}
	turnID, err := service.liveManager.StartTurn(ctx, session, meta, input, runtimeOptions)
	if err != nil {
		return "", invalidError(err.Error(), err)
	}
	resolvedThreadID := threadID
	if service.stores != nil && service.stores.SessionMeta != nil {
		if latest, ok, getErr := service.stores.SessionMeta.Get(ctx, session.ID); getErr == nil && ok && latest != nil {
			if updatedThreadID := strings.TrimSpace(latest.ThreadID); updatedThreadID != "" {
				resolvedThreadID = updatedThreadID
			}
		}
	}
	now := time.Now().UTC()
	if service.stores != nil && service.stores.SessionMeta != nil {
		_, _ = service.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			ThreadID:     resolvedThreadID,
			LastTurnID:   turnID,
			LastActiveAt: &now,
		})
	}
	return turnID, nil
}

func (codexConversationAdapter) SubscribeEvents(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	if session == nil {
		return nil, nil, invalidError("session is required", nil)
	}
	if service.liveManager == nil {
		return nil, nil, unavailableError("live manager not available", nil)
	}
	ch, cancel, err := service.liveManager.Subscribe(session, meta)
	if err != nil {
		return nil, nil, invalidError(err.Error(), err)
	}
	return ch, cancel, nil
}

func (codexConversationAdapter) Approve(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, requestID int, decision string, responses []string, acceptSettings map[string]any) error {
	if session == nil {
		return invalidError("session is required", nil)
	}
	if service.liveManager == nil {
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
	if err := service.liveManager.Respond(ctx, session, meta, requestID, result); err != nil {
		return invalidError(err.Error(), err)
	}
	if service.stores != nil && service.stores.Approvals != nil {
		_ = service.stores.Approvals.Delete(ctx, session.ID, requestID)
	}
	now := time.Now().UTC()
	if service.stores != nil && service.stores.SessionMeta != nil {
		_, _ = service.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			LastActiveAt: &now,
		})
	}
	return nil
}

func (codexConversationAdapter) Interrupt(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta) error {
	if session == nil {
		return invalidError("session is required", nil)
	}
	if service.liveManager == nil {
		return unavailableError("live manager not available", nil)
	}
	if err := service.liveManager.Interrupt(ctx, session, meta); err != nil {
		return invalidError(err.Error(), err)
	}
	return nil
}

type claudeConversationAdapter struct {
	fallback           defaultConversationAdapter
	completionStrategy turnCompletionStrategy
	completionPolicy   claudeCompletionDecisionPolicy
	inputValidator     claudeInputValidator
	turnIDs            turnIDGenerator
}

type turnCompletionStrategy interface {
	ShouldPublishCompletion(beforeCount int, items []map[string]any) bool
	Source() string
}

type sessionServiceClaudeCompletionIO struct {
	service *SessionService
}

type claudeItemDeltaCompletionStrategy struct{}

func newClaudeConversationAdapter(fallback defaultConversationAdapter) claudeConversationAdapter {
	return claudeConversationAdapter{
		fallback:           fallback,
		completionStrategy: claudeItemDeltaCompletionStrategy{},
		turnIDs:            defaultTurnIDGenerator{},
	}
}

func (claudeConversationAdapter) Provider() string {
	return "claude"
}

func (a claudeConversationAdapter) History(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, source string, lines int) ([]map[string]any, error) {
	return a.fallback.History(ctx, service, session, meta, source, lines)
}

func (a claudeConversationAdapter) SendMessage(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, input []map[string]any) (string, error) {
	orchestrator := a.sendOrchestrator(service)
	return orchestrator.Send(ctx, service, session, meta, input)
}

func (a claudeConversationAdapter) completionStrategyOrDefault() turnCompletionStrategy {
	if a.completionStrategy == nil {
		return claudeItemDeltaCompletionStrategy{}
	}
	return a.completionStrategy
}

func (a claudeConversationAdapter) turnIDGeneratorOrDefault() turnIDGenerator {
	if a.turnIDs == nil {
		return defaultTurnIDGenerator{}
	}
	return a.turnIDs
}

func (a claudeConversationAdapter) completionPolicyOrDefault() claudeCompletionDecisionPolicy {
	if a.completionPolicy != nil {
		return a.completionPolicy
	}
	return defaultClaudeCompletionDecisionPolicy{strategy: a.completionStrategyOrDefault()}
}

func (a claudeConversationAdapter) inputValidatorOrDefault() claudeInputValidator {
	if a.inputValidator != nil {
		return a.inputValidator
	}
	return defaultClaudeInputValidator{}
}

func (a claudeConversationAdapter) sendOrchestrator(service *SessionService) claudeSendOrchestrator {
	completionIO := sessionServiceClaudeCompletionIO{service: service}
	return claudeSendOrchestrator{
		validator:           a.inputValidatorOrDefault(),
		transport:           defaultClaudeSendTransport{},
		turnIDs:             a.turnIDGeneratorOrDefault(),
		stateStore:          sessionServiceClaudeTurnStateStore{service: service},
		completionReader:    completionIO,
		completionPublisher: completionIO,
		completionPolicy:    a.completionPolicyOrDefault(),
	}
}

func (io sessionServiceClaudeCompletionIO) ReadSessionItems(sessionID string, lines int) ([]map[string]any, error) {
	if io.service == nil {
		return nil, nil
	}
	items, _, err := io.service.readSessionItems(sessionID, lines)
	return items, err
}

func (io sessionServiceClaudeCompletionIO) PublishTurnCompleted(session *types.Session, meta *types.SessionMeta, turnID, source string) {
	if io.service == nil {
		return
	}
	io.service.publishTurnCompleted(session, meta, turnID, source)
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

func (claudeConversationAdapter) SubscribeEvents(context.Context, *SessionService, *types.Session, *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	return nil, nil, invalidError("provider does not support events", nil)
}

func (claudeConversationAdapter) Approve(context.Context, *SessionService, *types.Session, *types.SessionMeta, int, string, []string, map[string]any) error {
	return invalidError("provider does not support approvals", nil)
}

func (claudeConversationAdapter) Interrupt(context.Context, *SessionService, *types.Session, *types.SessionMeta) error {
	return invalidError("provider does not support interrupt", nil)
}

type openCodeConversationAdapter struct {
	providerName string
	fallback     defaultConversationAdapter
}

func (a openCodeConversationAdapter) Provider() string {
	return a.providerName
}

func (a openCodeConversationAdapter) History(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, source string, lines int) ([]map[string]any, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	opID := logging.NewRequestID()
	baseFields := append(openCodeSessionLogFields(session, meta), logging.F("op_id", opID), logging.F("lines", lines))
	if service != nil && service.logger != nil && service.logger.Enabled(logging.Debug) {
		service.logger.Debug("opencode_history_request", baseFields...)
	}
	reconciler := newOpenCodeHistoryReconciler(service, session, meta)
	if syncResult, err := reconciler.Sync(ctx, lines); err == nil {
		if len(syncResult.items) > 0 {
			if service != nil && service.logger != nil && service.logger.Enabled(logging.Debug) {
				service.logger.Debug("opencode_history_remote_ok",
					append(baseFields, logging.F("items", len(syncResult.items)))...,
				)
			}
			return syncResult.items, nil
		}
		if service != nil && service.logger != nil && service.logger.Enabled(logging.Debug) {
			service.logger.Debug("opencode_history_remote_empty", baseFields...)
		}
	} else if service != nil && service.logger != nil {
		service.logger.Warn("opencode_history_remote_failed",
			append(append(baseFields, logging.F("fallback", "local_history")), openCodeErrorLogFields(err)...)...,
		)
	}
	return a.fallback.History(ctx, service, session, meta, source, lines)
}

func (a openCodeConversationAdapter) SendMessage(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, input []map[string]any) (string, error) {
	if session == nil {
		return "", invalidError("session is required", nil)
	}
	if service.manager == nil {
		return "", unavailableError("session manager not available", nil)
	}
	text := extractTextInput(input)
	if text == "" {
		return "", invalidError("text input is required", nil)
	}
	runtimeOptions := (*types.SessionRuntimeOptions)(nil)
	if meta != nil {
		runtimeOptions = types.CloneRuntimeOptions(meta.RuntimeOptions)
	}
	opID := logging.NewRequestID()
	baseFields := append(
		openCodeSessionLogFields(session, meta),
		logging.F("op_id", opID),
		logging.F("input_len", len(text)),
	)
	baseFields = append(baseFields, openCodeRuntimeLogFields(runtimeOptions)...)
	if service.logger != nil && service.logger.Enabled(logging.Debug) {
		service.logger.Debug("opencode_send_start", baseFields...)
	}

	if service.liveManager != nil {
		turnID, err := service.liveManager.StartTurn(ctx, session, meta, input, runtimeOptions)
		if err != nil && errors.Is(err, ErrSessionNotFound) {
			if resumeErr := openCodeResumeSessionForTurn(ctx, service, session, meta, runtimeOptions, baseFields); resumeErr != nil {
				return "", resumeErr
			}
			turnID, err = service.liveManager.StartTurn(ctx, session, meta, input, runtimeOptions)
		}
		if err != nil {
			if service.logger != nil {
				service.logger.Warn("opencode_send_turn_failed",
					append(append(baseFields, logging.F("stage", "live_turn")), openCodeErrorLogFields(err)...)...,
				)
			}
			return "", err
		}
		if service.logger != nil && service.logger.Enabled(logging.Debug) {
			service.logger.Debug("opencode_send_ok", append(baseFields, logging.F("stage", "live_turn"), logging.F("turn_id", turnID))...)
		}
		now := time.Now().UTC()
		if service.stores != nil && service.stores.SessionMeta != nil {
			_, _ = service.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
				SessionID:    session.ID,
				LastActiveAt: &now,
			})
		}
		return turnID, nil
	}

	reconciler := newOpenCodeHistoryReconciler(service, session, meta)
	payload := buildOpenCodeUserPayloadWithRuntime(text, runtimeOptions)
	if err := service.manager.SendInput(session.ID, payload); err != nil {
		if service.logger != nil {
			service.logger.Warn("opencode_send_input_failed",
				append(append(baseFields, logging.F("stage", "send_input")), openCodeErrorLogFields(err)...)...,
			)
		}
		if errors.Is(err, ErrSessionNotFound) {
			providerSessionID := ""
			if meta != nil {
				providerSessionID = meta.ProviderSessionID
			}
			if strings.TrimSpace(providerSessionID) == "" {
				return "", invalidError("provider session id not available", nil)
			}
			additionalDirectories, dirsErr := service.resolveAdditionalDirectoriesForSession(ctx, session, meta)
			if dirsErr != nil {
				return "", invalidError(dirsErr.Error(), dirsErr)
			}
			_, resumeErr := service.manager.ResumeSession(StartSessionConfig{
				Provider:              session.Provider,
				Cwd:                   session.Cwd,
				AdditionalDirectories: additionalDirectories,
				Env:                   session.Env,
				RuntimeOptions:        runtimeOptions,
				Resume:                true,
				ProviderSessionID:     providerSessionID,
			}, session)
			if resumeErr != nil {
				if service.logger != nil {
					service.logger.Warn("opencode_send_resume_failed",
						append(append(baseFields, logging.F("stage", "resume")), openCodeErrorLogFields(resumeErr)...)...,
					)
				}
				return "", invalidError(resumeErr.Error(), resumeErr)
			}
			if err := service.manager.SendInput(session.ID, payload); err != nil {
				if service.logger != nil {
					service.logger.Warn("opencode_send_retry_failed",
						append(append(baseFields, logging.F("stage", "send_after_resume")), openCodeErrorLogFields(err)...)...,
					)
				}
				reconciler.ReconcileBestEffort(ctx, "send_after_resume_error")
				return "", invalidError(err.Error(), err)
			}
		} else {
			reconciler.ReconcileBestEffort(ctx, "send_error")
			return "", invalidError(err.Error(), err)
		}
	}
	if service.logger != nil && service.logger.Enabled(logging.Debug) {
		service.logger.Debug("opencode_send_ok", baseFields...)
	}
	openCodeSchedulePostSendReconcile(service, session, meta, "send_ok")
	now := time.Now().UTC()
	if service.stores != nil && service.stores.SessionMeta != nil {
		_, _ = service.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			LastActiveAt: &now,
		})
	}
	return "", nil
}

func openCodeSchedulePostSendReconcile(service *SessionService, session *types.Session, meta *types.SessionMeta, reason string) {
	if service == nil || session == nil {
		return
	}
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		trimmedReason = "send"
	}
	go func() {
		reconciler := newOpenCodeHistoryReconciler(service, session, meta)
		delay := 2 * time.Second
		for attempt := 1; attempt <= 9; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			result, err := reconciler.Sync(ctx, 200)
			cancel()
			if err == nil && len(result.backfilled) > 0 {
				if service.logger != nil {
					service.logger.Info("opencode_post_send_reconcile_backfilled",
						append(
							openCodeSessionLogFields(session, meta),
							logging.F("reason", trimmedReason),
							logging.F("attempt", attempt),
							logging.F("items", len(result.backfilled)),
						)...,
					)
				}
				return
			}
			if attempt == 9 {
				break
			}
			timer := time.NewTimer(delay)
			<-timer.C
			if delay < 10*time.Second {
				delay *= 2
			}
		}
		if service.logger != nil && service.logger.Enabled(logging.Debug) {
			service.logger.Debug("opencode_post_send_reconcile_exhausted",
				append(
					openCodeSessionLogFields(session, meta),
					logging.F("reason", trimmedReason),
				)...,
			)
		}
	}()
}

func openCodeResumeSessionForTurn(
	ctx context.Context,
	service *SessionService,
	session *types.Session,
	meta *types.SessionMeta,
	runtimeOptions *types.SessionRuntimeOptions,
	baseFields []logging.Field,
) error {
	if service == nil || service.manager == nil {
		return unavailableError("session manager not available", nil)
	}
	providerSessionID := ""
	if meta != nil {
		providerSessionID = strings.TrimSpace(meta.ProviderSessionID)
	}
	if providerSessionID == "" {
		return invalidError("provider session id not available", nil)
	}
	additionalDirectories, dirsErr := service.resolveAdditionalDirectoriesForSession(ctx, session, meta)
	if dirsErr != nil {
		return invalidError(dirsErr.Error(), dirsErr)
	}
	if _, resumeErr := service.manager.ResumeSession(StartSessionConfig{
		Provider:              session.Provider,
		Cwd:                   session.Cwd,
		AdditionalDirectories: additionalDirectories,
		Env:                   session.Env,
		RuntimeOptions:        runtimeOptions,
		Resume:                true,
		ProviderSessionID:     providerSessionID,
	}, session); resumeErr != nil {
		if service.logger != nil {
			service.logger.Warn("opencode_send_resume_failed",
				append(append(baseFields, logging.F("stage", "resume_live_turn")), openCodeErrorLogFields(resumeErr)...)...,
			)
		}
		return invalidError(resumeErr.Error(), resumeErr)
	}
	return nil
}

func (openCodeConversationAdapter) SubscribeEvents(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	if session == nil {
		return nil, nil, invalidError("session is required", nil)
	}
	providerSessionID := ""
	if meta != nil {
		providerSessionID = strings.TrimSpace(meta.ProviderSessionID)
	}
	if providerSessionID == "" {
		return nil, nil, invalidError("provider session id not available", nil)
	}
	opID := logging.NewRequestID()
	baseFields := append(openCodeSessionLogFields(session, meta), logging.F("op_id", opID))
	if service.logger != nil && service.logger.Enabled(logging.Debug) {
		service.logger.Debug("opencode_events_subscribe_start", baseFields...)
	}
	client, err := newOpenCodeClient(resolveOpenCodeClientConfig(session.Provider, loadCoreConfigOrDefault()))
	if err != nil {
		return nil, nil, invalidError(err.Error(), err)
	}
	directory := strings.TrimSpace(session.Cwd)
	upstream, upstreamCancel, err := client.SubscribeSessionEvents(ctx, providerSessionID, directory)
	if err != nil && directory != "" {
		if service.logger != nil {
			service.logger.Warn("opencode_events_subscribe_directory_failed",
				append(append(baseFields, logging.F("stage", "subscribe_with_directory")), openCodeErrorLogFields(err)...)...,
			)
		}
		// Fallback for servers that reject directory scoping on event streams.
		upstream, upstreamCancel, err = client.SubscribeSessionEvents(ctx, providerSessionID, "")
	}
	if err != nil {
		if service.logger != nil {
			service.logger.Warn("opencode_events_subscribe_failed",
				append(baseFields, openCodeErrorLogFields(err)...)...,
			)
		}
		return nil, nil, invalidError(err.Error(), err)
	}

	out := make(chan types.CodexEvent, 256)
	done := make(chan struct{})
	reconciler := newOpenCodeHistoryReconciler(service, session, meta)
	go func() {
		defer close(done)
		defer close(out)
		startedAt := time.Now()
		sawTurnCompleted := false
		eventCount := 0
		recoveredCount := 0
		firstMethod := ""
		lastMethod := ""
		closeReason := "unknown"
		defer func() {
			if service.logger != nil && service.logger.Enabled(logging.Debug) {
				service.logger.Debug("opencode_events_subscribe_close",
					append(
						baseFields,
						logging.F("close_reason", closeReason),
						logging.F("events", eventCount),
						logging.F("recovered_events", recoveredCount),
						logging.F("saw_turn_completed", sawTurnCompleted),
						logging.F("first_method", firstMethod),
						logging.F("last_method", lastMethod),
						logging.F("duration_ms", time.Since(startedAt).Milliseconds()),
					)...,
				)
			}
			if service.logger != nil && closeReason == "upstream_closed" && !sawTurnCompleted && recoveredCount == 0 {
				service.logger.Warn("opencode_events_closed_without_completion",
					append(baseFields,
						logging.F("events", eventCount),
						logging.F("duration_ms", time.Since(startedAt).Milliseconds()),
					)...,
				)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				closeReason = "context_done"
				return
			case event, ok := <-upstream:
				if !ok {
					closeReason = "upstream_closed"
					if ctx.Err() == nil {
						recoveredEvents := reconciler.RecoveredEvents(ctx, sawTurnCompleted)
						recoveredCount = len(recoveredEvents)
						for _, recovered := range recoveredEvents {
							if recovered.Method == "turn/completed" {
								turn := parseTurnEventFromParams(recovered.Params)
								service.publishTurnCompletedWithPayload(
									session,
									meta,
									turn.TurnID,
									"opencode_recovered_event",
									map[string]any{
										"turn_status": turn.Status,
										"turn_error":  turn.Error,
									},
								)
							}
							select {
							case <-ctx.Done():
								closeReason = "context_done"
								return
							case out <- recovered:
							}
						}
						if service.logger != nil && recoveredCount > 0 {
							service.logger.Info("opencode_events_recovered",
								append(baseFields, logging.F("recovered_events", recoveredCount))...,
							)
						}
					}
					return
				}
				eventCount++
				if eventCount == 1 {
					firstMethod = event.Method
				}
				lastMethod = event.Method
				if event.Method == "turn/completed" {
					sawTurnCompleted = true
					turn := parseTurnEventFromParams(event.Params)
					service.publishTurnCompletedWithPayload(
						session,
						meta,
						turn.TurnID,
						"opencode_event",
						map[string]any{
							"turn_status": turn.Status,
							"turn_error":  turn.Error,
						},
					)
				}
				if event.Method == "error" && service.logger != nil {
					service.logger.Warn("opencode_events_error_event",
						append(baseFields, logging.F("event_params", string(event.Params)))...,
					)
				}
				applyOpenCodeApprovalEvent(ctx, service, session, event)
				select {
				case <-ctx.Done():
					closeReason = "context_done"
					return
				case out <- event:
				}
			}
		}
	}()

	cancel := func() {
		upstreamCancel()
		<-done
	}
	return out, cancel, nil
}

func (openCodeConversationAdapter) Approve(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, requestID int, decision string, responses []string, _ map[string]any) error {
	if session == nil {
		return invalidError("session is required", nil)
	}
	if requestID < 0 {
		return invalidError("request id is required", nil)
	}
	if service == nil || service.stores == nil || service.stores.Approvals == nil {
		return unavailableError("approval store not available", nil)
	}
	record, ok, err := service.stores.Approvals.Get(ctx, session.ID, requestID)
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
	providerSessionID := ""
	if meta != nil {
		providerSessionID = strings.TrimSpace(meta.ProviderSessionID)
	}
	client, err := newOpenCodeClient(resolveOpenCodeClientConfig(session.Provider, loadCoreConfigOrDefault()))
	if err != nil {
		return invalidError(err.Error(), err)
	}
	if err := client.ReplyPermission(ctx, providerSessionID, permissionID, decision, responses, session.Cwd); err != nil {
		return invalidError(err.Error(), err)
	}
	if err := service.stores.Approvals.Delete(ctx, session.ID, requestID); err != nil {
		// Best-effort cleanup; avoid failing user action if remote decision succeeded.
		if service.logger != nil {
			service.logger.Warn("opencode_approval_delete_failed",
				logging.F("session_id", session.ID),
				logging.F("request_id", requestID),
				logging.F("error", err),
			)
		}
	}
	now := time.Now().UTC()
	if service.stores != nil && service.stores.SessionMeta != nil {
		_, _ = service.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			LastActiveAt: &now,
		})
	}
	return nil
}

func (openCodeConversationAdapter) Interrupt(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta) error {
	if session == nil {
		return invalidError("session is required", nil)
	}
	if service == nil || service.manager == nil {
		return unavailableError("session manager not available", nil)
	}
	if err := service.manager.InterruptSession(session.ID); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			providerSessionID := ""
			runtimeOptions := (*types.SessionRuntimeOptions)(nil)
			if meta != nil {
				providerSessionID = meta.ProviderSessionID
				runtimeOptions = types.CloneRuntimeOptions(meta.RuntimeOptions)
			}
			if strings.TrimSpace(providerSessionID) == "" {
				return notFoundError("session not found", err)
			}
			additionalDirectories, dirsErr := service.resolveAdditionalDirectoriesForSession(ctx, session, meta)
			if dirsErr != nil {
				return invalidError(dirsErr.Error(), dirsErr)
			}
			_, resumeErr := service.manager.ResumeSession(StartSessionConfig{
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
			if interruptErr := service.manager.InterruptSession(session.ID); interruptErr != nil {
				return invalidError(interruptErr.Error(), interruptErr)
			}
			return nil
		}
		return invalidError(err.Error(), err)
	}
	return nil
}

func applyOpenCodeApprovalEvent(ctx context.Context, service *SessionService, session *types.Session, event types.CodexEvent) {
	if service == nil || service.stores == nil || service.stores.Approvals == nil || session == nil {
		return
	}
	switch {
	case isApprovalMethod(event.Method) && event.ID != nil && *event.ID >= 0:
		_, _ = service.stores.Approvals.Upsert(ctx, &types.Approval{
			SessionID: session.ID,
			RequestID: *event.ID,
			Method:    event.Method,
			Params:    event.Params,
			CreatedAt: time.Now().UTC(),
		})
	case event.Method == "permission/replied":
		requestID := -1
		if event.ID != nil {
			requestID = *event.ID
		}
		if requestID < 0 && len(event.Params) > 0 {
			payload := map[string]any{}
			if err := json.Unmarshal(event.Params, &payload); err == nil {
				if parsed, ok := asInt(payload["request_id"]); ok {
					requestID = parsed
				}
			}
		}
		if requestID >= 0 {
			_ = service.stores.Approvals.Delete(ctx, session.ID, requestID)
		}
	}
}
