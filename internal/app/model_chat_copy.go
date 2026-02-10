package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) copyBlockByViewportPosition(col, line int) bool {
	if col < 0 || line < 0 || len(m.contentBlocks) == 0 || len(m.contentBlockSpans) == 0 {
		return false
	}
	absolute := m.viewport.YOffset + line
	for _, span := range m.contentBlockSpans {
		if span.CopyLine != absolute {
			continue
		}
		if span.CopyStart < 0 || span.CopyEnd < span.CopyStart {
			continue
		}
		if col < span.CopyStart || col > span.CopyEnd {
			continue
		}
		return m.copyBlockByIndex(span.BlockIndex)
	}
	return false
}

func (m *Model) copyBlockByIndex(index int) bool {
	if index < 0 || index >= len(m.contentBlocks) {
		return false
	}
	text := strings.TrimSpace(m.contentBlocks[index].Text)
	if text == "" {
		m.setCopyStatusWarning("nothing to copy")
		return true
	}
	m.copyWithStatus(text, "message copied")
	return true
}

func (m *Model) pinBlockByViewportPosition(col, line int) (bool, tea.Cmd) {
	if col < 0 || line < 0 || len(m.contentBlocks) == 0 || len(m.contentBlockSpans) == 0 {
		return false, nil
	}
	absolute := m.viewport.YOffset + line
	for _, span := range m.contentBlockSpans {
		if span.PinLine != absolute {
			continue
		}
		if span.PinStart < 0 || span.PinEnd < span.PinStart {
			continue
		}
		if col < span.PinStart || col > span.PinEnd {
			continue
		}
		return true, m.pinBlockByIndex(span.BlockIndex)
	}
	return false, nil
}

func (m *Model) pinBlockByIndex(index int) tea.Cmd {
	if index < 0 || index >= len(m.contentBlocks) {
		m.setValidationStatus("no message selected")
		return nil
	}
	sessionID := strings.TrimSpace(m.selectedSessionID())
	if m.mode == uiModeCompose {
		if composeID := strings.TrimSpace(m.composeSessionID()); composeID != "" {
			sessionID = composeID
		}
	}
	if sessionID == "" {
		m.setValidationStatus("select a session to pin")
		return nil
	}
	block := m.contentBlocks[index]
	snippet := strings.TrimSpace(block.Text)
	if snippet == "" {
		m.setValidationStatus("selected message is empty")
		return nil
	}
	m.setStatusMessage("pinning message")
	return pinSessionNoteCmd(m.sessionAPI, sessionID, block, snippet)
}

func (m *Model) deleteNoteByViewportPosition(col, line int) bool {
	if col < 0 || line < 0 || len(m.contentBlocks) == 0 || len(m.contentBlockSpans) == 0 {
		return false
	}
	absolute := m.viewport.YOffset + line
	for _, span := range m.contentBlockSpans {
		if !isNoteRole(span.Role) {
			continue
		}
		if span.DeleteLine != absolute {
			continue
		}
		if span.DeleteStart < 0 || span.DeleteEnd < span.DeleteStart {
			continue
		}
		if col < span.DeleteStart || col > span.DeleteEnd {
			continue
		}
		return m.confirmDeleteNoteByBlockIndex(span.BlockIndex)
	}
	return false
}

func (m *Model) confirmDeleteNoteByBlockIndex(index int) bool {
	noteID := m.noteIDByBlockIndex(index)
	if noteID == "" {
		m.setValidationStatus("select a note to delete")
		return true
	}
	m.confirmDeleteNote(noteID)
	return true
}

func (m *Model) noteIDByBlockIndex(index int) string {
	if index < 0 || index >= len(m.contentBlocks) {
		return ""
	}
	block := m.contentBlocks[index]
	if !isNoteRole(block.Role) {
		return ""
	}
	return strings.TrimSpace(block.ID)
}

func (m *Model) toggleReasoningByViewportPosition(col, line int) bool {
	if col < 0 || line < 0 || len(m.contentBlocks) == 0 || len(m.contentBlockSpans) == 0 {
		return false
	}
	absolute := m.viewport.YOffset + line
	for _, span := range m.contentBlockSpans {
		if span.Role != ChatRoleReasoning {
			continue
		}
		if span.ToggleLine != absolute {
			continue
		}
		if span.ToggleStart < 0 || span.ToggleEnd < span.ToggleStart {
			continue
		}
		if col < span.ToggleStart || col > span.ToggleEnd {
			continue
		}
		return m.toggleReasoningByIndex(span.BlockIndex)
	}
	return false
}
