package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDefaultPickerPasteNormalizerSanitizesWhitespaceAndEscapes(t *testing.T) {
	normalizer := defaultPickerPasteNormalizer{}
	got := normalizer.NormalizePickerPaste("  \x1b[31mclaude\x1b[0m \r\n\tsonnet  ")
	if got != "claude sonnet" {
		t.Fatalf("expected normalized paste text, got %q", got)
	}
}

func TestDefaultPickerPasteNormalizerTruncatesAtRuneLimit(t *testing.T) {
	normalizer := defaultPickerPasteNormalizer{}
	got := normalizer.NormalizePickerPaste(strings.Repeat("a", maxPickerPasteRunes+32))
	if len([]rune(got)) != maxPickerPasteRunes {
		t.Fatalf("expected %d runes, got %d", maxPickerPasteRunes, len([]rune(got)))
	}
}

func TestPickerTypeAheadControllerPasteAppendsNormalizedQuery(t *testing.T) {
	picker := NewSelectPicker(40, 5)
	picker.SetOptions([]selectOption{
		{id: "claude", label: "Claude"},
		{id: "codex", label: "Codex"},
	})
	controller := newPickerTypeAheadController(nil, defaultPickerPasteNormalizer{})
	if !controller.Handle(tea.PasteMsg{Content: "\x1b[32mcld\x1b[0m\n"}, picker) {
		t.Fatalf("expected paste to update picker query")
	}
	if got := picker.Query(); got != "cld" {
		t.Fatalf("expected query to be normalized, got %q", got)
	}
	if got := picker.SelectedID(); got != "claude" {
		t.Fatalf("expected filtered selection to be claude, got %q", got)
	}
}

type staticPickerPasteNormalizer struct {
	value string
}

func (n staticPickerPasteNormalizer) NormalizePickerPaste(string) string {
	return n.value
}

func TestWithPickerPasteNormalizerOverridesDefault(t *testing.T) {
	m := NewModel(nil, WithPickerPasteNormalizer(staticPickerPasteNormalizer{value: "cod"}))
	picker := NewSelectPicker(40, 5)
	picker.SetOptions([]selectOption{
		{id: "claude", label: "Claude"},
		{id: "codex", label: "Codex"},
	})
	if !m.applyPickerPaste(tea.PasteMsg{Content: "ignored"}, picker) {
		t.Fatalf("expected paste to be handled")
	}
	if got := picker.Query(); got != "cod" {
		t.Fatalf("expected injected normalizer output, got %q", got)
	}
}
