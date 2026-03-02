package app

type defaultHighlightContextPolicy struct{}

func NewDefaultHighlightContextPolicy() highlightContextPolicy {
	return defaultHighlightContextPolicy{}
}

func (defaultHighlightContextPolicy) AllowsContext(mode uiMode, context highlightContext) bool {
	switch context {
	case highlightContextSidebar:
		return true
	case highlightContextChatTranscript:
		return mode == uiModeNormal || mode == uiModeCompose
	case highlightContextMainNotes:
		return mode == uiModeNotes || mode == uiModeAddNote
	case highlightContextSideNotesPanel:
		return true
	default:
		return false
	}
}
