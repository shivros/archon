package config

import (
	"strings"
	"testing"
)

func TestUpdateUIThemeAtPathRequiresPath(t *testing.T) {
	if err := updateUIThemeAtPath("   ", "nordic"); err == nil {
		t.Fatalf("expected empty path error")
	}
}

func TestUpdateUIThemeAtPathReturnsReadErrorForDirectory(t *testing.T) {
	path := t.TempDir()
	if err := updateUIThemeAtPath(path, "nordic"); err == nil {
		t.Fatalf("expected read error for directory path")
	}
}

func TestUpdateUIThemeDocumentInsertsNameLineWhenMissing(t *testing.T) {
	doc := "[theme]\n# keep comment\n\n[chat]\ntimestamp_mode = \"iso\"\n"
	got := updateUIThemeDocument(doc, "Adwaita Dark")
	if !strings.Contains(got, "[theme]\n# keep comment\n\nname = \"adwaita_dark\"\n[chat]") {
		t.Fatalf("expected name line insertion before next section, got %q", got)
	}
}

func TestWithThemeNamePreservesCommentAndCRLF(t *testing.T) {
	line := "\tname = \"default\" # keep\r"
	got := withThemeName(line, "Solarized Light")
	want := "\tname = \"solarized_light\" # keep\r"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestSplitInlineCommentHandlesEscapedQuoteAndHash(t *testing.T) {
	line := `name = "value with \"#\" marker" # trailing`
	body, comment := splitInlineComment(line)
	if !strings.Contains(body, `\"#\" marker`) {
		t.Fatalf("expected escaped hash to stay in body, got %q", body)
	}
	if comment != "# trailing" {
		t.Fatalf("expected trailing comment, got %q", comment)
	}
}

func TestParseSectionNameRejectsMalformedHeaders(t *testing.T) {
	if _, ok := parseSectionName("[]"); ok {
		t.Fatalf("expected empty section header to be rejected")
	}
	if _, ok := parseSectionName("[   ]"); ok {
		t.Fatalf("expected blank section name to be rejected")
	}
	if name, ok := parseSectionName("[THEME]\r"); !ok || name != "theme" {
		t.Fatalf("expected normalized theme section name, got (%q, %t)", name, ok)
	}
}

func TestThemeNameLineIndexClampsBounds(t *testing.T) {
	lines := []string{`name = "default"`}
	if idx := themeNameLineIndex(lines, -5, 99); idx != 0 {
		t.Fatalf("expected clamped search bounds to find line at 0, got %d", idx)
	}
}

func TestInsertLineClampsBounds(t *testing.T) {
	lines := []string{"a", "b"}
	got := insertLine(append([]string{}, lines...), -2, "x")
	if strings.Join(got, ",") != "x,a,b" {
		t.Fatalf("expected prepend insert, got %#v", got)
	}
	got = insertLine(append([]string{}, lines...), 9, "y")
	if strings.Join(got, ",") != "a,b,y" {
		t.Fatalf("expected append insert, got %#v", got)
	}
}
