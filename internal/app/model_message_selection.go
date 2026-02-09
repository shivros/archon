package app

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) reduceMessageSelectionKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.messageSelectActive {
		return false, nil
	}
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		m.clearMessageSelection()
		return false, nil
	}
	switch msg.String() {
	case "esc", "v":
		m.exitMessageSelection("message selection cleared")
		return true, nil
	case "j", "down":
		m.moveMessageSelection(1)
		return true, nil
	case "k", "up":
		m.moveMessageSelection(-1)
		return true, nil
	case "g":
		m.setMessageSelectionIndex(0)
		return true, nil
	case "G":
		m.setMessageSelectionIndex(len(m.contentBlocks) - 1)
		return true, nil
	case "y":
		m.copySelectedMessage()
		return true, nil
	case "p":
		return true, m.pinSelectedMessage()
	case "enter":
		if m.toggleReasoningByIndex(m.messageSelectIndex) {
			m.setMessageSelectionStatus()
		}
		return true, nil
	case "q":
		return true, tea.Quit
	default:
		// Keep message selection modal so navigation keys don't trigger other actions.
		return true, nil
	}
}

func (m *Model) enterMessageSelection() {
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		return
	}
	if len(m.contentBlocks) == 0 || len(m.contentBlockSpans) == 0 {
		m.setValidationStatus("no messages to select")
		return
	}
	m.messageSelectActive = true
	idx := m.visibleMessageSelectionIndex()
	if idx < 0 {
		idx = len(m.contentBlocks) - 1
	}
	m.messageSelectIndex = idx
	m.focusMessageSelection()
	m.setMessageSelectionStatus()
	m.renderViewport()
}

func (m *Model) exitMessageSelection(status string) {
	m.clearMessageSelection()
	if status != "" {
		m.setStatusMessage(status)
	}
	m.renderViewport()
}

func (m *Model) clearMessageSelection() {
	m.messageSelectActive = false
	m.messageSelectIndex = -1
}

func (m *Model) clampMessageSelection() {
	if !m.messageSelectActive {
		m.messageSelectIndex = -1
		return
	}
	if len(m.contentBlocks) == 0 {
		m.clearMessageSelection()
		return
	}
	if m.messageSelectIndex < 0 {
		m.messageSelectIndex = 0
		return
	}
	if m.messageSelectIndex >= len(m.contentBlocks) {
		m.messageSelectIndex = len(m.contentBlocks) - 1
	}
}

func (m *Model) selectedMessageRenderIndex() int {
	if !m.messageSelectActive {
		return -1
	}
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		return -1
	}
	if len(m.contentBlocks) == 0 {
		return -1
	}
	if m.messageSelectIndex < 0 {
		return 0
	}
	if m.messageSelectIndex >= len(m.contentBlocks) {
		return len(m.contentBlocks) - 1
	}
	return m.messageSelectIndex
}

func (m *Model) visibleMessageSelectionIndex() int {
	if len(m.contentBlockSpans) == 0 {
		return -1
	}
	start := m.viewport.YOffset
	end := start + m.viewport.Height - 1
	for i := len(m.contentBlockSpans) - 1; i >= 0; i-- {
		span := m.contentBlockSpans[i]
		if span.EndLine < start || span.StartLine > end {
			continue
		}
		return span.BlockIndex
	}
	return m.contentBlockSpans[len(m.contentBlockSpans)-1].BlockIndex
}

func (m *Model) isBlockBodyViewportLine(span renderedBlockSpan, absolute int) bool {
	if absolute < span.StartLine || absolute > span.EndLine {
		return false
	}
	start := span.StartLine
	end := span.EndLine
	if span.CopyLine >= 0 {
		start = span.CopyLine + 1
	}
	if span.BlockIndex >= 0 && span.BlockIndex < len(m.contentBlocks) {
		block := m.contentBlocks[span.BlockIndex]
		if block.Role == ChatRoleUser && block.Status != ChatStatusNone {
			end--
		}
	}
	return absolute >= start && absolute <= end
}

func lineContentBounds(line string) (int, int, bool) {
	runes := []rune(line)
	if len(runes) == 0 {
		return 0, 0, false
	}
	start := -1
	end := -1
	for i, r := range runes {
		if !unicode.IsSpace(r) {
			start = i
			break
		}
	}
	if start < 0 {
		return 0, 0, false
	}
	for i := len(runes) - 1; i >= start; i-- {
		if !unicode.IsSpace(runes[i]) {
			end = i
			break
		}
	}
	if end < start {
		return 0, 0, false
	}
	return start, end, true
}

func (m *Model) blockIndexByViewportPoint(col, line int) int {
	if line < 0 || col < 0 || len(m.contentBlocks) == 0 || len(m.contentBlockSpans) == 0 {
		return -1
	}
	absolute := m.viewport.YOffset + line
	lines := m.currentLines()
	if absolute < 0 || absolute >= len(lines) {
		return -1
	}
	contentStart, contentEnd, ok := lineContentBounds(lines[absolute])
	if !ok || col < contentStart || col > contentEnd {
		return -1
	}
	for _, span := range m.contentBlockSpans {
		if !m.isBlockBodyViewportLine(span, absolute) {
			continue
		}
		return span.BlockIndex
	}
	return -1
}

func (m *Model) selectMessageByViewportPoint(col, line int) bool {
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		return false
	}
	index := m.blockIndexByViewportPoint(col, line)
	if index < 0 {
		return false
	}
	m.setMessageSelectionIndex(index)
	return true
}

func (m *Model) moveMessageSelection(delta int) {
	if len(m.contentBlocks) == 0 {
		m.exitMessageSelection("no messages to select")
		return
	}
	if delta == 0 {
		m.setMessageSelectionStatus()
		return
	}
	idx := m.messageSelectIndex
	if idx < 0 {
		idx = 0
	}
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.contentBlocks) {
		idx = len(m.contentBlocks) - 1
	}
	m.setMessageSelectionIndex(idx)
}

func (m *Model) setMessageSelectionIndex(index int) {
	if len(m.contentBlocks) == 0 {
		m.exitMessageSelection("no messages to select")
		return
	}
	if index < 0 {
		index = 0
	}
	if index >= len(m.contentBlocks) {
		index = len(m.contentBlocks) - 1
	}
	m.messageSelectActive = true
	m.messageSelectIndex = index
	m.focusMessageSelection()
	m.setMessageSelectionStatus()
	m.renderViewport()
}

func (m *Model) focusMessageSelection() {
	if len(m.contentBlockSpans) == 0 {
		return
	}
	var selected *renderedBlockSpan
	for i := range m.contentBlockSpans {
		span := &m.contentBlockSpans[i]
		if span.BlockIndex == m.messageSelectIndex {
			selected = span
			break
		}
	}
	if selected == nil {
		return
	}
	if m.viewport.Height <= 0 {
		return
	}
	start := selected.StartLine
	end := selected.EndLine
	visibleStart := m.viewport.YOffset
	visibleEnd := visibleStart + m.viewport.Height - 1
	if start < visibleStart {
		m.viewport.YOffset = start
	}
	if end > visibleEnd {
		m.viewport.YOffset = end - m.viewport.Height + 1
		if m.viewport.YOffset < 0 {
			m.viewport.YOffset = 0
		}
	}
	m.pauseFollow(false)
}

func (m *Model) setMessageSelectionStatus() {
	if len(m.contentBlocks) == 0 || m.messageSelectIndex < 0 || m.messageSelectIndex >= len(m.contentBlocks) {
		return
	}
	role := strings.ToLower(chatRoleLabel(m.contentBlocks[m.messageSelectIndex].Role))
	m.setStatusMessage(fmt.Sprintf("message %d/%d selected (%s) - y copy, esc exit", m.messageSelectIndex+1, len(m.contentBlocks), role))
}

func (m *Model) copySelectedMessage() {
	if m.messageSelectIndex < 0 || m.messageSelectIndex >= len(m.contentBlocks) {
		m.setCopyStatusWarning("no message selected")
		return
	}
	_ = m.copyBlockByIndex(m.messageSelectIndex)
}
