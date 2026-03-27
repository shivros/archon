package app

import (
	"strings"

	"control/internal/providers"
	"control/internal/types"
)

type composeFileSearchContext struct {
	Supported bool
	Scope     types.FileSearchScope
	HasScope  bool
}

type composeFileSearchContextResolver interface {
	ResolveComposeFileSearchContext(m *Model) composeFileSearchContext
}

type defaultComposeFileSearchContextResolver struct{}

func WithComposeFileSearchContextResolver(resolver composeFileSearchContextResolver) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.composeFileSearchContextResolver = resolver
	}
}

func (m *Model) composeFileSearchContextResolverOrDefault() composeFileSearchContextResolver {
	if m == nil || m.composeFileSearchContextResolver == nil {
		return defaultComposeFileSearchContextResolver{}
	}
	return m.composeFileSearchContextResolver
}

func (defaultComposeFileSearchContextResolver) ResolveComposeFileSearchContext(m *Model) composeFileSearchContext {
	if m == nil {
		return composeFileSearchContext{}
	}
	provider := strings.TrimSpace(m.composeProvider())
	if provider == "" {
		return composeFileSearchContext{}
	}

	ctx := composeFileSearchContext{
		Supported: providers.CapabilitiesFor(provider).SupportsFileSearch,
	}
	if sessionID := strings.TrimSpace(m.composeSessionID()); sessionID != "" {
		if capabilities, ok := m.sessionTranscriptCapabilitiesForSession(sessionID); ok && capabilities != nil {
			ctx.Supported = capabilities.SupportsFileSearch
		}
		ctx.Scope = types.FileSearchScope{
			Provider:  provider,
			SessionID: sessionID,
		}
		ctx.HasScope = true
		return ctx
	}
	if m.newSession != nil {
		ctx.Scope = types.FileSearchScope{
			Provider:    provider,
			WorkspaceID: strings.TrimSpace(m.newSession.workspaceID),
			WorktreeID:  strings.TrimSpace(m.newSession.worktreeID),
		}
		ctx.HasScope = true
	}
	return ctx
}
