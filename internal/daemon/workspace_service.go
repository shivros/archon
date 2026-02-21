package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"control/internal/store"
	"control/internal/types"
)

type WorkspaceService struct {
	workspaces WorkspaceStore
	worktrees  WorktreeStore
	paths      WorkspacePathResolver
}

type CreateWorktreeRequest struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Name   string `json:"name,omitempty"`
}

type WorkspaceUpdateRequest struct {
	Name           *string   `json:"name,omitempty"`
	RepoPath       *string   `json:"repo_path,omitempty"`
	SessionSubpath *string   `json:"session_subpath,omitempty"`
	GroupIDs       *[]string `json:"group_ids,omitempty"`
}

func NewWorkspaceService(stores *Stores) *WorkspaceService {
	return NewWorkspaceServiceWithPathResolver(stores, nil)
}

func NewWorkspaceServiceWithPathResolver(stores *Stores, paths WorkspacePathResolver) *WorkspaceService {
	resolver := workspacePathResolverOrDefault(paths)
	if stores == nil {
		return &WorkspaceService{paths: resolver}
	}
	return &WorkspaceService{
		workspaces: stores.Workspaces,
		worktrees:  stores.Worktrees,
		paths:      resolver,
	}
}

func (s *WorkspaceService) List(ctx context.Context) ([]*types.Workspace, error) {
	if s.workspaces == nil {
		return nil, unavailableError("workspace store not available", nil)
	}
	return s.workspaces.List(ctx)
}

func (s *WorkspaceService) Create(ctx context.Context, req *types.Workspace) (*types.Workspace, error) {
	if s.workspaces == nil {
		return nil, unavailableError("workspace store not available", nil)
	}
	if req == nil {
		return nil, invalidError("workspace payload is required", nil)
	}
	if err := workspacePathResolverOrDefault(s.paths).ValidateWorkspace(req.RepoPath, req.SessionSubpath); err != nil {
		return nil, invalidError(err.Error(), err)
	}
	ws, err := s.workspaces.Add(ctx, req)
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
	return ws, nil
}

func (s *WorkspaceService) Update(ctx context.Context, id string, req *WorkspaceUpdateRequest) (*types.Workspace, error) {
	if s.workspaces == nil {
		return nil, unavailableError("workspace store not available", nil)
	}
	if strings.TrimSpace(id) == "" {
		return nil, invalidError("workspace id is required", nil)
	}
	if req == nil {
		return nil, invalidError("workspace payload is required", nil)
	}

	existing, ok, err := s.workspaces.Get(ctx, id)
	if err != nil {
		return nil, unavailableError(err.Error(), err)
	}
	if !ok {
		return nil, notFoundError("workspace not found", store.ErrWorkspaceNotFound)
	}

	providedRepoPath := ""
	if req.RepoPath != nil {
		providedRepoPath = strings.TrimSpace(*req.RepoPath)
	}
	providedSessionSubpath := ""
	sessionSubpathProvided := req.SessionSubpath != nil
	if req.SessionSubpath != nil {
		providedSessionSubpath = strings.TrimSpace(*req.SessionSubpath)
	}
	name := ""
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	merged := &types.Workspace{
		ID:             id,
		Name:           name,
		RepoPath:       providedRepoPath,
		SessionSubpath: existing.SessionSubpath,
		GroupIDs:       nil,
	}
	if merged.Name == "" {
		merged.Name = existing.Name
	}
	if merged.RepoPath == "" {
		merged.RepoPath = existing.RepoPath
	}
	if sessionSubpathProvided {
		merged.SessionSubpath = providedSessionSubpath
	}
	if req.GroupIDs != nil {
		merged.GroupIDs = append([]string(nil), (*req.GroupIDs)...)
	} else {
		merged.GroupIDs = existing.GroupIDs
	}
	if req.RepoPath != nil || req.SessionSubpath != nil {
		if err := workspacePathResolverOrDefault(s.paths).ValidateWorkspace(merged.RepoPath, merged.SessionSubpath); err != nil {
			return nil, invalidError(err.Error(), err)
		}
	}

	ws, err := s.workspaces.Update(ctx, merged)
	if err != nil {
		if errors.Is(err, store.ErrWorkspaceNotFound) {
			return nil, notFoundError("workspace not found", err)
		}
		return nil, invalidError(err.Error(), err)
	}
	return ws, nil
}

func (s *WorkspaceService) Delete(ctx context.Context, id string) error {
	if s.workspaces == nil {
		return unavailableError("workspace store not available", nil)
	}
	if strings.TrimSpace(id) == "" {
		return invalidError("workspace id is required", nil)
	}
	if err := s.workspaces.Delete(ctx, id); err != nil {
		if errors.Is(err, store.ErrWorkspaceNotFound) {
			return notFoundError("workspace not found", err)
		}
		return invalidError(err.Error(), err)
	}
	return nil
}

func (s *WorkspaceService) ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error) {
	if s.worktrees == nil {
		return nil, unavailableError("worktree store not available", nil)
	}
	if strings.TrimSpace(workspaceID) == "" {
		return nil, invalidError("workspace id is required", nil)
	}
	worktrees, err := s.worktrees.ListWorktrees(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, store.ErrWorkspaceNotFound) {
			return nil, notFoundError("workspace not found", err)
		}
		return nil, invalidError(err.Error(), err)
	}
	return worktrees, nil
}

func (s *WorkspaceService) AddWorktree(ctx context.Context, workspaceID string, req *types.Worktree) (*types.Worktree, error) {
	if s.worktrees == nil {
		return nil, unavailableError("worktree store not available", nil)
	}
	if strings.TrimSpace(workspaceID) == "" {
		return nil, invalidError("workspace id is required", nil)
	}
	if req == nil {
		return nil, invalidError("worktree payload is required", nil)
	}
	if err := validateWorkspacePath(req.Path); err != nil {
		return nil, invalidError(err.Error(), err)
	}
	wt, err := s.worktrees.AddWorktree(ctx, workspaceID, req)
	if err != nil {
		if errors.Is(err, store.ErrWorkspaceNotFound) {
			return nil, notFoundError("workspace not found", err)
		}
		return nil, invalidError(err.Error(), err)
	}
	return wt, nil
}

func (s *WorkspaceService) UpdateWorktree(ctx context.Context, workspaceID, worktreeID string, req *types.Worktree) (*types.Worktree, error) {
	if s.worktrees == nil {
		return nil, unavailableError("worktree store not available", nil)
	}
	if strings.TrimSpace(workspaceID) == "" {
		return nil, invalidError("workspace id is required", nil)
	}
	if strings.TrimSpace(worktreeID) == "" {
		return nil, invalidError("worktree id is required", nil)
	}
	if req == nil {
		return nil, invalidError("worktree payload is required", nil)
	}

	worktrees, err := s.worktrees.ListWorktrees(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, store.ErrWorkspaceNotFound) {
			return nil, notFoundError("workspace not found", err)
		}
		return nil, invalidError(err.Error(), err)
	}
	var existing *types.Worktree
	for _, candidate := range worktrees {
		if candidate != nil && candidate.ID == worktreeID {
			existing = candidate
			break
		}
	}
	if existing == nil {
		return nil, notFoundError("worktree not found", store.ErrWorktreeNotFound)
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = existing.Name
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		path = existing.Path
	}
	if strings.TrimSpace(req.Path) != "" {
		if err := validateWorkspacePath(path); err != nil {
			return nil, invalidError(err.Error(), err)
		}
	}

	wt, err := s.worktrees.UpdateWorktree(ctx, workspaceID, &types.Worktree{
		ID:                    worktreeID,
		Name:                  name,
		Path:                  path,
		NotificationOverrides: mergeWorktreeNotificationOverrides(existing.NotificationOverrides, req.NotificationOverrides),
	})
	if err != nil {
		if errors.Is(err, store.ErrWorkspaceNotFound) {
			return nil, notFoundError("workspace not found", err)
		}
		if errors.Is(err, store.ErrWorktreeNotFound) {
			return nil, notFoundError("worktree not found", err)
		}
		return nil, invalidError(err.Error(), err)
	}
	return wt, nil
}

func mergeWorktreeNotificationOverrides(existing, incoming *types.NotificationSettingsPatch) *types.NotificationSettingsPatch {
	if incoming == nil {
		return types.CloneNotificationSettingsPatch(existing)
	}
	return types.CloneNotificationSettingsPatch(incoming)
}

func (s *WorkspaceService) DeleteWorktree(ctx context.Context, workspaceID, worktreeID string) error {
	if s.worktrees == nil {
		return unavailableError("worktree store not available", nil)
	}
	if strings.TrimSpace(workspaceID) == "" {
		return invalidError("workspace id is required", nil)
	}
	if strings.TrimSpace(worktreeID) == "" {
		return invalidError("worktree id is required", nil)
	}
	if err := s.worktrees.DeleteWorktree(ctx, workspaceID, worktreeID); err != nil {
		if errors.Is(err, store.ErrWorktreeNotFound) || errors.Is(err, store.ErrWorkspaceNotFound) {
			return notFoundError("worktree not found", err)
		}
		return invalidError(err.Error(), err)
	}
	return nil
}

func (s *WorkspaceService) ListAvailableWorktrees(ctx context.Context, workspaceID string) ([]*types.GitWorktree, error) {
	if s.workspaces == nil {
		return nil, unavailableError("workspace store not available", nil)
	}
	if strings.TrimSpace(workspaceID) == "" {
		return nil, invalidError("workspace id is required", nil)
	}
	ws, ok, err := s.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return nil, unavailableError(err.Error(), err)
	}
	if !ok {
		return nil, notFoundError("workspace not found", store.ErrWorkspaceNotFound)
	}
	worktrees, err := listGitWorktrees(ws.RepoPath)
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
	return worktrees, nil
}

func (s *WorkspaceService) CreateWorktree(ctx context.Context, workspaceID string, req *CreateWorktreeRequest) (*types.Worktree, error) {
	if s.workspaces == nil || s.worktrees == nil {
		return nil, unavailableError("workspace store not available", nil)
	}
	if strings.TrimSpace(workspaceID) == "" {
		return nil, invalidError("workspace id is required", nil)
	}
	if req == nil {
		return nil, invalidError("worktree payload is required", nil)
	}
	ws, ok, err := s.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return nil, unavailableError(err.Error(), err)
	}
	if !ok {
		return nil, notFoundError("workspace not found", store.ErrWorkspaceNotFound)
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return nil, invalidError("worktree path is required", nil)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(ws.RepoPath, path)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
	if err := createGitWorktree(ws.RepoPath, path, req.Branch); err != nil {
		return nil, invalidError(err.Error(), err)
	}
	return s.AddWorktree(ctx, workspaceID, &types.Worktree{
		Name: strings.TrimSpace(req.Name),
		Path: path,
	})
}

func validateWorkspacePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("path is not a directory")
	}
	return nil
}
