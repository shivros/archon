package app

import (
	"strings"
	"testing"
)

func TestSelectPickerTypeAheadFiltersSelection(t *testing.T) {
	picker := NewSelectPicker(40, 5)
	picker.SetOptions([]selectOption{
		{id: "a", label: "alpha"},
		{id: "b", label: "beta"},
		{id: "g", label: "gamma"},
	})
	if !picker.AppendQuery("gm") {
		t.Fatalf("expected query append to change picker state")
	}
	if picker.SelectedID() != "g" {
		t.Fatalf("expected fuzzy query to select gamma, got %q", picker.SelectedID())
	}
	if !picker.ClearQuery() {
		t.Fatalf("expected query clear to change picker state")
	}
	if picker.SelectedID() != "g" {
		t.Fatalf("expected selection to remain on gamma after clearing filter, got %q", picker.SelectedID())
	}
}

func TestMultiSelectPickerTypeAheadToggle(t *testing.T) {
	picker := NewMultiSelectPicker(40, 5)
	picker.SetOptions([]multiSelectOption{
		{id: "ws-1", label: "frontend"},
		{id: "ws-2", label: "backend"},
		{id: "ws-3", label: "ops"},
	})
	if !picker.AppendQuery("bck") {
		t.Fatalf("expected query append to change picker state")
	}
	if !picker.Toggle() {
		t.Fatalf("expected toggle to select filtered option")
	}
	ids := picker.SelectedIDs()
	if len(ids) != 1 || ids[0] != "ws-2" {
		t.Fatalf("expected backend workspace to be selected, got %#v", ids)
	}
}

func TestFuzzyPickerScoreSubsequence(t *testing.T) {
	if _, ok := fuzzyPickerScore("g5c", "gpt-5.3-codex"); !ok {
		t.Fatalf("expected subsequence query to match")
	}
	if _, ok := fuzzyPickerScore("zzz", "gpt-5.3-codex"); ok {
		t.Fatalf("expected unmatched query to fail")
	}
}

func TestProviderPickerTypeAheadFiltersProviders(t *testing.T) {
	picker := NewProviderPicker(40, 6)
	picker.Enter("")
	if !picker.AppendQuery("cld") {
		t.Fatalf("expected provider query to update picker state")
	}
	if picker.Selected() != "claude" {
		t.Fatalf("expected fuzzy provider selection to resolve to claude, got %q", picker.Selected())
	}
}

func TestSelectPickerViewShowsFilterLine(t *testing.T) {
	picker := NewSelectPicker(40, 5)
	picker.SetOptions([]selectOption{
		{id: "alpha", label: "Alpha"},
		{id: "beta", label: "Beta"},
	})
	if !picker.AppendQuery("alp") {
		t.Fatalf("expected query append to update picker")
	}
	view := picker.View()
	lines := strings.Split(view, "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "/ alp") {
		t.Fatalf("expected first line to include filter query, got %q", view)
	}
}

func TestSelectPickerHandleClickUsesRowsAfterFilterLine(t *testing.T) {
	picker := NewSelectPicker(40, 5)
	picker.SetOptions([]selectOption{
		{id: "alpha", label: "Alpha"},
		{id: "beta", label: "Beta"},
	})
	if !picker.HandleClick(2) {
		t.Fatalf("expected click row to select second list item")
	}
	if picker.SelectedID() != "beta" {
		t.Fatalf("expected beta to be selected, got %q", picker.SelectedID())
	}
}

func TestSelectPickerSanitizesOptionLabelNewlines(t *testing.T) {
	picker := NewSelectPicker(40, 6)
	picker.SetOptions([]selectOption{
		{id: "one", label: "One\n"},
		{id: "two", label: "Two\r\n"},
		{id: "three", label: "Three\tItem"},
	})

	view := picker.View()
	if strings.Contains(view, "One\n\n") || strings.Contains(view, "Two\n\n") {
		t.Fatalf("expected no blank line between picker items, got %q", view)
	}
	if !strings.Contains(view, " One") || !strings.Contains(view, " Two") || !strings.Contains(view, " Three Item") {
		t.Fatalf("expected sanitized single-line labels, got %q", view)
	}
}
