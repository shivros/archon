package app

import (
	"context"
	"errors"
	"strings"
)

const (
	requestScopeSessionLoad    = "session_load"
	requestScopeProviderOption = "provider_options"
	requestScopeSessionStart   = "session_start"
	requestScopeWorktrees      = "worktrees"
	requestScopeDebugStream    = "debug_stream"
	requestScopeRecentsPrefix  = "recents_watch:"
)

type requestScope struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (m *Model) replaceRequestScope(name string) context.Context {
	if m == nil {
		return context.Background()
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return context.Background()
	}
	m.cancelRequestScope(name)
	if m.requestScopes == nil {
		m.requestScopes = map[string]requestScope{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.requestScopes[name] = requestScope{ctx: ctx, cancel: cancel}
	return ctx
}

func (m *Model) requestScopeContext(name string) context.Context {
	if m == nil {
		return context.Background()
	}
	name = strings.TrimSpace(name)
	if name == "" || m.requestScopes == nil {
		return context.Background()
	}
	scope, ok := m.requestScopes[name]
	if !ok || scope.ctx == nil {
		return context.Background()
	}
	return scope.ctx
}

func (m *Model) hasRequestScope(name string) bool {
	if m == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" || m.requestScopes == nil {
		return false
	}
	_, ok := m.requestScopes[name]
	return ok
}

func (m *Model) cancelRequestScope(name string) {
	if m == nil {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" || m.requestScopes == nil {
		return
	}
	scope, ok := m.requestScopes[name]
	if !ok {
		return
	}
	if scope.cancel != nil {
		scope.cancel()
	}
	delete(m.requestScopes, name)
}

func (m *Model) cancelRequestScopesWithPrefix(prefix string) {
	if m == nil {
		return
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || len(m.requestScopes) == 0 {
		return
	}
	for key, scope := range m.requestScopes {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if scope.cancel != nil {
			scope.cancel()
		}
		delete(m.requestScopes, key)
	}
}

func recentsRequestScopeName(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return requestScopeRecentsPrefix
	}
	return requestScopeRecentsPrefix + sessionID
}

func isCanceledRequestError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled)
}
