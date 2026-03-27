package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"control/internal/store"
	"control/internal/types"
)

func TestDaemonFileSearchRootResolverUsesWorkspaceSessionPathAndAdditionalDirectories(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	sessionDir := filepath.Join(repoDir, "packages", "pennies")
	backendDir := filepath.Join(repoDir, "packages", "backend")
	sharedDir := filepath.Join(repoDir, "shared")
	for _, dir := range []string{sessionDir, backendDir, sharedDir} {
		if err := ensureDir(dir); err != nil {
			t.Fatalf("ensureDir(%q): %v", dir, err)
		}
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{
		RepoPath:              repoDir,
		SessionSubpath:        filepath.Join("packages", "pennies"),
		AdditionalDirectories: []string{"../backend", "../../shared"},
	})
	if err != nil {
		t.Fatalf("Add workspace: %v", err)
	}

	resolver := NewDaemonFileSearchRootResolver(&Stores{
		Workspaces: workspaceStore,
		Worktrees:  workspaceStore,
	}, nil)
	roots, err := resolver.ResolveRoots(ctx, types.FileSearchScope{WorkspaceID: ws.ID})
	if err != nil {
		t.Fatalf("ResolveRoots: %v", err)
	}
	if len(roots) != 3 {
		t.Fatalf("expected 3 roots, got %#v", roots)
	}
	if !roots[0].Primary || roots[0].Path != sessionDir {
		t.Fatalf("expected primary session root %q, got %#v", sessionDir, roots[0])
	}
	if roots[1].Path != backendDir || roots[1].DisplayBase != sessionDir {
		t.Fatalf("unexpected backend root: %#v", roots[1])
	}
	if roots[2].Path != sharedDir || roots[2].DisplayBase != sessionDir {
		t.Fatalf("unexpected shared root: %#v", roots[2])
	}
}

func TestDaemonFileSearchRootResolverUsesCWDWithWorkspaceAdditionalDirectories(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	sessionDir := filepath.Join(repoDir, "packages", "pennies")
	backendDir := filepath.Join(repoDir, "packages", "backend")
	for _, dir := range []string{sessionDir, backendDir} {
		if err := ensureDir(dir); err != nil {
			t.Fatalf("ensureDir(%q): %v", dir, err)
		}
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{
		RepoPath:              repoDir,
		SessionSubpath:        filepath.Join("packages", "pennies"),
		AdditionalDirectories: []string{"../backend"},
	})
	if err != nil {
		t.Fatalf("Add workspace: %v", err)
	}

	resolver := NewDaemonFileSearchRootResolver(&Stores{
		Workspaces: workspaceStore,
		Worktrees:  workspaceStore,
	}, nil)
	roots, err := resolver.ResolveRoots(ctx, types.FileSearchScope{
		WorkspaceID: ws.ID,
		Cwd:         sessionDir,
	})
	if err != nil {
		t.Fatalf("ResolveRoots: %v", err)
	}
	if len(roots) != 2 || roots[0].Path != sessionDir || roots[1].Path != backendDir {
		t.Fatalf("unexpected roots: %#v", roots)
	}
}

func TestDaemonFileSearchRootResolverUsesWorktreeSessionPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	workspaceSessionDir := filepath.Join(repoDir, "app")
	worktreeDir := filepath.Join(base, "wt-feature")
	worktreeSessionDir := filepath.Join(worktreeDir, "app")
	for _, dir := range []string{workspaceSessionDir, worktreeSessionDir} {
		if err := ensureDir(dir); err != nil {
			t.Fatalf("ensureDir(%q): %v", dir, err)
		}
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{
		RepoPath:       repoDir,
		SessionSubpath: "app",
	})
	if err != nil {
		t.Fatalf("Add workspace: %v", err)
	}
	wt, err := workspaceStore.AddWorktree(ctx, ws.ID, &types.Worktree{
		ID:   "wt-1",
		Path: worktreeDir,
	})
	if err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	resolver := NewDaemonFileSearchRootResolver(&Stores{
		Workspaces: workspaceStore,
		Worktrees:  workspaceStore,
	}, nil)
	roots, err := resolver.ResolveRoots(ctx, types.FileSearchScope{
		WorkspaceID: ws.ID,
		WorktreeID:  wt.ID,
	})
	if err != nil {
		t.Fatalf("ResolveRoots: %v", err)
	}
	if len(roots) != 1 || roots[0].Path != worktreeSessionDir {
		t.Fatalf("unexpected roots: %#v", roots)
	}
}

func TestDaemonFileSearchRootResolverRejectsMissingPrimaryRoot(t *testing.T) {
	t.Parallel()
	resolver := NewDaemonFileSearchRootResolver(nil, nil)
	_, err := resolver.ResolveRoots(context.Background(), types.FileSearchScope{Provider: "opencode"})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorInvalid {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDaemonFileSearchRootResolverRejectsMissingWorkspaceAdditionalDirectory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	sessionDir := filepath.Join(repoDir, "packages", "pennies")
	if err := ensureDir(sessionDir); err != nil {
		t.Fatalf("ensureDir: %v", err)
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{
		RepoPath:              repoDir,
		SessionSubpath:        filepath.Join("packages", "pennies"),
		AdditionalDirectories: []string{"../missing"},
	})
	if err != nil {
		t.Fatalf("Add workspace: %v", err)
	}

	resolver := NewDaemonFileSearchRootResolver(&Stores{
		Workspaces: workspaceStore,
		Worktrees:  workspaceStore,
	}, nil)
	_, err = resolver.ResolveRoots(ctx, types.FileSearchScope{WorkspaceID: ws.ID})
	if err == nil {
		t.Fatalf("expected missing additional directory error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing directory error, got %v", err)
	}
}

func TestPassthroughFileSearchRootResolverUsesDirectCWD(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	resolver := NewPassthroughFileSearchRootResolver()
	roots, err := resolver.ResolveRoots(context.Background(), types.FileSearchScope{Cwd: dir})
	if err != nil {
		t.Fatalf("ResolveRoots: %v", err)
	}
	if len(roots) != 1 || roots[0].Path != dir || !roots[0].Primary {
		t.Fatalf("unexpected roots: %#v", roots)
	}
}

func TestPassthroughFileSearchRootResolverRejectsMissingCWD(t *testing.T) {
	t.Parallel()
	resolver := NewPassthroughFileSearchRootResolver()
	_, err := resolver.ResolveRoots(context.Background(), types.FileSearchScope{})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorInvalid {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDaemonFileSearchRootResolverRejectsMissingWorkspaceStore(t *testing.T) {
	t.Parallel()
	resolver := NewDaemonFileSearchRootResolver(&Stores{}, nil)
	_, err := resolver.ResolveRoots(context.Background(), types.FileSearchScope{WorkspaceID: "ws-1"})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

type errWorkspaceStore struct{ err error }

func (s errWorkspaceStore) List(context.Context) ([]*types.Workspace, error) { return nil, s.err }
func (s errWorkspaceStore) Get(context.Context, string) (*types.Workspace, bool, error) {
	return nil, false, s.err
}
func (s errWorkspaceStore) Add(context.Context, *types.Workspace) (*types.Workspace, error) {
	return nil, s.err
}
func (s errWorkspaceStore) Update(context.Context, *types.Workspace) (*types.Workspace, error) {
	return nil, s.err
}
func (s errWorkspaceStore) Delete(context.Context, string) error { return s.err }

type errWorktreeStore struct{ err error }

func (s errWorktreeStore) ListWorktrees(context.Context, string) ([]*types.Worktree, error) {
	return nil, s.err
}
func (s errWorktreeStore) AddWorktree(context.Context, string, *types.Worktree) (*types.Worktree, error) {
	return nil, s.err
}
func (s errWorktreeStore) UpdateWorktree(context.Context, string, *types.Worktree) (*types.Worktree, error) {
	return nil, s.err
}
func (s errWorktreeStore) DeleteWorktree(context.Context, string, string) error { return s.err }

func TestDaemonFileSearchRootResolverPropagatesWorkspaceLookupError(t *testing.T) {
	t.Parallel()
	resolver := NewDaemonFileSearchRootResolver(&Stores{
		Workspaces: errWorkspaceStore{err: errors.New("workspace store down")},
	}, nil)
	_, err := resolver.ResolveRoots(context.Background(), types.FileSearchScope{WorkspaceID: "ws-1"})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDaemonFileSearchRootResolverRejectsMissingWorktreeStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	if err := ensureDir(repoDir); err != nil {
		t.Fatalf("ensureDir: %v", err)
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{RepoPath: repoDir})
	if err != nil {
		t.Fatalf("Add workspace: %v", err)
	}
	resolver := NewDaemonFileSearchRootResolver(&Stores{Workspaces: workspaceStore}, nil)
	_, err = resolver.ResolveRoots(ctx, types.FileSearchScope{WorkspaceID: ws.ID, WorktreeID: "wt-1"})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDaemonFileSearchRootResolverPropagatesWorktreeLookupError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	if err := ensureDir(repoDir); err != nil {
		t.Fatalf("ensureDir: %v", err)
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{RepoPath: repoDir})
	if err != nil {
		t.Fatalf("Add workspace: %v", err)
	}
	resolver := NewDaemonFileSearchRootResolver(&Stores{
		Workspaces: workspaceStore,
		Worktrees:  errWorktreeStore{err: errors.New("worktrees down")},
	}, nil)
	_, err = resolver.ResolveRoots(ctx, types.FileSearchScope{WorkspaceID: ws.ID, WorktreeID: "wt-1"})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorInvalid {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDaemonFileSearchRootResolverRejectsInvalidCWDPath(t *testing.T) {
	t.Parallel()
	resolver := NewPassthroughFileSearchRootResolver()
	_, err := resolver.ResolveRoots(context.Background(), types.FileSearchScope{Cwd: filepath.Join(t.TempDir(), "missing")})
	serviceErr, ok := err.(*ServiceError)
	if !ok || serviceErr.Kind != ServiceErrorInvalid {
		t.Fatalf("unexpected error: %#v", err)
	}
}
