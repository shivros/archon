package app

import (
	"context"
	"strings"
	"testing"

	"control/internal/client"
	"control/internal/types"
)

func TestCommandsCompileWithNarrowMocks(t *testing.T) {
	workspacesCmd := fetchWorkspacesCmd(&workspaceListMock{})
	if workspacesCmd == nil {
		t.Fatalf("expected workspaces command")
	}
	if _, ok := workspacesCmd().(workspacesMsg); !ok {
		t.Fatalf("expected workspacesMsg result")
	}

	sendCmd := sendSessionCmd(&sessionSendMock{}, "s1", "hello", 7)
	if sendCmd == nil {
		t.Fatalf("expected send command")
	}
	sendResult, ok := sendCmd().(sendMsg)
	if !ok {
		t.Fatalf("expected sendMsg result")
	}
	if sendResult.id != "s1" || sendResult.token != 7 {
		t.Fatalf("unexpected send result: %+v", sendResult)
	}

	updateCmd := updateSessionCmd(&sessionUpdateMock{}, "s1", "renamed")
	if updateCmd == nil {
		t.Fatalf("expected update session command")
	}
	if _, ok := updateCmd().(updateSessionMsg); !ok {
		t.Fatalf("expected updateSessionMsg result")
	}

	startCmd := startSessionCmd(&workspaceSessionStartMock{}, "ws1", "", "codex", "hello", nil)
	if startCmd == nil {
		t.Fatalf("expected start session command")
	}
	if _, ok := startCmd().(startSessionMsg); !ok {
		t.Fatalf("expected startSessionMsg result")
	}

	longSnippet := strings.Repeat("x", 2048)
	pinAPI := &sessionPinMock{}
	pinCmd := pinSessionNoteCmd(pinAPI, "s1", ChatBlock{ID: "b1", Role: ChatRoleAgent, Text: "hello"}, longSnippet)
	if pinCmd == nil {
		t.Fatalf("expected pin command")
	}
	if _, ok := pinCmd().(notePinnedMsg); !ok {
		t.Fatalf("expected notePinnedMsg result")
	}
	if pinAPI.lastRequest.SourceSnippet != longSnippet {
		t.Fatalf("expected full snippet to be preserved, got len=%d want=%d", len(pinAPI.lastRequest.SourceSnippet), len(longSnippet))
	}

	moveCmd := moveNoteCmd(&noteUpdateMock{}, &types.Note{ID: "n1", Scope: types.NoteScopeSession, SessionID: "s1"}, noteScopeTarget{
		Scope:       types.NoteScopeWorktree,
		WorkspaceID: "ws1",
		WorktreeID:  "wt1",
	})
	if moveCmd == nil {
		t.Fatalf("expected move command")
	}
	if _, ok := moveCmd().(noteMovedMsg); !ok {
		t.Fatalf("expected noteMovedMsg result")
	}

	createAPI := &workspaceCreateMock{}
	createCmd := createWorkspaceCmd(createAPI, "/tmp/repo", "packages/pennies", "Repo")
	if createCmd == nil {
		t.Fatalf("expected create workspace command")
	}
	createResult, ok := createCmd().(createWorkspaceMsg)
	if !ok {
		t.Fatalf("expected createWorkspaceMsg result")
	}
	if createResult.workspace == nil {
		t.Fatalf("expected created workspace")
	}
	if createAPI.lastWorkspace == nil || createAPI.lastWorkspace.SessionSubpath != "packages/pennies" {
		t.Fatalf("expected session_subpath to be forwarded, got %#v", createAPI.lastWorkspace)
	}

}

type workspaceListMock struct{}

func (m *workspaceListMock) ListWorkspaces(context.Context) ([]*types.Workspace, error) {
	return []*types.Workspace{{ID: "ws1", Name: "Workspace"}}, nil
}

type sessionSendMock struct{}

func (m *sessionSendMock) SendMessage(context.Context, string, client.SendSessionRequest) (*client.SendSessionResponse, error) {
	return &client.SendSessionResponse{OK: true, TurnID: "turn-1"}, nil
}

type sessionUpdateMock struct{}

func (m *sessionUpdateMock) UpdateSession(context.Context, string, client.UpdateSessionRequest) error {
	return nil
}

type workspaceSessionStartMock struct{}

func (m *workspaceSessionStartMock) StartWorkspaceSession(context.Context, string, string, client.StartSessionRequest) (*types.Session, error) {
	return &types.Session{ID: "s1", Provider: "codex"}, nil
}

type sessionPinMock struct {
	lastRequest client.PinSessionNoteRequest
}

func (m *sessionPinMock) PinSessionMessage(_ context.Context, _ string, req client.PinSessionNoteRequest) (*types.Note, error) {
	m.lastRequest = req
	return &types.Note{
		ID: "n1",
		Source: &types.NoteSource{
			Snippet: req.SourceSnippet,
		},
	}, nil
}

type noteUpdateMock struct{}

func (m *noteUpdateMock) UpdateNote(_ context.Context, id string, note *types.Note) (*types.Note, error) {
	if note == nil {
		return nil, nil
	}
	clone := *note
	clone.ID = strings.TrimSpace(id)
	return &clone, nil
}

type workspaceCreateMock struct {
	lastWorkspace *types.Workspace
}

func (m *workspaceCreateMock) CreateWorkspace(_ context.Context, workspace *types.Workspace) (*types.Workspace, error) {
	m.lastWorkspace = workspace
	if workspace == nil {
		return nil, nil
	}
	clone := *workspace
	clone.ID = "ws-created"
	return &clone, nil
}
