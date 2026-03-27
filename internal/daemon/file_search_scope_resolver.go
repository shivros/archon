package daemon

import (
	"context"
	"strings"

	"control/internal/providers"
	"control/internal/types"
)

type FileSearchScopeResolver interface {
	ResolveScope(ctx context.Context, scope types.FileSearchScope) (types.FileSearchScope, error)
}

type FileSearchSessionLookup interface {
	GetSession(id string) (*types.Session, bool)
}

type FileSearchSessionRecordLookup interface {
	GetRecord(ctx context.Context, sessionID string) (*types.SessionRecord, bool, error)
}

type FileSearchSessionMetaLookup interface {
	Get(ctx context.Context, sessionID string) (*types.SessionMeta, bool, error)
}

type passthroughFileSearchScopeResolver struct{}

func NewPassthroughFileSearchScopeResolver() FileSearchScopeResolver {
	return passthroughFileSearchScopeResolver{}
}

func (passthroughFileSearchScopeResolver) ResolveScope(_ context.Context, scope types.FileSearchScope) (types.FileSearchScope, error) {
	normalized := normalizeFileSearchScope(scope)
	if normalized.SessionID != "" {
		return types.FileSearchScope{}, invalidError("scope.session_id requires a session-backed file search scope resolver", nil)
	}
	if normalized.Provider == "" {
		return types.FileSearchScope{}, invalidError("scope.provider is required when scope.session_id is empty", nil)
	}
	return normalized, nil
}

type daemonFileSearchScopeResolver struct {
	manager  FileSearchSessionLookup
	sessions FileSearchSessionRecordLookup
	meta     FileSearchSessionMetaLookup
}

func NewDaemonFileSearchScopeResolver(manager *SessionManager, stores *Stores) FileSearchScopeResolver {
	var sessionLookup FileSearchSessionLookup
	if manager != nil {
		sessionLookup = manager
	}
	var sessions FileSearchSessionRecordLookup
	var meta FileSearchSessionMetaLookup
	if stores != nil {
		sessions = stores.Sessions
		meta = stores.SessionMeta
	}
	return daemonFileSearchScopeResolver{
		manager:  sessionLookup,
		sessions: sessions,
		meta:     meta,
	}
}

func scopeResolverOrDefault(resolver FileSearchScopeResolver) FileSearchScopeResolver {
	if resolver == nil {
		return NewPassthroughFileSearchScopeResolver()
	}
	return resolver
}

func (r daemonFileSearchScopeResolver) ResolveScope(ctx context.Context, scope types.FileSearchScope) (types.FileSearchScope, error) {
	normalized := normalizeFileSearchScope(scope)
	if normalized.SessionID == "" {
		if normalized.Provider == "" {
			return types.FileSearchScope{}, invalidError("scope.provider is required when scope.session_id is empty", nil)
		}
		return normalized, nil
	}

	resolved, err := r.resolveScopeFromSession(ctx, normalized.SessionID)
	if err != nil {
		return types.FileSearchScope{}, err
	}
	if err := ensureFileSearchScopeMatch("provider", normalized.Provider, resolved.Provider); err != nil {
		return types.FileSearchScope{}, err
	}
	if err := ensureFileSearchScopeMatch("workspace_id", normalized.WorkspaceID, resolved.WorkspaceID); err != nil {
		return types.FileSearchScope{}, err
	}
	if err := ensureFileSearchScopeMatch("worktree_id", normalized.WorktreeID, resolved.WorktreeID); err != nil {
		return types.FileSearchScope{}, err
	}
	if err := ensureFileSearchScopeMatch("cwd", normalized.Cwd, resolved.Cwd); err != nil {
		return types.FileSearchScope{}, err
	}
	return resolved, nil
}

func (r daemonFileSearchScopeResolver) resolveScopeFromSession(ctx context.Context, sessionID string) (types.FileSearchScope, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return types.FileSearchScope{}, invalidError("scope.session_id is required", nil)
	}
	scope := types.FileSearchScope{SessionID: sessionID}
	found := false

	if r.manager != nil {
		if session, ok := r.manager.GetSession(sessionID); ok && session != nil {
			scope.Provider = providers.Normalize(session.Provider)
			scope.Cwd = strings.TrimSpace(session.Cwd)
			found = true
		}
	}
	if r.sessions != nil {
		record, ok, err := r.sessions.GetRecord(ctx, sessionID)
		if err != nil {
			return types.FileSearchScope{}, unavailableError("session lookup failed", err)
		}
		if ok && record != nil && record.Session != nil {
			if scope.Provider == "" {
				scope.Provider = providers.Normalize(record.Session.Provider)
			}
			if scope.Cwd == "" {
				scope.Cwd = strings.TrimSpace(record.Session.Cwd)
			}
			found = true
		}
	}
	if r.meta != nil {
		meta, ok, err := r.meta.Get(ctx, sessionID)
		if err != nil {
			return types.FileSearchScope{}, unavailableError("session metadata lookup failed", err)
		}
		if ok && meta != nil {
			scope.WorkspaceID = strings.TrimSpace(meta.WorkspaceID)
			scope.WorktreeID = strings.TrimSpace(meta.WorktreeID)
			found = true
		}
	}
	if !found {
		return types.FileSearchScope{}, notFoundError("session not found", ErrSessionNotFound)
	}
	if scope.Provider == "" {
		return types.FileSearchScope{}, invalidError("session provider is required for file search", nil)
	}
	return scope, nil
}

func ensureFileSearchScopeMatch(field, provided, resolved string) error {
	provided = strings.TrimSpace(provided)
	resolved = strings.TrimSpace(resolved)
	if provided == "" || resolved == "" || provided == resolved {
		return nil
	}
	return invalidError("scope."+field+" does not match the selected session", nil)
}
