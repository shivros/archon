package app

import "control/internal/types"

type debugPanelRenderer interface {
	Resize(width, height int)
	SetContent(content string)
	View() (string, int)
}

type debugPanelNavigator interface {
	ScrollUp(lines int) bool
	ScrollDown(lines int) bool
	PageUp() bool
	PageDown() bool
	ScrollLeft(cols int) bool
	ScrollRight(cols int) bool
	GotoTop() bool
	GotoBottom() bool
}

type debugPanelMetrics interface {
	Height() int
}

type debugPanelView interface {
	debugPanelRenderer
	debugPanelNavigator
	debugPanelMetrics
}

type debugStreamViewModel interface {
	SetStream(ch <-chan types.DebugEvent, cancel func())
	Reset()
	Close()
	Lines() []string
	HasStream() bool
	Content() string
	ConsumeTick() (lines []string, changed bool, closed bool)
}
