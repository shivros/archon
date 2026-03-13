package transcriptadapters

import (
	"encoding/json"
	"testing"
)

type extractorStringer string

func (s extractorStringer) String() string {
	return string(s)
}

func TestCodexTranscriptTextExtractorExtractCoversInputShapes(t *testing.T) {
	extractor := newCodexTranscriptTextExtractor()

	if got := extractor.Extract(json.Number("42")); got != "42" {
		t.Fatalf("expected json.Number extraction, got %q", got)
	}
	if got := extractor.Extract([]map[string]any{
		{"text": "first"},
		{"text": " second"},
	}); got != "first second" {
		t.Fatalf("expected []map extraction to preserve concatenation, got %q", got)
	}
	if got := extractor.Extract(map[string]any{
		"content": map[string]any{"text": "nested"},
	}); got != "nested" {
		t.Fatalf("expected map fallback extraction, got %q", got)
	}
	if got := extractor.Extract(extractorStringer("ignored")); got != "" {
		t.Fatalf("expected unsupported input type to resolve empty text, got %q", got)
	}
}

func TestCodexTranscriptTextExtractorPrefersDirectMapText(t *testing.T) {
	got := extractCodexTextFromMap(map[string]any{
		"text":    "direct",
		"content": map[string]any{"text": "nested"},
	})
	if got != "direct" {
		t.Fatalf("expected direct map text to take precedence, got %q", got)
	}
}

func TestExtractCodexTextFromMapSlicePreservesWhitespaceFragments(t *testing.T) {
	got := extractCodexTextFromMapSlice([]map[string]any{
		{"text": "  "},
		{"text": "a"},
		{"text": "b"},
	})
	if got != "  ab" {
		t.Fatalf("expected whitespace fragments preserved during map slice extraction, got %q", got)
	}
}

func TestExtractCodexTextFromMapSliceEmptyInput(t *testing.T) {
	if got := extractCodexTextFromMapSlice(nil); got != "" {
		t.Fatalf("expected empty map-slice extraction result, got %q", got)
	}
}

func TestJoinLosslessTextEdgeCases(t *testing.T) {
	if got := joinLosslessText(nil); got != "" {
		t.Fatalf("expected empty join result, got %q", got)
	}
	if got := joinLosslessText([]string{"x"}); got != "x" {
		t.Fatalf("expected single-value join passthrough, got %q", got)
	}
}

func TestExtractCodexTextFromAnySlicePreservesNewlineFragments(t *testing.T) {
	got := extractCodexTextFromAnySlice([]any{
		map[string]any{"text": "conventions."},
		map[string]any{"text": "\n\n"},
		map[string]any{"text": "A few repo anchors..."},
	})
	if got != "conventions.\n\nA few repo anchors..." {
		t.Fatalf("expected newline fragments to be preserved, got %q", got)
	}
}

func TestFirstNonEmptyExtractedNilExtractor(t *testing.T) {
	if got := firstNonEmptyExtracted(nil, "x"); got != "" {
		t.Fatalf("expected nil extractor to resolve empty, got %q", got)
	}
}
