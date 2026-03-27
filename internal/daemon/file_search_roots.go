package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"control/internal/store"
	"control/internal/types"
	"control/internal/workspacepaths"
)

type FileSearchRoot struct {
	Path        string
	DisplayBase string
	Primary     bool
}

type FileSearchRootResolver interface {
	ResolveRoots(ctx context.Context, scope types.FileSearchScope) ([]FileSearchRoot, error)
}

type fileSearchRootContext struct {
	workspace *types.Workspace
	worktree  *types.Worktree
}

type fileSearchRootContextLoader interface {
	Load(ctx context.Context, scope types.FileSearchScope) (fileSearchRootContext, error)
}

type passthroughFileSearchRootResolver struct {
	checker workspacepaths.DirChecker
}

func NewPassthroughFileSearchRootResolver() FileSearchRootResolver {
	return passthroughFileSearchRootResolver{checker: workspacepaths.OSDirChecker()}
}

func (r passthroughFileSearchRootResolver) ResolveRoots(_ context.Context, scope types.FileSearchScope) ([]FileSearchRoot, error) {
	scope = normalizeFileSearchScope(scope)
	if scope.Cwd == "" {
		return nil, invalidError("scope.cwd is required for direct file search roots", nil)
	}
	root, err := normalizeFileSearchRootPath(scope.Cwd, r.checker)
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
	return []FileSearchRoot{{
		Path:        root,
		DisplayBase: root,
		Primary:     true,
	}}, nil
}

type daemonFileSearchRootResolver struct {
	contextLoader fileSearchRootContextLoader
	paths         WorkspacePathResolver
	checker       workspacepaths.DirChecker
}

func NewDaemonFileSearchRootResolver(stores *Stores, paths WorkspacePathResolver) FileSearchRootResolver {
	resolver := daemonFileSearchRootResolver{
		contextLoader: newDaemonFileSearchRootContextLoader(stores),
		paths:         workspacePathResolverOrDefault(paths),
		checker:       workspacepaths.OSDirChecker(),
	}
	return resolver
}

func fileSearchRootResolverOrDefault(resolver FileSearchRootResolver) FileSearchRootResolver {
	if resolver != nil {
		return resolver
	}
	return NewPassthroughFileSearchRootResolver()
}

func (r daemonFileSearchRootResolver) ResolveRoots(ctx context.Context, scope types.FileSearchScope) ([]FileSearchRoot, error) {
	scope = normalizeFileSearchScope(scope)
	context, err := r.loadContext(ctx, scope)
	if err != nil {
		return nil, err
	}

	primary, err := r.resolvePrimaryRoot(scope, context)
	if err != nil {
		return nil, err
	}

	roots := []FileSearchRoot{{
		Path:        primary,
		DisplayBase: primary,
		Primary:     true,
	}}
	if context.workspace == nil || len(context.workspace.AdditionalDirectories) == 0 {
		return roots, nil
	}

	dirs, err := workspacepaths.ResolveAdditionalDirectories(primary, context.workspace.AdditionalDirectories, r.checker)
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
	seen := map[string]struct{}{primary: {}}
	for _, directory := range dirs {
		directory = filepath.Clean(strings.TrimSpace(directory))
		if directory == "" {
			continue
		}
		if _, ok := seen[directory]; ok {
			continue
		}
		seen[directory] = struct{}{}
		roots = append(roots, FileSearchRoot{
			Path:        directory,
			DisplayBase: primary,
		})
	}
	return roots, nil
}

func (r daemonFileSearchRootResolver) loadContext(ctx context.Context, scope types.FileSearchScope) (fileSearchRootContext, error) {
	if r.contextLoader == nil {
		return fileSearchRootContext{}, nil
	}
	return r.contextLoader.Load(ctx, scope)
}

func (r daemonFileSearchRootResolver) resolvePrimaryRoot(scope types.FileSearchScope, context fileSearchRootContext) (string, error) {
	if scope.Cwd != "" {
		root, err := normalizeFileSearchRootPath(scope.Cwd, r.checker)
		if err != nil {
			return "", invalidError(err.Error(), err)
		}
		return root, nil
	}
	if context.workspace == nil {
		return "", invalidError("file search scope must resolve to a search root", nil)
	}
	if context.worktree != nil {
		path, err := r.paths.ResolveWorktreeSessionPath(context.workspace, context.worktree)
		if err != nil {
			return "", invalidError(err.Error(), err)
		}
		return filepath.Clean(path), nil
	}
	path, err := r.paths.ResolveWorkspaceSessionPath(context.workspace)
	if err != nil {
		return "", invalidError(err.Error(), err)
	}
	return filepath.Clean(path), nil
}

type daemonFileSearchRootContextLoader struct {
	workspaces WorkspaceStore
	worktrees  WorktreeStore
}

func newDaemonFileSearchRootContextLoader(stores *Stores) fileSearchRootContextLoader {
	loader := daemonFileSearchRootContextLoader{}
	if stores != nil {
		loader.workspaces = stores.Workspaces
		loader.worktrees = stores.Worktrees
	}
	return loader
}

func (l daemonFileSearchRootContextLoader) Load(ctx context.Context, scope types.FileSearchScope) (fileSearchRootContext, error) {
	if scope.WorktreeID != "" && scope.WorkspaceID == "" {
		return fileSearchRootContext{}, invalidError("scope.workspace_id is required when scope.worktree_id is set", nil)
	}
	if scope.WorkspaceID == "" {
		return fileSearchRootContext{}, nil
	}
	if l.workspaces == nil {
		return fileSearchRootContext{}, unavailableError("workspace store not available", nil)
	}
	workspace, ok, err := l.workspaces.Get(ctx, scope.WorkspaceID)
	if err != nil {
		return fileSearchRootContext{}, unavailableError(err.Error(), err)
	}
	if !ok || workspace == nil {
		return fileSearchRootContext{}, notFoundError("workspace not found", store.ErrWorkspaceNotFound)
	}
	context := fileSearchRootContext{workspace: workspace}
	if scope.WorktreeID == "" {
		return context, nil
	}
	if l.worktrees == nil {
		return fileSearchRootContext{}, unavailableError("worktree store not available", nil)
	}
	entries, err := l.worktrees.ListWorktrees(ctx, workspace.ID)
	if err != nil {
		if errors.Is(err, store.ErrWorkspaceNotFound) {
			return fileSearchRootContext{}, notFoundError("workspace not found", err)
		}
		return fileSearchRootContext{}, invalidError(err.Error(), err)
	}
	for _, wt := range entries {
		if wt != nil && wt.ID == scope.WorktreeID {
			context.worktree = wt
			return context, nil
		}
	}
	return fileSearchRootContext{}, notFoundError("worktree not found", store.ErrWorktreeNotFound)
}

func normalizeFileSearchRootPath(path string, checker workspacepaths.DirChecker) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("file search root path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if err := workspacepaths.ValidateDirectory(abs, checker); err != nil {
		return "", err
	}
	return abs, nil
}
