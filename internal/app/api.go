package app

import (
	"context"

	"control/internal/client"
	"control/internal/types"
)

type WorkspaceAPI interface {
	ListWorkspaces(ctx context.Context) ([]*types.Workspace, error)
	CreateWorkspace(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error)
	ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error)
	ListAvailableWorktrees(ctx context.Context, workspaceID string) ([]*types.GitWorktree, error)
	AddWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error)
	CreateWorktree(ctx context.Context, workspaceID string, req client.CreateWorktreeRequest) (*types.Worktree, error)
}

type SessionAPI interface {
	ListSessionsWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error)
	TailItems(ctx context.Context, id string, lines int) (*client.TailItemsResponse, error)
	History(ctx context.Context, id string, lines int) (*client.TailItemsResponse, error)
	TailStream(ctx context.Context, id, stream string) (<-chan types.LogEvent, func(), error)
	EventStream(ctx context.Context, id string) (<-chan types.CodexEvent, func(), error)
	KillSession(ctx context.Context, id string) error
	MarkSessionExited(ctx context.Context, id string) error
	SendMessage(ctx context.Context, id string, req client.SendSessionRequest) (*client.SendSessionResponse, error)
}

type StateAPI interface {
	GetAppState(ctx context.Context) (*types.AppState, error)
	UpdateAppState(ctx context.Context, state *types.AppState) (*types.AppState, error)
}

type ClientAPI struct {
	client *client.Client
}

func NewClientAPI(client *client.Client) *ClientAPI {
	return &ClientAPI{client: client}
}

func (a *ClientAPI) ListWorkspaces(ctx context.Context) ([]*types.Workspace, error) {
	return a.client.ListWorkspaces(ctx)
}

func (a *ClientAPI) CreateWorkspace(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error) {
	return a.client.CreateWorkspace(ctx, workspace)
}

func (a *ClientAPI) ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error) {
	return a.client.ListWorktrees(ctx, workspaceID)
}

func (a *ClientAPI) ListAvailableWorktrees(ctx context.Context, workspaceID string) ([]*types.GitWorktree, error) {
	return a.client.ListAvailableWorktrees(ctx, workspaceID)
}

func (a *ClientAPI) AddWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error) {
	return a.client.AddWorktree(ctx, workspaceID, worktree)
}

func (a *ClientAPI) CreateWorktree(ctx context.Context, workspaceID string, req client.CreateWorktreeRequest) (*types.Worktree, error) {
	return a.client.CreateWorktree(ctx, workspaceID, req)
}

func (a *ClientAPI) ListSessionsWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return a.client.ListSessionsWithMeta(ctx)
}

func (a *ClientAPI) TailItems(ctx context.Context, id string, lines int) (*client.TailItemsResponse, error) {
	return a.client.TailItems(ctx, id, lines)
}

func (a *ClientAPI) History(ctx context.Context, id string, lines int) (*client.TailItemsResponse, error) {
	return a.client.History(ctx, id, lines)
}

func (a *ClientAPI) TailStream(ctx context.Context, id, stream string) (<-chan types.LogEvent, func(), error) {
	return a.client.TailStream(ctx, id, stream)
}

func (a *ClientAPI) EventStream(ctx context.Context, id string) (<-chan types.CodexEvent, func(), error) {
	return a.client.EventStream(ctx, id)
}

func (a *ClientAPI) KillSession(ctx context.Context, id string) error {
	return a.client.KillSession(ctx, id)
}

func (a *ClientAPI) MarkSessionExited(ctx context.Context, id string) error {
	return a.client.MarkSessionExited(ctx, id)
}

func (a *ClientAPI) SendMessage(ctx context.Context, id string, req client.SendSessionRequest) (*client.SendSessionResponse, error) {
	return a.client.SendMessage(ctx, id, req)
}

func (a *ClientAPI) GetAppState(ctx context.Context) (*types.AppState, error) {
	return a.client.GetAppState(ctx)
}

func (a *ClientAPI) UpdateAppState(ctx context.Context, state *types.AppState) (*types.AppState, error) {
	return a.client.UpdateAppState(ctx, state)
}
