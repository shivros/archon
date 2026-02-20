package app

type activeInputContext struct {
	input  *TextInput
	footer InputFooterProvider
	frame  InputPanelFrame
}

func (c activeInputContext) panel() InputPanel {
	return InputPanel{
		Input:  c.input,
		Footer: c.footer,
		Frame:  c.frame,
	}
}

func (m *Model) activeInputContext() (activeInputContext, bool) {
	if m == nil {
		return activeInputContext{}, false
	}
	switch m.mode {
	case uiModeCompose:
		if m.chatInput == nil {
			return activeInputContext{}, false
		}
		return activeInputContext{
			input:  m.chatInput,
			footer: InputFooterFunc(m.composeControlsLine),
			frame:  m.inputFrame(InputFrameTargetCompose),
		}, true
	case uiModeApprovalResponse:
		if m.approvalInput == nil {
			return activeInputContext{}, false
		}
		return activeInputContext{
			input:  m.approvalInput,
			footer: InputFooterFunc(m.approvalResponseFooter),
		}, true
	case uiModeAddNote:
		if m.noteInput == nil {
			return activeInputContext{}, false
		}
		return activeInputContext{
			input: m.noteInput,
			frame: m.inputFrame(InputFrameTargetAddNote),
		}, true
	case uiModeSearch:
		if m.searchInput == nil {
			return activeInputContext{}, false
		}
		return activeInputContext{input: m.searchInput}, true
	case uiModeRecents:
		if m.recentsReplySessionID == "" || m.recentsReplyInput == nil {
			return activeInputContext{}, false
		}
		return activeInputContext{
			input:  m.recentsReplyInput,
			footer: InputFooterFunc(m.recentsReplyFooter),
		}, true
	default:
		return activeInputContext{}, false
	}
}

func (m *Model) activeInput() *TextInput {
	ctx, ok := m.activeInputContext()
	if !ok {
		return nil
	}
	return ctx.input
}

func (m *Model) activeInputPanel() (InputPanel, bool) {
	ctx, ok := m.activeInputContext()
	if !ok {
		return InputPanel{}, false
	}
	return ctx.panel(), true
}

func (m *Model) activeInputPanelLayout() (InputPanelLayout, bool) {
	panel, ok := m.activeInputPanel()
	if !ok {
		return InputPanelLayout{}, false
	}
	return BuildInputPanelLayout(panel), true
}
