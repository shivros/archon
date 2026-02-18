package app

type activeInputContext struct {
	input  *TextInput
	footer InputFooterProvider
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
		return activeInputContext{input: m.noteInput}, true
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
