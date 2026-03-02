package app

import tea "charm.land/bubbletea/v2"

type transcriptHighlightSurface struct {
	port transcriptHighlightPort
}

func NewTranscriptHighlightSurface(port transcriptHighlightPort) highlightSurface {
	return transcriptHighlightSurface{port: port}
}

func (transcriptHighlightSurface) Context() highlightContext {
	return highlightContextChatTranscript
}

func (s transcriptHighlightSurface) PointFromMouse(msg tea.MouseMsg, layout mouseLayout) (highlightPoint, bool) {
	if s.port == nil {
		return highlightPoint{}, false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart {
		return highlightPoint{}, false
	}
	if layout.panelVisible && layout.panelWidth > 0 && mouse.X >= layout.panelStart {
		return highlightPoint{}, false
	}
	if mouse.Y < 1 || mouse.Y > s.port.ViewportHeight() || s.port.MouseOverInput(mouse.Y) {
		return highlightPoint{}, false
	}
	index := s.port.BlockIndexByViewportPoint(mouse.X-layout.rightStart, mouse.Y-1)
	if index < 0 {
		return highlightPoint{}, false
	}
	return highlightPoint{BlockIndex: index}, true
}

func (transcriptHighlightSurface) RangeFromPoints(anchor, focus highlightPoint) (highlightRange, bool) {
	if anchor.BlockIndex < 0 || focus.BlockIndex < 0 {
		return highlightRange{}, false
	}
	start := anchor.BlockIndex
	end := focus.BlockIndex
	if start > end {
		start, end = end, start
	}
	return highlightRange{BlockStart: start, BlockEnd: end}, true
}

type mainNotesHighlightSurface struct {
	transcriptHighlightSurface
}

func NewMainNotesHighlightSurface(port transcriptHighlightPort) highlightSurface {
	return mainNotesHighlightSurface{transcriptHighlightSurface{port: port}}
}

func (mainNotesHighlightSurface) Context() highlightContext {
	return highlightContextMainNotes
}

type notesPanelHighlightSurface struct {
	port notesPanelHighlightPort
}

func NewNotesPanelHighlightSurface(port notesPanelHighlightPort) highlightSurface {
	return notesPanelHighlightSurface{port: port}
}

func (notesPanelHighlightSurface) Context() highlightContext {
	return highlightContextSideNotesPanel
}

func (s notesPanelHighlightSurface) PointFromMouse(msg tea.MouseMsg, layout mouseLayout) (highlightPoint, bool) {
	if s.port == nil || !s.port.NotesPanelOpen() || !s.port.NotesPanelVisible() || !layout.panelVisible || layout.panelWidth <= 0 {
		return highlightPoint{}, false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.panelStart || mouse.X >= layout.panelStart+layout.panelWidth {
		return highlightPoint{}, false
	}
	if mouse.Y < 1 || mouse.Y > s.port.NotesPanelViewportHeight() {
		return highlightPoint{}, false
	}
	index := s.port.NotePanelBlockIndexByViewportPoint(mouse.X-layout.panelStart, mouse.Y-1)
	if index < 0 {
		return highlightPoint{}, false
	}
	return highlightPoint{BlockIndex: index}, true
}

func (notesPanelHighlightSurface) RangeFromPoints(anchor, focus highlightPoint) (highlightRange, bool) {
	if anchor.BlockIndex < 0 || focus.BlockIndex < 0 {
		return highlightRange{}, false
	}
	start := anchor.BlockIndex
	end := focus.BlockIndex
	if start > end {
		start, end = end, start
	}
	return highlightRange{BlockStart: start, BlockEnd: end}, true
}

type sidebarHighlightSurface struct {
	port sidebarHighlightPort
}

func NewSidebarHighlightSurface(port sidebarHighlightPort) highlightSurface {
	return sidebarHighlightSurface{port: port}
}

func (sidebarHighlightSurface) Context() highlightContext {
	return highlightContextSidebar
}

func (s sidebarHighlightSurface) PointFromMouse(msg tea.MouseMsg, layout mouseLayout) (highlightPoint, bool) {
	if s.port == nil || layout.listWidth <= 0 || s.port.SidebarWidth() <= 0 {
		return highlightPoint{}, false
	}
	mouse := msg.Mouse()
	if mouse.X < 0 || mouse.X >= layout.listWidth {
		return highlightPoint{}, false
	}
	if layout.barWidth > 0 && mouse.X >= layout.barStart {
		return highlightPoint{}, false
	}
	key := s.port.SidebarItemKeyAtRow(mouse.Y)
	if key == "" {
		return highlightPoint{}, false
	}
	return highlightPoint{SidebarRow: mouse.Y, SidebarKey: key}, true
}

func (s sidebarHighlightSurface) RangeFromPoints(anchor, focus highlightPoint) (highlightRange, bool) {
	if s.port == nil {
		return highlightRange{}, false
	}
	keys := s.port.SidebarHighlightedKeysBetweenRows(anchor.SidebarRow, focus.SidebarRow)
	if len(keys) == 0 {
		return highlightRange{}, false
	}
	return highlightRange{BlockStart: -1, BlockEnd: -1, SidebarKeys: keys}, true
}
