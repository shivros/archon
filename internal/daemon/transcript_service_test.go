package daemon

import (
	"strings"
	"testing"

	"control/internal/daemon/transcriptdomain"
)

func TestTranscriptProjectionDropsStaleUpdates(t *testing.T) {
	projection := NewTranscriptProjector("s1", "codex", "")
	newer := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
		Delta:     []transcriptdomain.Block{{Kind: "assistant", Text: "new"}},
	}
	older := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
		Delta:     []transcriptdomain.Block{{Kind: "assistant", Text: "old"}},
	}
	if !projection.Apply(newer) {
		t.Fatalf("expected first event to apply")
	}
	if projection.Apply(older) {
		t.Fatalf("expected stale event to be rejected")
	}
	if got := len(projection.Snapshot().Blocks); got != 1 {
		t.Fatalf("expected only one block after stale drop, got %d", got)
	}
}

func TestTranscriptProjectionAssignsDefaultSnapshotRevision(t *testing.T) {
	projection := NewTranscriptProjector("s1", "codex", "")
	snapshot := projection.Snapshot()
	if snapshot.Revision.String() != "1" {
		t.Fatalf("expected default snapshot revision 1, got %q", snapshot.Revision.String())
	}
}

func TestTranscriptProjectionAppliesTurnLifecycle(t *testing.T) {
	projection := NewTranscriptProjector("s1", "codex", "")
	started := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventTurnStarted,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
		Turn: &transcriptdomain.TurnState{
			State:  transcriptdomain.TurnStateRunning,
			TurnID: "t1",
		},
	}
	completed := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventTurnCompleted,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
		Turn: &transcriptdomain.TurnState{
			State:  transcriptdomain.TurnStateCompleted,
			TurnID: "t1",
		},
	}
	if !projection.Apply(started) {
		t.Fatalf("expected turn started to apply")
	}
	if !projection.Apply(completed) {
		t.Fatalf("expected turn completed to apply")
	}
	snapshot := projection.Snapshot()
	if snapshot.Turn.State != transcriptdomain.TurnStateCompleted {
		t.Fatalf("expected completed state, got %q", snapshot.Turn.State)
	}
	if snapshot.Turn.TurnID != "t1" {
		t.Fatalf("expected turn id t1, got %q", snapshot.Turn.TurnID)
	}
}

func TestTranscriptProjectionSupportsNonNumericRevisionBase(t *testing.T) {
	projection := NewTranscriptProjector("s1", "codex", transcriptdomain.MustParseRevisionToken("rev-A"))
	next := projection.NextRevision()
	if next.String() == "" || !strings.HasPrefix(next.String(), "rev-A.") {
		t.Fatalf("expected lexical revision derived from base, got %q", next.String())
	}
	event := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  next,
		Delta:     []transcriptdomain.Block{{Kind: "assistant", Text: "ok"}},
	}
	if !projection.Apply(event) {
		t.Fatalf("expected event with derived lexical revision to apply")
	}
}

func TestTranscriptProjectionRejectsSessionMismatch(t *testing.T) {
	projection := NewTranscriptProjector("s1", "codex", "")
	event := transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "other",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
		Delta:     []transcriptdomain.Block{{Kind: "assistant", Text: "x"}},
	}
	if projection.Apply(event) {
		t.Fatalf("expected session mismatch to be rejected")
	}
}

func TestTranscriptProjectionReplaceOverwritesBlocksAndTurn(t *testing.T) {
	projection := NewTranscriptProjector("s1", "codex", "")
	_ = projection.Apply(transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
		Delta:     []transcriptdomain.Block{{Kind: "assistant", Text: "old"}},
	})
	replace := transcriptdomain.TranscriptSnapshot{
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
		Blocks:    []transcriptdomain.Block{{Kind: "assistant", Text: "new"}},
		Turn:      transcriptdomain.TurnState{State: transcriptdomain.TurnStateRunning, TurnID: "t9"},
	}
	if !projection.Apply(transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventReplace,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
		Replace:   &replace,
	}) {
		t.Fatalf("expected replace event to apply")
	}
	snapshot := projection.Snapshot()
	if len(snapshot.Blocks) != 1 || snapshot.Blocks[0].Text != "new" {
		t.Fatalf("expected blocks to be replaced, got %#v", snapshot.Blocks)
	}
	if snapshot.Turn.TurnID != "t9" {
		t.Fatalf("expected turn to be replaced, got %#v", snapshot.Turn)
	}
	if projection.ActiveTurnID() != "t9" {
		t.Fatalf("expected active turn id t9, got %q", projection.ActiveTurnID())
	}
}

func TestTranscriptProjectionRejectsUnsupportedKind(t *testing.T) {
	projection := NewTranscriptProjector("s1", "codex", "")
	if projection.Apply(transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventKind("unknown"),
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
	}) {
		t.Fatalf("expected unsupported event kind to be rejected")
	}
}
