package app

import (
	"context"
	"errors"
	"testing"

	"control/internal/daemon/transcriptdomain"
)

type transcriptStreamContextRecorder struct {
	ctx context.Context
}

func (r *transcriptStreamContextRecorder) TranscriptStream(ctx context.Context, _ string, _ string) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	r.ctx = ctx
	select {
	case <-ctx.Done():
		return nil, func() {}, ctx.Err()
	default:
	}
	ch := make(chan transcriptdomain.TranscriptEvent)
	return ch, func() {}, nil
}

func TestOpenTranscriptStreamCmdWithContextUsesParentContext(t *testing.T) {
	api := &transcriptStreamContextRecorder{}
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	msg, ok := openTranscriptStreamCmdWithContext(api, "s1", "rev-1", parent)().(transcriptStreamMsg)
	if !ok {
		t.Fatalf("expected transcriptStreamMsg")
	}
	if api.ctx != parent {
		t.Fatalf("expected wrapper to pass parent context to transcript stream open")
	}
	if !errors.Is(msg.err, context.Canceled) {
		t.Fatalf("expected parent cancellation to propagate, got %v", msg.err)
	}
}
