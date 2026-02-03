package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

func TestWorkspaceStoreCRUD(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "workspaces.json")
	store := NewFileWorkspaceStore(path)

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	ws, err := store.Add(ctx, &types.Workspace{
		RepoPath: repoDir,
		Provider: "codex",
	})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}
	if ws.ID == "" {
		t.Fatalf("expected id")
	}
	if ws.Name != filepath.Base(repoDir) {
		t.Fatalf("expected name %q, got %q", filepath.Base(repoDir), ws.Name)
	}
	if ws.RepoPath != repoDir {
		t.Fatalf("expected repo path %q, got %q", repoDir, ws.RepoPath)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(list))
	}

	ws.Name = "Custom"
	updated, err := store.Update(ctx, ws)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Custom" {
		t.Fatalf("expected updated name")
	}

	if err := store.Delete(ctx, ws.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, err = store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list")
	}
}

func TestWorkspaceStoreNormalizesPath(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "workspaces.json")
	store := NewFileWorkspaceStore(path)

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	rel, err := filepath.Rel(tmp, repoDir)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	ws, err := store.Add(ctx, &types.Workspace{
		RepoPath: rel,
		Provider: "codex",
	})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}
	if ws.RepoPath != repoDir {
		t.Fatalf("expected abs path %q, got %q", repoDir, ws.RepoPath)
	}
}

func TestWorktreeStoreCRUD(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "workspaces.json")
	store := NewFileWorkspaceStore(path)

	repoDir := filepath.Join(tmp, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	ws, err := store.Add(ctx, &types.Workspace{
		RepoPath: repoDir,
		Provider: "codex",
	})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}

	wtDir := filepath.Join(repoDir, "worktree")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}

	wt, err := store.AddWorktree(ctx, ws.ID, &types.Worktree{Path: wtDir})
	if err != nil {
		t.Fatalf("add worktree: %v", err)
	}
	if wt.Name != filepath.Base(wtDir) {
		t.Fatalf("expected default name")
	}

	list, err := store.ListWorktrees(ctx, ws.ID)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(list))
	}

	if err := store.DeleteWorktree(ctx, ws.ID, wt.ID); err != nil {
		t.Fatalf("delete worktree: %v", err)
	}
	list, err = store.ListWorktrees(ctx, ws.ID)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list")
	}
}

func TestWorktreeRequiresWorkspace(t *testing.T) {
	ctx := context.Background()
	store := NewFileWorkspaceStore(filepath.Join(t.TempDir(), "workspaces.json"))

	_, err := store.AddWorktree(ctx, "missing", &types.Worktree{Path: "/tmp"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestWorkspaceUpdatedAt(t *testing.T) {
	ctx := context.Background()
	store := NewFileWorkspaceStore(filepath.Join(t.TempDir(), "workspaces.json"))

	repoDir := t.TempDir()
	ws, err := store.Add(ctx, &types.Workspace{RepoPath: repoDir, Provider: "codex"})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	originalUpdated := ws.UpdatedAt
	time.Sleep(10 * time.Millisecond)

	ws.Name = "Renamed"
	updated, err := store.Update(ctx, ws)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !updated.UpdatedAt.After(originalUpdated) {
		t.Fatalf("expected updated_at to advance")
	}
}

func TestDefaultNameFallback(t *testing.T) {
	name := defaultName(string(filepath.Separator))
	if strings.TrimSpace(name) == "" {
		t.Fatalf("expected fallback name")
	}
}
