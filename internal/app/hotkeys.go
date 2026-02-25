package app

import "strings"

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
	HotkeyGuidedWorkflow
)

type Hotkey struct {
	Key      string
	Command  string
	Label    string
	Context  HotkeyContext
	Priority int
}

type HotkeyResolver interface {
	ActiveContexts(*Model) []HotkeyContext
}

func DefaultHotkeys() []Hotkey {
	return []Hotkey{
		{Key: "ctrl+b", Command: KeyCommandToggleSidebar, Label: "sidebar", Context: HotkeyGlobal, Priority: 10},
		{Key: "ctrl+o", Command: KeyCommandToggleNotesPanel, Label: "notes panel", Context: HotkeyGlobal, Priority: 10},
		{Key: "ctrl+d", Command: KeyCommandToggleDebugStreams, Label: "debug streams", Context: HotkeyGlobal, Priority: 10},
		{Key: "H/J/K/L", Label: "debug pan/scroll", Context: HotkeySidebar, Priority: 58},
		{Key: "shift+pgup/pgdn", Label: "debug page", Context: HotkeySidebar, Priority: 59},
		{Key: "shift+home/end", Label: "debug top/bottom", Context: HotkeySidebar, Priority: 60},
		{Key: "ctrl+m", Command: KeyCommandMenu, Label: "menu", Context: HotkeyGlobal, Priority: 11},
		{Key: "q", Command: KeyCommandQuit, Label: "quit", Context: HotkeyGlobal, Priority: 90},
		{Key: "a", Command: KeyCommandAddWorkspace, Label: "add workspace", Context: HotkeySidebar, Priority: 20},
		{Key: "t", Command: KeyCommandAddWorktree, Label: "add worktree", Context: HotkeySidebar, Priority: 21},
		{Key: "m", Command: KeyCommandRename, Label: "rename", Context: HotkeySidebar, Priority: 22},
		{Key: "enter", Command: KeyCommandOpenChat, Label: "open", Context: HotkeySidebar, Priority: 23},
		{Key: "c", Command: KeyCommandCompose, Label: "compose", Context: HotkeySidebar, Priority: 24},
		{Key: "w", Command: KeyCommandStartGuidedWorkflow, Label: "new workflow", Context: HotkeySidebar, Priority: 25},
		{Key: "ctrl+n", Command: KeyCommandNewSession, Label: "new session", Context: HotkeySidebar, Priority: 26},
		{Key: "O", Command: KeyCommandOpenNotes, Label: "notes", Context: HotkeySidebar, Priority: 27},
		{Key: "n", Command: KeyCommandNotesNew, Label: "new note", Context: HotkeySidebar, Priority: 28},
		{Key: "1/2/3", Label: "toggle note scope", Context: HotkeySidebar, Priority: 29},
		{Key: "0", Command: KeyCommandToggleNotesAll, Label: "all note scopes", Context: HotkeySidebar, Priority: 30},
		{Key: "d", Command: KeyCommandDismissSelection, Label: "dismiss/delete", Context: HotkeySidebar, Priority: 31},
		{Key: "u", Command: KeyCommandUndismissSession, Label: "undismiss", Context: HotkeySidebar, Priority: 32},
		{Key: "D", Command: KeyCommandToggleDismissed, Label: "toggle dismissed", Context: HotkeySidebar, Priority: 33},
		{Key: "ctrl+g", Command: KeyCommandCopySessionID, Label: "copy id", Context: HotkeySidebar, Priority: 34},
		{Key: "x", Command: KeyCommandKillSession, Label: "kill", Context: HotkeySidebar, Priority: 34},
		{Key: "i", Command: KeyCommandInterruptSession, Label: "interrupt/stop", Context: HotkeySidebar, Priority: 35},
		{Key: "←/→", Label: "collapse/expand", Context: HotkeySidebar, Priority: 39},
		{Key: "esc", Label: "close", Context: HotkeyContextMenu, Priority: 5},
		{Key: "enter", Label: "select", Context: HotkeyContextMenu, Priority: 6},
		{Key: "j/k/↑/↓", Label: "move", Context: HotkeyContextMenu, Priority: 7},
		{Key: "y/enter", Label: "confirm", Context: HotkeyConfirm, Priority: 5},
		{Key: "n/esc", Label: "cancel", Context: HotkeyConfirm, Priority: 6},
		{Key: "h/l/←/→", Label: "switch", Context: HotkeyConfirm, Priority: 7},
		{Key: "y", Command: KeyCommandApprove, Label: "approve", Context: HotkeyApproval, Priority: 5},
		{Key: "x", Command: KeyCommandDecline, Label: "decline", Context: HotkeyApproval, Priority: 6},
		{Key: "j/k/↑/↓", Label: "move", Context: HotkeySidebar, Priority: 40},
		{Key: "r", Command: KeyCommandRefresh, Label: "refresh", Context: HotkeySidebar, Priority: 50},
		{Key: "g/gg", Command: KeyCommandViewportTop, Label: "top", Context: HotkeySidebar, Priority: 52},
		{Key: "G", Command: KeyCommandViewportBottom, Label: "bottom", Context: HotkeySidebar, Priority: 53},
		{Key: "{/}", Label: "jump", Context: HotkeySidebar, Priority: 54},
		{Key: "/", Command: KeyCommandOpenSearch, Label: "search", Context: HotkeySidebar, Priority: 55},
		{Key: "n/N", Label: "next/prev", Context: HotkeySidebar, Priority: 56},
		{Key: "ctrl+f", Label: "page down", Context: HotkeySidebar, Priority: 57},
		{Key: "pgup/pgdn", Label: "scroll", Context: HotkeySidebar, Priority: 61},
		{Key: "e", Command: KeyCommandToggleReasoning, Label: "toggle reasoning", Context: HotkeySidebar, Priority: 65},
		{Key: "alt+←", Command: KeyCommandHistoryBack, Label: "history back", Context: HotkeySidebar, Priority: 66},
		{Key: "alt+→", Command: KeyCommandHistoryForward, Label: "history forward", Context: HotkeySidebar, Priority: 67},
		{Key: "p", Command: KeyCommandPauseFollow, Label: "pause", Context: HotkeySidebar, Priority: 70},
		{Key: "esc", Label: "cancel", Context: HotkeySearch, Priority: 10},
		{Key: "enter", Label: "search", Context: HotkeySearch, Priority: 11},
		{Key: "j/k/↑/↓", Label: "setup choice", Context: HotkeyGuidedWorkflow, Priority: 10},
		{Key: "enter", Label: "continue/start", Context: HotkeyGuidedWorkflow, Priority: 11},
		{Key: "a/v/p", Label: "checkpoint action", Context: HotkeyGuidedWorkflow, Priority: 12},
		{Key: "r", Label: "refresh timeline", Context: HotkeyGuidedWorkflow, Priority: 13},
		{Key: "esc", Label: "close", Context: HotkeyGuidedWorkflow, Priority: 14},
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
		{Key: "ctrl+g", Command: KeyCommandCopySessionID, Label: "copy id", Context: HotkeyChatInput, Priority: 12},
		{Key: "ctrl+c", Command: KeyCommandInputClear, Label: "clear", Context: HotkeyChatInput, Priority: 13},
		{Key: "up", Command: KeyCommandInputLineUp, Label: "line up", Context: HotkeyChatInput, Priority: 14},
		{Key: "down", Command: KeyCommandInputLineDown, Label: "line down", Context: HotkeyChatInput, Priority: 15},
		{Key: "ctrl+↑/ctrl+↓", Label: "history", Context: HotkeyChatInput, Priority: 16},
		{Key: "ctrl+a", Command: KeyCommandInputSelectAll, Label: "select all", Context: HotkeyChatInput, Priority: 17},
		{Key: "ctrl+z", Command: KeyCommandInputUndo, Label: "undo", Context: HotkeyChatInput, Priority: 18},
		{Key: "ctrl+y", Command: KeyCommandInputRedo, Label: "redo", Context: HotkeyChatInput, Priority: 19},
	}
}

func ResolveHotkeys(hotkeys []Hotkey, bindings *Keybindings) []Hotkey {
	if len(hotkeys) == 0 {
		return nil
	}
	resolved := make([]Hotkey, len(hotkeys))
	copy(resolved, hotkeys)
	for i := range resolved {
		if strings.TrimSpace(resolved[i].Command) == "" {
			continue
		}
		resolved[i].Key = bindings.KeyFor(resolved[i].Command, resolved[i].Key)
	}
	return resolved
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
	if m.mode == uiModeGuidedWorkflow {
		return []HotkeyContext{HotkeyGlobal, HotkeyGuidedWorkflow}
	}
	contexts := []HotkeyContext{HotkeyGlobal}
	switch m.mode {
	case uiModeAddWorkspace:
		contexts = append(contexts, HotkeyAddWorkspace)
	case uiModeEditWorkspace:
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
