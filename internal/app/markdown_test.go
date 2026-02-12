package app

import "testing"

func TestBuildStyleConfigDisablesDocumentOuterMargins(t *testing.T) {
	cfg := buildStyleConfig()
	if cfg.Document.StylePrimitive.BlockPrefix != "" {
		t.Fatalf("expected empty document block prefix, got %q", cfg.Document.StylePrimitive.BlockPrefix)
	}
	if cfg.Document.StylePrimitive.BlockSuffix != "" {
		t.Fatalf("expected empty document block suffix, got %q", cfg.Document.StylePrimitive.BlockSuffix)
	}
	if cfg.Document.Margin == nil {
		t.Fatalf("expected document margin pointer")
	}
	if *cfg.Document.Margin != 0 {
		t.Fatalf("expected document margin 0, got %d", *cfg.Document.Margin)
	}
}
