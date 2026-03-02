package app

import "strings"

type modelHighlightAdapter struct {
	model *Model
}

func newModelHighlightAdapter(model *Model) *modelHighlightAdapter {
	return &modelHighlightAdapter{model: model}
}

func (a *modelHighlightAdapter) CurrentUIMode() uiMode {
	if a == nil || a.model == nil {
		return uiModeNormal
	}
	return a.model.mode
}

func (a *modelHighlightAdapter) ViewportHeight() int {
	if a == nil || a.model == nil {
		return 0
	}
	return a.model.viewport.Height()
}

func (a *modelHighlightAdapter) MouseOverInput(y int) bool {
	if a == nil || a.model == nil {
		return false
	}
	return a.model.mouseOverInput(y)
}

func (a *modelHighlightAdapter) BlockIndexByViewportPoint(col, line int) int {
	if a == nil || a.model == nil {
		return -1
	}
	return a.model.blockIndexByViewportPoint(col, line)
}

func (a *modelHighlightAdapter) NotesPanelOpen() bool {
	return a != nil && a.model != nil && a.model.notesPanelOpen
}

func (a *modelHighlightAdapter) NotesPanelVisible() bool {
	return a != nil && a.model != nil && a.model.notesPanelVisible
}

func (a *modelHighlightAdapter) NotesPanelViewportHeight() int {
	if a == nil || a.model == nil {
		return 0
	}
	return a.model.notesPanelViewport.Height()
}

func (a *modelHighlightAdapter) NotePanelBlockIndexByViewportPoint(col, line int) int {
	if a == nil || a.model == nil {
		return -1
	}
	return a.model.notePanelBlockIndexByViewportPoint(col, line)
}

func (a *modelHighlightAdapter) SidebarWidth() int {
	if a == nil || a.model == nil {
		return 0
	}
	return a.model.sidebarWidth()
}

func (a *modelHighlightAdapter) SidebarItemKeyAtRow(row int) string {
	if a == nil || a.model == nil || a.model.sidebar == nil {
		return ""
	}
	entry := a.model.sidebar.ItemAtRow(row)
	if entry == nil {
		return ""
	}
	return strings.TrimSpace(entry.key())
}

func (a *modelHighlightAdapter) SidebarHighlightedKeysBetweenRows(fromRow, toRow int) map[string]struct{} {
	if a == nil || a.model == nil || a.model.sidebar == nil {
		return nil
	}
	return a.model.sidebar.HighlightedKeysBetweenRows(fromRow, toRow)
}
