package app

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const (
	KeyCommandMenu                 = "ui.menu"
	KeyCommandRename               = "ui.rename"
	KeyCommandQuit                 = "ui.quit"
	KeyCommandToggleSidebar        = "ui.toggleSidebar"
	KeyCommandToggleNotesPanel     = "ui.toggleNotesPanel"
	KeyCommandCopySessionID        = "ui.copySessionID"
	KeyCommandOpenSearch           = "ui.openSearch"
	KeyCommandSidebarFilter        = "ui.sidebarFilter"
	KeyCommandSidebarSortReverse   = "ui.sidebarSortReverse"
	KeyCommandViewportTop          = "ui.viewportTop"
	KeyCommandViewportBottom       = "ui.viewportBottom"
	KeyCommandSectionPrev          = "ui.sectionPrev"
	KeyCommandSectionNext          = "ui.sectionNext"
	KeyCommandSearchPrev           = "ui.searchPrev"
	KeyCommandSearchNext           = "ui.searchNext"
	KeyCommandNewSession           = "ui.newSession"
	KeyCommandAddWorkspace         = "ui.addWorkspace"
	KeyCommandAddWorktree          = "ui.addWorktree"
	KeyCommandCompose              = "ui.compose"
	KeyCommandStartGuidedWorkflow  = "ui.startGuidedWorkflow"
	KeyCommandOpenNotes            = "ui.openNotes"
	KeyCommandOpenChat             = "ui.openChat"
	KeyCommandRefresh              = "ui.refresh"
	KeyCommandKillSession          = "ui.killSession"
	KeyCommandInterruptSession     = "ui.interruptSession"
	KeyCommandDismissSelection     = "ui.dismissSelection"
	KeyCommandDismissSession       = "ui.dismissSession" // legacy alias; normalized to ui.dismissSelection
	KeyCommandUndismissSession     = "ui.undismissSession"
	KeyCommandToggleDismissed      = "ui.toggleDismissed"
	KeyCommandToggleNotesWorkspace = "ui.toggleNotesWorkspace"
	KeyCommandToggleNotesWorktree  = "ui.toggleNotesWorktree"
	KeyCommandToggleNotesSession   = "ui.toggleNotesSession"
	KeyCommandToggleNotesAll       = "ui.toggleNotesAll"
	KeyCommandPauseFollow          = "ui.pauseFollow"
	KeyCommandToggleReasoning      = "ui.toggleReasoning"
	KeyCommandToggleMessageSelect  = "ui.toggleMessageSelect"
	KeyCommandHistoryBack          = "ui.historyBack"
	KeyCommandHistoryForward       = "ui.historyForward"
	KeyCommandInputClear           = "ui.inputClear"
	KeyCommandComposeClearInput    = "ui.composeClearInput" // legacy alias; normalized to ui.inputClear
	KeyCommandComposeModel         = "ui.composeModel"
	KeyCommandComposeReasoning     = "ui.composeReasoning"
	KeyCommandComposeAccess        = "ui.composeAccess"
	KeyCommandInputSubmit          = "ui.inputSubmit"
	KeyCommandInputNewline         = "ui.inputNewline"
	KeyCommandInputLineUp          = "ui.inputLineUp"
	KeyCommandInputLineDown        = "ui.inputLineDown"
	KeyCommandInputWordLeft        = "ui.inputWordLeft"
	KeyCommandInputWordRight       = "ui.inputWordRight"
	KeyCommandInputDeleteWordLeft  = "ui.inputDeleteWordLeft"
	KeyCommandInputDeleteWordRight = "ui.inputDeleteWordRight"
	KeyCommandInputSelectAll       = "ui.inputSelectAll"
	KeyCommandInputUndo            = "ui.inputUndo"
	KeyCommandInputRedo            = "ui.inputRedo"
	KeyCommandApprove              = "ui.approve"
	KeyCommandDecline              = "ui.decline"
	KeyCommandNotesNew             = "ui.notesNew"
)

var defaultKeybindingByCommand = map[string]string{
	KeyCommandMenu:                 "ctrl+m",
	KeyCommandRename:               "m",
	KeyCommandQuit:                 "q",
	KeyCommandToggleSidebar:        "ctrl+b",
	KeyCommandToggleNotesPanel:     "ctrl+o",
	KeyCommandCopySessionID:        "ctrl+g",
	KeyCommandOpenSearch:           "/",
	KeyCommandSidebarFilter:        "ctrl+f",
	KeyCommandSidebarSortReverse:   "alt+r",
	KeyCommandViewportTop:          "g",
	KeyCommandViewportBottom:       "G",
	KeyCommandSectionPrev:          "{",
	KeyCommandSectionNext:          "}",
	KeyCommandSearchPrev:           "N",
	KeyCommandSearchNext:           "n",
	KeyCommandNewSession:           "ctrl+n",
	KeyCommandAddWorkspace:         "a",
	KeyCommandAddWorktree:          "t",
	KeyCommandCompose:              "c",
	KeyCommandStartGuidedWorkflow:  "w",
	KeyCommandOpenNotes:            "O",
	KeyCommandOpenChat:             "enter",
	KeyCommandRefresh:              "r",
	KeyCommandKillSession:          "x",
	KeyCommandInterruptSession:     "i",
	KeyCommandDismissSelection:     "d",
	KeyCommandUndismissSession:     "u",
	KeyCommandToggleDismissed:      "D",
	KeyCommandToggleNotesWorkspace: "1",
	KeyCommandToggleNotesWorktree:  "2",
	KeyCommandToggleNotesSession:   "3",
	KeyCommandToggleNotesAll:       "0",
	KeyCommandPauseFollow:          "p",
	KeyCommandToggleReasoning:      "e",
	KeyCommandToggleMessageSelect:  "v",
	KeyCommandHistoryBack:          "alt+left",
	KeyCommandHistoryForward:       "alt+right",
	KeyCommandInputClear:           "ctrl+c",
	KeyCommandComposeModel:         "ctrl+1",
	KeyCommandComposeReasoning:     "ctrl+2",
	KeyCommandComposeAccess:        "ctrl+3",
	KeyCommandInputSubmit:          "enter",
	KeyCommandInputNewline:         "shift+enter",
	KeyCommandInputLineUp:          "up",
	KeyCommandInputLineDown:        "down",
	KeyCommandInputWordLeft:        "ctrl+left",
	KeyCommandInputWordRight:       "ctrl+right",
	KeyCommandInputDeleteWordLeft:  "alt+backspace",
	KeyCommandInputDeleteWordRight: "alt+delete",
	KeyCommandInputSelectAll:       "ctrl+a",
	KeyCommandInputUndo:            "ctrl+z",
	KeyCommandInputRedo:            "ctrl+y",
	KeyCommandApprove:              "y",
	KeyCommandDecline:              "x",
	KeyCommandNotesNew:             "n",
}

type Keybindings struct {
	byCommand map[string]string
	remap     map[string]string
}

type keybindingEntry struct {
	Command string `json:"command"`
	Key     string `json:"key"`
}

func DefaultKeybindings() *Keybindings {
	return NewKeybindings(nil)
}

func NewKeybindings(overrides map[string]string) *Keybindings {
	byCommand := make(map[string]string, len(defaultKeybindingByCommand))
	for command, key := range defaultKeybindingByCommand {
		byCommand[command] = key
	}
	for command, key := range normalizeKeybindingOverrides(overrides) {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := defaultKeybindingByCommand[command]; !ok {
			continue
		}
		byCommand[command] = key
	}
	remap := map[string]string{}
	ambiguous := map[string]struct{}{}
	commands := make([]string, 0, len(defaultKeybindingByCommand))
	for command := range defaultKeybindingByCommand {
		commands = append(commands, command)
	}
	sort.Strings(commands)
	for _, command := range commands {
		defaultKey := defaultKeybindingByCommand[command]
		key := byCommand[command]
		if strings.TrimSpace(key) == "" || key == defaultKey {
			continue
		}
		if _, bad := ambiguous[key]; bad {
			continue
		}
		if existing, ok := remap[key]; ok && existing != defaultKey {
			delete(remap, key)
			ambiguous[key] = struct{}{}
			continue
		}
		remap[key] = defaultKey
	}
	return &Keybindings{
		byCommand: byCommand,
		remap:     remap,
	}
}

func LoadKeybindings(path string) (*Keybindings, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return DefaultKeybindings(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultKeybindings(), nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return DefaultKeybindings(), nil
	}
	overrides, err := parseKeybindingOverrides(data)
	if err != nil {
		return nil, err
	}
	return NewKeybindings(overrides), nil
}

func (k *Keybindings) KeyFor(command, fallback string) string {
	command = normalizeKeybindingCommand(command)
	if command == "" {
		return fallback
	}
	if k != nil {
		if key := strings.TrimSpace(k.byCommand[command]); key != "" {
			return key
		}
	}
	if key := strings.TrimSpace(defaultKeybindingByCommand[command]); key != "" {
		return key
	}
	return fallback
}

func (k *Keybindings) Bindings() map[string]string {
	out := make(map[string]string, len(defaultKeybindingByCommand))
	commands := KnownKeybindingCommands()
	for _, command := range commands {
		out[command] = k.KeyFor(command, defaultKeybindingByCommand[command])
	}
	return out
}

func (k *Keybindings) Remap(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return key
	}
	if k != nil {
		if canonical, ok := k.remap[key]; ok && canonical != "" {
			return canonical
		}
	}
	return key
}

func (m *Model) applyKeybindings(bindings *Keybindings) {
	if bindings == nil {
		bindings = DefaultKeybindings()
	}
	m.keybindings = bindings
	m.hotkeys = NewHotkeyRenderer(ResolveHotkeys(DefaultHotkeys(), bindings), DefaultHotkeyResolver{})
	m.updateDelegate()
}

func (m *Model) keyString(msg tea.KeyMsg) string {
	if m == nil {
		return msg.String()
	}
	key := msg.String()
	if m.keybindings == nil {
		return key
	}
	return m.keybindings.Remap(key)
}

func (m *Model) keyForCommand(command, fallback string) string {
	if m == nil || m.keybindings == nil {
		return fallback
	}
	return m.keybindings.KeyFor(command, fallback)
}

func (m *Model) keyMatchesCommand(msg tea.KeyMsg, command, fallback string) bool {
	bound := strings.TrimSpace(m.keyForCommand(command, fallback))
	if bound != "" && strings.TrimSpace(msg.String()) == bound {
		return true
	}
	canonical := strings.TrimSpace(fallback)
	return canonical != "" && strings.TrimSpace(m.keyString(msg)) == canonical
}

func (m *Model) keyMatchesOverriddenCommand(msg tea.KeyMsg, command, fallback string) bool {
	bound := strings.TrimSpace(m.keyForCommand(command, fallback))
	canonical := strings.TrimSpace(fallback)
	if bound == "" || bound == canonical {
		return false
	}
	return strings.TrimSpace(msg.String()) == bound
}

func parseKeybindingOverrides(data []byte) (map[string]string, error) {
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] == '[' {
		var entries []keybindingEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, err
		}
		out := map[string]string{}
		for _, entry := range entries {
			command := normalizeKeybindingCommand(entry.Command)
			if command == "" {
				continue
			}
			if _, ok := defaultKeybindingByCommand[command]; !ok {
				continue
			}
			key := strings.TrimSpace(entry.Key)
			if key == "" {
				continue
			}
			out[command] = key
		}
		return out, nil
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for command, key := range raw {
		command = normalizeKeybindingCommand(command)
		if command == "" {
			continue
		}
		if _, ok := defaultKeybindingByCommand[command]; !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[command] = key
	}
	return out, nil
}

func normalizeKeybindingCommand(command string) string {
	command = strings.TrimSpace(command)
	switch command {
	case KeyCommandDismissSession:
		return KeyCommandDismissSelection
	case KeyCommandComposeClearInput:
		return KeyCommandInputClear
	default:
		return command
	}
}

func normalizeKeybindingOverrides(overrides map[string]string) map[string]string {
	if len(overrides) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(overrides))
	for command, key := range overrides {
		command = normalizeKeybindingCommand(command)
		if command == "" {
			continue
		}
		normalized[command] = key
	}
	return normalized
}

func KnownKeybindingCommands() []string {
	keys := make([]string, 0, len(defaultKeybindingByCommand))
	for command := range defaultKeybindingByCommand {
		keys = append(keys, command)
	}
	sort.Strings(keys)
	return keys
}
