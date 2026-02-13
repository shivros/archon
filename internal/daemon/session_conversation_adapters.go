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
		return claudeConversationAdapter{fallback: fallback}
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
	if service.live == nil {
		return "", unavailableError("live codex manager not available", nil)
	}
	workspacePath := service.resolveWorkspacePath(ctx, meta)
	codexHome := resolveCodexHome(session.Cwd, workspacePath)
	turnID, err := service.live.StartTurn(ctx, session, meta, codexHome, input)
	if err != nil {
		return "", invalidError(err.Error(), err)
	}
	now := time.Now().UTC()
	if service.stores != nil && service.stores.SessionMeta != nil {
		_, _ = service.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			ThreadID:     threadID,
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
	if service.live == nil {
		return nil, nil, unavailableError("live codex manager not available", nil)
	}
	workspacePath := service.resolveWorkspacePath(ctx, meta)
	codexHome := resolveCodexHome(session.Cwd, workspacePath)
	ch, cancel, err := service.live.Subscribe(session, meta, codexHome)
	if err != nil {
		return nil, nil, invalidError(err.Error(), err)
	}
	return ch, cancel, nil
}

func (codexConversationAdapter) Approve(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, requestID int, decision string, responses []string, acceptSettings map[string]any) error {
	if session == nil {
		return invalidError("session is required", nil)
	}
	if service.live == nil {
		return unavailableError("live codex manager not available", nil)
	}
	workspacePath := service.resolveWorkspacePath(ctx, meta)
	codexHome := resolveCodexHome(session.Cwd, workspacePath)
	result := map[string]any{
		"decision": decision,
	}
	if len(responses) > 0 {
		result["responses"] = responses
	}
	if len(acceptSettings) > 0 {
		result["acceptSettings"] = acceptSettings
	}
	if err := service.live.Respond(ctx, session, meta, codexHome, requestID, result); err != nil {
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
	if service.live == nil {
		return unavailableError("live codex manager not available", nil)
	}
	workspacePath := service.resolveWorkspacePath(ctx, meta)
	codexHome := resolveCodexHome(session.Cwd, workspacePath)
	if err := service.live.Interrupt(ctx, session, meta, codexHome); err != nil {
		return invalidError(err.Error(), err)
	}
	return nil
}

type claudeConversationAdapter struct {
	fallback defaultConversationAdapter
}

func (claudeConversationAdapter) Provider() string {
	return "claude"
}

func (a claudeConversationAdapter) History(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, source string, lines int) ([]map[string]any, error) {
	return a.fallback.History(ctx, service, session, meta, source, lines)
}

func (claudeConversationAdapter) SendMessage(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, input []map[string]any) (string, error) {
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
	payload := buildClaudeUserPayloadWithRuntime(text, runtimeOptions)
	if err := service.manager.SendInput(session.ID, payload); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			providerSessionID := ""
			if meta != nil {
				providerSessionID = meta.ProviderSessionID
			}
			if strings.TrimSpace(providerSessionID) == "" {
				return "", invalidError("provider session id not available", nil)
			}
			if strings.TrimSpace(session.Cwd) == "" {
				return "", invalidError("session cwd is required", nil)
			}
			_, resumeErr := service.manager.ResumeSession(StartSessionConfig{
				Provider:          session.Provider,
				Cwd:               session.Cwd,
				Env:               session.Env,
				RuntimeOptions:    runtimeOptions,
				Resume:            true,
				ProviderSessionID: providerSessionID,
			}, session)
			if resumeErr != nil {
				return "", invalidError(resumeErr.Error(), resumeErr)
			}
			if err := service.manager.SendInput(session.ID, payload); err != nil {
				return "", invalidError(err.Error(), err)
			}
		} else {
			return "", invalidError(err.Error(), err)
		}
	}
	now := time.Now().UTC()
	if service.stores != nil && service.stores.SessionMeta != nil {
		_, _ = service.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			LastActiveAt: &now,
		})
	}
	return "", nil
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
	payload := buildOpenCodeUserPayloadWithRuntime(text, runtimeOptions)
	if err := service.manager.SendInput(session.ID, payload); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			providerSessionID := ""
			if meta != nil {
				providerSessionID = meta.ProviderSessionID
			}
			if strings.TrimSpace(providerSessionID) == "" {
				return "", invalidError("provider session id not available", nil)
			}
			_, resumeErr := service.manager.ResumeSession(StartSessionConfig{
				Provider:          session.Provider,
				Cwd:               session.Cwd,
				Env:               session.Env,
				RuntimeOptions:    runtimeOptions,
				Resume:            true,
				ProviderSessionID: providerSessionID,
			}, session)
			if resumeErr != nil {
				return "", invalidError(resumeErr.Error(), resumeErr)
			}
			if err := service.manager.SendInput(session.ID, payload); err != nil {
				return "", invalidError(err.Error(), err)
			}
		} else {
			return "", invalidError(err.Error(), err)
		}
	}
	now := time.Now().UTC()
	if service.stores != nil && service.stores.SessionMeta != nil {
		_, _ = service.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:    session.ID,
			LastActiveAt: &now,
		})
	}
	return "", nil
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
	client, err := newOpenCodeClient(resolveOpenCodeClientConfig(session.Provider, loadCoreConfigOrDefault()))
	if err != nil {
		return nil, nil, invalidError(err.Error(), err)
	}
	directory := strings.TrimSpace(session.Cwd)
	upstream, upstreamCancel, err := client.SubscribeSessionEvents(ctx, providerSessionID, directory)
	if err != nil && directory != "" {
		// Fallback for servers that reject directory scoping on event streams.
		upstream, upstreamCancel, err = client.SubscribeSessionEvents(ctx, providerSessionID, "")
	}
	if err != nil {
		return nil, nil, invalidError(err.Error(), err)
	}

	out := make(chan types.CodexEvent, 256)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-upstream:
				if !ok {
					return
				}
				applyOpenCodeApprovalEvent(ctx, service, session, event)
				select {
				case <-ctx.Done():
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

func (openCodeConversationAdapter) Approve(ctx context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta, requestID int, decision string, _ []string, _ map[string]any) error {
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
	if err := client.ReplyPermission(ctx, providerSessionID, permissionID, decision, session.Cwd); err != nil {
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

func (openCodeConversationAdapter) Interrupt(_ context.Context, service *SessionService, session *types.Session, meta *types.SessionMeta) error {
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
			_, resumeErr := service.manager.ResumeSession(StartSessionConfig{
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
