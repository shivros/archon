package app

import (
	"strings"

	"github.com/atotto/clipboard"
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
	if err := clipboard.WriteAll(text); err != nil {
		m.setCopyStatusError("copy failed: " + err.Error())
		return true
	}
	m.setCopyStatusInfo("message copied")
	return true
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
