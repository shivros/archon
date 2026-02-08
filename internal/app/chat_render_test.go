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
