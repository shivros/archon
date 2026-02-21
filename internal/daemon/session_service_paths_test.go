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

func TestResolveWorktreePathUsesWorkspaceSessionSubpath(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	sessionDir := filepath.Join(repoDir, "packages", "pennies")
	if err := ensureDir(sessionDir); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	ws, err := workspaceStore.Add(ctx, &types.Workspace{
		RepoPath:       repoDir,
		SessionSubpath: filepath.Join("packages", "pennies"),
	})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}
	service := &SessionService{
		stores: &Stores{
			Workspaces: workspaceStore,
			Worktrees:  workspaceStore,
		},
	}

	cwd, root, err := service.resolveWorktreePath(ctx, ws.ID, "")
	if err != nil {
		t.Fatalf("resolveWorktreePath: %v", err)
	}
	if cwd != sessionDir {
		t.Fatalf("expected cwd %q, got %q", sessionDir, cwd)
	}
	if root != repoDir {
		t.Fatalf("expected repo root %q, got %q", repoDir, root)
	}
}

func TestResolveWorktreePathUsesWorktreeSessionSubpath(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	if err := ensureDir(filepath.Join(repoDir, "packages", "pennies")); err != nil {
		t.Fatalf("mkdir repo session dir: %v", err)
	}

	ws, err := workspaceStore.Add(ctx, &types.Workspace{
		RepoPath:       repoDir,
		SessionSubpath: filepath.Join("packages", "pennies"),
	})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}
	worktreeDir := filepath.Join(base, "repo-wt")
	worktreeSessionDir := filepath.Join(worktreeDir, "packages", "pennies")
	if err := ensureDir(worktreeSessionDir); err != nil {
		t.Fatalf("mkdir worktree session dir: %v", err)
	}
	wt, err := workspaceStore.AddWorktree(ctx, ws.ID, &types.Worktree{Path: worktreeDir})
	if err != nil {
		t.Fatalf("add worktree: %v", err)
	}

	service := &SessionService{
		stores: &Stores{
			Workspaces: workspaceStore,
			Worktrees:  workspaceStore,
		},
	}
	cwd, root, err := service.resolveWorktreePath(ctx, ws.ID, wt.ID)
	if err != nil {
		t.Fatalf("resolveWorktreePath: %v", err)
	}
	if cwd != worktreeSessionDir {
		t.Fatalf("expected cwd %q, got %q", worktreeSessionDir, cwd)
	}
	if root != repoDir {
		t.Fatalf("expected repo root %q, got %q", repoDir, root)
	}
}

func TestResolveWorktreePathFailsWhenWorktreeSessionPathMissing(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	if err := ensureDir(filepath.Join(repoDir, "packages", "pennies")); err != nil {
		t.Fatalf("mkdir repo session dir: %v", err)
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{
		RepoPath:       repoDir,
		SessionSubpath: filepath.Join("packages", "pennies"),
	})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}
	worktreeDir := filepath.Join(base, "repo-wt")
	if err := ensureDir(worktreeDir); err != nil {
		t.Fatalf("mkdir worktree dir: %v", err)
	}
	wt, err := workspaceStore.AddWorktree(ctx, ws.ID, &types.Worktree{Path: worktreeDir})
	if err != nil {
		t.Fatalf("add worktree: %v", err)
	}
	service := &SessionService{
		stores: &Stores{
			Workspaces: workspaceStore,
			Worktrees:  workspaceStore,
		},
	}
	if _, _, err := service.resolveWorktreePath(ctx, ws.ID, wt.ID); err == nil {
		t.Fatalf("expected missing worktree session path error")
	}
}

func TestWithWorkspacePathResolverOption(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	repoDir := filepath.Join(base, "repo")
	if err := ensureDir(repoDir); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{RepoPath: repoDir})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}
	customPath := filepath.Join(base, "custom-cwd")
	resolver := &stubWorkspacePathResolver{
		workspacePath: customPath,
	}
	service := NewSessionService(nil, &Stores{
		Workspaces: workspaceStore,
		Worktrees:  workspaceStore,
	}, nil, nil, WithWorkspacePathResolver(resolver))

	got, _, err := service.resolveWorktreePath(ctx, ws.ID, "")
	if err != nil {
		t.Fatalf("resolveWorktreePath: %v", err)
	}
	if got != customPath {
		t.Fatalf("expected custom resolver path %q, got %q", customPath, got)
	}
}

type stubWorkspacePathResolver struct {
	validateErr   error
	workspacePath string
	worktreePath  string
}

func (s *stubWorkspacePathResolver) ValidateWorkspace(_, _ string) error {
	return s.validateErr
}

func (s *stubWorkspacePathResolver) ResolveWorkspaceSessionPath(_ *types.Workspace) (string, error) {
	if s.workspacePath == "" {
		return "", errors.New("workspace path missing")
	}
	return s.workspacePath, nil
}

func (s *stubWorkspacePathResolver) ResolveWorktreeSessionPath(_ *types.Workspace, _ *types.Worktree) (string, error) {
	if s.worktreePath == "" {
		return "", errors.New("worktree path missing")
	}
	return s.worktreePath, nil
}

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}
