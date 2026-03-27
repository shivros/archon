package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"

	xansi "github.com/charmbracelet/x/ansi"
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

func TestTranscriptSnapshotKeepsLoadingVisibleUntilAsyncRenderCompletesWhenAsyncEnabled(t *testing.T) {
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
	if m.renderVersion != before {
		t.Fatalf("expected loading snapshot to defer viewport paint until async render completes")
	}
	if got := latestAssistantBlockText(m.currentBlocks()); got != "snapshot reply" {
		t.Fatalf("expected projected assistant block in model state, got %q", got)
	}
	if !m.loading {
		t.Fatalf("expected loading to remain visible until async render completes")
	}
	if plain := xansi.Strip(fmt.Sprint(m.View().Content)); !strings.Contains(plain, "Loading...") {
		t.Fatalf("expected loading overlay while async render is pending, got %q", plain)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.consumeCompletedViewportRender()
		if strings.Contains(strings.ToLower(xansi.Strip(m.renderedText)), "snapshot reply") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(strings.ToLower(xansi.Strip(m.renderedText)), "snapshot reply") {
		t.Fatalf("expected async render to eventually apply snapshot reply, got %q", m.renderedText)
	}
	if m.loading {
		t.Fatalf("expected loading to clear after async render applies")
	}
}

func TestLoadSelectedSessionUsesAsyncViewportRenderForCachedTranscript(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	WithAsyncViewportRendering(true)(&m)
	WithRenderPipeline(delayedRenderPipeline{delay: 100 * time.Millisecond})(&m)
	m.resize(120, 40)
	m.setContentText("previous session")
	item := m.selectedItem()
	if item == nil || item.session == nil {
		t.Fatalf("expected selected session item")
	}
	m.transcriptCache[item.key()] = []ChatBlock{{Role: ChatRoleAgent, Text: "cached reply"}}

	before := m.renderVersion
	_ = m.loadSelectedSession(item)

	if m.renderVersion != before {
		t.Fatalf("expected cached session load to avoid sync viewport rendering")
	}
	if !m.loading {
		t.Fatalf("expected loading to stay visible while cached transcript render is pending")
	}
	if plain := xansi.Strip(fmt.Sprint(m.View().Content)); !strings.Contains(plain, "Loading...") {
		t.Fatalf("expected loading overlay during cached async render, got %q", plain)
	}
	if strings.Contains(strings.ToLower(xansi.Strip(m.renderedText)), "cached reply") {
		t.Fatalf("expected cached transcript not to replace viewport until async render completes")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.consumeCompletedViewportRender()
		if strings.Contains(strings.ToLower(xansi.Strip(m.renderedText)), "cached reply") {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(strings.ToLower(xansi.Strip(m.renderedText)), "cached reply") {
		t.Fatalf("expected cached transcript to render asynchronously, got %q", m.renderedText)
	}
}
