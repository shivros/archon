//go:build linux

package app

import "testing"

func TestLinuxFileLinkOpenPolicyRejectsEmptyOpenTarget(t *testing.T) {
	policy := linuxFileLinkOpenPolicy{}
	if _, err := policy.BuildCommand(ResolvedFileLink{}); err == nil {
		t.Fatalf("expected empty open target to be rejected")
	}
}

func TestLinuxFileLinkOpenPolicyUsesURLTarget(t *testing.T) {
	policy := linuxFileLinkOpenPolicy{}
	target := "https://example.com/docs"
	command, err := policy.BuildCommand(ResolvedFileLink{Kind: FileLinkTargetKindURL, URL: target})
	if err != nil {
		t.Fatalf("unexpected policy error: %v", err)
	}
	if command.Name != "xdg-open" {
		t.Fatalf("expected linux opener xdg-open, got %#v", command)
	}
	if len(command.Args) != 1 || command.Args[0] != target {
		t.Fatalf("expected URL target arg, got %#v", command)
	}
}
