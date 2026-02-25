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
	GotoTop() bool
	GotoBottom() bool
}

type debugPanelMetrics interface {
	Height() int
	YOffset() int
}

type debugPanelView interface {
	debugPanelRenderer
	debugPanelNavigator
	debugPanelMetrics
}

type debugStreamConsumer interface {
	SetStream(ch <-chan types.DebugEvent, cancel func())
	Reset()
	Close()
	HasStream() bool
	ConsumeTick() (lines []string, changed bool, closed bool)
}

type debugStreamSnapshot interface {
	Entries() []DebugStreamEntry
}

type DebugPanelPresentationState struct {
	ExpandedByID map[string]bool
}

type DebugPanelPresentation struct {
	Blocks       []ChatBlock
	MetaByID     map[string]ChatBlockMetaPresentation
	CopyTextByID map[string]string
}

type debugPanelPresenter interface {
	Present(entries []DebugStreamEntry, width int, state DebugPanelPresentationState) DebugPanelPresentation
}

type debugPanelBlocksRenderer interface {
	Render(blocks []ChatBlock, width int, metaByID map[string]ChatBlockMetaPresentation) (rendered string, spans []renderedBlockSpan)
}

type debugPanelControlHit struct {
	BlockID   string
	ControlID ChatMetaControlID
}

type debugPanelInteractionService interface {
	HitTest(spans []renderedBlockSpan, yOffset int, col, line int) (debugPanelControlHit, bool)
}
