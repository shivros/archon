package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"control/internal/types"
)

func TestResolverResolveWorkspaceSessionPath(t *testing.T) {
	resolver := NewWorkspacePathResolver()
	repoDir := t.TempDir()
	sessionPath := filepath.Join(repoDir, "packages", "pennies")
	if err := os.MkdirAll(sessionPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ws := &types.Workspace{
		RepoPath:       repoDir,
		SessionSubpath: filepath.Join("packages", "pennies"),
	}
	got, err := resolver.ResolveWorkspaceSessionPath(ws)
	if err != nil {
		t.Fatalf("ResolveWorkspaceSessionPath: %v", err)
	}
	want := filepath.Join(repoDir, "packages", "pennies")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolverResolveWorktreeSessionPath(t *testing.T) {
	resolver := NewWorkspacePathResolver()
	root := t.TempDir()
	ws := &types.Workspace{
		RepoPath:       root,
		SessionSubpath: filepath.Join("packages", "pennies"),
	}
	wt := &types.Worktree{Path: filepath.Join(root, "wt-a")}
	if err := os.MkdirAll(filepath.Join(wt.Path, "packages", "pennies"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := resolver.ResolveWorktreeSessionPath(ws, wt)
	if err != nil {
		t.Fatalf("ResolveWorktreeSessionPath: %v", err)
	}
	want := filepath.Join(root, "wt-a", "packages", "pennies")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolverValidateWorkspace(t *testing.T) {
	resolver := NewWorkspacePathResolver()
	root := t.TempDir()
	pkgPath := filepath.Join(root, "packages", "pennies")
	if err := os.MkdirAll(pkgPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := resolver.ValidateWorkspace(root, filepath.Join("packages", "pennies")); err != nil {
		t.Fatalf("ValidateWorkspace should pass: %v", err)
	}
	err := resolver.ValidateWorkspace(root, filepath.Join("packages", "missing"))
	if err == nil {
		t.Fatalf("expected missing subpath validation error")
	}
}
