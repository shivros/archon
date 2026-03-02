package app

type highlightContext int

const (
	highlightContextNone highlightContext = iota
	highlightContextSidebar
	highlightContextChatTranscript
	highlightContextMainNotes
	highlightContextSideNotesPanel
)

type highlightPoint struct {
	BlockIndex int
	SidebarRow int
	SidebarKey string
}

type highlightRange struct {
	Context      highlightContext
	BlockStart   int
	BlockEnd     int
	SidebarKeys  map[string]struct{}
	HasSelection bool
}

type highlightState struct {
	Active   bool
	Dragging bool
	Context  highlightContext
	Anchor   highlightPoint
	Focus    highlightPoint
	Range    highlightRange
}
