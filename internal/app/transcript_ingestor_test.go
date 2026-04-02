package app

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
)

type ingestorStubIdentityPolicy struct {
	identity         transcriptdomain.MessageIdentity
	equivalent       bool
	canFinalize      bool
	identityOverride func(block transcriptdomain.TranscriptIdentityBlock) transcriptdomain.MessageIdentity
}

func (p ingestorStubIdentityPolicy) Identity(block transcriptdomain.TranscriptIdentityBlock) transcriptdomain.MessageIdentity {
	if p.identityOverride != nil {
		return p.identityOverride(block)
	}
	return p.identity
}

func (p ingestorStubIdentityPolicy) Equivalent(_, _ transcriptdomain.TranscriptIdentityBlock) bool {
	return p.equivalent
}

func (p ingestorStubIdentityPolicy) CanFinalizeReplace(_, _ transcriptdomain.TranscriptIdentityBlock) bool {
	return p.canFinalize
}

func TestFindTranscriptDedupeCandidateIndexUsesStableIdentityOnly(t *testing.T) {
	existing := []ChatBlock{
		{Role: ChatRoleAgent, Text: "one", ID: "id-1", TurnID: "turn-1", ProviderMessageID: "pm-1"},
		{Role: ChatRoleAgent, Text: "two", TurnID: "turn-2"},
	}

	if got := findTranscriptDedupeCandidateIndex(existing, ChatBlock{Role: ChatRoleAgent, ProviderMessageID: "pm-1"}); got != 0 {
		t.Fatalf("expected provider message id dedupe candidate at index 0, got %d", got)
	}
	if got := findTranscriptDedupeCandidateIndex(existing, ChatBlock{Role: ChatRoleAgent, TurnID: "turn-2"}); got != -1 {
		t.Fatalf("expected turn-only candidate not to dedupe without stable identity, got %d", got)
	}
	if got := findTranscriptDedupeCandidateIndex(existing, ChatBlock{Role: ChatRoleUser, TurnID: "turn-2"}); got != -1 {
		t.Fatalf("expected role mismatch to avoid dedupe candidate, got %d", got)
	}
}

func TestMergeTranscriptDedupeCandidateBranchCoverage(t *testing.T) {
	current := ChatBlock{Role: ChatRoleAgent, Text: "hello world", ID: "msg-1"}

	next, changed, deduped := mergeTranscriptDedupeCandidate(current, ChatBlock{Role: ChatRoleAgent, ID: "msg-1", Text: "hello world!!!"}, true)
	if !changed || !deduped || next.Text != "hello world!!!" {
		t.Fatalf("expected finalized superset to replace existing text, got next=%#v changed=%v deduped=%v", next, changed, deduped)
	}

	next, changed, deduped = mergeTranscriptDedupeCandidate(current, ChatBlock{Role: ChatRoleAgent, ID: "msg-1", Text: "hello"}, true)
	if changed || !deduped || next.Text != "hello world" {
		t.Fatalf("expected finalized shorter text to keep current block, got next=%#v changed=%v deduped=%v", next, changed, deduped)
	}

	next, changed, deduped = mergeTranscriptDedupeCandidate(current, ChatBlock{Role: ChatRoleAgent, ID: "msg-1", Text: "different"}, false)
	if changed || deduped || next.Text != "hello world" {
		t.Fatalf("expected divergent non-finalized text not to dedupe, got next=%#v changed=%v deduped=%v", next, changed, deduped)
	}

	createdAt := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	next, changed, deduped = mergeTranscriptDedupeCandidate(
		ChatBlock{Role: ChatRoleAgent, ID: "msg-2", Text: "same"},
		ChatBlock{Role: ChatRoleAgent, ID: "msg-2", Text: "same", TurnID: "t1", ProviderMessageID: "pm-1", CreatedAt: createdAt},
		false,
	)
	if !changed || !deduped {
		t.Fatalf("expected metadata enrichment to mark changed+deduped, got next=%#v changed=%v deduped=%v", next, changed, deduped)
	}
	if next.TurnID != "t1" || next.ProviderMessageID != "pm-1" || !next.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected metadata enrichment to propagate identifiers, got %#v", next)
	}

	next, changed, deduped = mergeTranscriptDedupeCandidate(ChatBlock{Role: ChatRoleAgent}, ChatBlock{Role: ChatRoleAgent, ID: "msg-3", Text: "filled"}, false)
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

	next, changed, dedupeHits := applyTranscriptDeltaWithFinalizationDedupe(existing, delta, map[int]struct{}{}, nil, nil)
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

	next, changed, dedupeHits := applyTranscriptDeltaWithFinalizationDedupe(existing, delta, map[int]struct{}{0: {}}, nil, nil)
	if !changed || dedupeHits != 1 {
		t.Fatalf("expected finalized delta to dedupe and replace, got changed=%v dedupeHits=%d next=%#v", changed, dedupeHits, next)
	}
	if len(next) != 1 || next[0].Text != "alpha beta" {
		t.Fatalf("expected finalized dedupe to replace existing block, got %#v", next)
	}
}

func TestApplyTranscriptDeltaWithFinalizationDedupeRejectsAmbiguousTurnFallback(t *testing.T) {
	existing := []ChatBlock{
		{Role: ChatRoleAgent, Text: "alpha", TurnID: "turn-1"},
		{Role: ChatRoleAgent, Text: "beta", TurnID: "turn-1"},
	}
	delta := []transcriptdomain.Block{
		{
			Kind: "assistant_message",
			Role: "assistant",
			Text: "alpha beta",
			Meta: map[string]any{"turn_id": "turn-1"},
		},
	}

	next, changed, dedupeHits := applyTranscriptDeltaWithFinalizationDedupe(existing, delta, map[int]struct{}{0: {}}, nil, nil)
	if !changed || dedupeHits != 0 {
		t.Fatalf("expected ambiguous finalized candidate to append without dedupe hit, got changed=%v dedupeHits=%d next=%#v", changed, dedupeHits, next)
	}
	if len(next) != 3 || next[2].Text != "alpha beta" {
		t.Fatalf("expected ambiguous finalized candidate to append, got %#v", next)
	}
}

func TestApplyTranscriptDeltaWithFinalizationDedupeUsesStableIdentityBeforeTurnFallback(t *testing.T) {
	existing := []ChatBlock{
		{Role: ChatRoleAgent, Text: "first", ID: "msg-1", TurnID: "turn-1"},
		{Role: ChatRoleAgent, Text: "second", TurnID: "turn-1"},
	}
	delta := []transcriptdomain.Block{
		{
			ID:   "msg-1",
			Kind: "assistant_message",
			Role: "assistant",
			Text: "first finalized",
			Meta: map[string]any{"turn_id": "turn-1"},
		},
	}

	next, changed, dedupeHits := applyTranscriptDeltaWithFinalizationDedupe(existing, delta, map[int]struct{}{0: {}}, nil, nil)
	if !changed || dedupeHits != 1 {
		t.Fatalf("expected stable identity finalized replace to dedupe, got changed=%v dedupeHits=%d next=%#v", changed, dedupeHits, next)
	}
	if len(next) != 2 || next[0].Text != "first finalized" {
		t.Fatalf("expected stable identity match to replace first block, got %#v", next)
	}
}

func TestApplyTranscriptDeltaWithFinalizationDedupeDoesNotTurnFallbackWhenStableIdentityMissing(t *testing.T) {
	existing := []ChatBlock{
		{Role: ChatRoleAgent, Text: "existing", ID: "msg-old", TurnID: "turn-1"},
	}
	delta := []transcriptdomain.Block{
		{
			ID:   "msg-new",
			Kind: "assistant_message",
			Role: "assistant",
			Text: "candidate finalized",
			Meta: map[string]any{"turn_id": "turn-1"},
		},
	}

	next, changed, dedupeHits := applyTranscriptDeltaWithFinalizationDedupe(existing, delta, map[int]struct{}{0: {}}, nil, nil)
	if !changed || dedupeHits != 0 {
		t.Fatalf("expected unmatched stable identity to append without finalized dedupe hit, got changed=%v dedupeHits=%d next=%#v", changed, dedupeHits, next)
	}
	if len(next) != 2 || next[0].ID != "msg-old" || next[1].ID != "msg-new" {
		t.Fatalf("expected unmatched stable identity candidate to append, got %#v", next)
	}
}

func TestTranscriptBlocksShareIdentityUsesSharedIdentityPolicy(t *testing.T) {
	if !transcriptBlocksShareIdentity(
		ChatBlock{Role: ChatRoleAgent, ProviderMessageID: "pm-1"},
		ChatBlock{Role: ChatRoleAgent, ProviderMessageID: "pm-1"},
	) {
		t.Fatalf("expected shared provider message identity match")
	}
	if transcriptBlocksShareIdentity(
		ChatBlock{Role: ChatRoleAgent, ProviderMessageID: "pm-1"},
		ChatBlock{Role: ChatRoleAgent, ProviderMessageID: "pm-2"},
	) {
		t.Fatalf("expected different provider message identities not to match")
	}
}

func TestMergeTranscriptDedupeCandidateWithPolicyRejectsFinalizedReplace(t *testing.T) {
	next, changed, deduped, reason := mergeTranscriptDedupeCandidateWithPolicy(
		ChatBlock{Role: ChatRoleAgent, ID: "msg-1", Text: "current"},
		ChatBlock{Role: ChatRoleAgent, ID: "msg-1", Text: "candidate"},
		true,
		false,
		ingestorStubIdentityPolicy{equivalent: false, canFinalize: false},
	)
	if changed || deduped {
		t.Fatalf("expected finalized replace rejection to avoid merge/dedupe, got next=%#v changed=%v deduped=%v reason=%s", next, changed, deduped, reason)
	}
	if reason != "finalized_replace_rejected_by_policy" {
		t.Fatalf("expected finalized policy rejection reason, got %q", reason)
	}
}

func TestMergeTranscriptDedupeCandidateWithPolicyFinalizedEmptyCandidate(t *testing.T) {
	next, changed, deduped, reason := mergeTranscriptDedupeCandidateWithPolicy(
		ChatBlock{Role: ChatRoleAgent, ID: "msg-1", Text: "current"},
		ChatBlock{Role: ChatRoleAgent, ID: "msg-1", Text: " \n"},
		true,
		false,
		ingestorStubIdentityPolicy{equivalent: true, canFinalize: true},
	)
	if !deduped || changed {
		t.Fatalf("expected finalized empty candidate to dedupe without content change, got next=%#v changed=%v deduped=%v reason=%s", next, changed, deduped, reason)
	}
	if reason != "finalized_candidate_empty" {
		t.Fatalf("expected finalized empty candidate reason, got %q", reason)
	}
}

func TestFindTranscriptFinalizedReplacementCandidateTurnFallbackNoCandidate(t *testing.T) {
	match := findTranscriptFinalizedReplacementCandidate(
		[]ChatBlock{{Role: ChatRoleAgent, TurnID: "turn-1", Text: "alpha"}},
		ChatBlock{Role: ChatRoleAgent, TurnID: "turn-2", Text: "beta"},
		ingestorStubIdentityPolicy{
			identityOverride: func(block transcriptdomain.TranscriptIdentityBlock) transcriptdomain.MessageIdentity {
				return transcriptdomain.MessageIdentity{
					Role:  strings.ToLower(strings.TrimSpace(block.Role)),
					Scope: transcriptdomain.MessageIdentityScopeNone,
				}
			},
			equivalent:  false,
			canFinalize: false,
		},
	)
	if match.Index != -1 || match.Ambiguous || match.Reason != "turn_fallback_no_candidate" {
		t.Fatalf("expected turn fallback no-candidate result, got %#v", match)
	}
}

func TestApplyTranscriptDeltaWithFinalizationDedupeDropsReplayDuplicateNonFinalized(t *testing.T) {
	existing := []ChatBlock{
		{Role: ChatRoleAgent, ID: "msg-1", Text: "hello"},
	}
	delta := []transcriptdomain.Block{
		{
			ID:   "msg-1",
			Kind: "assistant_delta",
			Role: "assistant",
			Text: "hello",
		},
	}
	next, changed, dedupeHits := applyTranscriptDeltaWithFinalizationDedupe(existing, delta, nil, nil, nil)
	if changed || dedupeHits != 0 {
		t.Fatalf("expected replay duplicate to be dropped without finalized dedupe hit, got changed=%v dedupeHits=%d next=%#v", changed, dedupeHits, next)
	}
	if len(next) != 1 || next[0].Text != "hello" {
		t.Fatalf("expected original block preserved, got %#v", next)
	}
}

func TestApplyTranscriptDeltaWithFinalizationDedupeLogsDecisions(t *testing.T) {
	restore := captureIngestorStdLogOutput(t)
	defer restore()

	existing := []ChatBlock{
		{Role: ChatRoleAgent, ID: "msg-1", Text: "hello"},
	}
	delta := []transcriptdomain.Block{
		{
			ID:   "msg-1",
			Kind: "assistant_delta",
			Role: "assistant",
			Text: "hello",
		},
	}
	_, _, _ = applyTranscriptDeltaWithFinalizationDedupe(existing, delta, nil, nil, nil)
	logOutput := currentCapturedIngestorStdLog()
	if !strings.Contains(logOutput, "transcript_ingestor_dedupe decision=dropped_replay_duplicate") {
		t.Fatalf("expected dropped replay duplicate log, got %q", logOutput)
	}
}

var transcriptIngestorLogBuffer bytes.Buffer

func captureIngestorStdLogOutput(t *testing.T) func() {
	t.Helper()
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetFlags(0)
	transcriptIngestorLogBuffer.Reset()
	log.SetOutput(&transcriptIngestorLogBuffer)
	return func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}
}

func currentCapturedIngestorStdLog() string {
	return transcriptIngestorLogBuffer.String()
}

func TestApplyTranscriptDeltaDoesNotDropIncrementalStreamingDeltas(t *testing.T) {
	// Simulate a codex-style streaming sequence: multiple incremental
	// deltas with the same item ID and Kind "agentmessage". Small
	// chunks like " " and "`" must NOT be dropped even though they
	// are substrings of earlier accumulated text.
	existing := []ChatBlock{}
	deltas := []transcriptdomain.Block{
		{ID: "msg-1", Kind: "agentmessage", Role: "assistant", Text: "The answer is"},
		{ID: "msg-1", Kind: "agentmessage", Role: "assistant", Text: " "},
		{ID: "msg-1", Kind: "agentmessage", Role: "assistant", Text: "`"},
		{ID: "msg-1", Kind: "agentmessage", Role: "assistant", Text: "Path"},
		{ID: "msg-1", Kind: "agentmessage", Role: "assistant", Text: "`"},
		{ID: "msg-1", Kind: "agentmessage", Role: "assistant", Text: " normalization"},
	}

	// Apply each delta one at a time (as the streaming ingestor does).
	blocks := existing
	for _, d := range deltas {
		blocks, _, _ = applyTranscriptDeltaWithFinalizationDedupe(
			blocks, []transcriptdomain.Block{d}, map[int]struct{}{}, nil, nil,
		)
	}

	if len(blocks) != 1 {
		t.Fatalf("expected one coalesced block, got %d: %#v", len(blocks), blocks)
	}
	want := "The answer is `Path` normalization"
	if blocks[0].Text != want {
		t.Fatalf("expected %q, got %q", want, blocks[0].Text)
	}
}
