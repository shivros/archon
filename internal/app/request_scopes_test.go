package app

import (
	"testing"

	"control/internal/types"
)

func TestReplaceRequestScopeCancelsPreviousScope(t *testing.T) {
	m := NewModel(nil)
	first := m.replaceRequestScope(requestScopeSessionLoad)
	second := m.replaceRequestScope(requestScopeSessionLoad)

	select {
	case <-first.Done():
	default:
		t.Fatalf("expected first scope context to be canceled")
	}
	if err := second.Err(); err != nil {
		t.Fatalf("expected second scope context to remain active, got err=%v", err)
	}
}

func TestResetStreamCancelsSessionScopes(t *testing.T) {
	m := NewModel(nil)
	loadCtx := m.replaceRequestScope(requestScopeSessionLoad)
	startCtx := m.replaceRequestScope(requestScopeSessionStart)

	m.resetStream()

	select {
	case <-loadCtx.Done():
	default:
		t.Fatalf("expected session load scope to be canceled")
	}
	select {
	case <-startCtx.Done():
	default:
		t.Fatalf("expected session start scope to be canceled")
	}
	if _, ok := m.requestScopes[requestScopeSessionLoad]; ok {
		t.Fatalf("expected session load scope entry to be removed")
	}
	if _, ok := m.requestScopes[requestScopeSessionStart]; ok {
		t.Fatalf("expected session start scope entry to be removed")
	}
}

func TestWorktreeRefreshWorkspaceIDsPrefersActiveWorkspace(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{
		{ID: "ws-1"},
		{ID: "ws-2"},
	}
	if got := m.worktreeRefreshWorkspaceIDs(); len(got) != 1 || got[0] != "ws-1" {
		t.Fatalf("expected fallback first workspace, got=%v", got)
	}

	m.appState.ActiveWorkspaceID = "ws-2"
	if got := m.worktreeRefreshWorkspaceIDs(); len(got) != 1 || got[0] != "ws-2" {
		t.Fatalf("expected active workspace only, got=%v", got)
	}

	m.appState.ActiveWorkspaceID = unassignedWorkspaceID
	if got := m.worktreeRefreshWorkspaceIDs(); len(got) != 1 || got[0] != "ws-1" {
		t.Fatalf("expected unassigned active workspace to fallback, got=%v", got)
	}
}

func TestClearPendingComposeOptionRequestCancelsProviderScope(t *testing.T) {
	m := NewModel(nil)
	ctx := m.replaceRequestScope(requestScopeProviderOption)
	m.pendingComposeOptionTarget = composeOptionModel
	m.pendingComposeOptionFor = "opencode"

	m.clearPendingComposeOptionRequest()

	select {
	case <-ctx.Done():
	default:
		t.Fatalf("expected provider option scope to be canceled")
	}
	if m.pendingComposeOptionTarget != composeOptionNone {
		t.Fatalf("expected pending compose option target to reset")
	}
	if m.pendingComposeOptionFor != "" {
		t.Fatalf("expected pending compose option provider to reset")
	}
}
