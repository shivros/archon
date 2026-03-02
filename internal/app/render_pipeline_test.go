package app

import (
	"strings"
	"testing"
)

func TestDefaultRenderPipelineRendersRawContent(t *testing.T) {
	pipeline := NewDefaultRenderPipeline()
	result := pipeline.Render(RenderRequest{
		Width:          80,
		RawContent:     "## Hello\n\nworld",
		EscapeMarkdown: false,
	})

	if result.Text == "" {
		t.Fatalf("expected rendered markdown output")
	}
	if len(result.PlainLines) == 0 {
		t.Fatalf("expected plain lines")
	}
	if len(result.Spans) != 0 {
		t.Fatalf("expected no spans for raw markdown render")
	}
}

func TestDefaultRenderPipelineRenderRawPreservesANSI(t *testing.T) {
	pipeline := NewDefaultRenderPipeline()
	result := pipeline.Render(RenderRequest{
		Width:      80,
		RawContent: "\x1b[38;5;118mselected\x1b[0m option",
		RenderRaw:  true,
	})

	if !strings.Contains(result.Text, "\x1b[38;5;118mselected\x1b[0m") {
		t.Fatalf("expected ANSI sequence to be preserved, got %q", result.Text)
	}
}

func TestDefaultRenderPipelineRendersBlocksWithSpans(t *testing.T) {
	pipeline := NewDefaultRenderPipeline()
	result := pipeline.Render(RenderRequest{
		Width:    100,
		MaxLines: 2000,
		Blocks:   []ChatBlock{{ID: "1", Role: ChatRoleUser, Text: "hello"}, {ID: "2", Role: ChatRoleAgent, Text: "hi there"}},
		Selection: RenderSelection{
			PrimaryIndex: 0,
		},
	})

	if result.Text == "" {
		t.Fatalf("expected rendered chat output")
	}
	if len(result.Spans) != 2 {
		t.Fatalf("expected spans for each block, got %d", len(result.Spans))
	}
	if len(result.Lines) == 0 || len(result.PlainLines) == 0 {
		t.Fatalf("expected lines and plain lines")
	}
}

func TestDefaultRenderPipelineSelectionImpactsRenderedOutput(t *testing.T) {
	pipeline := NewDefaultRenderPipeline()
	req := RenderRequest{
		Width: 100,
		Blocks: []ChatBlock{
			{ID: "1", Role: ChatRoleUser, Text: "hello"},
			{ID: "2", Role: ChatRoleAgent, Text: "hi there"},
		},
	}
	base := pipeline.Render(req)
	highlighted := pipeline.Render(RenderRequest{
		Width:  req.Width,
		Blocks: req.Blocks,
		Selection: RenderSelection{
			RangeStart: 1,
			RangeEnd:   1,
		},
	})
	if base.Text == highlighted.Text {
		t.Fatalf("expected selection range to change rendered output")
	}
}

func TestRenderResultCacheEvictsOldestKey(t *testing.T) {
	cache := newRenderResultCache(1)
	cache.Set(1, RenderResult{Text: "first"})
	cache.Set(2, RenderResult{Text: "second"})

	if _, ok := cache.Get(1); ok {
		t.Fatalf("expected oldest key to be evicted")
	}
	if got, ok := cache.Get(2); !ok || got.Text != "second" {
		t.Fatalf("expected newest key retained, got %#v (ok=%v)", got, ok)
	}
}

func TestWithRenderPipelineIgnoresNilPipeline(t *testing.T) {
	m := NewModel(nil)
	original := m.renderPipeline

	WithRenderPipeline(nil)(&m)

	if m.renderPipeline != original {
		t.Fatalf("expected nil pipeline option to be ignored")
	}
}

func TestNewRenderResultCacheMinimumSizeIsOne(t *testing.T) {
	cache := newRenderResultCache(0)
	cache.Set(1, RenderResult{Text: "first"})
	cache.Set(2, RenderResult{Text: "second"})

	if _, ok := cache.Get(1); ok {
		t.Fatalf("expected min-size cache to evict oldest entry")
	}
	if _, ok := cache.Get(2); !ok {
		t.Fatalf("expected newest entry to remain")
	}
}

func TestRenderRawContentUsesDefaultWidthForRaw(t *testing.T) {
	out := renderRawContent("line one line two line three", false, true, 0)
	if out == "" {
		t.Fatalf("expected wrapped raw content output")
	}
}
