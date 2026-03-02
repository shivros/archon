package app

import tea "charm.land/bubbletea/v2"

func (m *Model) reduceHighlightMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	m.rebindHighlightAdapter()
	if m == nil || m.highlight == nil {
		return false
	}
	mouse := msg.Mouse()
	switch msg.(type) {
	case tea.MouseClickMsg:
		if mouse.Button != tea.MouseLeft {
			return false
		}
		if m.highlight.Begin(msg, layout) {
			m.syncHighlightPresentation()
			return false
		}
		if m.highlight.Clear() {
			m.syncHighlightPresentation()
		}
		return false
	case tea.MouseMotionMsg:
		if !m.highlight.Update(msg, layout) {
			return false
		}
		m.syncHighlightPresentation()
		return true
	case tea.MouseReleaseMsg:
		if mouse.Button != tea.MouseLeft {
			return false
		}
		committed := m.highlight.End(msg, layout)
		m.syncHighlightPresentation()
		return committed
	default:
		return false
	}
}

func (m *Model) syncHighlightPresentation() {
	if m == nil {
		return
	}
	state := highlightState{}
	if m.highlight != nil {
		state = m.highlight.State()
	}
	if m.sidebar != nil {
		if state.Range.Context == highlightContextSidebar && len(state.Range.SidebarKeys) > 0 {
			m.sidebar.SetHighlightedKeys(state.Range.SidebarKeys)
		} else {
			m.sidebar.SetHighlightedKeys(nil)
		}
	}
	m.renderViewport()
	if m.notesPanelOpen {
		m.renderNotesPanel()
	}
}

func (m *Model) highlightedMainBlockRange() (int, int, bool) {
	m.rebindHighlightAdapter()
	if m == nil || m.highlight == nil {
		return -1, -1, false
	}
	state := m.highlight.State()
	if !state.Range.HasSelection {
		return -1, -1, false
	}
	if state.Range.Context != highlightContextChatTranscript && state.Range.Context != highlightContextMainNotes {
		return -1, -1, false
	}
	if state.Range.BlockStart < 0 || state.Range.BlockEnd < state.Range.BlockStart {
		return -1, -1, false
	}
	return state.Range.BlockStart, state.Range.BlockEnd, true
}

func (m *Model) highlightedNotesPanelBlockRange() (int, int, bool) {
	m.rebindHighlightAdapter()
	if m == nil || m.highlight == nil {
		return -1, -1, false
	}
	state := m.highlight.State()
	if !state.Range.HasSelection || state.Range.Context != highlightContextSideNotesPanel {
		return -1, -1, false
	}
	if state.Range.BlockStart < 0 || state.Range.BlockEnd < state.Range.BlockStart {
		return -1, -1, false
	}
	return state.Range.BlockStart, state.Range.BlockEnd, true
}

func (m *Model) rebindHighlightAdapter() {
	if m == nil || m.highlightAdapter == nil {
		return
	}
	m.highlightAdapter.model = m
}
