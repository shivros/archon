package app

import tea "charm.land/bubbletea/v2"

type highlightCoordinator interface {
	Begin(msg tea.MouseMsg, layout mouseLayout) bool
	Update(msg tea.MouseMsg, layout mouseLayout) bool
	End(msg tea.MouseMsg, layout mouseLayout) bool
	Clear() bool
	State() highlightState
}

type highlightSurface interface {
	Context() highlightContext
	PointFromMouse(msg tea.MouseMsg, layout mouseLayout) (highlightPoint, bool)
	RangeFromPoints(anchor, focus highlightPoint) (highlightRange, bool)
}

type highlightContextPolicy interface {
	AllowsContext(mode uiMode, context highlightContext) bool
}

type highlightModeSource interface {
	CurrentUIMode() uiMode
}

type transcriptHighlightPort interface {
	ViewportHeight() int
	MouseOverInput(y int) bool
	BlockIndexByViewportPoint(col, line int) int
}

type notesPanelHighlightPort interface {
	NotesPanelOpen() bool
	NotesPanelVisible() bool
	NotesPanelViewportHeight() int
	NotePanelBlockIndexByViewportPoint(col, line int) int
}

type sidebarHighlightPort interface {
	SidebarWidth() int
	SidebarItemKeyAtRow(row int) string
	SidebarHighlightedKeysBetweenRows(fromRow, toRow int) map[string]struct{}
}
