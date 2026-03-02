package app

import "testing"

func TestModelNotePanelBlockIndexByViewportPointRespectsBodyLines(t *testing.T) {
	m := NewModel(nil)
	m.notesPanelBlocks = []ChatBlock{{ID: "n1"}, {ID: "n2"}}
	m.notesPanelSpans = []renderedBlockSpan{
		{BlockIndex: 0, StartLine: 0, EndLine: 2, CopyLine: 0},
		{BlockIndex: 1, StartLine: 3, EndLine: 4, CopyLine: -1},
	}
	m.notesPanelViewport.SetContent("copy\nbody one\nbody two\nnote two\ntail")

	if idx := m.notePanelBlockIndexByViewportPoint(0, 0); idx != -1 {
		t.Fatalf("expected copy controls line to be excluded, got %d", idx)
	}
	if idx := m.notePanelBlockIndexByViewportPoint(0, 1); idx != 0 {
		t.Fatalf("expected first body line to map to block 0, got %d", idx)
	}
	if idx := m.notePanelBlockIndexByViewportPoint(0, 3); idx != 1 {
		t.Fatalf("expected second span line to map to block 1, got %d", idx)
	}
}

func TestModelIsPanelSpanBodyLineHandlesCopyAndNonCopySpans(t *testing.T) {
	m := NewModel(nil)

	withCopy := renderedBlockSpan{StartLine: 2, EndLine: 5, CopyLine: 3}
	if m.isPanelSpanBodyLine(withCopy, 3) {
		t.Fatalf("expected copy line to be excluded from body")
	}
	if !m.isPanelSpanBodyLine(withCopy, 4) {
		t.Fatalf("expected line after copy line to be included")
	}

	noCopy := renderedBlockSpan{StartLine: 6, EndLine: 8, CopyLine: -1}
	if !m.isPanelSpanBodyLine(noCopy, 6) {
		t.Fatalf("expected start line included when no copy line is present")
	}
	if m.isPanelSpanBodyLine(noCopy, 9) {
		t.Fatalf("expected out-of-range line excluded")
	}
}
