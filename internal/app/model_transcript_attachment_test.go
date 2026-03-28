package app

import (
	"context"
	"errors"
	"testing"

	"control/internal/client"
	"control/internal/daemon/transcriptdomain"
)

type transcriptAttachmentOpenAPIStub struct {
	streamCtx   context.Context
	streamID    string
	streamAfter string
}

func (*transcriptAttachmentOpenAPIStub) GetTranscriptSnapshot(context.Context, string, int) (*client.TranscriptSnapshotResponse, error) {
	return &client.TranscriptSnapshotResponse{}, nil
}

func (s *transcriptAttachmentOpenAPIStub) TranscriptStream(ctx context.Context, id, afterRevision string) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	s.streamCtx = ctx
	s.streamID = id
	s.streamAfter = afterRevision
	select {
	case <-ctx.Done():
		return nil, func() {}, ctx.Err()
	default:
	}
	ch := make(chan transcriptdomain.TranscriptEvent)
	return ch, func() {}, nil
}

func TestRequestTranscriptStreamOpenCmdWithContextUsesParentContext(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := &transcriptAttachmentOpenAPIStub{}
	m.sessionTranscriptAPI = api

	parent, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := m.requestTranscriptStreamOpenCmdWithContext(
		"s1",
		"rev-1",
		transcriptAttachmentSourceSelectionLoad,
		"",
		parent,
	)
	if cmd == nil {
		t.Fatalf("expected transcript stream open command")
	}
	msg, ok := cmd().(transcriptStreamMsg)
	if !ok {
		t.Fatalf("expected transcriptStreamMsg")
	}
	if api.streamCtx != parent {
		t.Fatalf("expected parent context to be passed through")
	}
	if api.streamID != "s1" || api.streamAfter != "rev-1" {
		t.Fatalf("unexpected transcript stream args id=%q after=%q", api.streamID, api.streamAfter)
	}
	if !errors.Is(msg.err, context.Canceled) {
		t.Fatalf("expected canceled parent context to propagate, got %v", msg.err)
	}
	if msg.generation == 0 {
		t.Fatalf("expected generation-aware stream open")
	}
	if msg.source != transcriptAttachmentSourceSelectionLoad {
		t.Fatalf("expected source %q, got %q", transcriptAttachmentSourceSelectionLoad, msg.source)
	}
}

func TestRequestTranscriptStreamOpenCmdWithContextDetachesExistingStreamAndRecordsReconnectAttempt(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := &transcriptAttachmentOpenAPIStub{}
	m.sessionTranscriptAPI = api
	sink := NewInMemoryTranscriptBoundaryMetricsSink()
	WithTranscriptBoundaryMetricsSink(sink)(&m)

	oldCanceled := false
	if m.transcriptStream != nil {
		m.transcriptStream.SetStream(make(chan transcriptdomain.TranscriptEvent), func() { oldCanceled = true })
	}

	cmd := m.requestTranscriptStreamOpenCmdWithContext(
		"s1",
		"",
		transcriptAttachmentSourceSessionStart,
		transcriptSourceSendMsg,
		context.Background(),
	)
	if cmd == nil {
		t.Fatalf("expected transcript stream open command")
	}
	if !oldCanceled {
		t.Fatalf("expected existing transcript stream to be canceled before opening a new one")
	}
	if m.transcriptStream != nil && m.transcriptStream.HasStream() {
		t.Fatalf("expected existing transcript stream to be detached")
	}

	msg, ok := cmd().(transcriptStreamMsg)
	if !ok {
		t.Fatalf("expected transcriptStreamMsg")
	}
	if msg.generation == 0 {
		t.Fatalf("expected generation-aware stream open")
	}
	if msg.source != transcriptAttachmentSourceSessionStart {
		t.Fatalf("expected source %q, got %q", transcriptAttachmentSourceSessionStart, msg.source)
	}

	metrics := sink.Snapshot()
	if len(metrics) == 0 {
		t.Fatalf("expected reconnect-attempt metric to be recorded")
	}
	last := metrics[len(metrics)-1]
	if last.Name != transcriptMetricReconnect || last.Outcome != transcriptOutcomeAttempt || last.Stream != "transcript" || last.Source != transcriptSourceSendMsg {
		t.Fatalf("unexpected reconnect-attempt metric: %#v", last)
	}
}
