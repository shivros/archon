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
	if got := picker.SelectedIndex(); got != 0 {
		t.Fatalf("expected initial selected index 0, got %d", got)
	}
	if !picker.Move(1) {
		t.Fatalf("expected selection move to succeed")
	}
	if got := picker.SelectedIndex(); got != 1 {
		t.Fatalf("expected selected index 1 after move, got %d", got)
	}
}

func TestGuidedWorkflowTemplatePickerQueryFiltering(t *testing.T) {
	picker := newGuidedWorkflowTemplatePicker()
	picker.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "solid_phase_delivery", Name: "SOLID Phase Delivery"},
		{ID: "bug_triage", Name: "Bug Triage"},
	}, "")

	if !picker.AppendQuery("bug") {
		t.Fatalf("expected query append to filter options")
	}
	selected, ok := picker.Selected()
	if !ok || selected.id != "bug_triage" {
		t.Fatalf("expected bug_triage selected after filtering, got %#v", selected)
	}
	if !picker.BackspaceQuery() {
		t.Fatalf("expected backspace query to update picker state")
	}
	if !picker.ClearQuery() {
		t.Fatalf("expected clear query to reset picker state")
	}

	picker.BeginLoad()
	if picker.AppendQuery("solid") {
		t.Fatalf("expected query append to be ignored while loading")
	}
}

func TestGuidedWorkflowTemplatePickerSetSizeAndClickSelection(t *testing.T) {
	picker := newGuidedWorkflowTemplatePicker()
	picker.SetSize(0, 0)
	if picker.picker == nil {
		t.Fatalf("expected internal picker")
	}
	if picker.picker.width != minViewportWidth {
		t.Fatalf("expected width fallback %d, got %d", minViewportWidth, picker.picker.width)
	}
	if picker.picker.height != 8 {
		t.Fatalf("expected height fallback 8, got %d", picker.picker.height)
	}

	picker.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "alpha", Name: "Alpha"},
		{ID: "beta", Name: "Beta"},
	}, "")

	if picker.HandleClick(-1) {
		t.Fatalf("expected negative row click to be ignored")
	}
	if picker.HandleClick(0) {
		t.Fatalf("expected query row click to be ignored")
	}
	if !picker.HandleClick(2) {
		t.Fatalf("expected second option click to select")
	}
	selected, ok := picker.Selected()
	if !ok || selected.id != "beta" {
		t.Fatalf("expected beta selected after click, got %#v", selected)
	}

	picker.BeginLoad()
	if picker.HandleClick(1) {
		t.Fatalf("expected clicks to be ignored while loading")
	}
}

func TestGuidedWorkflowTemplateLabelFormatting(t *testing.T) {
	cases := []struct {
		name   string
		option guidedWorkflowTemplateOption
		want   string
	}{
		{
			name:   "both empty",
			option: guidedWorkflowTemplateOption{},
			want:   "",
		},
		{
			name: "id only",
			option: guidedWorkflowTemplateOption{
				id: "template-id",
			},
			want: "template-id",
		},
		{
			name: "name only",
			option: guidedWorkflowTemplateOption{
				name: "Template Name",
			},
			want: "Template Name",
		},
		{
			name: "name equals id",
			option: guidedWorkflowTemplateOption{
				id:   "Template Name",
				name: "Template Name",
			},
			want: "Template Name",
		},
		{
			name: "name and id",
			option: guidedWorkflowTemplateOption{
				id:   "template-id",
				name: "Template Name",
			},
			want: "Template Name (template-id)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := guidedWorkflowTemplateLabel(tc.option); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestGuidedWorkflowTemplatePickerNilGuards(t *testing.T) {
	var picker *guidedWorkflowTemplatePicker
	if picker.ensurePicker() != nil {
		t.Fatalf("expected nil ensurePicker result")
	}
	if picker.Query() != "" {
		t.Fatalf("expected nil picker query to be empty")
	}
	if picker.AppendQuery("x") {
		t.Fatalf("expected nil picker append query to be ignored")
	}
	if picker.BackspaceQuery() {
		t.Fatalf("expected nil picker backspace query to be ignored")
	}
	if picker.ClearQuery() {
		t.Fatalf("expected nil picker clear query to be ignored")
	}
	if picker.HandleClick(1) {
		t.Fatalf("expected nil picker click to be ignored")
	}
	if picker.View() != "" {
		t.Fatalf("expected nil picker view to be empty")
	}
	picker.SetSize(10, 2)
	picker.SetTemplates(nil, "")
	picker.BeginLoad()
	picker.SetError(assertErr{"ignored"})
	picker.Reset()
}

type assertErr struct{ text string }

func (e assertErr) Error() string { return e.text }
