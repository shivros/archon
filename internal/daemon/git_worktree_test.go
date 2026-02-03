package daemon

import "testing"

func TestParseGitWorktreeList(t *testing.T) {
	output := `
worktree /repo
HEAD 1234567
branch refs/heads/main

worktree /repo/wt1
HEAD 89abcd0
branch refs/heads/feature/foo

worktree /repo/wt2
HEAD deadbeef
detached
`
	entries := parseGitWorktreeList(output)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Path != "/repo" || entries[0].Branch != "main" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Branch != "feature/foo" {
		t.Fatalf("unexpected second branch: %+v", entries[1])
	}
	if !entries[2].Detached {
		t.Fatalf("expected detached entry")
	}
}
