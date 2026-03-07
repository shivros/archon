package app

import (
	"strings"
	"sync"
	"testing"
)

func TestBuildStyleConfigDisablesDocumentOuterMargins(t *testing.T) {
	cfg := buildStyleConfig(true)
	if cfg.Document.BlockPrefix != "" {
		t.Fatalf("expected empty document block prefix, got %q", cfg.Document.BlockPrefix)
	}
	if cfg.Document.BlockSuffix != "" {
		t.Fatalf("expected empty document block suffix, got %q", cfg.Document.BlockSuffix)
	}
	if cfg.Document.Margin == nil {
		t.Fatalf("expected document margin pointer")
	}
	if *cfg.Document.Margin != 0 {
		t.Fatalf("expected document margin 0, got %d", *cfg.Document.Margin)
	}
}

func TestRenderMarkdownConcurrentAccess(t *testing.T) {
	original := markdownBackgroundDark()
	t.Cleanup(func() {
		_ = setMarkdownBackgroundDark(original)
	})
	_ = setMarkdownBackgroundDark(true)

	input := strings.Join([]string{
		"# Heading",
		"",
		"- item one",
		"- item two",
		"",
		"> quote",
		"",
		"```go",
		"println(\"hello\")",
		"```",
	}, "\n")

	const (
		workers    = 16
		iterations = 50
	)

	var wg sync.WaitGroup
	results := make(chan string, workers*iterations)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				results <- renderMarkdown(input, 48)
			}
		}()
	}
	wg.Wait()
	close(results)

	for rendered := range results {
		if strings.TrimSpace(rendered) == "" {
			t.Fatalf("expected rendered markdown output")
		}
		if !strings.Contains(rendered, "Heading") {
			t.Fatalf("expected heading in rendered output, got %q", rendered)
		}
	}
}
