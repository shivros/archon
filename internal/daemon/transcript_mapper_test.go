package daemon

import (
	"encoding/json"
	"testing"

	"control/internal/daemon/transcriptadapters"
	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

func TestDefaultTranscriptMapperUnknownProviderEventReturnsNil(t *testing.T) {
	mapper := NewDefaultTranscriptMapper(nil)
	mapped := mapper.MapEvent("unknown-provider", transcriptadapters.MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
	}, types.CodexEvent{Method: "turn/started"})
	if len(mapped) != 0 {
		t.Fatalf("expected no mapped events for unknown provider, got %d", len(mapped))
	}
}

func TestDefaultTranscriptMapperUnknownProviderItemUsesGenericDeltaFallback(t *testing.T) {
	mapper := NewDefaultTranscriptMapper(nil)
	mapped := mapper.MapItem("unknown-provider", transcriptadapters.MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("1"),
	}, map[string]any{"type": "assistant", "text": "hello"})
	if len(mapped) != 1 {
		t.Fatalf("expected one generic delta event, got %d", len(mapped))
	}
	if mapped[0].Kind != transcriptdomain.TranscriptEventDelta {
		t.Fatalf("expected delta kind, got %q", mapped[0].Kind)
	}
}

func TestDefaultTranscriptMapperKnownProviderEventMaps(t *testing.T) {
	mapper := NewDefaultTranscriptMapper(nil)
	mapped := mapper.MapEvent("codex", transcriptadapters.MappingContext{
		SessionID: "s1",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
	}, types.CodexEvent{Method: "turn/started", Params: json.RawMessage(`{"turnId":"t1"}`)})
	if len(mapped) != 1 {
		t.Fatalf("expected one mapped event for codex, got %d", len(mapped))
	}
	if mapped[0].Kind != transcriptdomain.TranscriptEventTurnStarted {
		t.Fatalf("expected turn started kind, got %q", mapped[0].Kind)
	}
}
