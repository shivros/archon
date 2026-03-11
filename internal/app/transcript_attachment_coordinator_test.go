package app

import "testing"

func TestDefaultTranscriptAttachmentCoordinatorLifecycle(t *testing.T) {
	coord := NewDefaultTranscriptAttachmentCoordinator()

	first := coord.Begin(" s1 ", "", " 1 ")
	if first.SessionID != "s1" {
		t.Fatalf("expected trimmed session id, got %#v", first)
	}
	if first.Generation != 1 {
		t.Fatalf("expected first generation=1, got %#v", first)
	}
	if first.Source != transcriptAttachmentSourceUnknown {
		t.Fatalf("expected unknown source normalization, got %#v", first.Source)
	}
	if first.AfterRevision != "1" {
		t.Fatalf("expected trimmed after revision, got %#v", first.AfterRevision)
	}

	second := coord.Begin("s1", transcriptAttachmentSourceSelectionLoad, "2")
	if second.Generation != 2 {
		t.Fatalf("expected second generation=2, got %#v", second)
	}

	current, ok := coord.Current("s1")
	if !ok || current.Generation != second.Generation {
		t.Fatalf("expected current generation %#v, got ok=%v current=%#v", second, ok, current)
	}

	stale := coord.Evaluate("s1", first.Generation)
	if stale.Accept || stale.Reason != transcriptReasonReconnectStaleGeneration {
		t.Fatalf("expected stale generation rejection, got %#v", stale)
	}

	accepted := coord.Evaluate("s1", second.Generation)
	if !accepted.Accept || accepted.Reason != transcriptReasonReconnectMatchedSession {
		t.Fatalf("expected current generation acceptance, got %#v", accepted)
	}

	coord.MarkGenerationUnhealthy("s1", second.Generation, "")
	rejected := coord.Evaluate("s1", second.Generation)
	if rejected.Accept || rejected.Reason != transcriptReasonReconnectUnhealthyGeneration {
		t.Fatalf("expected unhealthy generation rejection, got %#v", rejected)
	}

	coord.ClearSession("s1")
	if _, ok := coord.Current("s1"); ok {
		t.Fatalf("expected clear session to remove current generation")
	}
}

func TestDefaultTranscriptAttachmentCoordinatorReset(t *testing.T) {
	coord := NewDefaultTranscriptAttachmentCoordinator()
	coord.Begin("s1", transcriptAttachmentSourceSelectionLoad, "1")
	coord.Begin("s2", transcriptAttachmentSourceRecovery, "5")

	coord.Reset()

	if _, ok := coord.Current("s1"); ok {
		t.Fatalf("expected s1 state cleared after reset")
	}
	if _, ok := coord.Current("s2"); ok {
		t.Fatalf("expected s2 state cleared after reset")
	}

	generation := coord.Begin("s1", transcriptAttachmentSourceSelectionLoad, "")
	if generation.Generation != 1 {
		t.Fatalf("expected generation counter reset after reset, got %#v", generation)
	}
}

func TestWithTranscriptAttachmentCoordinatorOption(t *testing.T) {
	custom := NewDefaultTranscriptAttachmentCoordinator()
	model := NewModel(nil, WithTranscriptAttachmentCoordinator(custom))
	if model.transcriptAttachmentCoordinator != custom {
		t.Fatalf("expected custom attachment coordinator to be installed")
	}

	model = NewModel(nil, WithTranscriptAttachmentCoordinator(nil))
	if model.transcriptAttachmentCoordinator == nil {
		t.Fatalf("expected nil option to install default attachment coordinator")
	}
}
