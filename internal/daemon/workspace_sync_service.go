package daemon

import (
	"context"

	"control/internal/types"
)

type WorkspaceSyncService struct {
	base   *WorkspaceService
	syncer SessionSyncer
}

func NewWorkspaceSyncService(stores *Stores, syncer SessionSyncer) *WorkspaceSyncService {
	return &WorkspaceSyncService{
		base:   NewWorkspaceService(stores),
		syncer: syncer,
	}
}

func (s *WorkspaceSyncService) List(ctx context.Context) ([]*types.Workspace, error) {
	return s.base.List(ctx)
}

func (s *WorkspaceSyncService) Create(ctx context.Context, req *types.Workspace) (*types.Workspace, error) {
	ws, err := s.base.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	s.syncWorkspace(ws)
	return ws, nil
}

func (s *WorkspaceSyncService) Update(ctx context.Context, id string, req *types.Workspace) (*types.Workspace, error) {
	return s.base.Update(ctx, id, req)
}

func (s *WorkspaceSyncService) Delete(ctx context.Context, id string) error {
	return s.base.Delete(ctx, id)
}

func (s *WorkspaceSyncService) ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error) {
	return s.base.ListWorktrees(ctx, workspaceID)
}

func (s *WorkspaceSyncService) AddWorktree(ctx context.Context, workspaceID string, req *types.Worktree) (*types.Worktree, error) {
	wt, err := s.base.AddWorktree(ctx, workspaceID, req)
	if err != nil {
		return nil, err
	}
	s.syncWorkspace(&types.Workspace{ID: workspaceID})
	return wt, nil
}

func (s *WorkspaceSyncService) DeleteWorktree(ctx context.Context, workspaceID, worktreeID string) error {
	return s.base.DeleteWorktree(ctx, workspaceID, worktreeID)
}

func (s *WorkspaceSyncService) ListAvailableWorktrees(ctx context.Context, workspaceID string) ([]*types.GitWorktree, error) {
	return s.base.ListAvailableWorktrees(ctx, workspaceID)
}

func (s *WorkspaceSyncService) CreateWorktree(ctx context.Context, workspaceID string, req *CreateWorktreeRequest) (*types.Worktree, error) {
	wt, err := s.base.CreateWorktree(ctx, workspaceID, req)
	if err != nil {
		return nil, err
	}
	s.syncWorkspace(&types.Workspace{ID: workspaceID})
	return wt, nil
}

func (s *WorkspaceSyncService) syncWorkspace(ws *types.Workspace) {
	if ws == nil || ws.ID == "" || s.syncer == nil {
		return
	}
	go func(id string) {
		_ = s.syncer.SyncWorkspace(context.Background(), id)
	}(ws.ID)
}
