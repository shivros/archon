package app

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRenderStatusLinePrioritizesStatusOverHelp(t *testing.T) {
	width := 12
	help := strings.Repeat("h", 64)
	status := "ready"

	line := renderStatusLine(width, help, status)
	if !strings.HasSuffix(line, status) {
		t.Fatalf("expected status suffix %q in rendered line %q", status, line)
	}
	if got := lipgloss.Width(line); got != width {
		t.Fatalf("expected rendered width %d, got %d (%q)", width, got, line)
	}

	start, end, ok := statusLineStatusBounds(width, help, status)
	if !ok {
		t.Fatalf("expected status bounds to remain visible")
	}
	if got := end - start + 1; got != lipgloss.Width(status) {
		t.Fatalf("expected status bounds width %d, got %d", lipgloss.Width(status), got)
	}
}

func TestRenderStatusLineTruncatesStatusOnlyAsLastResort(t *testing.T) {
	width := 5
	help := "hotkeys"
	status := "sending"

	line := renderStatusLine(width, help, status)
	want := truncateToWidth(status, width)
	if line != want {
		t.Fatalf("expected status-only render %q, got %q", want, line)
	}

	start, end, ok := statusLineStatusBounds(width, help, status)
	if !ok {
		t.Fatalf("expected status bounds")
	}
	if start != 0 || end != width-1 {
		t.Fatalf("expected status bounds [0,%d], got [%d,%d]", width-1, start, end)
	}
}

func TestRenderStatusLineWithoutStatusUsesHelp(t *testing.T) {
	width := 6
	help := "help text"

	line := renderStatusLine(width, help, "")
	want := truncateToWidth(help, width)
	if line != want {
		t.Fatalf("expected help-only render %q, got %q", want, line)
	}

	if _, _, ok := statusLineStatusBounds(width, help, ""); ok {
		t.Fatalf("expected no status bounds when status is empty")
	}
}
