package daemon

import "testing"

func TestResolveWorkspacePatchRepoPath(t *testing.T) {
	existing := "/tmp/existing"
	if got := resolveWorkspacePatchRepoPath(existing, nil); got != existing {
		t.Fatalf("expected existing repo path, got %q", got)
	}

	incoming := " /tmp/updated "
	if got := resolveWorkspacePatchRepoPath(existing, &incoming); got != "/tmp/updated" {
		t.Fatalf("expected trimmed incoming repo path, got %q", got)
	}
}

func TestResolveWorkspacePatchName(t *testing.T) {
	existingName := "Existing Name"
	repoPath := "/tmp/repo-two"

	if got := resolveWorkspacePatchName(existingName, repoPath, nil); got != existingName {
		t.Fatalf("expected existing name when incoming is nil, got %q", got)
	}

	custom := "  Custom Name  "
	if got := resolveWorkspacePatchName(existingName, repoPath, &custom); got != "Custom Name" {
		t.Fatalf("expected trimmed custom name, got %q", got)
	}

	blank := "   "
	if got := resolveWorkspacePatchName(existingName, repoPath, &blank); got != "repo-two" {
		t.Fatalf("expected default repo basename, got %q", got)
	}
}

func TestDefaultWorkspaceNameEdgeCases(t *testing.T) {
	if got := defaultWorkspaceName("/tmp/repo"); got != "repo" {
		t.Fatalf("expected basename repo, got %q", got)
	}
	if got := defaultWorkspaceName("/"); got != "/" {
		t.Fatalf("expected root path to stay root, got %q", got)
	}
	if got := defaultWorkspaceName("."); got != "." {
		t.Fatalf("expected dot path to stay dot, got %q", got)
	}
}
