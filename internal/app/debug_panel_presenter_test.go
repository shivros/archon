package app

import (
	"strings"
	"testing"
)

func TestDefaultDebugPanelPresenterTruncatesAndAddsToggleControl(t *testing.T) {
	presenter := NewDefaultDebugPanelPresenter(DefaultDebugPanelDisplayPolicy())
	entries := []DebugStreamEntry{{ID: "debug-1", Display: "l1\nl2\nl3\nl4\nl5\nl6", Raw: "l1\nl2\nl3\nl4\nl5\nl6"}}
	presentation := presenter.Present(entries, 60, DebugPanelPresentationState{ExpandedByID: map[string]bool{}})
	if len(presentation.Blocks) != 1 {
		t.Fatalf("expected one block, got %d", len(presentation.Blocks))
	}
	if !strings.Contains(presentation.Blocks[0].Text, "truncated") {
		t.Fatalf("expected truncated preview text, got %q", presentation.Blocks[0].Text)
	}
	meta := presentation.MetaByID["debug-1"]
	if len(meta.Controls) != 2 {
		t.Fatalf("expected copy and toggle controls, got %#v", meta.Controls)
	}
}

func TestDefaultDebugPanelPresenterExpandedUsesFullPayload(t *testing.T) {
	presenter := NewDefaultDebugPanelPresenter(DefaultDebugPanelDisplayPolicy())
	payload := "l1\nl2\nl3\nl4\nl5\nl6"
	entries := []DebugStreamEntry{{ID: "debug-1", Display: payload, Raw: payload}}
	presentation := presenter.Present(entries, 60, DebugPanelPresentationState{ExpandedByID: map[string]bool{"debug-1": true}})
	if presentation.Blocks[0].Text != payload {
		t.Fatalf("expected expanded block to include full payload, got %q", presentation.Blocks[0].Text)
	}
	meta := presentation.MetaByID["debug-1"]
	if len(meta.Controls) < 2 || meta.Controls[1].Label != "[Collapse]" {
		t.Fatalf("expected collapse toggle in expanded state, got %#v", meta.Controls)
	}
}

func TestDefaultDebugPanelPresenterNormalizesPolicyAndHandlesFallbacks(t *testing.T) {
	presenter := NewDefaultDebugPanelPresenter(DebugPanelDisplayPolicy{PreviewMaxLines: 0, TruncationHint: "", EmptyPayloadLabel: ""})
	entries := []DebugStreamEntry{{ID: "", Display: "", Raw: "raw fallback"}}
	presentation := presenter.Present(entries, 0, DebugPanelPresentationState{ExpandedByID: map[string]bool{}})
	if len(presentation.Blocks) != 1 {
		t.Fatalf("expected one block, got %d", len(presentation.Blocks))
	}
	if presentation.Blocks[0].ID == "" || !strings.HasPrefix(presentation.Blocks[0].ID, "debug-entry-") {
		t.Fatalf("expected synthesized debug block id, got %q", presentation.Blocks[0].ID)
	}
	if text := presentation.CopyTextByID[presentation.Blocks[0].ID]; text != "raw fallback" {
		t.Fatalf("expected raw fallback copy text, got %q", text)
	}
}

func TestParseDebugTimestampCoversInvalidInput(t *testing.T) {
	if parsed := parseDebugTimestamp("not-a-time"); !parsed.IsZero() {
		t.Fatalf("expected invalid timestamp to return zero, got %v", parsed)
	}
	if parsed := parseDebugTimestamp("2026-02-25T15:30:00.123456789Z"); parsed.IsZero() {
		t.Fatalf("expected valid timestamp to parse")
	}
}
