package app

import (
	"testing"

	"control/internal/daemon/transcriptdomain"
)

func TestTranscriptStreamControllerAppliesEventsInRevisionOrder(t *testing.T) {
	controller := NewTranscriptStreamController(16)
	ch := make(chan transcriptdomain.TranscriptEvent, 8)
	controller.SetStream(ch, nil)

	ch <- transcriptdomain.TranscriptEvent{
		Kind:         transcriptdomain.TranscriptEventStreamStatus,
		Revision:     transcriptdomain.MustParseRevisionToken("1"),
		StreamStatus: transcriptdomain.StreamStatusReady,
	}
	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventReplace,
		Revision: transcriptdomain.MustParseRevisionToken("2"),
		Replace: &transcriptdomain.TranscriptSnapshot{
			Revision: transcriptdomain.MustParseRevisionToken("2"),
			Blocks: []transcriptdomain.Block{
				{Kind: "assistant_message", Role: "assistant", Text: "first"},
			},
		},
	}
	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("3"),
		Delta: []transcriptdomain.Block{
			{Kind: "assistant_delta", Role: "assistant", Text: "second"},
		},
	}

	changed, closed, signal, events := controller.ConsumeTick()
	if closed {
		t.Fatalf("expected open stream")
	}
	if !changed {
		t.Fatalf("expected changed=true for replace/delta")
	}
	if !signal {
		t.Fatalf("expected signal=true for ready/replace/delta events")
	}
	if events != 3 {
		t.Fatalf("expected 3 events, got %d", events)
	}
	if got := controller.StreamStatus(); got != transcriptdomain.StreamStatusReady {
		t.Fatalf("expected ready stream status, got %q", got)
	}
	if got := controller.Revision(); got != "3" {
		t.Fatalf("expected revision 3, got %q", got)
	}
	blocks := controller.Blocks()
	if len(blocks) != 2 || blocks[0].Text != "first" || blocks[1].Text != "second" {
		t.Fatalf("unexpected transcript blocks: %#v", blocks)
	}
}

func TestTranscriptStreamControllerDropsStaleAndEqualRevisions(t *testing.T) {
	controller := NewTranscriptStreamController(16)
	ch := make(chan transcriptdomain.TranscriptEvent, 8)
	controller.SetStream(ch, nil)

	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventReplace,
		Revision: transcriptdomain.MustParseRevisionToken("5"),
		Replace: &transcriptdomain.TranscriptSnapshot{
			Revision: transcriptdomain.MustParseRevisionToken("5"),
			Blocks: []transcriptdomain.Block{
				{Kind: "assistant_message", Role: "assistant", Text: "latest"},
			},
		},
	}
	_, _, _, _ = controller.ConsumeTick()

	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("5"),
		Delta: []transcriptdomain.Block{
			{Kind: "assistant_delta", Role: "assistant", Text: "equal-ignored"},
		},
	}
	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("4"),
		Delta: []transcriptdomain.Block{
			{Kind: "assistant_delta", Role: "assistant", Text: "stale-ignored"},
		},
	}

	changed, _, signal, events := controller.ConsumeTick()
	if changed || signal {
		t.Fatalf("expected stale/equal events to be ignored")
	}
	if events != 2 {
		t.Fatalf("expected two consumed events, got %d", events)
	}
	blocks := controller.Blocks()
	if len(blocks) != 1 || blocks[0].Text != "latest" {
		t.Fatalf("expected stale/equal events not to mutate blocks, got %#v", blocks)
	}
}

func TestTranscriptStreamControllerSetSnapshotAndClose(t *testing.T) {
	controller := NewTranscriptStreamController(4)
	changed, applied := controller.SetSnapshot(transcriptdomain.TranscriptSnapshot{
		Revision: transcriptdomain.MustParseRevisionToken("7"),
		Blocks: []transcriptdomain.Block{
			{Kind: "assistant_message", Role: "assistant", Text: "snapshot"},
		},
	})
	if !changed || !applied {
		t.Fatalf("expected snapshot to apply")
	}
	changed, applied = controller.SetSnapshot(transcriptdomain.TranscriptSnapshot{
		Revision: transcriptdomain.MustParseRevisionToken("7"),
		Blocks: []transcriptdomain.Block{
			{Kind: "assistant_message", Role: "assistant", Text: "same-revision"},
		},
	})
	if changed || applied {
		t.Fatalf("expected equal revision snapshot to be rejected")
	}

	ch := make(chan transcriptdomain.TranscriptEvent)
	controller.SetStream(ch, nil)
	close(ch)
	_, closed, _, _ := controller.ConsumeTick()
	if !closed {
		t.Fatalf("expected closed=true when transcript stream channel closes")
	}
	if got := controller.StreamStatus(); got != transcriptdomain.StreamStatusClosed {
		t.Fatalf("expected closed stream status after channel close, got %q", got)
	}
}
