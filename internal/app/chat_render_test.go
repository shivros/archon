package app

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

func TestReasoningPreviewTextTruncates(t *testing.T) {
	text := "one\ntwo\nthree\nfour\nfive"
	preview, truncated := reasoningPreviewText(text, 3, 100)
	if !truncated {
		t.Fatalf("expected truncated preview")
	}
	if strings.Contains(preview, "four") {
		t.Fatalf("expected preview to truncate lines, got %q", preview)
	}
}

func TestRenderChatBlocksCollapsedReasoningShowsHint(t *testing.T) {
	blocks := []ChatBlock{
		{
			ID:        "reasoning-1",
			Role:      ChatRoleReasoning,
			Text:      "line1\nline2\nline3\nline4\nline5",
			Collapsed: true,
		},
	}
	rendered, _ := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "collapsed, press e or click to expand") {
		t.Fatalf("expected collapsed hint in rendered output: %q", plain)
	}
}

func TestRenderChatBlocksExpandedReasoningOmitsHint(t *testing.T) {
	blocks := []ChatBlock{
		{
			ID:        "reasoning-1",
			Role:      ChatRoleReasoning,
			Text:      "line1\nline2\nline3\nline4\nline5",
			Collapsed: false,
		},
	}
	rendered, _ := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if strings.Contains(plain, "collapsed, press e or click to expand") {
		t.Fatalf("did not expect collapsed hint in expanded output: %q", plain)
	}
}

func TestRenderChatBlocksShowsCopyControlPerMessage(t *testing.T) {
	blocks := []ChatBlock{
		{Role: ChatRoleAgent, Text: "hello"},
	}
	rendered, spans := renderChatBlocks(blocks, 80, 2000)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "[Copy]") {
		t.Fatalf("expected copy control in rendered output: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one rendered span, got %d", len(spans))
	}
	if spans[0].CopyLine < 0 || spans[0].CopyStart < 0 || spans[0].CopyEnd < spans[0].CopyStart {
		t.Fatalf("expected copy hitbox metadata, got %#v", spans[0])
	}
}

func TestRenderChatBlocksWithSelectionShowsSelectedMarker(t *testing.T) {
	blocks := []ChatBlock{
		{Role: ChatRoleAgent, Text: "hello"},
	}
	rendered, spans := renderChatBlocksWithSelection(blocks, 80, 2000, 0)
	plain := xansi.Strip(rendered)
	if !strings.Contains(plain, "Selected") {
		t.Fatalf("expected selected marker in rendered output: %q", plain)
	}
	if len(spans) != 1 {
		t.Fatalf("expected one rendered span, got %d", len(spans))
	}
	if spans[0].CopyLine <= spans[0].StartLine {
		t.Fatalf("expected copy line to account for selected marker, got span %#v", spans[0])
	}
}
