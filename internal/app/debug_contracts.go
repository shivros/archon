package app

import "control/internal/types"

type debugPanelView interface {
	Resize(width, height int)
	SetContent(content string)
	View() (string, int)
}

type debugStreamViewModel interface {
	SetStream(ch <-chan types.DebugEvent, cancel func())
	Reset()
	Lines() []string
	HasStream() bool
	Content() string
	ConsumeTick() (lines []string, changed bool, closed bool)
}
