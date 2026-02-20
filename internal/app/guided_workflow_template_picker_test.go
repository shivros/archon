package app

import (
	"testing"

	"control/internal/guidedworkflows"
)

func TestGuidedWorkflowTemplatePickerSortsAndDedupes(t *testing.T) {
	picker := newGuidedWorkflowTemplatePicker()
	picker.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "zeta", Name: "Zeta"},
		{ID: "alpha", Name: "Alpha"},
		{ID: "ALPHA", Name: "Alpha Duplicate"},
		{ID: "", Name: "Missing ID"},
	}, "")

	options := picker.Options()
	if len(options) != 2 {
		t.Fatalf("expected 2 options after dedupe/filter, got %d", len(options))
	}
	if options[0].id != "alpha" || options[1].id != "zeta" {
		t.Fatalf("unexpected sorted options: %#v", options)
	}
	selected, ok := picker.Selected()
	if !ok || selected.id != "alpha" {
		t.Fatalf("expected alpha selected by default, got %#v", selected)
	}
}

func TestGuidedWorkflowTemplatePickerPreservesSelectionByID(t *testing.T) {
	picker := newGuidedWorkflowTemplatePicker()
	picker.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "alpha", Name: "Alpha"},
		{ID: "beta", Name: "Beta"},
	}, "")
	if !picker.Move(1) {
		t.Fatalf("expected selection move to succeed")
	}
	selected, ok := picker.Selected()
	if !ok || selected.id != "beta" {
		t.Fatalf("expected beta selected after move, got %#v", selected)
	}

	picker.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "gamma", Name: "Gamma"},
		{ID: "beta", Name: "Beta"},
		{ID: "alpha", Name: "Alpha"},
	}, "beta")
	selected, ok = picker.Selected()
	if !ok || selected.id != "beta" {
		t.Fatalf("expected beta selection preserved, got %#v", selected)
	}
}

func TestGuidedWorkflowTemplatePickerLoadingAndError(t *testing.T) {
	picker := newGuidedWorkflowTemplatePicker()
	if !picker.Loading() {
		t.Fatalf("expected picker to start in loading state")
	}
	picker.SetError(assertErr{"template load failed"})
	if picker.Loading() {
		t.Fatalf("expected loading false after error")
	}
	if picker.Error() == "" {
		t.Fatalf("expected non-empty error text")
	}
	picker.BeginLoad()
	if !picker.Loading() {
		t.Fatalf("expected loading true after BeginLoad")
	}
	if picker.Error() != "" {
		t.Fatalf("expected BeginLoad to clear error")
	}
}

func TestGuidedWorkflowTemplatePickerHasSelectionAndClamp(t *testing.T) {
	picker := newGuidedWorkflowTemplatePicker()
	if picker.HasSelection() {
		t.Fatalf("expected no selection while empty")
	}

	picker.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "alpha", Name: "Alpha"},
		{ID: "beta", Name: "Beta"},
	}, "")
	if !picker.HasSelection() {
		t.Fatalf("expected selection after templates load")
	}

	// Force an out-of-range index and verify accessors clamp safely.
	picker.index = 99
	if got := picker.SelectedIndex(); got != 1 {
		t.Fatalf("expected clamped selection index 1, got %d", got)
	}
	selected, ok := picker.Selected()
	if !ok || selected.id != "beta" {
		t.Fatalf("expected clamped selection on beta, got %#v", selected)
	}

	picker.index = -5
	if got := picker.SelectedIndex(); got != 0 {
		t.Fatalf("expected clamped selection index 0, got %d", got)
	}
}

type assertErr struct{ text string }

func (e assertErr) Error() string { return e.text }
