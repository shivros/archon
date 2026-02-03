package app

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"control/internal/types"
)

type SidebarController struct {
	list     list.Model
	delegate *sidebarDelegate
	selected map[string]struct{}
}

func NewSidebarController() *SidebarController {
	items := []list.Item{}
	delegate := &sidebarDelegate{}
	mlist := list.New(items, delegate, minListWidth, minContentHeight)
	mlist.Title = "Workspaces"
	mlist.SetShowHelp(false)
	mlist.SetFilteringEnabled(false)
	mlist.SetShowPagination(false)
	mlist.SetShowStatusBar(false)
	mlist.Styles.Title = headerStyle
	return &SidebarController{
		list:     mlist,
		delegate: delegate,
		selected: map[string]struct{}{},
	}
}

func (c *SidebarController) View() string {
	return c.list.View()
}

func (c *SidebarController) Update(msg tea.Msg) tea.Cmd {
	updated, cmd := c.list.Update(msg)
	c.list = updated
	return cmd
}

func (c *SidebarController) SetSize(width, height int) {
	c.list.SetSize(width, height)
}

func (c *SidebarController) CursorDown() {
	c.list.CursorDown()
}

func (c *SidebarController) CursorUp() {
	c.list.CursorUp()
}

func (c *SidebarController) Select(idx int) {
	c.list.Select(idx)
}

func (c *SidebarController) Index() int {
	return c.list.Index()
}

func (c *SidebarController) Items() []list.Item {
	return c.list.Items()
}

func (c *SidebarController) SelectedItem() *sidebarItem {
	item := c.list.SelectedItem()
	if item == nil {
		return nil
	}
	entry, ok := item.(*sidebarItem)
	if !ok {
		return nil
	}
	return entry
}

func (c *SidebarController) SelectedKey() string {
	item := c.SelectedItem()
	if item == nil {
		return ""
	}
	return item.key()
}

func (c *SidebarController) SelectedSessionID() string {
	item := c.SelectedItem()
	if item == nil || !item.isSession() {
		return ""
	}
	return item.session.ID
}

func (c *SidebarController) SelectionCount() int {
	return len(c.selected)
}

func (c *SidebarController) SelectedSessionIDs() []string {
	if len(c.selected) == 0 {
		if id := c.SelectedSessionID(); id != "" {
			return []string{id}
		}
		return nil
	}
	var ids []string
	for _, item := range c.list.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.session == nil {
			continue
		}
		if _, ok := c.selected[entry.session.ID]; ok {
			ids = append(ids, entry.session.ID)
		}
	}
	return ids
}

func (c *SidebarController) ToggleSelection() bool {
	item := c.SelectedItem()
	if item == nil || !item.isSession() || item.session == nil {
		return false
	}
	id := item.session.ID
	if _, ok := c.selected[id]; ok {
		delete(c.selected, id)
	} else {
		c.selected[id] = struct{}{}
	}
	c.syncDelegate()
	return true
}

func (c *SidebarController) RemoveSelection(ids []string) {
	if len(ids) == 0 {
		return
	}
	for _, id := range ids {
		delete(c.selected, id)
	}
	c.syncDelegate()
}

func (c *SidebarController) PruneSelection(sessions []*types.Session) {
	keep := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		keep[session.ID] = struct{}{}
	}
	for id := range c.selected {
		if _, ok := keep[id]; !ok {
			delete(c.selected, id)
		}
	}
	c.syncDelegate()
}

func (c *SidebarController) AdvanceToNextSession() bool {
	items := c.list.Items()
	if len(items) == 0 {
		return false
	}
	start := c.list.Index() + 1
	for i := start; i < len(items); i++ {
		entry, ok := items[i].(*sidebarItem)
		if !ok || entry == nil || !entry.isSession() {
			continue
		}
		c.list.Select(i)
		return true
	}
	return false
}

func (c *SidebarController) Apply(workspaces []*types.Workspace, worktrees map[string][]*types.Worktree, sessions []*types.Session, meta map[string]*types.SessionMeta, activeWorkspaceID, activeWorktreeID string) *sidebarItem {
	items := buildSidebarItems(workspaces, worktrees, sessions, meta)
	selectedKey := c.SelectedKey()
	c.list.SetItems(items)
	if len(items) == 0 {
		return nil
	}
	selectedIdx := selectSidebarIndex(items, selectedKey, activeWorkspaceID, activeWorktreeID)
	c.list.Select(selectedIdx)
	return c.SelectedItem()
}

func (c *SidebarController) SetActive(activeWorkspaceID, activeWorktreeID string) {
	if c.delegate != nil {
		c.delegate.activeWorkspaceID = activeWorkspaceID
		c.delegate.activeWorktreeID = activeWorktreeID
	}
	c.syncDelegate()
}

func (c *SidebarController) syncDelegate() {
	if c.delegate != nil {
		c.delegate.selectedSessions = c.selected
	}
}

func selectSidebarIndex(items []list.Item, selectedKey, activeWorkspaceID, activeWorktreeID string) int {
	if len(items) == 0 {
		return 0
	}
	if selectedKey != "" {
		for i, item := range items {
			entry, ok := item.(*sidebarItem)
			if !ok {
				continue
			}
			if entry.key() == selectedKey {
				return i
			}
		}
	}
	if activeWorktreeID != "" {
		for i, item := range items {
			entry, ok := item.(*sidebarItem)
			if !ok || entry.kind != sidebarSession || entry.meta == nil {
				continue
			}
			if entry.meta.WorktreeID == activeWorktreeID {
				return i
			}
		}
		for i, item := range items {
			entry, ok := item.(*sidebarItem)
			if !ok || entry.kind != sidebarWorktree || entry.worktree == nil {
				continue
			}
			if entry.worktree.ID == activeWorktreeID {
				return i
			}
		}
	}
	if activeWorkspaceID != "" {
		for i, item := range items {
			entry, ok := item.(*sidebarItem)
			if !ok || entry.kind != sidebarSession {
				continue
			}
			if entry.workspaceID() == activeWorkspaceID {
				return i
			}
		}
		for i, item := range items {
			entry, ok := item.(*sidebarItem)
			if !ok || entry.kind != sidebarWorkspace {
				continue
			}
			if entry.workspaceID() == activeWorkspaceID {
				return i
			}
		}
	}
	for i, item := range items {
		entry, ok := item.(*sidebarItem)
		if ok && entry.kind == sidebarSession {
			return i
		}
	}
	return 0
}
