package app

import "testing"

func TestExtractMarkdownInlineLinks(t *testing.T) {
	links := extractMarkdownInlineLinks("See [file](/tmp/main.go:42) and [doc](file:///tmp/readme.md#L12)")
	if len(links) != 2 {
		t.Fatalf("expected two links, got %#v", links)
	}
	if links[0].Label != "file" || links[0].Target != "/tmp/main.go:42" {
		t.Fatalf("unexpected first link: %#v", links[0])
	}
	if links[1].Label != "doc" || links[1].Target != "file:///tmp/readme.md#L12" {
		t.Fatalf("unexpected second link: %#v", links[1])
	}
}

func TestBuildRenderedLinkHitsFindsBubbleLinkCoordinates(t *testing.T) {
	plainLines := []string{
		"  box border",
		"  open /tmp/main.go now",
		"  trailing",
	}
	hits := buildRenderedLinkHits("open [/tmp/main.go](/tmp/main.go)", plainLines, 10)
	if len(hits) != 1 {
		t.Fatalf("expected one hit, got %#v", hits)
	}
	if hits[0].Line != 11 {
		t.Fatalf("expected hit line 11, got %#v", hits[0])
	}
	if hits[0].Target != "/tmp/main.go" {
		t.Fatalf("unexpected target: %#v", hits[0])
	}
	if hits[0].End < hits[0].Start {
		t.Fatalf("invalid hit range: %#v", hits[0])
	}
}

func TestExtractMarkdownInlineLinksSkipsImagesAndHandlesEscapedParens(t *testing.T) {
	input := "![img](/tmp/x.png) and [link](file:///tmp/(a\\)b).md)"
	links := extractMarkdownInlineLinks(input)
	if len(links) != 1 {
		t.Fatalf("expected one non-image link, got %#v", links)
	}
	if links[0].Label != "link" {
		t.Fatalf("unexpected label: %#v", links[0])
	}
}

func TestBuildRenderedLinkHitsNoMatchReturnsNil(t *testing.T) {
	hits := buildRenderedLinkHits("open [missing](/tmp/main.go)", []string{"different content"}, 2)
	if hits != nil {
		t.Fatalf("expected nil hits on no match, got %#v", hits)
	}
}

func TestBuildRenderedLinkHitsDuplicateLabelProgressesSearch(t *testing.T) {
	lines := []string{
		"open main then main",
	}
	hits := buildRenderedLinkHits("[main](/tmp/a) [main](/tmp/b)", lines, 0)
	if len(hits) != 2 {
		t.Fatalf("expected two hits, got %#v", hits)
	}
	if hits[0].Start == hits[1].Start {
		t.Fatalf("expected second duplicate label to map to later position: %#v", hits)
	}
}

func TestFindRenderedLabelPositionFallbacks(t *testing.T) {
	lines := []string{"alpha beta", "gamma delta"}
	if _, _, ok := findRenderedLabelPosition(lines, "", 0, 0); ok {
		t.Fatalf("expected empty label miss")
	}
	line, col, ok := findRenderedLabelPosition(lines, "delta", -1, 0)
	if !ok || line != 1 || col < 0 {
		t.Fatalf("expected fallback search success, got line=%d col=%d ok=%v", line, col, ok)
	}
}
