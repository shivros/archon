package app

import "math"

const splitLayoutVersion = 1

type SplitPreference struct {
	Columns int
	Ratio   float64
}

func defaultSidebarWidth(totalWidth int) int {
	listWidth := clamp(totalWidth/3, minListWidth, maxListWidth)
	if totalWidth-listWidth-1 < minViewportWidth {
		listWidth = max(minListWidth, totalWidth/2)
	}
	return listWidth
}

func resolveSidebarWidth(totalWidth int, collapsed bool, pref *SplitPreference) int {
	if collapsed {
		return 0
	}
	listWidth := defaultSidebarWidth(totalWidth)
	if pref != nil {
		listWidth = preferredSplitColumns(totalWidth, minListWidth, maxListWidth, listWidth, pref)
	}
	return clampSidebarWidthForTerminal(totalWidth, listWidth)
}

func resolveSidePanelWidth(viewportWidth int, splitPref *SplitPreference, panelWidth int) int {
	defaultWidth := clamp(viewportWidth/3, sidePanelMinWidth, sidePanelMaxWidth)
	size := panelWidth
	if size <= 0 {
		size = preferredSplitColumns(viewportWidth, sidePanelMinWidth, sidePanelMaxWidth, defaultWidth, splitPref)
	}
	return clampSidePanelWidthForViewport(viewportWidth, size)
}

func preferredSplitColumns(total, minWidth, maxWidth, fallback int, pref *SplitPreference) int {
	if pref == nil {
		return clamp(fallback, minWidth, maxWidth)
	}
	if pref.Columns > 0 {
		return clamp(pref.Columns, minWidth, maxWidth)
	}
	if pref.Ratio > 0 && pref.Ratio < 1 {
		columns := int(math.Round(pref.Ratio * float64(total)))
		return clamp(columns, minWidth, maxWidth)
	}
	return clamp(fallback, minWidth, maxWidth)
}

func clampSidebarWidthForTerminal(totalWidth, width int) int {
	if totalWidth <= 0 {
		return 0
	}
	width = clamp(width, minListWidth, maxListWidth)
	if totalWidth-width-1 < minViewportWidth {
		width = max(minListWidth, totalWidth/2)
	}
	if width >= totalWidth {
		width = max(0, totalWidth-1)
	}
	return max(0, width)
}

func clampSidePanelWidthForViewport(viewportWidth, width int) int {
	if viewportWidth <= 0 {
		return 0
	}
	width = clamp(width, sidePanelMinWidth, sidePanelMaxWidth)
	maxAllowed := viewportWidth - minViewportWidth - 1
	if maxAllowed >= sidePanelMinWidth && width > maxAllowed {
		width = maxAllowed
	}
	if width >= viewportWidth {
		width = max(0, viewportWidth-1)
	}
	return max(0, width)
}

func captureSplitPreference(totalWidth, columns int, current *SplitPreference) *SplitPreference {
	if totalWidth <= 0 || columns <= 0 {
		return cloneSplitPreference(current)
	}
	next := &SplitPreference{
		Columns: columns,
		Ratio:   float64(columns) / float64(totalWidth),
	}
	if next.Ratio <= 0 || next.Ratio >= 1 {
		next.Ratio = 0
	}
	return next
}

func cloneSplitPreference(pref *SplitPreference) *SplitPreference {
	if pref == nil {
		return nil
	}
	clone := *pref
	return &clone
}

func splitPreferenceEqual(left, right *SplitPreference) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Columns == right.Columns && left.Ratio == right.Ratio
}

func sanitizeSplitPreference(pref *SplitPreference) *SplitPreference {
	if pref == nil {
		return nil
	}
	next := cloneSplitPreference(pref)
	if next.Columns < 0 {
		next.Columns = 0
	}
	if next.Ratio <= 0 || next.Ratio >= 1 {
		next.Ratio = 0
	}
	if next.Columns == 0 && next.Ratio == 0 {
		return nil
	}
	return next
}
