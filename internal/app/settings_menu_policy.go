package app

import (
	"sort"
	"strings"
)

type SettingsMenuOpenContext interface {
	Mode() uiMode
	IsConfirmOpen() bool
	IsContextMenuOpen() bool
	IsTopMenuActive() bool
	IsSettingsMenuOpen() bool
	IsStatusHistoryOpen() bool
}

type SettingsMenuEscPolicy interface {
	CanOpen(SettingsMenuOpenContext) bool
}

type defaultSettingsMenuEscPolicy struct{}

func (defaultSettingsMenuEscPolicy) CanOpen(ctx SettingsMenuOpenContext) bool {
	if ctx == nil || ctx.Mode() != uiModeNormal {
		return false
	}
	if ctx.IsConfirmOpen() || ctx.IsContextMenuOpen() || ctx.IsTopMenuActive() || ctx.IsSettingsMenuOpen() || ctx.IsStatusHistoryOpen() {
		return false
	}
	return true
}

func (m *Model) settingsMenuEscPolicyOrDefault() SettingsMenuEscPolicy {
	if m == nil {
		return defaultSettingsMenuEscPolicy{}
	}
	if m.settingsMenuEscPolicy == nil {
		m.settingsMenuEscPolicy = defaultSettingsMenuEscPolicy{}
	}
	return m.settingsMenuEscPolicy
}

type modelSettingsMenuOpenContext struct {
	model *Model
}

func (c modelSettingsMenuOpenContext) Mode() uiMode {
	if c.model == nil {
		return uiModeNormal
	}
	return c.model.mode
}

func (c modelSettingsMenuOpenContext) IsConfirmOpen() bool {
	return c.model != nil && c.model.confirm != nil && c.model.confirm.IsOpen()
}

func (c modelSettingsMenuOpenContext) IsContextMenuOpen() bool {
	return c.model != nil && c.model.contextMenu != nil && c.model.contextMenu.IsOpen()
}

func (c modelSettingsMenuOpenContext) IsTopMenuActive() bool {
	return c.model != nil && c.model.menu != nil && c.model.menu.IsActive()
}

func (c modelSettingsMenuOpenContext) IsSettingsMenuOpen() bool {
	return c.model != nil && c.model.settingsMenu != nil && c.model.settingsMenu.IsOpen()
}

func (c modelSettingsMenuOpenContext) IsStatusHistoryOpen() bool {
	return c.model != nil && c.model.statusHistoryOverlayOpen()
}

func (m *Model) settingsMenuOpenContext() SettingsMenuOpenContext {
	return modelSettingsMenuOpenContext{model: m}
}

type SettingsHotkeySource interface {
	ResolvedHotkeys() []Hotkey
}

type SettingsMenuHotkeyCatalog interface {
	Mappings(SettingsHotkeySource) []SettingsHotkeyMapping
}

type defaultSettingsMenuHotkeyCatalog struct{}

func (defaultSettingsMenuHotkeyCatalog) Mappings(source SettingsHotkeySource) []SettingsHotkeyMapping {
	if source == nil {
		return nil
	}
	hotkeys := source.ResolvedHotkeys()
	out := make([]SettingsHotkeyMapping, 0, len(hotkeys))
	for _, hk := range hotkeys {
		key := strings.TrimSpace(hk.Key)
		label := strings.TrimSpace(hk.Label)
		if key == "" || label == "" {
			continue
		}
		out = append(out, SettingsHotkeyMapping{
			Context:  settingsHotkeyContextLabel(hk.Context),
			Key:      key,
			Label:    label,
			Priority: hk.Priority,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Context == out[j].Context {
			if out[i].Priority == out[j].Priority {
				if out[i].Key == out[j].Key {
					return out[i].Label < out[j].Label
				}
				return out[i].Key < out[j].Key
			}
			return out[i].Priority < out[j].Priority
		}
		return out[i].Context < out[j].Context
	})
	return out
}

func (m *Model) settingsMenuHotkeyCatalogOrDefault() SettingsMenuHotkeyCatalog {
	if m == nil {
		return defaultSettingsMenuHotkeyCatalog{}
	}
	if m.settingsMenuHotkeyCatalog == nil {
		m.settingsMenuHotkeyCatalog = defaultSettingsMenuHotkeyCatalog{}
	}
	return m.settingsMenuHotkeyCatalog
}

type modelSettingsHotkeySource struct {
	model *Model
}

func (s modelSettingsHotkeySource) ResolvedHotkeys() []Hotkey {
	if s.model == nil {
		return nil
	}
	bindings := s.model.keybindings
	if bindings == nil {
		bindings = DefaultKeybindings()
	}
	return ResolveHotkeys(DefaultHotkeys(), bindings)
}

func (m *Model) settingsMenuHotkeySource() SettingsHotkeySource {
	return modelSettingsHotkeySource{model: m}
}

func settingsHotkeyContextLabel(context HotkeyContext) string {
	switch context {
	case HotkeyGlobal:
		return "global"
	case HotkeySidebar:
		return "sidebar"
	case HotkeyChatInput:
		return "chat-input"
	case HotkeyAddWorkspace:
		return "add-workspace"
	case HotkeyAddWorktree:
		return "add-worktree"
	case HotkeyPickProvider:
		return "pick-provider"
	case HotkeySearch:
		return "search"
	case HotkeyContextMenu:
		return "context-menu"
	case HotkeyConfirm:
		return "confirm"
	case HotkeyApproval:
		return "approval"
	case HotkeyGuidedWorkflow:
		return "guided-workflow"
	default:
		return "other"
	}
}
