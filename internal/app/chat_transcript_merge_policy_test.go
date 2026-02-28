package app

import (
	"strings"
	"testing"
	"time"
)

func TestDefaultAssistantMergePolicyShouldMergeGuards(t *testing.T) {
	policy := defaultAssistantMergePolicy{}
	now := time.Now().UTC()

	if policy.ShouldMerge(ChatBlock{Role: ChatRoleUser}, "next", assistantAppendContext{createdAt: now}) {
		t.Fatalf("expected role guard to reject merge")
	}
	if policy.ShouldMerge(ChatBlock{Role: ChatRoleAgent}, "   ", assistantAppendContext{createdAt: now}) {
		t.Fatalf("expected blank text guard to reject merge")
	}
	if policy.ShouldMerge(
		ChatBlock{Role: ChatRoleAgent, TurnID: "turn-1", CreatedAt: now},
		"next",
		assistantAppendContext{createdAt: now, turnID: "turn-2"},
	) {
		t.Fatalf("expected turn mismatch to reject merge")
	}
}

func TestDefaultAssistantMergePolicyShouldMergeMessageIDBranches(t *testing.T) {
	policy := defaultAssistantMergePolicy{}
	now := time.Now().UTC()

	if !policy.ShouldMerge(
		ChatBlock{Role: ChatRoleAgent, ProviderMessageID: "msg-1", CreatedAt: now},
		"next",
		assistantAppendContext{createdAt: now, providerMessageID: "msg-1"},
	) {
		t.Fatalf("expected same provider message id to merge")
	}
	if policy.ShouldMerge(
		ChatBlock{Role: ChatRoleAgent, ProviderMessageID: "msg-1", CreatedAt: now},
		"next",
		assistantAppendContext{createdAt: now, providerMessageID: "msg-2"},
	) {
		t.Fatalf("expected different provider message ids to split")
	}
	if policy.ShouldMerge(
		ChatBlock{Role: ChatRoleAgent, ProviderMessageID: "msg-1", CreatedAt: now},
		"next",
		assistantAppendContext{createdAt: now},
	) {
		t.Fatalf("expected one-sided provider message id to split")
	}
}

func TestStrictAssistantContinuationTable(t *testing.T) {
	now := time.Now().UTC()
	long := strings.Repeat("x", maxAssistantContinuationTextSize+1)
	tests := []struct {
		name string
		cur  string
		next string
		curT time.Time
		nxtT time.Time
		want bool
	}{
		{name: "lowercase_fragment_merges", cur: "Done", next: " and then", curT: now, nxtT: now.Add(100 * time.Millisecond), want: true},
		{name: "punctuation_uppercase_splits", cur: "Done.", next: " Next sentence", curT: now, nxtT: now.Add(100 * time.Millisecond), want: false},
		{name: "too_long_next_splits", cur: "Done", next: long, curT: now, nxtT: now.Add(100 * time.Millisecond), want: false},
		{name: "paragraph_break_splits", cur: "Done", next: "line1\n\nline2", curT: now, nxtT: now.Add(100 * time.Millisecond), want: false},
		{name: "gap_too_large_splits", cur: "Done", next: " and then", curT: now, nxtT: now.Add(maxAssistantContinuationGap + 10*time.Millisecond), want: false},
		{name: "zero_time_splits", cur: "Done", next: " and then", curT: time.Time{}, nxtT: now, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := strictAssistantContinuation(tc.cur, tc.next, tc.curT, tc.nxtT); got != tc.want {
				t.Fatalf("strictAssistantContinuation(%q,%q)=%v want %v", tc.cur, tc.next, got, tc.want)
			}
		})
	}
}

func TestPolicyHelperBranches(t *testing.T) {
	if !hasExplicitAssistantBoundary("Done.", " Next") {
		t.Fatalf("expected explicit boundary when punctuation followed by uppercase")
	}
	if hasExplicitAssistantBoundary("Done", " next") {
		t.Fatalf("expected no explicit boundary without terminating punctuation")
	}

	if !looksLikeAssistantFragment("Done", " and more") {
		t.Fatalf("expected connector fragment to be recognized")
	}
	if !looksLikeAssistantFragment("Done", "\"quote") {
		t.Fatalf("expected quote fragment to be recognized")
	}
	if !looksLikeAssistantFragment("Done:", "Next") {
		t.Fatalf("expected delimiter carry-on to be recognized")
	}
	if looksLikeAssistantFragment("Done.", "Next") {
		t.Fatalf("expected new sentence to not look like continuation")
	}

	if _, ok := firstNonSpaceRune("   "); ok {
		t.Fatalf("expected empty/space-only text to have no rune")
	}
}
