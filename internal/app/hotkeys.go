package app

type HotkeyContext int

const (
	HotkeyGlobal HotkeyContext = iota
	HotkeySidebar
	HotkeyChatInput
	HotkeyAddWorkspace
	HotkeyAddWorktree
)

type Hotkey struct {
	Key      string
	Label    string
	Context  HotkeyContext
	Priority int
}

type HotkeyResolver interface {
	ActiveContexts(*Model) []HotkeyContext
}

func DefaultHotkeys() []Hotkey {
	return []Hotkey{
		{Key: "ctrl+b", Label: "sidebar", Context: HotkeyGlobal, Priority: 10},
		{Key: "q", Label: "quit", Context: HotkeyGlobal, Priority: 90},
		{Key: "ctrl+c", Label: "quit", Context: HotkeyGlobal, Priority: 91},
		{Key: "a", Label: "add workspace", Context: HotkeySidebar, Priority: 20},
		{Key: "t", Label: "add worktree", Context: HotkeySidebar, Priority: 21},
		{Key: "enter", Label: "chat", Context: HotkeySidebar, Priority: 22},
		{Key: "c", Label: "compose", Context: HotkeySidebar, Priority: 23},
		{Key: "n", Label: "new session", Context: HotkeySidebar, Priority: 24},
		{Key: "space", Label: "select", Context: HotkeySidebar, Priority: 30},
		{Key: "d", Label: "dismiss", Context: HotkeySidebar, Priority: 31},
		{Key: "j/k/↑/↓", Label: "move", Context: HotkeySidebar, Priority: 40},
		{Key: "r", Label: "refresh", Context: HotkeySidebar, Priority: 50},
		{Key: "pgup/pgdn", Label: "scroll", Context: HotkeySidebar, Priority: 60},
		{Key: "p", Label: "pause", Context: HotkeySidebar, Priority: 70},
		{Key: "esc", Label: "cancel", Context: HotkeyAddWorkspace, Priority: 10},
		{Key: "enter", Label: "continue", Context: HotkeyAddWorkspace, Priority: 11},
		{Key: "esc", Label: "cancel", Context: HotkeyAddWorktree, Priority: 10},
		{Key: "enter", Label: "continue", Context: HotkeyAddWorktree, Priority: 11},
		{Key: "j/k/↑/↓", Label: "move", Context: HotkeyAddWorktree, Priority: 12},
		{Key: "esc", Label: "cancel", Context: HotkeyChatInput, Priority: 10},
		{Key: "enter", Label: "send", Context: HotkeyChatInput, Priority: 11},
	}
}

type DefaultHotkeyResolver struct{}

func (r DefaultHotkeyResolver) ActiveContexts(m *Model) []HotkeyContext {
	contexts := []HotkeyContext{HotkeyGlobal}
	if m == nil {
		return contexts
	}
	switch m.mode {
	case uiModeAddWorkspace:
		contexts = append(contexts, HotkeyAddWorkspace)
	case uiModeAddWorktree:
		contexts = append(contexts, HotkeyAddWorktree)
	case uiModeCompose:
		if m.input != nil && m.input.IsChatFocused() {
			contexts = append(contexts, HotkeyChatInput)
		} else {
			contexts = append(contexts, HotkeySidebar)
		}
	default:
		contexts = append(contexts, HotkeySidebar)
	}
	return contexts
}
