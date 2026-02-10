package app

import (
	"math"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"control/internal/types"
)

type SidebarController struct {
	list     list.Model
	delegate *sidebarDelegate
	selected map[string]struct{}
}

const sidebarScrollbarWidth = 1
const sidebarScrollingEnabled = false

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
	view := c.list.View()
	if !sidebarScrollingEnabled {
		return view
	}
	bar := c.scrollbarView()
	if bar == "" {
		return view
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, view, bar)
}

func (c *SidebarController) Update(msg tea.Msg) tea.Cmd {
	updated, cmd := c.list.Update(msg)
	c.list = updated
	return cmd
}

func (c *SidebarController) SetSize(width, height int) {
	if !sidebarScrollingEnabled {
		c.list.SetSize(width, height)
		return
	}
	if width <= sidebarScrollbarWidth {
		c.list.SetSize(width, height)
		return
	}
	c.list.SetSize(width-sidebarScrollbarWidth, height)
}

func (c *SidebarController) CursorDown() {
	c.list.CursorDown()
}

func (c *SidebarController) CursorUp() {
	c.list.CursorUp()
}

func (c *SidebarController) Scroll(lines int) bool {
	if !sidebarScrollingEnabled {
		return false
	}
	if lines == 0 {
		return false
	}
	steps := lines
	if steps < 0 {
		steps = -steps
	}
	for i := 0; i < steps; i++ {
		if lines < 0 {
			c.list.CursorUp()
		} else {
			c.list.CursorDown()
		}
	}
	return true
}

func (c *SidebarController) Select(idx int) {
	c.list.Select(idx)
}

func (c *SidebarController) SelectBySessionID(id string) bool {
	if id == "" {
		return false
	}
	for i, item := range c.list.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.session == nil {
			continue
		}
		if entry.session.ID == id {
			c.list.Select(i)
			return true
		}
	}
	return false
}

func (c *SidebarController) SelectByRow(row int) {
	if row < 0 {
		return
	}
	headerRows := c.headerRows()
	idx := row - headerRows
	if idx < 0 {
		return
	}
	items := c.list.VisibleItems()
	if len(items) == 0 {
		return
	}
	itemHeight := 1
	itemSpacing := 0
	if c.delegate != nil {
		if h := c.delegate.Height(); h > 0 {
			itemHeight = h
		}
		itemSpacing = c.delegate.Spacing()
	}
	step := itemHeight + itemSpacing
	if step <= 0 {
		step = 1
	}
	pageIndex := idx / step
	perPage := c.list.Paginator.PerPage
	if perPage <= 0 {
		perPage = len(items)
	}
	start := c.list.Paginator.Page * perPage
	if start >= len(items) {
		start = 0
	}
	end := start + perPage - 1
	if end >= len(items) {
		end = len(items) - 1
	}
	target := start + pageIndex
	if target > end {
		target = end
	}
	if target < 0 {
		target = 0
	}
	c.list.Select(target)
}

func (c *SidebarController) ItemAtRow(row int) *sidebarItem {
	if row < 0 {
		return nil
	}
	headerRows := c.headerRows()
	idx := row - headerRows
	if idx < 0 {
		return nil
	}
	items := c.list.VisibleItems()
	if len(items) == 0 {
		return nil
	}
	itemHeight := 1
	itemSpacing := 0
	if c.delegate != nil {
		if h := c.delegate.Height(); h > 0 {
			itemHeight = h
		}
		itemSpacing = c.delegate.Spacing()
	}
	step := itemHeight + itemSpacing
	if step <= 0 {
		step = 1
	}
	pageIndex := idx / step
	perPage := c.list.Paginator.PerPage
	if perPage <= 0 {
		perPage = len(items)
	}
	start := c.list.Paginator.Page * perPage
	if start >= len(items) {
		start = 0
	}
	end := start + perPage - 1
	if end >= len(items) {
		end = len(items) - 1
	}
	target := start + pageIndex
	if target > end {
		target = end
	}
	if target < 0 {
		target = 0
	}
	entry, ok := items[target].(*sidebarItem)
	if !ok {
		return nil
	}
	return entry
}

func (c *SidebarController) ScrollbarWidth() int {
	if !sidebarScrollingEnabled {
		return 0
	}
	return sidebarScrollbarWidth
}

func (c *SidebarController) ScrollbarSelect(row int) bool {
	if !sidebarScrollingEnabled {
		return false
	}
	if c == nil {
		return false
	}
	height := c.list.Height()
	if height <= 0 {
		return false
	}
	headerRows := c.headerRows()
	trackHeight := height - headerRows
	if trackHeight <= 0 {
		return false
	}
	total := len(c.list.VisibleItems())
	if total == 0 {
		return false
	}
	itemsOnPage := c.list.Paginator.PerPage
	if itemsOnPage <= 0 || itemsOnPage > total {
		itemsOnPage = total
	}
	maxStart := total - itemsOnPage
	if maxStart <= 0 {
		return false
	}
	trackRow := row - headerRows
	if trackRow < 0 {
		trackRow = 0
	}
	if trackRow >= trackHeight {
		trackRow = trackHeight - 1
	}
	targetStart := 0
	denom := trackHeight - 1
	if denom > 0 {
		targetStart = int(math.Round(float64(trackRow) / float64(denom) * float64(maxStart*3)))
	}
	if targetStart < 0 {
		targetStart = 0
	}
	if targetStart > maxStart {
		targetStart = maxStart
	}
	c.list.Select(targetStart)
	return true
}

func (c *SidebarController) scrollbarView() string {
	if !sidebarScrollingEnabled {
		return ""
	}
	if c == nil {
		return ""
	}
	height := c.list.Height()
	if height <= 0 {
		return ""
	}
	total := len(c.list.VisibleItems())
	itemsOnPage := c.list.Paginator.ItemsOnPage(total)
	if itemsOnPage <= 0 {
		itemsOnPage = total
	}
	headerRows := c.headerRows()
	trackHeight := height - headerRows
	if trackHeight < 1 {
		trackHeight = 1
	}

	barLines := make([]string, 0, height)
	for i := 0; i < headerRows && i < height; i++ {
		barLines = append(barLines, strings.Repeat(" ", sidebarScrollbarWidth))
	}

	if total <= itemsOnPage || total == 0 || trackHeight <= 0 {
		for i := 0; i < trackHeight; i++ {
			barLines = append(barLines, strings.Repeat(" ", sidebarScrollbarWidth))
		}
		return scrollbarTrackStyle.Render(strings.Join(barLines, "\n"))
	}

	thumbHeight := int(math.Round(float64(itemsOnPage) / float64(total) * float64(trackHeight)))
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	if thumbHeight > trackHeight {
		thumbHeight = trackHeight
	}
	maxStart := trackHeight - thumbHeight
	if maxStart < 0 {
		maxStart = 0
	}
	startIdx, _ := c.list.Paginator.GetSliceBounds(total)
	denom := total - itemsOnPage
	startPos := 0
	if denom > 0 && maxStart > 0 {
		startPos = int(math.Round(float64(startIdx) / float64(denom) * float64(maxStart)))
	}
	if startPos < 0 {
		startPos = 0
	}
	if startPos > maxStart {
		startPos = maxStart
	}

	for i := 0; i < trackHeight; i++ {
		if i >= startPos && i < startPos+thumbHeight {
			barLines = append(barLines, scrollbarThumbStyle.Render(strings.Repeat("┃", sidebarScrollbarWidth)))
		} else {
			barLines = append(barLines, scrollbarTrackStyle.Render(strings.Repeat("│", sidebarScrollbarWidth)))
		}
	}
	return strings.Join(barLines, "\n")
}

func (c *SidebarController) headerRows() int {
	if c == nil {
		return 0
	}
	rows := 0
	if c.list.ShowTitle() {
		rows += 1 + c.list.Styles.TitleBar.GetPaddingTop() + c.list.Styles.TitleBar.GetPaddingBottom()
	}
	if c.list.ShowStatusBar() {
		rows += 1 + c.list.Styles.StatusBar.GetPaddingTop() + c.list.Styles.StatusBar.GetPaddingBottom()
	}
	if c.list.ShowPagination() {
		rows++
	}
	if c.list.ShowHelp() {
		rows += 1 + c.list.Styles.HelpStyle.GetPaddingTop() + c.list.Styles.HelpStyle.GetPaddingBottom()
	}
	return rows
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
