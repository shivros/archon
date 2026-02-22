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
		RepoPath:              repoDir,
		SessionSubpath:        "packages/pennies/",
		AdditionalDirectories: []string{" ../backend ", "../shared", "../backend"},
		GroupIDs:              []string{"group-1", " group-1 ", "ungrouped", ""},
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
	if ws.SessionSubpath != filepath.Join("packages", "pennies") {
		t.Fatalf("expected normalized session subpath, got %q", ws.SessionSubpath)
	}
	if len(ws.AdditionalDirectories) != 2 {
		t.Fatalf("expected normalized additional directories, got %#v", ws.AdditionalDirectories)
	}
	if ws.AdditionalDirectories[0] != filepath.Clean("../backend") {
		t.Fatalf("unexpected first additional directory: %q", ws.AdditionalDirectories[0])
	}
	if ws.AdditionalDirectories[1] != filepath.Clean("../shared") {
		t.Fatalf("unexpected second additional directory: %q", ws.AdditionalDirectories[1])
	}
	if len(ws.GroupIDs) != 1 || ws.GroupIDs[0] != "group-1" {
		t.Fatalf("expected normalized group ids")
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
	})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}

	wtDir := filepath.Join(repoDir, "worktree")
	if err := os.MkdirAll(wtDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}

	enabled := false
	wt, err := store.AddWorktree(ctx, ws.ID, &types.Worktree{
		Path: wtDir,
		NotificationOverrides: &types.NotificationSettingsPatch{
			Enabled: &enabled,
			Methods: []types.NotificationMethod{types.NotificationMethodBell},
		},
	})
	if err != nil {
		t.Fatalf("add worktree: %v", err)
	}
	if wt.Name != filepath.Base(wtDir) {
		t.Fatalf("expected default name")
	}
	if wt.NotificationOverrides == nil || wt.NotificationOverrides.Enabled == nil || *wt.NotificationOverrides.Enabled {
		t.Fatalf("expected worktree notification override to persist")
	}

	list, err := store.ListWorktrees(ctx, ws.ID)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(list))
	}
	time.Sleep(10 * time.Millisecond)
	wt.Name = "Renamed Worktree"
	updated, err := store.UpdateWorktree(ctx, ws.ID, wt)
	if err != nil {
		t.Fatalf("update worktree: %v", err)
	}
	if updated.Name != "Renamed Worktree" {
		t.Fatalf("expected updated worktree name")
	}
	if updated.NotificationOverrides == nil || len(updated.NotificationOverrides.Methods) != 1 || updated.NotificationOverrides.Methods[0] != types.NotificationMethodBell {
		t.Fatalf("expected worktree notification override to survive update")
	}
	if !updated.UpdatedAt.After(updated.CreatedAt) {
		t.Fatalf("expected worktree updated_at to advance")
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

func TestWorkspaceGroupStoreCRUD(t *testing.T) {
	ctx := context.Background()
	store := NewFileWorkspaceStore(filepath.Join(t.TempDir(), "workspaces.json"))

	group, err := store.AddGroup(ctx, &types.WorkspaceGroup{Name: "Work"})
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	if group.ID == "" {
		t.Fatalf("expected group id")
	}

	list, err := store.ListGroups(ctx)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 group, got %d", len(list))
	}

	group.Name = "Personal"
	updated, err := store.UpdateGroup(ctx, group)
	if err != nil {
		t.Fatalf("update group: %v", err)
	}
	if updated.Name != "Personal" {
		t.Fatalf("expected updated name")
	}

	if err := store.DeleteGroup(ctx, group.ID); err != nil {
		t.Fatalf("delete group: %v", err)
	}
	list, err = store.ListGroups(ctx)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list")
	}
}

func TestWorkspaceUpdatedAt(t *testing.T) {
	ctx := context.Background()
	store := NewFileWorkspaceStore(filepath.Join(t.TempDir(), "workspaces.json"))

	repoDir := t.TempDir()
	ws, err := store.Add(ctx, &types.Workspace{RepoPath: repoDir})
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

func TestWorkspaceStoreRejectsInvalidSessionSubpath(t *testing.T) {
	ctx := context.Background()
	store := NewFileWorkspaceStore(filepath.Join(t.TempDir(), "workspaces.json"))

	repoDir := t.TempDir()
	tests := []string{
		filepath.Join(string(filepath.Separator), "tmp", "abs"),
		"..",
		filepath.Join("..", "outside"),
	}
	for _, subpath := range tests {
		_, err := store.Add(ctx, &types.Workspace{
			RepoPath:       repoDir,
			SessionSubpath: subpath,
		})
		if err == nil {
			t.Fatalf("expected invalid session subpath error for %q", subpath)
		}
	}
}

func TestWorkspaceStoreRejectsInvalidAdditionalDirectories(t *testing.T) {
	ctx := context.Background()
	store := NewFileWorkspaceStore(filepath.Join(t.TempDir(), "workspaces.json"))

	repoDir := t.TempDir()
	_, err := store.Add(ctx, &types.Workspace{
		RepoPath:              repoDir,
		AdditionalDirectories: []string{"../backend", " "},
	})
	if err == nil {
		t.Fatalf("expected invalid additional directory error")
	}
}
