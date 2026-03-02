package app

import "strings"

func (m *Model) notePanelBlockIndexByViewportPoint(col, line int) int {
	if m == nil || col < 0 || line < 0 || len(m.notesPanelBlocks) == 0 || len(m.notesPanelSpans) == 0 {
		return -1
	}
	absolute := m.notesPanelViewport.YOffset() + line
	lines := strings.Split(m.notesPanelViewport.View(), "\n")
	if absolute < 0 || absolute >= len(lines) {
		return -1
	}
	contentStart, contentEnd, ok := lineContentBounds(lines[absolute])
	if !ok || col < contentStart || col > contentEnd {
		return -1
	}
	for _, span := range m.notesPanelSpans {
		if !m.isPanelSpanBodyLine(span, absolute) {
			continue
		}
		return span.BlockIndex
	}
	return -1
}

func (m *Model) isPanelSpanBodyLine(span renderedBlockSpan, absolute int) bool {
	if absolute < span.StartLine || absolute > span.EndLine {
		return false
	}
	start := span.StartLine
	if span.CopyLine >= 0 {
		start = span.CopyLine + 1
	}
	return absolute >= start && absolute <= span.EndLine
}
