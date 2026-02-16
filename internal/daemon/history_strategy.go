package daemon

import (
	"context"

	"control/internal/logging"
	"control/internal/providers"
	"control/internal/types"
)

type conversationHistoryStrategy interface {
	Provider() string
	History(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, source string, lines int) ([]map[string]any, error)
}

type conversationHistoryStrategyRegistry struct {
	fallback conversationHistoryStrategy
	byName   map[string]conversationHistoryStrategy
}

func newConversationHistoryStrategyRegistry(extra ...conversationHistoryStrategy) *conversationHistoryStrategyRegistry {
	fallback := defaultConversationHistoryStrategy{}
	registry := &conversationHistoryStrategyRegistry{
		fallback: fallback,
		byName:   map[string]conversationHistoryStrategy{},
	}
	for _, def := range providers.All() {
		name := providers.Normalize(def.Name)
		if name == "" {
			continue
		}
		if strategy := defaultConversationHistoryStrategyFor(def, fallback); strategy != nil {
			registry.byName[name] = strategy
		}
	}
	for _, strategy := range extra {
		if strategy == nil {
			continue
		}
		name := providers.Normalize(strategy.Provider())
		if name == "" {
			continue
		}
		registry.byName[name] = strategy
	}
	return registry
}

func defaultConversationHistoryStrategyFor(def providers.Definition, fallback defaultConversationHistoryStrategy) conversationHistoryStrategy {
	switch def.Runtime {
	case providers.RuntimeCodex:
		return codexConversationHistoryStrategy{fallback: fallback}
	case providers.RuntimeClaude:
		return claudeConversationHistoryStrategy{fallback: fallback}
	case providers.RuntimeOpenCodeServer:
		return openCodeConversationHistoryStrategy{providerName: providers.Normalize(def.Name), fallback: fallback}
	default:
		return nil
	}
}

func (r *conversationHistoryStrategyRegistry) strategyFor(provider string) conversationHistoryStrategy {
	if r == nil {
		return defaultConversationHistoryStrategy{}
	}
	if strategy, ok := r.byName[providers.Normalize(provider)]; ok && strategy != nil {
		return strategy
	}
	if r.fallback != nil {
		return r.fallback
	}
	return defaultConversationHistoryStrategy{}
}

type defaultConversationHistoryStrategy struct{}

func (defaultConversationHistoryStrategy) Provider() string {
	return "*"
}

func (defaultConversationHistoryStrategy) History(ctx context.Context, service *SessionService, session *types.Session, _ *types.SessionMeta, _ string, lines int) ([]map[string]any, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	if service == nil {
		return nil, unavailableError("session service not available", nil)
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

type codexConversationHistoryStrategy struct {
	fallback defaultConversationHistoryStrategy
}

func (codexConversationHistoryStrategy) Provider() string {
	return "codex"
}

func (a codexConversationHistoryStrategy) History(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, source string, lines int) ([]map[string]any, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	threadID := resolveThreadID(session, meta)
	if source == sessionSourceCodex || threadID != "" {
		return service.tailCodexThread(ctx, session, threadID, lines)
	}
	return a.fallback.History(ctx, service, session, meta, source, lines)
}

type claudeConversationHistoryStrategy struct {
	fallback defaultConversationHistoryStrategy
}

func (claudeConversationHistoryStrategy) Provider() string {
	return "claude"
}

func (a claudeConversationHistoryStrategy) History(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, source string, lines int) ([]map[string]any, error) {
	return a.fallback.History(ctx, service, session, meta, source, lines)
}

type openCodeConversationHistoryStrategy struct {
	providerName string
	fallback     defaultConversationHistoryStrategy
}

func (a openCodeConversationHistoryStrategy) Provider() string {
	return a.providerName
}

func (a openCodeConversationHistoryStrategy) History(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, source string, lines int) ([]map[string]any, error) {
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
