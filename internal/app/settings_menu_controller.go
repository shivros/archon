package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type SettingsMenuAction int

const (
	SettingsMenuActionNone SettingsMenuAction = iota
	SettingsMenuActionApplyTheme
	SettingsMenuActionQuit
)

type settingsMenuScreen int

const (
	settingsMenuScreenRoot settingsMenuScreen = iota
	settingsMenuScreenHelp
	settingsMenuScreenTheme
)

type SettingsMenuItem struct {
	ID     string
	Title  string
	Screen settingsMenuScreen
	Action SettingsMenuAction
}

type SettingsThemeItem struct {
	ID    string
	Title string
}

const settingsMenuDefaultThemeID = "default"

type SettingsMenuController struct {
	open          bool
	screen        settingsMenuScreen
	selected      int
	helpOffset    int
	items         []SettingsMenuItem
	themeItems    []SettingsThemeItem
	themeSelected int
	activeThemeID string
}

type SettingsHotkeyMapping struct {
	Context  string
	Key      string
	Label    string
	Priority int
}

func DefaultSettingsMenuItems() []SettingsMenuItem {
	return []SettingsMenuItem{
		{ID: "help", Title: "HELP", Screen: settingsMenuScreenHelp, Action: SettingsMenuActionNone},
		{ID: "theme", Title: "THEME", Screen: settingsMenuScreenTheme, Action: SettingsMenuActionNone},
		{ID: "quit", Title: "QUIT", Screen: settingsMenuScreenRoot, Action: SettingsMenuActionQuit},
	}
}

func NewSettingsMenuController(items ...SettingsMenuItem) *SettingsMenuController {
	resolved := items
	if len(resolved) == 0 {
		resolved = DefaultSettingsMenuItems()
	}
	out := make([]SettingsMenuItem, 0, len(resolved))
	for _, item := range resolved {
		if item.ID == "" {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		out = DefaultSettingsMenuItems()
	}
	c := &SettingsMenuController{
		items:         out,
		activeThemeID: settingsMenuDefaultThemeID,
	}
	c.SetSelectedThemeID(c.activeThemeID)
	return c
}

func (c *SettingsMenuController) IsOpen() bool {
	return c != nil && c.open
}

func (c *SettingsMenuController) Open() {
	if c == nil {
		return
	}
	c.open = true
	c.screen = settingsMenuScreenRoot
	c.selected = 0
	c.helpOffset = 0
	c.SetSelectedThemeID(c.activeThemeID)
}

func (c *SettingsMenuController) Close() {
	if c == nil {
		return
	}
	c.open = false
	c.screen = settingsMenuScreenRoot
	c.selected = 0
	c.helpOffset = 0
	c.SetSelectedThemeID(c.activeThemeID)
}

func (c *SettingsMenuController) HandleKey(msg tea.KeyMsg) (bool, SettingsMenuAction) {
	if c == nil || !c.open {
		return false, SettingsMenuActionNone
	}
	switch c.screen {
	case settingsMenuScreenHelp:
		switch msg.String() {
		case "esc":
			c.screen = settingsMenuScreenRoot
			c.helpOffset = 0
			return true, SettingsMenuActionNone
		case "up", "k":
			if c.helpOffset > 0 {
				c.helpOffset--
			}
			return true, SettingsMenuActionNone
		case "down", "j":
			c.helpOffset++
			return true, SettingsMenuActionNone
		case "pgup":
			c.helpOffset = max(0, c.helpOffset-10)
			return true, SettingsMenuActionNone
		case "pgdown":
			c.helpOffset += 10
			return true, SettingsMenuActionNone
		case "home":
			c.helpOffset = 0
			return true, SettingsMenuActionNone
		case "end":
			c.helpOffset = 1 << 20
			return true, SettingsMenuActionNone
		}
		return true, SettingsMenuActionNone
	case settingsMenuScreenTheme:
		switch msg.String() {
		case "esc":
			c.screen = settingsMenuScreenRoot
			return true, SettingsMenuActionNone
		case "up", "k":
			if c.themeSelected > 0 {
				c.themeSelected--
			}
			return true, SettingsMenuActionNone
		case "down", "j":
			if c.themeSelected < len(c.themeItems)-1 {
				c.themeSelected++
			}
			return true, SettingsMenuActionNone
		case "enter":
			if c.selectedThemeID() == "" {
				return true, SettingsMenuActionNone
			}
			return true, SettingsMenuActionApplyTheme
		case "q":
			return true, SettingsMenuActionQuit
		}
		return true, SettingsMenuActionNone
	default:
		switch msg.String() {
		case "esc":
			c.Close()
			return true, SettingsMenuActionNone
		case "up", "k":
			if c.selected > 0 {
				c.selected--
			}
			return true, SettingsMenuActionNone
		case "down", "j":
			if c.selected < len(c.items)-1 {
				c.selected++
			}
			return true, SettingsMenuActionNone
		case "enter":
			if len(c.items) == 0 || c.selected < 0 || c.selected >= len(c.items) {
				return true, SettingsMenuActionNone
			}
			item := c.items[c.selected]
			if item.Action != SettingsMenuActionNone {
				return true, item.Action
			}
			if item.Screen == settingsMenuScreenHelp {
				c.screen = settingsMenuScreenHelp
				c.helpOffset = 0
			}
			if item.Screen == settingsMenuScreenTheme {
				c.screen = settingsMenuScreenTheme
				c.SetSelectedThemeID(c.activeThemeID)
			}
			return true, SettingsMenuActionNone
		case "q":
			return true, SettingsMenuActionQuit
		}
		return true, SettingsMenuActionNone
	}
}

func (c *SettingsMenuController) SetThemeItems(items []SettingsThemeItem) {
	if c == nil {
		return
	}
	next := make([]SettingsThemeItem, 0, len(items))
	for _, item := range items {
		id := canonicalSettingsThemeID(item.ID)
		if id == "" {
			continue
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = strings.ToUpper(id)
		}
		next = append(next, SettingsThemeItem{ID: id, Title: title})
	}
	c.themeItems = next
	c.SetSelectedThemeID(c.activeThemeID)
}

func (c *SettingsMenuController) ThemeItems() []SettingsThemeItem {
	if c == nil {
		return nil
	}
	out := make([]SettingsThemeItem, 0, len(c.themeItems))
	out = append(out, c.themeItems...)
	return out
}

func (c *SettingsMenuController) ActiveThemeID() string {
	if c == nil {
		return settingsMenuDefaultThemeID
	}
	id := normalizeSettingsThemeID(c.activeThemeID)
	if id == "" {
		return settingsMenuDefaultThemeID
	}
	return id
}

func (c *SettingsMenuController) SetActiveThemeID(themeID string) {
	if c == nil {
		return
	}
	c.activeThemeID = normalizeSettingsThemeID(themeID)
	if c.activeThemeID == "" {
		c.activeThemeID = settingsMenuDefaultThemeID
	}
}

func (c *SettingsMenuController) SelectedThemeID() string {
	if c == nil {
		return ""
	}
	return c.selectedThemeID()
}

func (c *SettingsMenuController) SetSelectedThemeID(themeID string) {
	if c == nil || len(c.themeItems) == 0 {
		return
	}
	want := normalizeSettingsThemeID(themeID)
	for idx, item := range c.themeItems {
		if normalizeSettingsThemeID(item.ID) == want {
			c.themeSelected = idx
			return
		}
	}
	c.themeSelected = 0
}

func (c *SettingsMenuController) selectedThemeID() string {
	if c == nil || len(c.themeItems) == 0 {
		return ""
	}
	if c.themeSelected < 0 {
		c.themeSelected = 0
	}
	if c.themeSelected >= len(c.themeItems) {
		c.themeSelected = len(c.themeItems) - 1
	}
	return normalizeSettingsThemeID(c.themeItems[c.themeSelected].ID)
}

func normalizeSettingsThemeID(raw string) string {
	value := canonicalSettingsThemeID(raw)
	if value == "" {
		return settingsMenuDefaultThemeID
	}
	return value
}

func canonicalSettingsThemeID(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}
