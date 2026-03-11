package app

import (
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
)

func TestFindTranscriptDedupeCandidateIndexUsesIdentityAndTurnFallback(t *testing.T) {
	existing := []ChatBlock{
		{Role: ChatRoleAgent, Text: "one", ID: "id-1", TurnID: "turn-1", ProviderMessageID: "pm-1"},
		{Role: ChatRoleAgent, Text: "two", TurnID: "turn-2"},
	}

	if got := findTranscriptDedupeCandidateIndex(existing, ChatBlock{Role: ChatRoleAgent, ProviderMessageID: "pm-1"}); got != 0 {
		t.Fatalf("expected provider message id dedupe candidate at index 0, got %d", got)
	}
	if got := findTranscriptDedupeCandidateIndex(existing, ChatBlock{Role: ChatRoleAgent, TurnID: "turn-2"}); got != 1 {
		t.Fatalf("expected turn-id fallback dedupe candidate at index 1, got %d", got)
	}
	if got := findTranscriptDedupeCandidateIndex(existing, ChatBlock{Role: ChatRoleUser, TurnID: "turn-2"}); got != -1 {
		t.Fatalf("expected role mismatch to avoid dedupe candidate, got %d", got)
	}
}

func TestMergeTranscriptDedupeCandidateBranchCoverage(t *testing.T) {
	current := ChatBlock{Role: ChatRoleAgent, Text: "hello world"}

	next, changed, deduped := mergeTranscriptDedupeCandidate(current, ChatBlock{Role: ChatRoleAgent, Text: "hello world!!!"}, true)
	if !changed || !deduped || next.Text != "hello world!!!" {
		t.Fatalf("expected finalized superset to replace existing text, got next=%#v changed=%v deduped=%v", next, changed, deduped)
	}

	next, changed, deduped = mergeTranscriptDedupeCandidate(current, ChatBlock{Role: ChatRoleAgent, Text: "hello"}, true)
	if changed || !deduped || next.Text != "hello world" {
		t.Fatalf("expected finalized shorter text to keep current block, got next=%#v changed=%v deduped=%v", next, changed, deduped)
	}

	next, changed, deduped = mergeTranscriptDedupeCandidate(current, ChatBlock{Role: ChatRoleAgent, Text: "different"}, false)
	if changed || deduped || next.Text != "hello world" {
		t.Fatalf("expected divergent non-finalized text not to dedupe, got next=%#v changed=%v deduped=%v", next, changed, deduped)
	}

	createdAt := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	next, changed, deduped = mergeTranscriptDedupeCandidate(
		ChatBlock{Role: ChatRoleAgent, Text: "same"},
		ChatBlock{Role: ChatRoleAgent, Text: "same", TurnID: "t1", ProviderMessageID: "pm-1", CreatedAt: createdAt},
		false,
	)
	if !changed || !deduped {
		t.Fatalf("expected metadata enrichment to mark changed+deduped, got next=%#v changed=%v deduped=%v", next, changed, deduped)
	}
	if next.TurnID != "t1" || next.ProviderMessageID != "pm-1" || !next.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected metadata enrichment to propagate identifiers, got %#v", next)
	}

	next, changed, deduped = mergeTranscriptDedupeCandidate(ChatBlock{Role: ChatRoleAgent}, ChatBlock{Role: ChatRoleAgent, Text: "filled"}, false)
	if !changed || !deduped || next.Text != "filled" {
		t.Fatalf("expected empty-current text to accept candidate text, got next=%#v changed=%v deduped=%v", next, changed, deduped)
	}
}

func TestApplyTranscriptDeltaWithFinalizationDedupeAppendsWhenNotFinalized(t *testing.T) {
	existing := []ChatBlock{
		{Role: ChatRoleAgent, Text: "alpha", TurnID: "turn-1"},
	}
	delta := []transcriptdomain.Block{
		{
			Kind: "assistant_delta",
			Role: "assistant",
			Text: "omega",
			Meta: map[string]any{"turn_id": "turn-1"},
		},
	}

	next, changed, dedupeHits := applyTranscriptDeltaWithFinalizationDedupe(existing, delta, map[int]struct{}{})
	if !changed || dedupeHits != 0 {
		t.Fatalf("expected non-finalized divergent delta to append without dedupe hit, got changed=%v dedupeHits=%d next=%#v", changed, dedupeHits, next)
	}
	if len(next) != 2 || next[1].Text != "omega" {
		t.Fatalf("expected appended divergent block, got %#v", next)
	}
}

func TestApplyTranscriptDeltaWithFinalizationDedupeReplacesWhenFinalized(t *testing.T) {
	existing := []ChatBlock{
		{Role: ChatRoleAgent, Text: "alpha", TurnID: "turn-1"},
	}
	delta := []transcriptdomain.Block{
		{
			Kind: "assistant_message",
			Role: "assistant",
			Text: "alpha beta",
			Meta: map[string]any{"turn_id": "turn-1"},
		},
	}

	next, changed, dedupeHits := applyTranscriptDeltaWithFinalizationDedupe(existing, delta, map[int]struct{}{0: {}})
	if !changed || dedupeHits != 1 {
		t.Fatalf("expected finalized delta to dedupe and replace, got changed=%v dedupeHits=%d next=%#v", changed, dedupeHits, next)
	}
	if len(next) != 1 || next[0].Text != "alpha beta" {
		t.Fatalf("expected finalized dedupe to replace existing block, got %#v", next)
	}
}
