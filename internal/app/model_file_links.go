package app

import (
	tea "charm.land/bubbletea/v2"
)

func (m *Model) openTranscriptFileLinkByViewportPosition(col, line int) (bool, tea.Cmd) {
	if col < 0 || line < 0 || len(m.contentBlockSpans) == 0 {
		return false, nil
	}
	absolute := m.viewport.YOffset() + line
	for _, span := range m.contentBlockSpans {
		for _, hit := range span.LinkHits {
			if hit.Line != absolute {
				continue
			}
			if hit.Start < 0 || hit.End < hit.Start {
				continue
			}
			if col < hit.Start || col > hit.End {
				continue
			}
			return true, m.openFileLinkCmd(hit.Target)
		}
	}
	return false, nil
}

func (m *Model) openNotesPanelFileLinkByViewportPosition(col, line int) (bool, tea.Cmd) {
	if col < 0 || line < 0 || len(m.notesPanelSpans) == 0 {
		return false, nil
	}
	absolute := m.notesPanelViewport.YOffset() + line
	for _, span := range m.notesPanelSpans {
		for _, hit := range span.LinkHits {
			if hit.Line != absolute {
				continue
			}
			if hit.Start < 0 || hit.End < hit.Start {
				continue
			}
			if col < hit.Start || col > hit.End {
				continue
			}
			return true, m.openFileLinkCmd(hit.Target)
		}
	}
	return false, nil
}

func (m *Model) reduceTranscriptLinkLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart || mouse.Y < 1 || mouse.Y > m.viewport.Height() || m.mouseOverInput(mouse.Y) {
		return false
	}
	col := mouse.X - layout.rightStart
	line := mouse.Y - 1
	handled, cmd := m.openTranscriptFileLinkByViewportPosition(col, line)
	if cmd != nil {
		m.pendingMouseCmd = cmd
	}
	return handled
}
