package app

import (
	"fmt"
	"sort"
	"strings"

	"control/internal/types"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
)

type MenuAction int

const (
	MenuActionNone MenuAction = iota
	MenuActionCreateWorkspace
	MenuActionRenameWorkspace
	MenuActionDeleteWorkspace
	MenuActionEditWorkspaceGroups
	MenuActionCreateWorkspaceGroup
	MenuActionRenameWorkspaceGroup
	MenuActionDeleteWorkspaceGroup
	MenuActionAssignWorkspacesToGroup
)

const (
	menuActionCreate = iota
	menuActionRename
	menuActionDelete
	menuActionCount
)

type menuItem struct {
	ID    string
	Label string
}

type menuGroup struct {
	ID    string
	Label string
}

type submenuKind int

const (
	submenuNone submenuKind = iota
	submenuWorkspaces
	submenuWorkspaceGroups
)

type MenuController struct {
	items         []menuItem
	groups        []menuGroup
	selectedGroup map[string]bool
	menuIndex     int
	dropIndex     int
	submenuKind   submenuKind
	submenuIndex  int
	active        bool
	dropdownOpen  bool
}

func NewMenuController() *MenuController {
	return &MenuController{
		items:         []menuItem{{ID: "workspaces", Label: "Workspaces"}},
		groups:        []menuGroup{{ID: "ungrouped", Label: "Ungrouped"}},
		selectedGroup: map[string]bool{"ungrouped": true},
	}
}

func (m *MenuController) SetGroups(groups []*types.WorkspaceGroup) {
	if m == nil {
		return
	}
	next := make([]menuGroup, 0, 1+len(groups))
	next = append(next, menuGroup{ID: "ungrouped", Label: "Ungrouped"})
	for _, group := range groups {
		if group == nil {
			continue
		}
		label := strings.TrimSpace(group.Name)
		if label == "" {
			continue
		}
		next = append(next, menuGroup{ID: group.ID, Label: label})
	}
	if m.selectedGroup == nil {
		m.selectedGroup = map[string]bool{}
	}
	for id := range m.selectedGroup {
		if !groupIDExists(next, id) {
			delete(m.selectedGroup, id)
		}
	}
	m.groups = next
}

func (m *MenuController) SelectedGroupIDs() []string {
	if m == nil {
		return nil
	}
	out := make([]string, 0, len(m.selectedGroup))
	for id, selected := range m.selectedGroup {
		if selected {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (m *MenuController) SetSelectedGroupIDs(ids []string) {
	if m == nil {
		return
	}
	if m.selectedGroup == nil {
		m.selectedGroup = map[string]bool{}
	}
	for k := range m.selectedGroup {
		delete(m.selectedGroup, k)
	}
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		m.selectedGroup[trimmed] = true
	}
}

func (m *MenuController) IsOpen() bool {
	return m != nil && m.dropdownOpen
}

func (m *MenuController) IsActive() bool {
	return m != nil && m.active
}

func (m *MenuController) IsDropdownOpen() bool {
	return m != nil && m.dropdownOpen
}

func (m *MenuController) Toggle() {
	if m == nil {
		return
	}
	if m.active {
		m.CloseAll()
		return
	}
	m.OpenBar()
}

func (m *MenuController) OpenBar() {
	if m == nil {
		return
	}
	m.active = true
	if m.menuIndex < 0 {
		m.menuIndex = 0
	}
}

func (m *MenuController) OpenDropdown() {
	if m == nil {
		return
	}
	m.active = true
	m.dropdownOpen = true
	m.dropIndex = 0
	m.submenuKind = submenuNone
	m.submenuIndex = 0
}

func (m *MenuController) CloseDropdown() {
	if m == nil {
		return
	}
	m.dropdownOpen = false
	m.submenuKind = submenuNone
}

func (m *MenuController) OpenSubmenu(kind submenuKind) {
	if m == nil {
		return
	}
	m.submenuKind = kind
	m.submenuIndex = 0
}

func (m *MenuController) CloseSubmenu() {
	if m == nil {
		return
	}
	m.submenuKind = submenuNone
}

func (m *MenuController) CloseAll() {
	if m == nil {
		return
	}
	m.dropdownOpen = false
	m.submenuKind = submenuNone
	m.active = false
}

func (m *MenuController) HandleKey(msg tea.KeyMsg) (bool, MenuAction) {
	if m == nil || !m.active {
		return false, MenuActionNone
	}
	if m.submenuKind != submenuNone {
		maxIndex := m.submenuActionCount() - 1
		switch msg.String() {
		case "esc", "left", "h":
			m.CloseSubmenu()
			return true, MenuActionNone
		case "enter":
			return true, m.applySubmenuSelection()
		case "up", "k":
			if m.submenuIndex > 0 {
				m.submenuIndex--
			}
			return true, MenuActionNone
		case "down", "j":
			if m.submenuIndex < maxIndex {
				m.submenuIndex++
			}
			return true, MenuActionNone
		}
	}
	switch msg.String() {
	case "esc":
		m.CloseAll()
		return true, MenuActionNone
	case "enter":
		if !m.dropdownOpen {
			m.OpenDropdown()
			return true, MenuActionNone
		}
		return true, m.handleDropdownEnter()
	case "down":
		if !m.dropdownOpen {
			m.OpenDropdown()
			return true, MenuActionNone
		}
		if m.dropIndex < m.selectableCount()-1 {
			m.dropIndex++
		}
		return true, MenuActionNone
	case "up", "k":
		if m.dropdownOpen {
			if m.dropIndex > 0 {
				m.dropIndex--
			}
			return true, MenuActionNone
		}
		if m.menuIndex > 0 {
			m.menuIndex--
		}
		return true, MenuActionNone
	case "j":
		if m.dropdownOpen {
			if m.dropIndex < m.selectableCount()-1 {
				m.dropIndex++
			}
			return true, MenuActionNone
		}
		return true, MenuActionNone
	case " ", "space":
		if m.dropdownOpen && m.dropIndex >= 2 {
			m.toggleGroup(m.dropIndex - 2)
		}
		return true, MenuActionNone
	case "left", "h":
		if m.menuIndex > 0 {
			m.menuIndex--
		}
		return true, MenuActionNone
	case "right", "l":
		if m.menuIndex < len(m.items)-1 {
			m.menuIndex++
		}
		return true, MenuActionNone
	}
	return false, MenuActionNone
}

func (m *MenuController) HandleMouse(msg tea.MouseMsg, dropdownWidth int) (bool, MenuAction) {
	if m == nil {
		return false, MenuActionNone
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return false, MenuActionNone
	}
	if msg.Y == 0 {
		idx := m.menuItemIndexAt(msg.X)
		if idx >= 0 {
			m.menuIndex = idx
			m.OpenDropdown()
			return true, MenuActionNone
		}
		if m.active {
			m.CloseAll()
		}
		return false, MenuActionNone
	}
	if m.submenuKind != submenuNone && dropdownWidth > 0 && msg.X >= dropdownWidth+1 {
		row := msg.Y - 1
		if row < 0 {
			return true, MenuActionNone
		}
		actionIdx := row
		if actionIdx >= 0 && actionIdx < m.submenuActionCount() {
			m.submenuIndex = actionIdx
			return true, m.applySubmenuSelection()
		}
		return true, MenuActionNone
	}
	if !m.dropdownOpen {
		if m.active {
			m.CloseAll()
		}
		return false, MenuActionNone
	}
	if msg.Y > 0 && msg.Y <= m.DropdownHeight() {
		row := msg.Y - 1
		if row < 0 {
			return true, MenuActionNone
		}
		if row == 0 {
			m.dropIndex = 0
			m.OpenSubmenu(submenuWorkspaces)
			return true, MenuActionNone
		}
		if row == 1 {
			m.dropIndex = 1
			m.OpenSubmenu(submenuWorkspaceGroups)
			return true, MenuActionNone
		}
		groupRow := row - 2
		if groupRow < 0 {
			return true, MenuActionNone
		}
		m.dropIndex = 2 + clamp(groupRow, 0, max(0, len(m.groups)-1))
		if len(m.groups) > 0 {
			m.toggleGroup(groupRow)
		}
		return true, MenuActionNone
	}
	if m.active {
		m.CloseAll()
	}
	return false, MenuActionNone
}

func (m *MenuController) MenuBarView(width int) string {
	if m == nil || width <= 0 {
		return ""
	}
	parts := make([]string, 0, len(m.items))
	for i, item := range m.items {
		label := " " + item.Label + " "
		if m.active && i == m.menuIndex {
			label = menuBarActiveStyle.Render(label)
		} else {
			label = menuBarStyle.Render(label)
		}
		parts = append(parts, label)
	}
	line := strings.Join(parts, " ")
	return menuBarStyle.Width(width).Render(line)
}

func (m *MenuController) DropdownView(width int) string {
	if m == nil || !m.dropdownOpen {
		return ""
	}
	if width <= 0 {
		width = max(minListWidth, minViewportWidth)
	}
	lines := []string{}
	workspacesLine := " Workspaces ▶"
	if m.dropIndex == 0 {
		workspacesLine = selectedStyle.Render(workspacesLine)
	}
	lines = append(lines, workspacesLine)
	groupMenuLine := " Workspace Groups ▶"
	if m.dropIndex == 1 {
		groupMenuLine = selectedStyle.Render(groupMenuLine)
	}
	lines = append(lines, groupMenuLine)
	if len(m.groups) == 0 {
		line := " (no groups)"
		if m.dropIndex == 2 {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
		return menuDropStyle.Width(width).Render(strings.Join(lines, "\n"))
	}
	for i, group := range m.groups {
		selected := m.selectedGroup[group.ID]
		checkbox := "[ ]"
		if selected {
			checkbox = "[x]"
		}
		line := fmt.Sprintf(" %s %s", checkbox, group.Label)
		if m.dropIndex == 2+i {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return menuDropStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m *MenuController) DropdownHeight() int {
	if m == nil || !m.dropdownOpen {
		return 0
	}
	groupCount := len(m.groups)
	if groupCount == 0 {
		groupCount = 1
	}
	return 2 + groupCount
}

func (m *MenuController) selectableCount() int {
	if m == nil {
		return 0
	}
	count := 2 + len(m.groups)
	if len(m.groups) == 0 {
		count = 3
	}
	return count
}

func (m *MenuController) menuItemIndexAt(x int) int {
	if m == nil || len(m.items) == 0 || x < 0 {
		return -1
	}
	pos := 0
	for i, item := range m.items {
		label := " " + item.Label + " "
		width := xansi.StringWidth(label)
		if x >= pos && x < pos+width {
			return i
		}
		pos += width + 1
	}
	return -1
}

func (m *MenuController) handleDropdownEnter() MenuAction {
	if m.dropIndex == 0 {
		m.OpenSubmenu(submenuWorkspaces)
		return MenuActionNone
	}
	if m.dropIndex == 1 {
		m.OpenSubmenu(submenuWorkspaceGroups)
		return MenuActionNone
	}
	idx := m.dropIndex - 2
	if idx >= 0 && idx < len(m.groups) {
		m.toggleGroup(idx)
	}
	return MenuActionNone
}

func (m *MenuController) SubmenuView(width int) string {
	if m == nil || m.submenuKind == submenuNone {
		return ""
	}
	labels := m.submenuLabels()
	maxLen := 0
	for _, label := range labels {
		if len(label) > maxLen {
			maxLen = len(label)
		}
	}
	if width <= 0 {
		width = max(12, maxLen+2)
	}
	lines := make([]string, 0, len(labels))
	for i, label := range labels {
		line := " " + label
		if m.submenuIndex == i {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return menuDropStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m *MenuController) SubmenuHeight() int {
	if m == nil || m.submenuKind == submenuNone {
		return 0
	}
	return m.submenuActionCount()
}

func (m *MenuController) HasSubmenu() bool {
	return m != nil && m.submenuKind != submenuNone
}

func (m *MenuController) applySubmenuSelection() MenuAction {
	switch m.submenuKind {
	case submenuWorkspaces:
		switch m.submenuIndex {
		case menuActionCreate:
			return MenuActionCreateWorkspace
		case menuActionRename:
			return MenuActionRenameWorkspace
		case menuActionDelete:
			return MenuActionDeleteWorkspace
		case 3:
			return MenuActionEditWorkspaceGroups
		}
	case submenuWorkspaceGroups:
		switch m.submenuIndex {
		case menuActionCreate:
			return MenuActionCreateWorkspaceGroup
		case menuActionRename:
			return MenuActionRenameWorkspaceGroup
		case menuActionDelete:
			return MenuActionDeleteWorkspaceGroup
		case 3:
			return MenuActionAssignWorkspacesToGroup
		}
	}
	return MenuActionNone
}

func (m *MenuController) submenuLabels() []string {
	switch m.submenuKind {
	case submenuWorkspaceGroups:
		return []string{"Create Group", "Rename Group", "Delete Group", "Assign Workspaces"}
	case submenuWorkspaces:
		fallthrough
	default:
		return []string{"Create Workspace", "Rename Workspace", "Delete Workspace", "Edit Workspace Groups"}
	}
}

func (m *MenuController) submenuActionCount() int {
	return len(m.submenuLabels())
}

func groupIDExists(groups []menuGroup, id string) bool {
	for _, group := range groups {
		if group.ID == id {
			return true
		}
	}
	return false
}

func (m *MenuController) toggleGroup(index int) {
	if m == nil || index < 0 || index >= len(m.groups) {
		return
	}
	group := m.groups[index]
	if group.ID == "" {
		return
	}
	if m.selectedGroup == nil {
		m.selectedGroup = map[string]bool{}
	}
	m.selectedGroup[group.ID] = !m.selectedGroup[group.ID]
}
