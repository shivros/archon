package app

import (
	"context"
	"errors"
	"testing"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

func TestReduceMutationMessagesUpdateWorkspaceSuccessExitsEditMode(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"},
	}
	m.enterEditWorkspace("ws1")
	if m.mode != uiModeEditWorkspace {
		t.Fatalf("expected edit workspace mode, got %v", m.mode)
	}
	if m.editWorkspace == nil || m.editWorkspace.workspaceID != "ws1" {
		t.Fatalf("expected edit controller to track workspace id")
	}

	handled, cmd := m.reduceMutationMessages(updateWorkspaceMsg{
		workspace: &types.Workspace{ID: "ws1", Name: "Renamed"},
	})
	if !handled {
		t.Fatalf("expected update workspace message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected refresh command batch")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected success to exit edit mode, got %v", m.mode)
	}
	if m.renameWorkspaceID != "" {
		t.Fatalf("expected edit workspace id to clear, got %q", m.renameWorkspaceID)
	}
	if m.editWorkspace != nil && m.editWorkspace.workspaceID != "" {
		t.Fatalf("expected edit workspace controller to clear state, got %q", m.editWorkspace.workspaceID)
	}
	if m.status != "workspace updated" {
		t.Fatalf("unexpected status after success: %q", m.status)
	}
}

func TestReduceMutationMessagesUpdateWorkspaceErrorKeepsEditMode(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"},
	}
	m.enterEditWorkspace("ws1")
	if m.mode != uiModeEditWorkspace {
		t.Fatalf("expected edit workspace mode, got %v", m.mode)
	}

	handled, cmd := m.reduceMutationMessages(updateWorkspaceMsg{err: errors.New("boom")})
	if !handled {
		t.Fatalf("expected update workspace error message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command on error")
	}
	if m.mode != uiModeEditWorkspace {
		t.Fatalf("expected error to keep edit mode, got %v", m.mode)
	}
	if m.renameWorkspaceID != "ws1" {
		t.Fatalf("expected edit workspace id to remain set, got %q", m.renameWorkspaceID)
	}
	if m.status != "update workspace error: boom" {
		t.Fatalf("unexpected status after error: %q", m.status)
	}
}

func TestReduceMutationMessagesUpdateWorkspaceSuccessDoesNotExitOutsideEditMode(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNormal
	m.renameWorkspaceID = "keep"
	if m.editWorkspace == nil {
		t.Fatalf("expected edit workspace controller")
	}
	m.editWorkspace.workspaceID = "keep"

	handled, cmd := m.reduceMutationMessages(updateWorkspaceMsg{
		workspace: &types.Workspace{ID: "ws1", Name: "Renamed"},
	})
	if !handled {
		t.Fatalf("expected update workspace message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected refresh command")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to remain normal, got %v", m.mode)
	}
	if m.renameWorkspaceID != "keep" {
		t.Fatalf("expected renameWorkspaceID to remain unchanged, got %q", m.renameWorkspaceID)
	}
	if m.editWorkspace.workspaceID != "keep" {
		t.Fatalf("expected controller workspace id to remain unchanged, got %q", m.editWorkspace.workspaceID)
	}
}

func TestReduceMutationMessagesUpdateWorkspaceSuccessEmitsRefreshBatch(t *testing.T) {
	m := NewModel(nil)
	workspaceAPI := &workspaceRefreshAPIStub{}
	sessionAPI := &sessionSelectionRefreshStub{}
	m.workspaceAPI = workspaceAPI
	m.sessionSelectionAPI = sessionAPI
	m.enterEditWorkspace("ws1")

	handled, cmd := m.reduceMutationMessages(updateWorkspaceMsg{
		workspace: &types.Workspace{ID: "ws1", Name: "Renamed"},
	})
	if !handled {
		t.Fatalf("expected update workspace message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected refresh command batch")
	}

	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", msg)
	}
	if len(batch) != 3 {
		t.Fatalf("expected 3 commands in refresh batch, got %d", len(batch))
	}

	var (
		sawWorkspacesMsg      bool
		sawWorkspaceGroupsMsg bool
		sawSessionsMsg        bool
	)
	for _, batchedCmd := range batch {
		if batchedCmd == nil {
			t.Fatalf("expected non-nil batched command")
		}
		switch batchedCmd().(type) {
		case workspacesMsg:
			sawWorkspacesMsg = true
		case workspaceGroupsMsg:
			sawWorkspaceGroupsMsg = true
		case sessionsWithMetaMsg:
			sawSessionsMsg = true
		default:
			t.Fatalf("unexpected batched message type")
		}
	}
	if !sawWorkspacesMsg || !sawWorkspaceGroupsMsg || !sawSessionsMsg {
		t.Fatalf("expected workspace, group, and session refresh messages: workspaces=%v groups=%v sessions=%v", sawWorkspacesMsg, sawWorkspaceGroupsMsg, sawSessionsMsg)
	}
	if workspaceAPI.listWorkspacesCalls != 1 {
		t.Fatalf("expected exactly one workspace refresh call, got %d", workspaceAPI.listWorkspacesCalls)
	}
	if workspaceAPI.listWorkspaceGroupsCalls != 1 {
		t.Fatalf("expected exactly one workspace group refresh call, got %d", workspaceAPI.listWorkspaceGroupsCalls)
	}
	if sessionAPI.calls != 1 {
		t.Fatalf("expected exactly one sessions refresh call, got %d", sessionAPI.calls)
	}
}

type workspaceRefreshAPIStub struct {
	WorkspaceAPI
	listWorkspacesCalls      int
	listWorkspaceGroupsCalls int
}

func (s *workspaceRefreshAPIStub) ListWorkspaces(context.Context) ([]*types.Workspace, error) {
	s.listWorkspacesCalls++
	return []*types.Workspace{}, nil
}

func (s *workspaceRefreshAPIStub) ListWorkspaceGroups(context.Context) ([]*types.WorkspaceGroup, error) {
	s.listWorkspaceGroupsCalls++
	return []*types.WorkspaceGroup{}, nil
}

type sessionSelectionRefreshStub struct {
	calls int
}

func (s *sessionSelectionRefreshStub) ListSessionsWithMetaQuery(context.Context, SessionListQuery) ([]*types.Session, []*types.SessionMeta, error) {
	s.calls++
	return []*types.Session{}, []*types.SessionMeta{}, nil
}
