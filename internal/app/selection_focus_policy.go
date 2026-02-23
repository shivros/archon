package app

// SelectionFocusPolicy decides which pane owns focus after a sidebar selection
// changes. Implementations must be deterministic and side-effect free so
// selection transition services can evaluate policy safely during event handling.
type SelectionFocusPolicy interface {
	// ShouldExitGuidedWorkflowForSessionSelection returns true when guided
	// workflow mode should be exited before applying session selection behavior.
	// Implementations should return false when mode is not guided workflow.
	ShouldExitGuidedWorkflowForSessionSelection(mode uiMode, item *sidebarItem, source selectionChangeSource) bool
}

type defaultSelectionFocusPolicy struct{}

func DefaultSelectionFocusPolicy() SelectionFocusPolicy {
	return defaultSelectionFocusPolicy{}
}

func (defaultSelectionFocusPolicy) ShouldExitGuidedWorkflowForSessionSelection(mode uiMode, item *sidebarItem, _ selectionChangeSource) bool {
	if mode != uiModeGuidedWorkflow {
		return false
	}
	// Default behavior is source-agnostic: explicit session selection always
	// moves ownership from workflow timeline to session transcript.
	return item != nil && item.isSession()
}

func WithSelectionFocusPolicy(policy SelectionFocusPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.selectionFocusPolicy = DefaultSelectionFocusPolicy()
			return
		}
		m.selectionFocusPolicy = policy
	}
}

func (m *Model) selectionFocusPolicyOrDefault() SelectionFocusPolicy {
	if m == nil || m.selectionFocusPolicy == nil {
		return DefaultSelectionFocusPolicy()
	}
	return m.selectionFocusPolicy
}
