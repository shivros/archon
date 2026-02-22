package app

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

func TestWorkspaceEditFlowSubmitsPatchThroughModelReducer(t *testing.T) {
	m := NewModel(nil)
	capture := &captureWorkspaceUpdateAPI{}
	m.workspaceAPI = capture
	m.workspaces = []*types.Workspace{
		{
			ID:                    "ws1",
			Name:                  "Workspace One",
			RepoPath:              "/tmp/repo-one",
			SessionSubpath:        "packages/one",
			AdditionalDirectories: []string{"../backend"},
		},
	}

	m.enterEditWorkspace("ws1")
	if m.mode != uiModeEditWorkspace {
		t.Fatalf("expected edit workspace mode, got %v", m.mode)
	}
	if m.editWorkspace == nil || m.editWorkspace.input == nil {
		t.Fatalf("expected edit workspace input")
	}

	m.editWorkspace.input.SetValue("/tmp/repo-two")
	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || cmd != nil {
		t.Fatalf("expected first step to be handled without command")
	}

	m.editWorkspace.input.SetValue("")
	handled, cmd = m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || cmd != nil {
		t.Fatalf("expected second step to be handled without command")
	}

	m.editWorkspace.input.SetValue("")
	handled, cmd = m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || cmd != nil {
		t.Fatalf("expected third step to be handled without command")
	}

	m.editWorkspace.input.SetValue("Workspace Two")
	handled, cmd = m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected final step to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected update command on final step")
	}

	msg, ok := cmd().(updateWorkspaceMsg)
	if !ok {
		t.Fatalf("expected updateWorkspaceMsg, got %T", cmd())
	}
	if msg.err != nil {
		t.Fatalf("expected no update error, got %v", msg.err)
	}
	if capture.lastID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", capture.lastID)
	}
	if capture.lastPatch == nil {
		t.Fatalf("expected workspace patch")
	}
	if capture.lastPatch.RepoPath == nil || *capture.lastPatch.RepoPath != "/tmp/repo-two" {
		t.Fatalf("unexpected repo_path patch: %#v", capture.lastPatch.RepoPath)
	}
	if capture.lastPatch.SessionSubpath == nil || *capture.lastPatch.SessionSubpath != "" {
		t.Fatalf("unexpected session_subpath patch: %#v", capture.lastPatch.SessionSubpath)
	}
	if capture.lastPatch.AdditionalDirectories == nil || len(*capture.lastPatch.AdditionalDirectories) != 0 {
		t.Fatalf("expected additional directories clear patch, got %#v", capture.lastPatch.AdditionalDirectories)
	}
	if capture.lastPatch.Name == nil || *capture.lastPatch.Name != "Workspace Two" {
		t.Fatalf("unexpected name patch: %#v", capture.lastPatch.Name)
	}
	if m.status != "updating workspace" {
		t.Fatalf("expected status to reflect workspace update, got %q", m.status)
	}
}

func TestExitEditWorkspaceClearsState(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeEditWorkspace
	m.renameWorkspaceID = "ws1"
	if m.editWorkspace == nil {
		t.Fatalf("expected edit workspace controller")
	}
	m.editWorkspace.workspaceID = "ws1"

	m.exitEditWorkspace("done")

	if m.mode != uiModeNormal {
		t.Fatalf("expected normal mode, got %v", m.mode)
	}
	if m.renameWorkspaceID != "" {
		t.Fatalf("expected workspace id to clear")
	}
	if m.editWorkspace.workspaceID != "" {
		t.Fatalf("expected edit controller state to clear")
	}
	if m.status != "done" {
		t.Fatalf("expected status to update, got %q", m.status)
	}
}

type captureWorkspaceUpdateAPI struct {
	WorkspaceAPI
	lastID    string
	lastPatch *types.WorkspacePatch
}

func (c *captureWorkspaceUpdateAPI) UpdateWorkspace(_ context.Context, id string, patch *types.WorkspacePatch) (*types.Workspace, error) {
	c.lastID = id
	c.lastPatch = patch
	return &types.Workspace{
		ID:       id,
		RepoPath: "/tmp/repo-two",
		Name:     "Workspace Two",
	}, nil
}
