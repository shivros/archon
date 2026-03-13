package transcriptdomain

import (
	"encoding/json"
	"testing"
)

type identityTestStringer string

func (s identityTestStringer) String() string {
	return string(s)
}

func TestDefaultTranscriptIdentityPolicyUsesProviderMessageBeforeItem(t *testing.T) {
	policy := NewDefaultTranscriptIdentityPolicy()
	identity := policy.Identity(TranscriptIdentityBlock{
		ID:                "item-1",
		ProviderMessageID: "msg-1",
		Role:              "assistant",
		Meta: map[string]any{
			"provider_message_id": "msg-meta",
		},
	})
	if identity.Scope != MessageIdentityScopeProviderMessage {
		t.Fatalf("expected provider message scope, got %q", identity.Scope)
	}
	if identity.Value != "msg-1" {
		t.Fatalf("expected provider message id to win, got %#v", identity)
	}
}

func TestDefaultTranscriptIdentityPolicyEquivalentByProviderItem(t *testing.T) {
	policy := NewDefaultTranscriptIdentityPolicy()
	left := TranscriptIdentityBlock{ID: "item-1", Role: "assistant"}
	right := TranscriptIdentityBlock{ID: "item-1", Role: "assistant"}
	if !policy.Equivalent(left, right) {
		t.Fatalf("expected provider item equivalence")
	}
}

func TestDefaultTranscriptIdentityPolicyEquivalentByTurnScopedID(t *testing.T) {
	policy := NewDefaultTranscriptIdentityPolicy()
	left := TranscriptIdentityBlock{
		Role: "assistant",
		Meta: map[string]any{"turn_id": "turn-1", "turn_scoped_id": "segment-1"},
	}
	right := TranscriptIdentityBlock{
		Role: "assistant",
		Meta: map[string]any{"turn_id": "turn-1", "turn_scoped_id": "segment-1"},
	}
	if !policy.Equivalent(left, right) {
		t.Fatalf("expected turn-scoped equivalence")
	}
}

func TestDefaultTranscriptIdentityPolicyDoesNotEquateDifferentRoles(t *testing.T) {
	policy := NewDefaultTranscriptIdentityPolicy()
	left := TranscriptIdentityBlock{ProviderMessageID: "msg-1", Role: "assistant"}
	right := TranscriptIdentityBlock{ProviderMessageID: "msg-1", Role: "user"}
	if policy.Equivalent(left, right) {
		t.Fatalf("expected role mismatch to prevent equivalence")
	}
}

func TestTextSemanticsBoundaries(t *testing.T) {
	if !IsSemanticallyEmpty(" \n\t") {
		t.Fatalf("expected whitespace-only text to be semantically empty")
	}
	if IsSemanticallyEmpty("x") {
		t.Fatalf("expected non-empty text to be semantically non-empty")
	}
	original := "line 1\nline 2 "
	if got := PreserveText(original); got != original {
		t.Fatalf("expected PreserveText to be no-op, got %q", got)
	}
}

func TestMessageIdentityHasStableIdentity(t *testing.T) {
	if (MessageIdentity{}).HasStableIdentity() {
		t.Fatalf("expected empty identity to be unstable")
	}
	if !(MessageIdentity{Scope: MessageIdentityScopeProviderMessage, Value: "msg-1"}).HasStableIdentity() {
		t.Fatalf("expected provider-message identity to be stable")
	}
}

func TestDefaultTranscriptIdentityPolicyCanFinalizeReplaceDelegatesEquivalent(t *testing.T) {
	policy := NewDefaultTranscriptIdentityPolicy()
	current := TranscriptIdentityBlock{ProviderMessageID: "msg-1", Role: "assistant"}
	candidate := TranscriptIdentityBlock{ProviderMessageID: "msg-1", Role: "assistant"}
	if !policy.CanFinalizeReplace(current, candidate) {
		t.Fatalf("expected equivalent identity to allow finalized replacement")
	}

	candidate = TranscriptIdentityBlock{ProviderMessageID: "msg-2", Role: "assistant"}
	if policy.CanFinalizeReplace(current, candidate) {
		t.Fatalf("expected non-equivalent identity to reject finalized replacement")
	}
}

func TestIdentityAnyStringCoversStringerAndUnsupported(t *testing.T) {
	if got := identityAnyString(identityTestStringer("value")); got != "value" {
		t.Fatalf("expected fmt.Stringer-compatible value, got %q", got)
	}
	if got := identityAnyString(json.Number("42")); got != "42" {
		t.Fatalf("expected json-number-like value to resolve string, got %q", got)
	}
	if got := identityAnyString(struct{}{}); got != "" {
		t.Fatalf("expected unsupported type to resolve empty string, got %q", got)
	}
}

func TestIdentityRolesCompatibleAllowsEmptyRole(t *testing.T) {
	if !identityRolesCompatible("", "assistant") {
		t.Fatalf("expected empty-role compatibility to be allowed")
	}
}
