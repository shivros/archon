package app

import (
	"testing"

	"control/internal/types"
)

type alwaysTrueSelectionFocusPolicy struct{}

func (alwaysTrueSelectionFocusPolicy) ShouldExitGuidedWorkflowForSessionSelection(_ uiMode, _ *sidebarItem, _ selectionChangeSource) bool {
	return true
}

func TestDefaultSelectionFocusPolicyExitGuidedWorkflowContract(t *testing.T) {
	policy := DefaultSelectionFocusPolicy()
	sessionItem := &sidebarItem{
		kind:    sidebarSession,
		session: &types.Session{ID: "s1"},
	}
	if policy.ShouldExitGuidedWorkflowForSessionSelection(uiModeNormal, sessionItem, selectionChangeSourceUser) {
		t.Fatalf("expected normal mode to not exit guided workflow")
	}
	if !policy.ShouldExitGuidedWorkflowForSessionSelection(uiModeGuidedWorkflow, sessionItem, selectionChangeSourceUser) {
		t.Fatalf("expected guided workflow mode with session selection to exit")
	}
	if !policy.ShouldExitGuidedWorkflowForSessionSelection(uiModeGuidedWorkflow, sessionItem, selectionChangeSourceSystem) {
		t.Fatalf("expected default source-agnostic behavior for guided workflow session selection")
	}
	workflowItem := &sidebarItem{
		kind:       sidebarWorkflow,
		workflowID: "gwf-1",
	}
	if policy.ShouldExitGuidedWorkflowForSessionSelection(uiModeGuidedWorkflow, workflowItem, selectionChangeSourceUser) {
		t.Fatalf("expected non-session selection to keep guided workflow mode")
	}
}

func TestWithSelectionFocusPolicyOptionWiresCustomPolicy(t *testing.T) {
	custom := alwaysTrueSelectionFocusPolicy{}
	m := NewModel(nil, WithSelectionFocusPolicy(custom))
	if _, ok := m.selectionFocusPolicy.(alwaysTrueSelectionFocusPolicy); !ok {
		t.Fatalf("expected custom selection focus policy to be installed, got %T", m.selectionFocusPolicy)
	}
}

func TestWithSelectionFocusPolicyNilOptionFallsBackToDefault(t *testing.T) {
	m := NewModel(nil, WithSelectionFocusPolicy(nil))
	if m.selectionFocusPolicy == nil {
		t.Fatalf("expected default selection focus policy when nil option is provided")
	}
	if _, ok := m.selectionFocusPolicy.(defaultSelectionFocusPolicy); !ok {
		t.Fatalf("expected default selection focus policy type, got %T", m.selectionFocusPolicy)
	}
}

func TestSelectionFocusPolicyOrDefaultHandlesNilModel(t *testing.T) {
	var m *Model
	if m.selectionFocusPolicyOrDefault() == nil {
		t.Fatalf("expected non-nil default policy for nil model")
	}
}

func TestSelectionFocusPolicyOrDefaultHandlesNilPolicyField(t *testing.T) {
	m := NewModel(nil)
	m.selectionFocusPolicy = nil
	if m.selectionFocusPolicyOrDefault() == nil {
		t.Fatalf("expected non-nil default policy when policy field is nil")
	}
}

func TestWithSelectionFocusPolicyOptionHandlesNilModel(t *testing.T) {
	opt := WithSelectionFocusPolicy(alwaysTrueSelectionFocusPolicy{})
	opt(nil)
}
