package transcriptdomain

import (
	"testing"
	"time"
)

func TestProjectorTranscriptDedupePolicyDropsStableIdentityExactReplay(t *testing.T) {
	policy := NewProjectorTranscriptDedupePolicy(nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
		Meta: map[string]any{"provider_message_id": "pm-1"},
	}}
	decision := policy.ReplayDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
		Meta: map[string]any{"provider_message_id": "pm-1"},
	})
	if decision.Action != TranscriptDedupeActionDropDuplicate || !decision.Deduped {
		t.Fatalf("expected replay duplicate to be dropped, got %#v", decision)
	}
	if decision.Reason != "stable_identity_text_match" {
		t.Fatalf("expected stable identity text-match reason, got %#v", decision)
	}
}

func TestProjectorTranscriptDedupePolicyKeepsStableIdentityTextMismatch(t *testing.T) {
	policy := NewProjectorTranscriptDedupePolicy(nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
		Meta: map[string]any{"provider_message_id": "pm-1"},
	}}
	decision := policy.ReplayDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello world",
		Meta: map[string]any{"provider_message_id": "pm-1"},
	})
	if decision.Action != TranscriptDedupeActionAppend {
		t.Fatalf("expected mismatch replay to append, got %#v", decision)
	}
	if decision.Reason != "stable_identity_text_mismatch" {
		t.Fatalf("expected stable identity text-mismatch reason, got %#v", decision)
	}
}

func TestProjectorTranscriptDedupePolicyFinalizedDecisionMirrorsReplay(t *testing.T) {
	policy := NewProjectorTranscriptDedupePolicy(nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
		Meta: map[string]any{"provider_message_id": "pm-1"},
	}}
	replay := policy.ReplayDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
		Meta: map[string]any{"provider_message_id": "pm-1"},
	})
	finalized := policy.FinalizedDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
		Meta: map[string]any{"provider_message_id": "pm-1"},
	})
	if replay.Action != finalized.Action || replay.Reason != finalized.Reason {
		t.Fatalf("expected finalized decision to mirror replay decision, replay=%#v finalized=%#v", replay, finalized)
	}
}

func TestProjectorTranscriptDedupePolicyRejectsUnsupportedRole(t *testing.T) {
	policy := NewProjectorTranscriptDedupePolicy(nil)
	decision := policy.ReplayDecision(nil, TranscriptIdentityBlock{
		Role: "system",
		Text: "ignored",
	})
	if decision.Action != TranscriptDedupeActionAppend || decision.Reason != "unsupported_role" {
		t.Fatalf("expected unsupported role decision, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyReplayDropsExactIdentityMatch(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
	}}
	decision := policy.ReplayDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
	})
	if decision.Action != TranscriptDedupeActionDropDuplicate || !decision.Deduped {
		t.Fatalf("expected exact replay duplicate to drop, got %#v", decision)
	}
	if decision.Reason != "text_exact_match" {
		t.Fatalf("expected exact-match replay reason, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyReplayReplacesSupersetText(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
	}}
	decision := policy.ReplayDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello world",
	})
	if decision.Action != TranscriptDedupeActionReplaceExisting || !decision.Deduped {
		t.Fatalf("expected replay superset to replace existing, got %#v", decision)
	}
	if decision.Merged.Text != "hello world" {
		t.Fatalf("expected merged text to preserve candidate superset, got %#v", decision.Merged)
	}
}

func TestIngestorTranscriptDedupePolicyReplayAppendsWithoutStableIdentity(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	decision := policy.ReplayDecision(nil, TranscriptIdentityBlock{
		Role: "assistant",
		Text: "hello",
	})
	if decision.Action != TranscriptDedupeActionAppend || decision.Reason != "no_duplicate_identity_match" {
		t.Fatalf("expected no-stable-identity replay to append, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyReplayAppendsEmptyCandidateText(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
	}}
	decision := policy.ReplayDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "  \n",
	})
	if decision.Action != TranscriptDedupeActionAppend || decision.Reason != "candidate_text_empty" {
		t.Fatalf("expected empty-candidate replay to append, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyReplayDropsWhenCurrentTextSuperset(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello world",
	}}
	decision := policy.ReplayDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
	})
	if decision.Action != TranscriptDedupeActionDropDuplicate || !decision.Deduped {
		t.Fatalf("expected current-superset replay to drop, got %#v", decision)
	}
	if decision.Reason != "current_text_superset" {
		t.Fatalf("expected current-superset reason, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyReplayAppendsDivergentText(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "alpha",
	}}
	decision := policy.ReplayDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "omega",
	})
	if decision.Action != TranscriptDedupeActionAppend || decision.Reason != "identity_match_text_diverged" {
		t.Fatalf("expected divergent replay to append, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyFinalizedStableIdentityNoTurnFallback(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{
		ID:     "msg-old",
		Role:   "assistant",
		Text:   "old",
		TurnID: "turn-1",
	}}
	decision := policy.FinalizedDecision(existing, TranscriptIdentityBlock{
		ID:     "msg-new",
		Role:   "assistant",
		Text:   "new",
		TurnID: "turn-1",
	})
	if decision.Action != TranscriptDedupeActionAppend {
		t.Fatalf("expected stable identity miss to append, got %#v", decision)
	}
	if decision.Reason != "stable_identity_not_found" {
		t.Fatalf("expected stable identity miss reason, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyRejectsAmbiguousTurnFallback(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{
		{Role: "assistant", Text: "one", TurnID: "turn-1"},
		{Role: "assistant", Text: "two", TurnID: "turn-1"},
	}
	decision := policy.FinalizedDecision(existing, TranscriptIdentityBlock{
		Role:   "assistant",
		Text:   "final",
		TurnID: "turn-1",
	})
	if decision.Action != TranscriptDedupeActionRejectAmbiguous || !decision.Ambiguous {
		t.Fatalf("expected ambiguous turn fallback rejection, got %#v", decision)
	}
	if decision.Reason != "turn_fallback_ambiguous" {
		t.Fatalf("expected turn-fallback ambiguous reason, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyFinalizedNoExistingBlocks(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	decision := policy.FinalizedDecision(nil, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
	})
	if decision.Action != TranscriptDedupeActionAppend || decision.Reason != "no_existing_blocks" {
		t.Fatalf("expected no-existing finalized decision to append, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyFinalizedMissingTurnFallbackIdentity(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{Role: "assistant", Text: "hello"}}
	decision := policy.FinalizedDecision(existing, TranscriptIdentityBlock{
		Role: "assistant",
		Text: "candidate",
	})
	if decision.Action != TranscriptDedupeActionAppend || decision.Reason != "missing_turn_fallback_identity" {
		t.Fatalf("expected missing-turn fallback finalized decision, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyFinalizedDropsExactStableIdentityMatch(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
	}}
	decision := policy.FinalizedDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
	})
	if decision.Action != TranscriptDedupeActionDropDuplicate || !decision.Deduped {
		t.Fatalf("expected exact finalized stable identity to drop duplicate, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyFinalizedReplacesStableIdentityMatch(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello",
	}}
	decision := policy.FinalizedDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "hello world",
	})
	if decision.Action != TranscriptDedupeActionReplaceExisting || !decision.Deduped {
		t.Fatalf("expected finalized stable identity superset to replace, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyFinalizedRejectsAmbiguousStableIdentity(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{
		{ID: "msg-1", Role: "assistant", Text: "one"},
		{ID: "msg-1", Role: "assistant", Text: "two"},
	}
	decision := policy.FinalizedDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Role: "assistant",
		Text: "final",
	})
	if decision.Action != TranscriptDedupeActionRejectAmbiguous || decision.Reason != "stable_identity_ambiguous" {
		t.Fatalf("expected stable identity ambiguity rejection, got %#v", decision)
	}
}

func TestIngestorTranscriptDedupePolicyFinalizedTurnFallbackSingleCandidate(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{Role: "assistant", Text: "hello", TurnID: "turn-1"}}
	decision := policy.FinalizedDecision(existing, TranscriptIdentityBlock{
		Role:   "assistant",
		Text:   "hello world",
		TurnID: "turn-1",
	})
	if decision.Action != TranscriptDedupeActionReplaceExisting || !decision.Deduped {
		t.Fatalf("expected finalized turn fallback single candidate to replace, got %#v", decision)
	}
}

func TestDefaultTranscriptBlockMergePolicyPreservesMetadataAndCreatedAt(t *testing.T) {
	merge := NewDefaultTranscriptBlockMergePolicy(nil)
	now := time.Date(2026, 3, 12, 20, 0, 0, 0, time.UTC)
	next, changed, deduped, reason := merge.Merge(
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "hello"},
		TranscriptIdentityBlock{
			ID:                "msg-1",
			Role:              "assistant",
			Text:              "hello world",
			TurnID:            "turn-1",
			ProviderMessageID: "pm-1",
			CreatedAt:         now,
		},
		false,
		false,
	)
	if !changed || !deduped {
		t.Fatalf("expected merge to update metadata+text and dedupe, got next=%#v changed=%v deduped=%v reason=%s", next, changed, deduped, reason)
	}
	if next.TurnID != "turn-1" || next.ProviderMessageID != "pm-1" || !next.CreatedAt.Equal(now) {
		t.Fatalf("expected merged metadata on next block, got %#v", next)
	}
}

func TestDefaultTranscriptBlockMergePolicyCopiesCandidateMetaMap(t *testing.T) {
	merge := NewDefaultTranscriptBlockMergePolicy(nil)
	meta := map[string]any{"turn_id": "turn-1", "provider_message_id": "pm-1"}
	next, changed, deduped, reason := merge.Merge(
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "hello"},
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "hello", Meta: meta},
		false,
		false,
	)
	if !changed || !deduped {
		t.Fatalf("expected metadata clone path to dedupe with change, got next=%#v changed=%v deduped=%v reason=%s", next, changed, deduped, reason)
	}
	meta["turn_id"] = "mutated"
	if next.Meta["turn_id"] != "turn-1" {
		t.Fatalf("expected cloned metadata map to be immutable from caller mutations, got %#v", next.Meta)
	}
}

func TestDefaultTranscriptBlockMergePolicyFinalizedRejectsMismatchedIdentity(t *testing.T) {
	merge := NewDefaultTranscriptBlockMergePolicy(nil)
	_, changed, deduped, reason := merge.Merge(
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "hello"},
		TranscriptIdentityBlock{ID: "msg-2", Role: "assistant", Text: "hello world"},
		true,
		false,
	)
	if changed || deduped || reason != "finalized_replace_rejected_by_policy" {
		t.Fatalf("expected finalized mismatched identity rejection, got changed=%v deduped=%v reason=%s", changed, deduped, reason)
	}
}

func TestDefaultTranscriptBlockMergePolicyFinalizedEmptyCandidateDedupe(t *testing.T) {
	merge := NewDefaultTranscriptBlockMergePolicy(nil)
	_, changed, deduped, reason := merge.Merge(
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "hello"},
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: " \n"},
		true,
		false,
	)
	if changed || !deduped || reason != "finalized_candidate_empty" {
		t.Fatalf("expected finalized empty-candidate dedupe, got changed=%v deduped=%v reason=%s", changed, deduped, reason)
	}
}

func TestDefaultTranscriptBlockMergePolicyFinalizedReplacesLongerDivergentText(t *testing.T) {
	merge := NewDefaultTranscriptBlockMergePolicy(nil)
	next, changed, deduped, reason := merge.Merge(
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "alpha"},
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "omega longer"},
		true,
		false,
	)
	if !changed || !deduped || reason != "finalized_replace_longer_candidate" || next.Text != "omega longer" {
		t.Fatalf("expected finalized longer divergent replacement, got next=%#v changed=%v deduped=%v reason=%s", next, changed, deduped, reason)
	}
}

func TestDefaultTranscriptBlockMergePolicyFinalizedKeepsCurrentShorterCandidate(t *testing.T) {
	merge := NewDefaultTranscriptBlockMergePolicy(nil)
	next, changed, deduped, reason := merge.Merge(
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "longer current"},
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "short"},
		true,
		false,
	)
	if changed || !deduped || reason != "finalized_keep_current_shorter_candidate" || next.Text != "longer current" {
		t.Fatalf("expected finalized shorter candidate to keep current, got next=%#v changed=%v deduped=%v reason=%s", next, changed, deduped, reason)
	}
}

func TestDefaultTranscriptBlockMergePolicySkipsContainmentForIncrementalDelta(t *testing.T) {
	merge := NewDefaultTranscriptBlockMergePolicy(nil)
	// "the" is a substring of the accumulated text, but since the
	// candidate is an incremental streaming delta it must not be dropped.
	_, changed, deduped, reason := merge.Merge(
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "see the code in the file"},
		TranscriptIdentityBlock{ID: "msg-1", Kind: "agentmessage", Role: "assistant", Text: "the"},
		false,
		false,
	)
	if deduped || reason != "incremental_delta_diverged" {
		t.Fatalf("expected incremental delta to bypass containment, got changed=%v deduped=%v reason=%s", changed, deduped, reason)
	}
}

func TestDefaultTranscriptBlockMergePolicySkipsContainmentForShortSubstringDelta(t *testing.T) {
	merge := NewDefaultTranscriptBlockMergePolicy(nil)
	// "Path" is a substring of the accumulated text, but it's an
	// incremental delta and must not be dropped.
	_, _, deduped, reason := merge.Merge(
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "use Path normalization"},
		TranscriptIdentityBlock{ID: "msg-1", Kind: "agentmessage", Role: "assistant", Text: "Path"},
		false,
		false,
	)
	if deduped || reason != "incremental_delta_diverged" {
		t.Fatalf("expected incremental delta to bypass containment, got deduped=%v reason=%s", deduped, reason)
	}
}

func TestDefaultTranscriptBlockMergePolicyStillDropsCumulativeReplaySubset(t *testing.T) {
	merge := NewDefaultTranscriptBlockMergePolicy(nil)
	// With empty Kind (no incremental delta signal), the cumulative
	// replay path should still detect current-text-superset.
	_, _, deduped, reason := merge.Merge(
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "hello world"},
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "hello"},
		false,
		false,
	)
	if !deduped || reason != "current_text_superset" {
		t.Fatalf("expected cumulative replay subset to be dropped, got deduped=%v reason=%s", deduped, reason)
	}
}

func TestDefaultTranscriptBlockMergePolicyExactMatchStillWorksForDeltaKind(t *testing.T) {
	merge := NewDefaultTranscriptBlockMergePolicy(nil)
	// Exact text match fires before the incremental guard.
	_, _, deduped, reason := merge.Merge(
		TranscriptIdentityBlock{ID: "msg-1", Role: "assistant", Text: "hello"},
		TranscriptIdentityBlock{ID: "msg-1", Kind: "agentmessage", Role: "assistant", Text: "hello"},
		false,
		false,
	)
	if !deduped || reason != "text_exact_match" {
		t.Fatalf("expected exact match to still fire for delta kind, got deduped=%v reason=%s", deduped, reason)
	}
}

func TestIngestorDedupePolicyDoesNotDropIncrementalStreamingDelta(t *testing.T) {
	policy := NewIngestorTranscriptDedupePolicy(nil, nil)
	existing := []TranscriptIdentityBlock{{
		ID:   "msg-1",
		Role: "assistant",
		Text: "The answer is in the code",
	}}
	// "the" is a substring of accumulated text but must not be dropped.
	decision := policy.ReplayDecision(existing, TranscriptIdentityBlock{
		ID:   "msg-1",
		Kind: "agentmessage",
		Role: "assistant",
		Text: "the",
	})
	if decision.Action != TranscriptDedupeActionAppend {
		t.Fatalf("expected incremental streaming delta to append, got %#v", decision)
	}
	if decision.Reason != "incremental_delta_diverged" {
		t.Fatalf("expected incremental_delta_diverged reason, got %q", decision.Reason)
	}
}

func TestIsIncrementalDeltaKind(t *testing.T) {
	tests := []struct {
		kind string
		want bool
	}{
		{"", false},
		{"  ", false},
		{"agentmessage", true},
		{"AgentMessage", true},
		{"assistant_delta", true},
		{"agentMessage_delta", true},
		{"assistant_message", false},
		{"agent_message", false},
		{"message", false},
		{"completed", false},
	}
	for _, tt := range tests {
		if got := IsIncrementalDeltaKind(tt.kind); got != tt.want {
			t.Errorf("IsIncrementalDeltaKind(%q) = %v, want %v", tt.kind, got, tt.want)
		}
	}
}
