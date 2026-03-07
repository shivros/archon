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

	changed, closed, signal, signals := controller.ConsumeTick()
	if closed {
		t.Fatalf("expected open stream")
	}
	if !changed {
		t.Fatalf("expected changed=true for replace/delta")
	}
	if !signal {
		t.Fatalf("expected signal=true for ready/replace/delta events")
	}
	if signals.Events != 3 {
		t.Fatalf("expected 3 events, got %d", signals.Events)
	}
	if signals.ContentEvents != 2 {
		t.Fatalf("expected 2 content events, got %d", signals.ContentEvents)
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

	changed, _, signal, signals := controller.ConsumeTick()
	if changed || signal {
		t.Fatalf("expected stale/equal events to be ignored")
	}
	if signals.Events != 2 {
		t.Fatalf("expected two consumed events, got %d", signals.Events)
	}
	if signals.ContentEvents != 0 {
		t.Fatalf("expected stale/equal events to produce no content signals, got %d", signals.ContentEvents)
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

func TestTranscriptStreamControllerCoalescesAdjacentAssistantDeltasForSameMessage(t *testing.T) {
	controller := NewTranscriptStreamController(16)
	ch := make(chan transcriptdomain.TranscriptEvent, 4)
	controller.SetStream(ch, nil)

	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("1"),
		Delta: []transcriptdomain.Block{
			{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "hello"},
		},
	}
	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("2"),
		Delta: []transcriptdomain.Block{
			{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "world"},
		},
	}

	changed, closed, signal, _ := controller.ConsumeTick()
	if closed {
		t.Fatalf("expected open stream")
	}
	if !changed || !signal {
		t.Fatalf("expected coalesced deltas to mark content changed")
	}

	blocks := controller.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected one coalesced assistant block, got %#v", blocks)
	}
	if blocks[0].Text != "helloworld" {
		t.Fatalf("expected merged assistant text, got %#v", blocks[0])
	}
}
