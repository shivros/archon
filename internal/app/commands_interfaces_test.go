package app

import (
	"context"
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

	startCmd := startSessionCmd(&workspaceSessionStartMock{}, "ws1", "", "codex", "hello")
	if startCmd == nil {
		t.Fatalf("expected start session command")
	}
	if _, ok := startCmd().(startSessionMsg); !ok {
		t.Fatalf("expected startSessionMsg result")
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

type workspaceSessionStartMock struct{}

func (m *workspaceSessionStartMock) StartWorkspaceSession(context.Context, string, string, client.StartSessionRequest) (*types.Session, error) {
	return &types.Session{ID: "s1", Provider: "codex"}, nil
}
