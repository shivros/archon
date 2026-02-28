package app

import "testing"

func TestDefaultProviderDisplayPolicyKnownProvider(t *testing.T) {
	policy := DefaultProviderDisplayPolicy()
	if got := policy.DisplayName("claude"); got != "Claude" {
		t.Fatalf("expected Claude display name, got %q", got)
	}
}

func TestDefaultProviderDisplayPolicyUnknownProviderHumanized(t *testing.T) {
	policy := DefaultProviderDisplayPolicy()
	got := policy.DisplayName("anthropic/claude-sonnet")
	if got != "Anthropic Claude Sonnet" {
		t.Fatalf("expected humanized provider label, got %q", got)
	}
}

func TestDefaultProviderDisplayPolicyEmptyProvider(t *testing.T) {
	policy := DefaultProviderDisplayPolicy()
	if got := policy.DisplayName("   "); got != "Provider" {
		t.Fatalf("expected Provider fallback label, got %q", got)
	}
}
