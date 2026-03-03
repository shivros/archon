package app

import (
	"testing"

	"control/internal/daemon/transcriptdomain"
)

func TestTranscriptSnapshotStaleRevisionDoesNotOverwriteVisibleBlocks(t *testing.T) {
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	m := newPhase0ModelWithSession("codex")
	WithTranscriptBoundaryMetricsSink(sink)(&m)
	m.pendingSessionKey = "sess:s1"

	handled, _ := m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("2"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "new"}},
		},
	})
	if !handled {
		t.Fatalf("expected initial transcript snapshot to be handled")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "new" {
		t.Fatalf("expected latest transcript to be new, got %q", got)
	}

	handled, _ = m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("1"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "old"}},
		},
	})
	if !handled {
		t.Fatalf("expected stale snapshot message to be handled")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "new" {
		t.Fatalf("expected stale snapshot to be ignored, got %q", got)
	}

	metrics := sink.Snapshot()
	if len(metrics) == 0 {
		t.Fatalf("expected stale revision metric")
	}
	last := metrics[len(metrics)-1]
	if last.Name != transcriptMetricStaleRevision || last.Reason != transcriptReasonSnapshotSuperseded {
		t.Fatalf("unexpected stale snapshot metric: %#v", last)
	}
}

func TestTranscriptSnapshotEqualRevisionDoesNotOverwriteVisibleBlocks(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.pendingSessionKey = "sess:s1"

	handled, _ := m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("5"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "same-rev"}},
		},
	})
	if !handled {
		t.Fatalf("expected initial snapshot to be handled")
	}
	handled, _ = m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("5"),
			Blocks:    []transcriptdomain.Block{{Kind: "assistant_message", Role: "assistant", Text: "should-not-apply"}},
		},
	})
	if !handled {
		t.Fatalf("expected equal revision snapshot to be handled")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "same-rev" {
		t.Fatalf("expected equal revision snapshot to be ignored, got %q", got)
	}
}
