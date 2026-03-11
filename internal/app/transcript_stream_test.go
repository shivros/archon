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
	if signals.RevisionRewind {
		t.Fatalf("expected same-stream stale/equal events not to report rewind")
	}
	blocks := controller.Blocks()
	if len(blocks) != 1 || blocks[0].Text != "latest" {
		t.Fatalf("expected stale/equal events not to mutate blocks, got %#v", blocks)
	}
}

func TestTranscriptStreamControllerDetectsRevisionRewindOnFirstEventOfNewStream(t *testing.T) {
	controller := NewTranscriptStreamController(16)
	changed, applied := controller.SetSnapshot(transcriptdomain.TranscriptSnapshot{
		Revision: transcriptdomain.MustParseRevisionToken("10"),
		Blocks: []transcriptdomain.Block{
			{ID: "msg-1", Kind: "assistant_message", Role: "assistant", Text: "existing"},
		},
	})
	if !changed || !applied {
		t.Fatalf("expected seed snapshot to apply")
	}

	ch := make(chan transcriptdomain.TranscriptEvent, 2)
	controller.SetStream(ch, nil)
	ch <- transcriptdomain.TranscriptEvent{
		Kind:         transcriptdomain.TranscriptEventStreamStatus,
		Revision:     transcriptdomain.MustParseRevisionToken("1"),
		StreamStatus: transcriptdomain.StreamStatusReady,
	}
	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("2"),
		Delta: []transcriptdomain.Block{
			{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "rewound"},
		},
	}

	changed, closed, signal, signals := controller.ConsumeTick()
	if changed || closed {
		t.Fatalf("expected rewind batch not to mutate transcript state")
	}
	if !signal {
		t.Fatalf("expected rewind batch to emit recovery signal")
	}
	if !signals.RevisionRewind {
		t.Fatalf("expected rewind signal to be reported")
	}
	blocks := controller.Blocks()
	if len(blocks) != 1 || blocks[0].Text != "existing" {
		t.Fatalf("expected rewind batch to preserve prior blocks, got %#v", blocks)
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
			{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "hello "},
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
	if blocks[0].Text != "hello world" {
		t.Fatalf("expected merged assistant text, got %#v", blocks[0])
	}
}

func TestTranscriptStreamControllerFinalizedMessageSupersetDedupesPriorDeltas(t *testing.T) {
	controller := NewTranscriptStreamController(16)
	ch := make(chan transcriptdomain.TranscriptEvent, 4)
	controller.SetStream(ch, nil)

	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("1"),
		Delta: []transcriptdomain.Block{
			{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "hello "},
		},
	}
	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("2"),
		Delta: []transcriptdomain.Block{
			{ID: "msg-1", Kind: "assistant_delta", Role: "assistant", Text: "world"},
		},
	}
	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("3"),
		Delta: []transcriptdomain.Block{
			{
				ID:   "msg-1",
				Kind: "assistant_message",
				Role: "assistant",
				Text: "hello world!",
				Meta: map[string]any{"provider_message_id": "pm-1"},
			},
		},
	}

	changed, closed, signal, signals := controller.ConsumeTick()
	if closed || !changed || !signal {
		t.Fatalf("expected finalized-message batch to update stream state")
	}
	if signals.FinalizedEvents == 0 {
		t.Fatalf("expected finalized message classification, got %#v", signals)
	}
	if signals.FinalizedDedupes == 0 {
		t.Fatalf("expected finalized-message dedupe hit, got %#v", signals)
	}

	blocks := controller.Blocks()
	if len(blocks) != 1 {
		t.Fatalf("expected finalized dedupe to keep one assistant block, got %#v", blocks)
	}
	if blocks[0].Text != "hello world!" {
		t.Fatalf("expected finalized text to replace streamed text, got %#v", blocks[0])
	}
	if blocks[0].ProviderMessageID != "pm-1" {
		t.Fatalf("expected provider message id to be preserved, got %#v", blocks[0])
	}
}

func TestTranscriptStreamControllerNilReceiverNoops(t *testing.T) {
	var controller *TranscriptStreamController
	controller.Reset()
	controller.SetStream(nil, nil)

	if controller.HasStream() {
		t.Fatalf("expected nil controller to report no stream")
	}
	if got := controller.Blocks(); got != nil {
		t.Fatalf("expected nil controller blocks=nil, got %#v", got)
	}
	if got := controller.Revision(); got != "" {
		t.Fatalf("expected nil controller revision empty, got %q", got)
	}
	if got := controller.StreamStatus(); got != "" {
		t.Fatalf("expected nil controller status empty, got %q", got)
	}
	changed, applied := controller.SetSnapshot(transcriptdomain.TranscriptSnapshot{
		Revision: transcriptdomain.MustParseRevisionToken("1"),
	})
	if changed || applied {
		t.Fatalf("expected nil controller snapshot apply to noop")
	}
	changed, closed, signal, signals := controller.ConsumeTick()
	if changed || closed || signal || signals.Events != 0 {
		t.Fatalf("expected nil controller consume tick noop, got changed=%v closed=%v signal=%v signals=%#v", changed, closed, signal, signals)
	}
}

func TestTranscriptStreamControllerTurnFailureEmitsCompletionSignal(t *testing.T) {
	controller := NewTranscriptStreamController(8)
	ch := make(chan transcriptdomain.TranscriptEvent, 1)
	controller.SetStream(ch, nil)

	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventTurnFailed,
		Revision: transcriptdomain.MustParseRevisionToken("1"),
		Turn: &transcriptdomain.TurnState{
			TurnID: "turn-failed-1",
			State:  transcriptdomain.TurnStateFailed,
		},
	}

	changed, closed, signal, signals := controller.ConsumeTick()
	if changed || closed {
		t.Fatalf("expected turn failure to be control-only event")
	}
	if !signal {
		t.Fatalf("expected turn failure to emit signal")
	}
	if signals.ControlEvents != 1 || signals.ContentEvents != 0 {
		t.Fatalf("expected one control event and no content events, got %#v", signals)
	}
	if len(signals.CompletionSignals) != 1 || signals.CompletionSignals[0].TurnID != "turn-failed-1" {
		t.Fatalf("expected completion signal for failed turn, got %#v", signals.CompletionSignals)
	}
}

func TestTranscriptStreamControllerIgnoresStaleTurnCompletionAndUnknownEvents(t *testing.T) {
	controller := NewTranscriptStreamController(8)
	ch := make(chan transcriptdomain.TranscriptEvent, 3)
	controller.SetStream(ch, nil)

	changed, applied := controller.SetSnapshot(transcriptdomain.TranscriptSnapshot{
		Revision: transcriptdomain.MustParseRevisionToken("5"),
		Blocks: []transcriptdomain.Block{
			{Kind: "assistant_message", Role: "assistant", Text: "seed"},
		},
	})
	if !changed || !applied {
		t.Fatalf("expected seed snapshot to apply")
	}

	// Prime the stream to consume first-event rewind bookkeeping so this test
	// exercises same-stream stale/unknown handling.
	ch <- transcriptdomain.TranscriptEvent{
		Kind:         transcriptdomain.TranscriptEventStreamStatus,
		Revision:     transcriptdomain.MustParseRevisionToken("6"),
		StreamStatus: transcriptdomain.StreamStatusReady,
	}
	_, _, _, _ = controller.ConsumeTick()

	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventTurnCompleted,
		Revision: transcriptdomain.MustParseRevisionToken("4"),
		Turn: &transcriptdomain.TurnState{
			TurnID: "stale-turn",
			State:  transcriptdomain.TurnStateCompleted,
		},
	}
	ch <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventKind("unknown"),
		Revision: transcriptdomain.MustParseRevisionToken("6"),
	}

	changed, closed, signal, signals := controller.ConsumeTick()
	if changed || closed || signal {
		t.Fatalf("expected stale+unknown events to avoid content/signal changes")
	}
	if len(signals.CompletionSignals) != 0 {
		t.Fatalf("expected no completion signals for stale+unknown events, got %#v", signals.CompletionSignals)
	}
	blocks := controller.Blocks()
	if len(blocks) != 1 || blocks[0].Text != "seed" {
		t.Fatalf("expected stale+unknown events not to mutate blocks, got %#v", blocks)
	}
}

func TestTranscriptBlocksContainUserRelevantContentRules(t *testing.T) {
	if transcriptBlocksContainUserRelevantContent([]transcriptdomain.Block{
		{Kind: "provider_event", Role: "system", Text: "noise"},
	}) {
		t.Fatalf("expected provider_event blocks to be ignored")
	}
	if transcriptBlocksContainUserRelevantContent([]transcriptdomain.Block{
		{Kind: "status", Role: "system", Text: "still system"},
	}) {
		t.Fatalf("expected system-role text to be ignored")
	}
	if !transcriptBlocksContainUserRelevantContent([]transcriptdomain.Block{
		{Kind: "status", Role: "assistant", Text: ""},
	}) {
		t.Fatalf("expected assistant role to count as relevant")
	}
	if !transcriptBlocksContainUserRelevantContent([]transcriptdomain.Block{
		{Kind: "assistant_chunk", Role: "tool", Text: ""},
	}) {
		t.Fatalf("expected assistant-like kind to count as relevant")
	}
	if !transcriptBlocksContainUserRelevantContent([]transcriptdomain.Block{
		{Kind: "misc", Role: "observer", Text: "visible text"},
	}) {
		t.Fatalf("expected non-system text block to count as relevant")
	}
}
