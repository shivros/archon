package app

import (
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
)

type delayedRenderPipeline struct {
	delay time.Duration
	next  RenderPipeline
}

func (p delayedRenderPipeline) Render(req RenderRequest) RenderResult {
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	next := p.next
	if next == nil {
		next = NewDefaultRenderPipeline()
	}
	return next.Render(req)
}

func TestAppendUserMessageLocalRendersImmediatelyWhenAsyncEnabled(t *testing.T) {
	m := NewModel(nil,
		WithAsyncViewportRendering(true),
		WithRenderPipeline(delayedRenderPipeline{delay: 100 * time.Millisecond}),
	)
	m.resize(120, 40)

	before := m.renderVersion
	header := m.appendUserMessageLocal("codex", "hello from compose")

	if header != 0 {
		t.Fatalf("expected optimistic user header index 0, got %d", header)
	}
	if m.renderVersion <= before {
		t.Fatalf("expected optimistic user message to render immediately")
	}
	if got := len(m.viewport.View()); got == 0 {
		t.Fatalf("expected viewport content after optimistic render")
	}
}

func TestTranscriptSnapshotRendersImmediatelyWhileLoadingWhenAsyncEnabled(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	WithAsyncViewportRendering(true)(&m)
	WithRenderPipeline(delayedRenderPipeline{delay: 100 * time.Millisecond})(&m)
	m.resize(120, 40)
	m.pendingSessionKey = "sess:s1"
	m.loading = true
	m.loadingKey = "sess:s1"

	before := m.renderVersion
	handled, _ := m.reduceStateMessages(transcriptSnapshotMsg{
		id:  "s1",
		key: "sess:s1",
		snapshot: &transcriptdomain.TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("1"),
			Blocks: []transcriptdomain.Block{
				{Kind: "assistant_message", Role: "assistant", Text: "snapshot reply"},
			},
		},
	})
	if !handled {
		t.Fatalf("expected transcript snapshot to be handled")
	}
	if m.renderVersion <= before {
		t.Fatalf("expected loading snapshot to render immediately")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "snapshot reply" {
		t.Fatalf("expected visible assistant block, got %q", got)
	}
}
