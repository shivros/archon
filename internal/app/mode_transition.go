package app

type modeTransitionRequest struct {
	toMode      uiMode
	status      string
	focus       *inputFocus
	forceReflow bool
	before      func()
	after       func()
}

func (m *Model) applyModeTransition(req modeTransitionRequest) {
	if m == nil {
		return
	}
	beforeInputLines := m.modeInputLineCount()
	if req.before != nil {
		req.before()
	}
	m.mode = req.toMode
	if req.focus != nil && m.input != nil {
		switch *req.focus {
		case focusChatInput:
			m.input.FocusChatInput()
		default:
			m.input.FocusSidebar()
		}
	}
	if req.after != nil {
		req.after()
	}
	if req.status != "" {
		m.setStatusMessage(req.status)
	}
	afterInputLines := m.modeInputLineCount()
	needsReflow := req.forceReflow || beforeInputLines != afterInputLines
	if needsReflow && m.width > 0 && m.height > 0 {
		m.resize(m.width, m.height)
	}
}
