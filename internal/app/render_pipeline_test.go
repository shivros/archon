package app

import "testing"

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

func TestDefaultRenderPipelineRendersBlocksWithSpans(t *testing.T) {
	pipeline := NewDefaultRenderPipeline()
	result := pipeline.Render(RenderRequest{
		Width:              100,
		MaxLines:           2000,
		Blocks:             []ChatBlock{{ID: "1", Role: ChatRoleUser, Text: "hello"}, {ID: "2", Role: ChatRoleAgent, Text: "hi there"}},
		SelectedBlockIndex: 0,
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
