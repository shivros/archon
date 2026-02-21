package daemon

import (
	"errors"

	"control/internal/types"
	"control/internal/workspacepaths"
)

type WorkspacePathResolver interface {
	ValidateWorkspace(repoPath, sessionSubpath string) error
	ResolveWorkspaceSessionPath(workspace *types.Workspace) (string, error)
	ResolveWorktreeSessionPath(workspace *types.Workspace, worktree *types.Worktree) (string, error)
}

type defaultWorkspacePathResolver struct {
	checker workspacepaths.DirChecker
}

func NewWorkspacePathResolver() WorkspacePathResolver {
	return NewWorkspacePathResolverWithChecker(workspacepaths.OSDirChecker())
}

func NewWorkspacePathResolverWithChecker(checker workspacepaths.DirChecker) WorkspacePathResolver {
	if checker == nil {
		checker = workspacepaths.OSDirChecker()
	}
	return &defaultWorkspacePathResolver{checker: checker}
}

func (r *defaultWorkspacePathResolver) ValidateWorkspace(repoPath, sessionSubpath string) error {
	return workspacepaths.ValidateRootAndSessionPath(repoPath, sessionSubpath, r.checker)
}

func (r *defaultWorkspacePathResolver) ResolveWorkspaceSessionPath(workspace *types.Workspace) (string, error) {
	if workspace == nil {
		return "", errors.New("workspace is required")
	}
	path, err := workspacepaths.ResolveSessionPath(workspace.RepoPath, workspace.SessionSubpath)
	if err != nil {
		return "", err
	}
	if err := workspacepaths.ValidateDirectory(path, r.checker); err != nil {
		return "", err
	}
	return path, nil
}

func (r *defaultWorkspacePathResolver) ResolveWorktreeSessionPath(workspace *types.Workspace, worktree *types.Worktree) (string, error) {
	if workspace == nil {
		return "", errors.New("workspace is required")
	}
	if worktree == nil {
		return "", errors.New("worktree is required")
	}
	path, err := workspacepaths.ResolveSessionPath(worktree.Path, workspace.SessionSubpath)
	if err != nil {
		return "", err
	}
	if err := workspacepaths.ValidateDirectory(path, r.checker); err != nil {
		return "", err
	}
	return path, nil
}

func workspacePathResolverOrDefault(current WorkspacePathResolver) WorkspacePathResolver {
	if current != nil {
		return current
	}
	return NewWorkspacePathResolver()
}
