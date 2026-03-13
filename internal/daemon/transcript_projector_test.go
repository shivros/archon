package daemon

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"control/internal/daemon/transcriptdomain"
)

func TestTranscriptProjectorReplayDedupeUsesStableIdentityAndTextVerification(t *testing.T) {
	projector := NewTranscriptProjector("s1", "codex", "")
	first := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
		Delta: []transcriptdomain.Block{{
			ID:   "msg-1",
			Kind: "assistant_message",
			Role: "assistant",
			Text: "hello",
			Meta: map[string]any{"provider_message_id": "pm-1"},
		}},
	}
	if !projector.Apply(first) {
		t.Fatalf("expected first event to apply")
	}

	replay := first
	replay.Revision = transcriptdomain.MustParseRevisionToken("2")
	if projector.Apply(replay) {
		t.Fatalf("expected replay event with same identity+text to be dropped")
	}

	progression := first
	progression.Revision = transcriptdomain.MustParseRevisionToken("3")
	progression.Delta = []transcriptdomain.Block{{
		ID:   "msg-1",
		Kind: "assistant_message",
		Role: "assistant",
		Text: "hello world",
		Meta: map[string]any{"provider_message_id": "pm-1"},
	}}
	if !projector.Apply(progression) {
		t.Fatalf("expected same-identity different-text progression to apply")
	}

	blocks := projector.Snapshot().Blocks
	if len(blocks) != 2 {
		t.Fatalf("expected only replay event to be dropped, got blocks=%#v", blocks)
	}
}

func TestTranscriptProjectorReplayDedupeDoesNotUseTurnOnlyFallback(t *testing.T) {
	projector := NewTranscriptProjector("s1", "codex", "")
	base := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
		Delta: []transcriptdomain.Block{{
			Kind: "assistant_message",
			Role: "assistant",
			Text: "same text",
			Meta: map[string]any{"turn_id": "turn-1"},
		}},
	}
	if !projector.Apply(base) {
		t.Fatalf("expected initial event to apply")
	}
	replay := base
	replay.Revision = transcriptdomain.MustParseRevisionToken("2")
	if !projector.Apply(replay) {
		t.Fatalf("expected turn-only identity not to be deduped")
	}
	blocks := projector.Snapshot().Blocks
	if len(blocks) != 2 {
		t.Fatalf("expected both blocks retained without stable identity, got %#v", blocks)
	}
}

func TestTranscriptProjectorReplayDedupeSupportsExplicitTurnScopedIdentity(t *testing.T) {
	projector := NewTranscriptProjector("s1", "codex", "")
	first := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
		Delta: []transcriptdomain.Block{{
			Kind: "assistant_message",
			Role: "assistant",
			Text: "turn scoped content",
			Meta: map[string]any{
				"turn_id":        "turn-1",
				"turn_scoped_id": "segment-1",
			},
		}},
	}
	if !projector.Apply(first) {
		t.Fatalf("expected first turn-scoped event to apply")
	}
	replay := first
	replay.Revision = transcriptdomain.MustParseRevisionToken("2")
	if projector.Apply(replay) {
		t.Fatalf("expected explicit turn-scoped replay to be deduped")
	}
}

func TestTranscriptProjectorNextRevisionLexicalBase(t *testing.T) {
	projector := NewTranscriptProjector("s1", "codex", transcriptdomain.MustParseRevisionToken("seed"))
	next := projector.NextRevision()
	if got := next.String(); got != "seed.00000000000000000001" {
		t.Fatalf("expected lexical revision suffix, got %q", got)
	}
}

func TestTranscriptProjectorNextRevisionNumericModes(t *testing.T) {
	projector := NewTranscriptProjector("s1", "codex", "")
	if got := projector.NextRevision().String(); got != "1" {
		t.Fatalf("expected first numeric revision from zero base, got %q", got)
	}
	if got := projector.NextRevision().String(); got != "2" {
		t.Fatalf("expected incremented numeric revision, got %q", got)
	}

	projector = NewTranscriptProjector("s1", "codex", transcriptdomain.MustParseRevisionToken("9"))
	if got := projector.NextRevision().String(); got != "10" {
		t.Fatalf("expected numeric base increment, got %q", got)
	}
}

func TestTranscriptProjectorApplyReplaceTurnAndHeartbeatBranches(t *testing.T) {
	projector := NewTranscriptProjector("s1", "codex", "")

	if applied := projector.Apply(transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventReplace,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
		Replace: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("1"),
			Blocks: []transcriptdomain.Block{{
				Kind: "assistant_message",
				Role: "assistant",
				Text: "seed",
			}},
			Turn: transcriptdomain.TurnState{State: transcriptdomain.TurnStateIdle},
		},
	}); !applied {
		t.Fatalf("expected replace event to apply")
	}

	if applied := projector.Apply(transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventTurnStarted,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
		Turn: &transcriptdomain.TurnState{
			State:  transcriptdomain.TurnStateRunning,
			TurnID: "turn-1",
		},
	}); !applied {
		t.Fatalf("expected turn started event to apply")
	}
	if got := projector.ActiveTurnID(); got != "turn-1" {
		t.Fatalf("expected active turn to update, got %q", got)
	}

	if applied := projector.Apply(transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventHeartbeat,
		SessionID: "s1",
		Provider:  "codex",
	}); !applied {
		t.Fatalf("expected heartbeat event to apply")
	}
}

func TestTranscriptProjectorApplyRejectsInvalidAndStaleRevisions(t *testing.T) {
	projector := NewTranscriptProjector("s1", "codex", "")
	if applied := projector.Apply(transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
		Delta: []transcriptdomain.Block{{
			Kind: "assistant_message",
			Role: "assistant",
			Text: "ok",
			Meta: map[string]any{"provider_message_id": "pm-1"},
		}},
	}); !applied {
		t.Fatalf("expected first delta to apply")
	}

	if applied := projector.Apply(transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
		Delta: []transcriptdomain.Block{{
			Kind: "assistant_message",
			Role: "assistant",
			Text: "stale",
			Meta: map[string]any{"provider_message_id": "pm-2"},
		}},
	}); applied {
		t.Fatalf("expected stale revision to be rejected")
	}
}

func TestTranscriptProjectorDedupeDecisionLoggingIncludesReason(t *testing.T) {
	restore := captureStdLogOutput(t)
	defer restore()

	existing := []transcriptdomain.Block{{
		Kind: "assistant_message",
		Role: "assistant",
		Text: "hello",
		Meta: map[string]any{"provider_message_id": "pm-1"},
	}}
	incoming := []transcriptdomain.Block{
		{
			Kind: "assistant_message",
			Role: "assistant",
			Text: "hello",
			Meta: map[string]any{"provider_message_id": "pm-1"},
		},
		{
			Kind: "assistant_message",
			Role: "assistant",
			Text: "hello world",
			Meta: map[string]any{"provider_message_id": "pm-2"},
		},
	}
	_ = filterDuplicateTranscriptBlocks(
		existing,
		incoming,
		transcriptdomain.NewProjectorTranscriptDedupePolicy(nil),
		nil,
		"s1",
		"codex",
	)

	logOutput := currentCapturedStdLog()
	if !strings.Contains(logOutput, "transcript_projector_dedupe decision=dropped_replay_duplicate reason=stable_identity_text_match") {
		t.Fatalf("expected replay-drop log reason, got %q", logOutput)
	}
	if !strings.Contains(logOutput, "transcript_projector_dedupe decision=accepted_new reason=stable_identity_not_found") {
		t.Fatalf("expected accepted-new log reason, got %q", logOutput)
	}
}

func TestTranscriptRoleSupportsReplayDedupeCoverage(t *testing.T) {
	policy := transcriptdomain.NewProjectorTranscriptDedupePolicy(nil)
	assistantDecision := policy.ReplayDecision(
		nil,
		transcriptdomain.TranscriptIdentityBlock{
			Role: "assistant",
			Text: "hello",
		},
	)
	if assistantDecision.Reason != "missing_stable_identity" {
		t.Fatalf("expected assistant role to be replay-dedupe-eligible, got %#v", assistantDecision)
	}

	systemDecision := policy.ReplayDecision(
		nil,
		transcriptdomain.TranscriptIdentityBlock{
			Role: "system",
			Text: "hello",
		},
	)
	if systemDecision.Reason != "unsupported_role" {
		t.Fatalf("expected system role to be replay-dedupe-ineligible, got %#v", systemDecision)
	}
}

var transcriptProjectorLogBuffer bytes.Buffer

type projectorStubIdentityPolicy struct{}

func (projectorStubIdentityPolicy) Identity(transcriptdomain.TranscriptIdentityBlock) transcriptdomain.MessageIdentity {
	return transcriptdomain.MessageIdentity{
		Scope: transcriptdomain.MessageIdentityScopeProviderMessage,
		Value: "shared",
	}
}

func (projectorStubIdentityPolicy) Equivalent(_, _ transcriptdomain.TranscriptIdentityBlock) bool {
	return true
}

func (projectorStubIdentityPolicy) CanFinalizeReplace(_, _ transcriptdomain.TranscriptIdentityBlock) bool {
	return true
}

func TestTranscriptProjectorSupportsCustomIdentityPolicy(t *testing.T) {
	projector := NewTranscriptProjectorWithIdentityPolicy("s1", "codex", "", projectorStubIdentityPolicy{})
	first := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
		Delta: []transcriptdomain.Block{{
			ID:   "msg-1",
			Kind: "assistant_message",
			Role: "assistant",
			Text: "hello",
		}},
	}
	if !projector.Apply(first) {
		t.Fatalf("expected first event to apply")
	}

	replay := first
	replay.Revision = transcriptdomain.MustParseRevisionToken("2")
	replay.Delta = []transcriptdomain.Block{{
		ID:   "msg-2",
		Kind: "assistant_message",
		Role: "assistant",
		Text: "hello",
	}}
	if projector.Apply(replay) {
		t.Fatalf("expected custom policy to dedupe replay by injected identity")
	}
}

func captureStdLogOutput(t *testing.T) func() {
	t.Helper()
	previousWriter := log.Writer()
	previousFlags := log.Flags()
	log.SetFlags(0)
	transcriptProjectorLogBuffer.Reset()
	log.SetOutput(&transcriptProjectorLogBuffer)
	return func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}
}

func currentCapturedStdLog() string {
	return transcriptProjectorLogBuffer.String()
}
