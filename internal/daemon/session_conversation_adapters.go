package daemon

import (
	"context"
	"time"

	"control/internal/providers"
	"control/internal/types"
)

type adapterDeps struct {
	liveManager LiveManager
	stores      *Stores
}

type conversationAdapter interface {
	Provider() string
	SendMessage(ctx context.Context, deps adapterDeps, session *types.Session, meta *types.SessionMeta, input []map[string]any) (string, error)
	SubscribeEvents(ctx context.Context, deps adapterDeps, session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error)
	Approve(ctx context.Context, deps adapterDeps, session *types.Session, meta *types.SessionMeta, requestID int, decision string, responses []string, acceptSettings map[string]any) error
	Interrupt(ctx context.Context, deps adapterDeps, session *types.Session, meta *types.SessionMeta) error
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

func defaultConversationAdapterFor(def providers.Definition, _ defaultConversationAdapter) conversationAdapter {
	switch def.Runtime {
	case providers.RuntimeCodex, providers.RuntimeClaude, providers.RuntimeOpenCodeServer:
		return liveManagerConversationAdapter{providerName: providers.Normalize(def.Name)}
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

func (defaultConversationAdapter) SendMessage(context.Context, adapterDeps, *types.Session, *types.SessionMeta, []map[string]any) (string, error) {
	return "", invalidError("provider does not support messaging", nil)
}

func (defaultConversationAdapter) SubscribeEvents(context.Context, adapterDeps, *types.Session, *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
	return nil, nil, invalidError("provider does not support events", nil)
}

func (defaultConversationAdapter) Approve(context.Context, adapterDeps, *types.Session, *types.SessionMeta, int, string, []string, map[string]any) error {
	return invalidError("provider does not support approvals", nil)
}

func (defaultConversationAdapter) Interrupt(context.Context, adapterDeps, *types.Session, *types.SessionMeta) error {
	return invalidError("provider does not support interrupt", nil)
}

type liveManagerConversationAdapter struct {
	providerName string
}

func (a liveManagerConversationAdapter) Provider() string {
	return a.providerName
}

func (a liveManagerConversationAdapter) SendMessage(ctx context.Context, deps adapterDeps, session *types.Session, meta *types.SessionMeta, input []map[string]any) (string, error) {
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
	now := time.Now().UTC()
	if deps.stores != nil && deps.stores.SessionMeta != nil {
		_, _ = deps.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			LastTurnID:   turnID,
			LastActiveAt: &now,
		})
	}
	return turnID, nil
}

func (a liveManagerConversationAdapter) SubscribeEvents(ctx context.Context, deps adapterDeps, session *types.Session, meta *types.SessionMeta) (<-chan types.CodexEvent, func(), error) {
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

func (a liveManagerConversationAdapter) Approve(ctx context.Context, deps adapterDeps, session *types.Session, meta *types.SessionMeta, requestID int, decision string, responses []string, acceptSettings map[string]any) error {
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
	if deps.stores != nil && deps.stores.Approvals != nil {
		_ = deps.stores.Approvals.Delete(ctx, session.ID, requestID)
	}
	now := time.Now().UTC()
	if deps.stores != nil && deps.stores.SessionMeta != nil {
		_, _ = deps.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			LastActiveAt: &now,
		})
	}
	return nil
}

func (a liveManagerConversationAdapter) Interrupt(ctx context.Context, deps adapterDeps, session *types.Session, meta *types.SessionMeta) error {
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
