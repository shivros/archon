package app

type SessionProjectionPostProcessInput struct {
	Source    sessionProjectionSource
	SessionID string
	Blocks    []ChatBlock
}

type SessionProjectionPostProcessor interface {
	PostProcessSessionProjection(m *Model, input SessionProjectionPostProcessInput)
}

type workflowTurnFocusProjectionPostProcessor struct{}

func NewDefaultSessionProjectionPostProcessor() SessionProjectionPostProcessor {
	return workflowTurnFocusProjectionPostProcessor{}
}

func (workflowTurnFocusProjectionPostProcessor) PostProcessSessionProjection(m *Model, input SessionProjectionPostProcessInput) {
	if m == nil {
		return
	}
	m.applyPendingWorkflowTurnFocus(input.Source, input.SessionID, input.Blocks)
}

func WithSessionProjectionPostProcessor(processor SessionProjectionPostProcessor) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if processor == nil {
			m.sessionProjectionPostProcessor = NewDefaultSessionProjectionPostProcessor()
			return
		}
		m.sessionProjectionPostProcessor = processor
	}
}

func (m *Model) sessionProjectionPostProcessorOrDefault() SessionProjectionPostProcessor {
	if m == nil || m.sessionProjectionPostProcessor == nil {
		return NewDefaultSessionProjectionPostProcessor()
	}
	return m.sessionProjectionPostProcessor
}
