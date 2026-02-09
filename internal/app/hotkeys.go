package app

type HotkeyContext int

const (
	HotkeyGlobal HotkeyContext = iota
	HotkeySidebar
	HotkeyChatInput
	HotkeyAddWorkspace
	HotkeyAddWorktree
	HotkeyPickProvider
	HotkeySearch
	HotkeyContextMenu
	HotkeyConfirm
	HotkeyApproval
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
		{Key: "m", Label: "menu", Context: HotkeyGlobal, Priority: 11},
		{Key: "q", Label: "quit", Context: HotkeyGlobal, Priority: 90},
		{Key: "a", Label: "add workspace", Context: HotkeySidebar, Priority: 20},
		{Key: "t", Label: "add worktree", Context: HotkeySidebar, Priority: 21},
		{Key: "enter", Label: "chat", Context: HotkeySidebar, Priority: 22},
		{Key: "c", Label: "compose", Context: HotkeySidebar, Priority: 23},
		{Key: "ctrl+n", Label: "new session", Context: HotkeySidebar, Priority: 24},
		{Key: "O", Label: "notes", Context: HotkeySidebar, Priority: 25},
		{Key: "space", Label: "select", Context: HotkeySidebar, Priority: 30},
		{Key: "d", Label: "dismiss", Context: HotkeySidebar, Priority: 31},
		{Key: "ctrl+y", Label: "copy id", Context: HotkeySidebar, Priority: 32},
		{Key: "i", Label: "interrupt", Context: HotkeySidebar, Priority: 33},
		{Key: "esc", Label: "close", Context: HotkeyContextMenu, Priority: 5},
		{Key: "enter", Label: "select", Context: HotkeyContextMenu, Priority: 6},
		{Key: "j/k/↑/↓", Label: "move", Context: HotkeyContextMenu, Priority: 7},
		{Key: "y/enter", Label: "confirm", Context: HotkeyConfirm, Priority: 5},
		{Key: "n/esc", Label: "cancel", Context: HotkeyConfirm, Priority: 6},
		{Key: "h/l/←/→", Label: "switch", Context: HotkeyConfirm, Priority: 7},
		{Key: "y", Label: "approve", Context: HotkeyApproval, Priority: 5},
		{Key: "x", Label: "decline", Context: HotkeyApproval, Priority: 6},
		{Key: "j/k/↑/↓", Label: "move", Context: HotkeySidebar, Priority: 40},
		{Key: "r", Label: "refresh", Context: HotkeySidebar, Priority: 50},
		{Key: "g/gg", Label: "top", Context: HotkeySidebar, Priority: 52},
		{Key: "G", Label: "bottom", Context: HotkeySidebar, Priority: 53},
		{Key: "{/}", Label: "jump", Context: HotkeySidebar, Priority: 54},
		{Key: "/", Label: "search", Context: HotkeySidebar, Priority: 55},
		{Key: "n/N", Label: "next/prev", Context: HotkeySidebar, Priority: 56},
		{Key: "ctrl+f", Label: "page down", Context: HotkeySidebar, Priority: 57},
		{Key: "pgup/pgdn", Label: "scroll", Context: HotkeySidebar, Priority: 60},
		{Key: "e", Label: "toggle reasoning", Context: HotkeySidebar, Priority: 65},
		{Key: "p", Label: "pause", Context: HotkeySidebar, Priority: 70},
		{Key: "esc", Label: "cancel", Context: HotkeySearch, Priority: 10},
		{Key: "enter", Label: "search", Context: HotkeySearch, Priority: 11},
		{Key: "esc", Label: "cancel", Context: HotkeyAddWorkspace, Priority: 10},
		{Key: "enter", Label: "continue", Context: HotkeyAddWorkspace, Priority: 11},
		{Key: "esc", Label: "cancel", Context: HotkeyAddWorktree, Priority: 10},
		{Key: "enter", Label: "continue", Context: HotkeyAddWorktree, Priority: 11},
		{Key: "j/k/↑/↓", Label: "move", Context: HotkeyAddWorktree, Priority: 12},
		{Key: "esc", Label: "cancel", Context: HotkeyPickProvider, Priority: 10},
		{Key: "enter", Label: "select", Context: HotkeyPickProvider, Priority: 11},
		{Key: "j/k/↑/↓", Label: "move", Context: HotkeyPickProvider, Priority: 12},
		{Key: "esc", Label: "cancel", Context: HotkeyChatInput, Priority: 10},
		{Key: "enter", Label: "send", Context: HotkeyChatInput, Priority: 11},
		{Key: "ctrl+y", Label: "copy id", Context: HotkeyChatInput, Priority: 12},
		{Key: "ctrl+c", Label: "clear", Context: HotkeyChatInput, Priority: 13},
		{Key: "↑/↓", Label: "history", Context: HotkeyChatInput, Priority: 14},
	}
}

type DefaultHotkeyResolver struct{}

func (r DefaultHotkeyResolver) ActiveContexts(m *Model) []HotkeyContext {
	if m == nil {
		return []HotkeyContext{HotkeyGlobal}
	}
	if m.confirm != nil && m.confirm.IsOpen() {
		return []HotkeyContext{HotkeyGlobal, HotkeyConfirm}
	}
	if m.contextMenu != nil && m.contextMenu.IsOpen() {
		return []HotkeyContext{HotkeyGlobal, HotkeyContextMenu}
	}
	if m.mode == uiModeSearch {
		return []HotkeyContext{HotkeySearch}
	}
	contexts := []HotkeyContext{HotkeyGlobal}
	switch m.mode {
	case uiModeAddWorkspace:
		contexts = append(contexts, HotkeyAddWorkspace)
	case uiModeAddWorktree:
		contexts = append(contexts, HotkeyAddWorktree)
	case uiModePickProvider:
		contexts = append(contexts, HotkeyPickProvider)
	case uiModeCompose:
		if m.input != nil && m.input.IsChatFocused() {
			return []HotkeyContext{HotkeyChatInput}
		} else {
			contexts = append(contexts, HotkeySidebar)
		}
	default:
		contexts = append(contexts, HotkeySidebar)
	}
	if m.pendingApproval != nil && (m.input == nil || m.input.IsSidebarFocused()) {
		contexts = append(contexts, HotkeyApproval)
	}
	return contexts
}
