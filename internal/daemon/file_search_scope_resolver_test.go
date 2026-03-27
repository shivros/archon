package daemon

import (
	"context"
	"errors"
	"testing"

	"control/internal/types"
)

func TestPassthroughFileSearchScopeResolverRejectsSessionScope(t *testing.T) {
	resolver := NewPassthroughFileSearchScopeResolver()
	_, err := resolver.ResolveScope(context.Background(), types.FileSearchScope{SessionID: "sess-1"})
	if err == nil {
		t.Fatalf("expected session-backed scope to be rejected without daemon resolver")
	}
}

func TestPassthroughFileSearchScopeResolverAcceptsDirectProviderScope(t *testing.T) {
	resolver := NewPassthroughFileSearchScopeResolver()
	scope, err := resolver.ResolveScope(context.Background(), types.FileSearchScope{Provider: " codex ", Cwd: " /repo "})
	if err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	if scope.Provider != "codex" || scope.Cwd != "/repo" {
		t.Fatalf("unexpected scope: %#v", scope)
	}
}

func TestDaemonFileSearchScopeResolverHydratesSessionScope(t *testing.T) {
	resolver := NewDaemonFileSearchScopeResolver(nil, &Stores{
		Sessions: fileSearchStubSessionIndexStore{
			record: &types.SessionRecord{
				Session: &types.Session{ID: "sess-1", Provider: "codex", Cwd: "/repo"},
			},
		},
		SessionMeta: fileSearchStubSessionMetaStore{
			meta: &types.SessionMeta{SessionID: "sess-1", WorkspaceID: "ws-1", WorktreeID: "wt-1"},
		},
	})
	scope, err := resolver.ResolveScope(context.Background(), types.FileSearchScope{SessionID: "sess-1"})
	if err != nil {
		t.Fatalf("ResolveScope: %v", err)
	}
	if scope.Provider != "codex" || scope.Cwd != "/repo" || scope.WorkspaceID != "ws-1" || scope.WorktreeID != "wt-1" {
		t.Fatalf("unexpected scope: %#v", scope)
	}
}

func TestDaemonFileSearchScopeResolverRejectsConflictingProvider(t *testing.T) {
	resolver := NewDaemonFileSearchScopeResolver(nil, &Stores{
		Sessions: fileSearchStubSessionIndexStore{
			record: &types.SessionRecord{
				Session: &types.Session{ID: "sess-1", Provider: "codex"},
			},
		},
	})
	_, err := resolver.ResolveScope(context.Background(), types.FileSearchScope{
		SessionID: "sess-1",
		Provider:  "claude",
	})
	if err == nil {
		t.Fatalf("expected conflicting provider to fail")
	}
}

func TestDaemonFileSearchScopeResolverRejectsConflictingWorkspaceAndCwd(t *testing.T) {
	resolver := NewDaemonFileSearchScopeResolver(nil, &Stores{
		Sessions: fileSearchStubSessionIndexStore{
			record: &types.SessionRecord{
				Session: &types.Session{ID: "sess-1", Provider: "codex", Cwd: "/repo"},
			},
		},
		SessionMeta: fileSearchStubSessionMetaStore{
			meta: &types.SessionMeta{SessionID: "sess-1", WorkspaceID: "ws-1", WorktreeID: "wt-1"},
		},
	})
	_, err := resolver.ResolveScope(context.Background(), types.FileSearchScope{
		SessionID:   "sess-1",
		WorkspaceID: "ws-other",
		Cwd:         "/other",
	})
	if err == nil {
		t.Fatalf("expected conflicting scope to fail")
	}
}

func TestDaemonFileSearchScopeResolverReturnsNotFoundWhenSessionMissing(t *testing.T) {
	resolver := NewDaemonFileSearchScopeResolver(nil, &Stores{})
	_, err := resolver.ResolveScope(context.Background(), types.FileSearchScope{SessionID: "missing"})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorNotFound {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDaemonFileSearchScopeResolverReturnsUnavailableOnSessionLookupError(t *testing.T) {
	resolver := NewDaemonFileSearchScopeResolver(nil, &Stores{
		Sessions: errSessionIndexStore{err: errors.New("sessions down")},
	})
	_, err := resolver.ResolveScope(context.Background(), types.FileSearchScope{SessionID: "sess-1"})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDaemonFileSearchScopeResolverReturnsUnavailableOnMetadataLookupError(t *testing.T) {
	resolver := NewDaemonFileSearchScopeResolver(nil, &Stores{
		Sessions: fileSearchStubSessionIndexStore{
			record: &types.SessionRecord{Session: &types.Session{ID: "sess-1", Provider: "codex"}},
		},
		SessionMeta: errSessionMetaStore{err: errors.New("meta down")},
	})
	_, err := resolver.ResolveScope(context.Background(), types.FileSearchScope{SessionID: "sess-1"})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDaemonFileSearchScopeResolverRejectsMissingProviderAfterLookup(t *testing.T) {
	resolver := NewDaemonFileSearchScopeResolver(nil, &Stores{
		SessionMeta: fileSearchStubSessionMetaStore{
			meta: &types.SessionMeta{SessionID: "sess-1", WorkspaceID: "ws-1"},
		},
	})
	_, err := resolver.ResolveScope(context.Background(), types.FileSearchScope{SessionID: "sess-1"})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorInvalid {
		t.Fatalf("unexpected error: %#v", err)
	}
}
