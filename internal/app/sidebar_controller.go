package app

import (
	"math"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type SidebarController struct {
	list                  list.Model
	delegate              *sidebarDelegate
	selectedKey           string
	scrollOffset          int
	viewedSessionActivity map[string]string
	unreadSessions        map[string]struct{}
	unreadInitialized     bool
	expandByDefault       bool
	workspaceExpanded     map[string]bool
	worktreeExpanded      map[string]bool
	workflowExpanded      map[string]bool
	workspacesSnapshot    []*types.Workspace
	worktreesSnapshot     map[string][]*types.Worktree
	sessionsSnapshot      []*types.Session
	workflowRunsSnapshot  []*guidedworkflows.WorkflowRun
	metaSnapshot          map[string]*types.SessionMeta
	showDismissedSnapshot bool
	sessionParents        map[string]sessionSidebarParent
	recentsState          sidebarRecentsState
}

type sessionSidebarParent struct {
	workspaceID string
	worktreeID  string
	workflowID  string
}

const sidebarScrollbarWidth = 1
const sidebarScrollingEnabled = true

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
		list:                  mlist,
		delegate:              delegate,
		viewedSessionActivity: map[string]string{},
		unreadSessions:        map[string]struct{}{},
		expandByDefault:       true,
		workspaceExpanded:     map[string]bool{},
		worktreeExpanded:      map[string]bool{},
		workflowExpanded:      map[string]bool{},
		sessionParents:        map[string]sessionSidebarParent{},
	}
}

func (c *SidebarController) View() string {
	view := c.view()
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
	c.markSelectedSessionViewed()
	return cmd
}

func (c *SidebarController) SetSize(width, height int) {
	if !sidebarScrollingEnabled {
		c.list.SetSize(width, height)
		c.clampScrollOffset()
		return
	}
	if width <= sidebarScrollbarWidth {
		c.list.SetSize(width, height)
		c.clampScrollOffset()
		return
	}
	c.list.SetSize(width-sidebarScrollbarWidth, height)
	c.clampScrollOffset()
}

func (c *SidebarController) CursorDown() {
	items := c.list.Items()
	if len(items) == 0 {
		return
	}
	current := c.selectedIndex()
	if current < 0 {
		c.selectIndex(0)
		return
	}
	if current >= len(items)-1 {
		return
	}
	c.selectIndex(current + 1)
}

func (c *SidebarController) CursorUp() {
	items := c.list.Items()
	if len(items) == 0 {
		return
	}
	current := c.selectedIndex()
	if current < 0 {
		c.selectIndex(0)
		return
	}
	if current <= 0 {
		return
	}
	c.selectIndex(current - 1)
}

func (c *SidebarController) Scroll(lines int) bool {
	if !sidebarScrollingEnabled {
		return false
	}
	if lines == 0 {
		return false
	}
	return c.scrollTo(c.scrollOffset + lines)
}

func (c *SidebarController) Select(idx int) {
	c.selectIndex(idx)
}

func (c *SidebarController) SelectBySessionID(id string) bool {
	if id == "" {
		return false
	}
	if c.selectVisibleSessionByID(id) {
		return true
	}
	parent, ok := c.sessionParents[id]
	if !ok {
		if !c.canSelectSessionID(id) {
			return false
		}
		parent = sessionSidebarParent{workspaceID: unassignedWorkspaceID}
	}
	if strings.TrimSpace(parent.workspaceID) == "" {
		parent.workspaceID = unassignedWorkspaceID
	}
	changed := false
	if parent.workspaceID != "" {
		changed = c.setWorkspaceExpanded(parent.workspaceID, true) || changed
	}
	if parent.worktreeID != "" {
		changed = c.setWorktreeExpanded(parent.worktreeID, true) || changed
	}
	if parent.workflowID != "" {
		changed = c.setWorkflowExpanded(parent.workflowID, true) || changed
	}
	if changed {
		c.rebuild(c.SelectedKey())
	}
	return c.selectVisibleSessionByID(id)
}

func (c *SidebarController) SelectByWorktreeID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if c.selectVisibleWorktreeByID(id) {
		return true
	}
	workspaceID, ok := c.findWorkspaceIDForWorktree(id)
	if !ok {
		return false
	}
	if c.setWorkspaceExpanded(workspaceID, true) {
		c.rebuild(c.SelectedKey())
	}
	return c.selectVisibleWorktreeByID(id)
}

func (c *SidebarController) SelectByKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if c.selectVisibleByKey(key) {
		return true
	}
	if id := sessionIDFromSidebarKey(key); id != "" {
		return c.SelectBySessionID(id)
	}
	if strings.HasPrefix(key, "wt:") {
		return c.SelectByWorktreeID(strings.TrimSpace(strings.TrimPrefix(key, "wt:")))
	}
	return false
}

func (c *SidebarController) CanSelectKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if sidebarItemsContainKey(c.list.Items(), key) {
		return true
	}
	if id := sessionIDFromSidebarKey(key); id != "" {
		return c.canSelectSessionID(id)
	}
	if strings.HasPrefix(key, "wt:") {
		return c.canSelectWorktreeID(strings.TrimSpace(strings.TrimPrefix(key, "wt:")))
	}
	return false
}

func (c *SidebarController) selectVisibleSessionByID(id string) bool {
	for i, item := range c.list.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.session == nil {
			continue
		}
		if entry.session.ID == id {
			c.selectIndex(i)
			return true
		}
	}
	return false
}

func (c *SidebarController) selectVisibleWorktreeByID(id string) bool {
	for i, item := range c.list.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.worktree == nil {
			continue
		}
		if entry.worktree.ID == id {
			c.selectIndex(i)
			return true
		}
	}
	return false
}

func (c *SidebarController) selectVisibleByKey(key string) bool {
	for i, item := range c.list.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil {
			continue
		}
		if entry.key() == key {
			c.selectIndex(i)
			return true
		}
	}
	return false
}

func (c *SidebarController) canSelectSessionID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, session := range c.sessionsSnapshot {
		if session == nil {
			continue
		}
		if strings.TrimSpace(session.ID) == id {
			return true
		}
	}
	return false
}

func (c *SidebarController) canSelectWorktreeID(id string) bool {
	_, ok := c.findWorkspaceIDForWorktree(id)
	return ok
}

func (c *SidebarController) findWorkspaceIDForWorktree(worktreeID string) (string, bool) {
	worktreeID = strings.TrimSpace(worktreeID)
	if worktreeID == "" {
		return "", false
	}
	visibleWorkspaces := map[string]struct{}{}
	for _, ws := range c.workspacesSnapshot {
		if ws == nil {
			continue
		}
		id := strings.TrimSpace(ws.ID)
		if id == "" {
			continue
		}
		visibleWorkspaces[id] = struct{}{}
	}
	for workspaceID, worktrees := range c.worktreesSnapshot {
		workspaceID = strings.TrimSpace(workspaceID)
		if workspaceID == "" {
			continue
		}
		if _, ok := visibleWorkspaces[workspaceID]; !ok {
			continue
		}
		for _, worktree := range worktrees {
			if worktree == nil {
				continue
			}
			if strings.TrimSpace(worktree.ID) == worktreeID {
				return workspaceID, true
			}
		}
	}
	return "", false
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
	itemsOnPage := c.itemsPerView()
	if itemsOnPage <= 0 {
		itemsOnPage = len(items)
	}
	start := c.scrollOffset
	end := start + itemsOnPage - 1
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
	c.selectIndex(target)
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
	itemsOnPage := c.itemsPerView()
	if itemsOnPage <= 0 {
		itemsOnPage = len(items)
	}
	start := c.scrollOffset
	end := start + itemsOnPage - 1
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
		targetStart = int(math.Round(float64(trackRow) / float64(denom) * float64(maxStart)))
	}
	return c.scrollTo(targetStart)
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
	itemsOnPage := c.itemsPerView()
	if itemsOnPage <= 0 || itemsOnPage > total {
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
	startIdx := c.scrollOffset
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
		rows += lipgloss.Height(c.list.Styles.TitleBar.Render(c.list.Styles.Title.Render(c.list.Title)))
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
	if c == nil {
		return nil
	}
	if c.selectedKey == "" {
		return nil
	}
	for _, item := range c.list.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil {
			continue
		}
		if entry.key() == c.selectedKey {
			return entry
		}
	}
	return nil
}

func (c *SidebarController) SelectedKey() string {
	if c == nil {
		return ""
	}
	return c.selectedKey
}

func (c *SidebarController) SelectedSessionID() string {
	item := c.SelectedItem()
	if item == nil || !item.isSession() {
		return ""
	}
	return item.session.ID
}

func (c *SidebarController) AdvanceToNextSession() bool {
	items := c.list.Items()
	if len(items) == 0 {
		return false
	}
	start := c.selectedIndex() + 1
	if start < 0 {
		start = 0
	}
	for i := start; i < len(items); i++ {
		entry, ok := items[i].(*sidebarItem)
		if !ok || entry == nil || !entry.isSession() {
			continue
		}
		c.selectIndex(i)
		return true
	}
	return false
}

func (c *SidebarController) SetExpandByDefault(enabled bool) bool {
	if c == nil {
		return false
	}
	if c.expandByDefault == enabled {
		return false
	}
	c.expandByDefault = enabled
	c.rebuild(c.SelectedKey())
	return true
}

func (c *SidebarController) SetExpansionOverrides(workspaceExpanded, worktreeExpanded, workflowExpanded map[string]bool) {
	if c == nil {
		return
	}
	c.workspaceExpanded = cloneBoolMap(workspaceExpanded)
	c.worktreeExpanded = cloneBoolMap(worktreeExpanded)
	c.workflowExpanded = cloneBoolMap(workflowExpanded)
	c.rebuild(c.SelectedKey())
}

func (c *SidebarController) ExpansionOverrides() (map[string]bool, map[string]bool, map[string]bool) {
	if c == nil {
		return nil, nil, nil
	}
	return cloneBoolMap(c.workspaceExpanded), cloneBoolMap(c.worktreeExpanded), cloneBoolMap(c.workflowExpanded)
}

func (c *SidebarController) ToggleSelectedContainer() bool {
	item := c.SelectedItem()
	if item == nil || !item.collapsible {
		return false
	}
	switch item.kind {
	case sidebarWorkspace:
		if item.workspace == nil {
			return false
		}
		return c.SetWorkspaceExpanded(item.workspace.ID, !item.expanded)
	case sidebarWorktree:
		if item.worktree == nil {
			return false
		}
		return c.SetWorktreeExpanded(item.worktree.ID, !item.expanded)
	case sidebarWorkflow:
		runID := item.workflowRunID()
		if runID == "" {
			return false
		}
		return c.SetWorkflowExpanded(runID, !item.expanded)
	default:
		return false
	}
}

func (c *SidebarController) SetSelectedContainerExpanded(expanded bool) bool {
	item := c.SelectedItem()
	if item == nil || !item.collapsible {
		return false
	}
	switch item.kind {
	case sidebarWorkspace:
		if item.workspace == nil {
			return false
		}
		return c.SetWorkspaceExpanded(item.workspace.ID, expanded)
	case sidebarWorktree:
		if item.worktree == nil {
			return false
		}
		return c.SetWorktreeExpanded(item.worktree.ID, expanded)
	case sidebarWorkflow:
		runID := item.workflowRunID()
		if runID == "" {
			return false
		}
		return c.SetWorkflowExpanded(runID, expanded)
	default:
		return false
	}
}

func (c *SidebarController) SetWorkspaceExpanded(id string, expanded bool) bool {
	if c == nil || strings.TrimSpace(id) == "" {
		return false
	}
	if !c.setWorkspaceExpanded(id, expanded) {
		return false
	}
	c.rebuild(c.SelectedKey())
	return true
}

func (c *SidebarController) SetWorktreeExpanded(id string, expanded bool) bool {
	if c == nil || strings.TrimSpace(id) == "" {
		return false
	}
	if !c.setWorktreeExpanded(id, expanded) {
		return false
	}
	c.rebuild(c.SelectedKey())
	return true
}

func (c *SidebarController) SetWorkflowExpanded(id string, expanded bool) bool {
	if c == nil || strings.TrimSpace(id) == "" {
		return false
	}
	if !c.setWorkflowExpanded(id, expanded) {
		return false
	}
	c.rebuild(c.SelectedKey())
	return true
}

func (c *SidebarController) IsWorkspaceExpanded(id string) bool {
	if c == nil || strings.TrimSpace(id) == "" {
		return c != nil && c.expandByDefault
	}
	if value, ok := c.workspaceExpanded[id]; ok {
		return value
	}
	return c.expandByDefault
}

func (c *SidebarController) IsWorktreeExpanded(id string) bool {
	if c == nil || strings.TrimSpace(id) == "" {
		return c != nil && c.expandByDefault
	}
	if value, ok := c.worktreeExpanded[id]; ok {
		return value
	}
	return c.expandByDefault
}

func (c *SidebarController) IsWorkflowExpanded(id string) bool {
	if c == nil || strings.TrimSpace(id) == "" {
		return c != nil && c.expandByDefault
	}
	if value, ok := c.workflowExpanded[id]; ok {
		return value
	}
	return c.expandByDefault
}

func (c *SidebarController) Apply(workspaces []*types.Workspace, worktrees map[string][]*types.Worktree, sessions []*types.Session, workflowRuns []*guidedworkflows.WorkflowRun, meta map[string]*types.SessionMeta, activeWorkspaceID, activeWorktreeID string, showDismissed bool) *sidebarItem {
	viewAnchorKey := c.viewAnchorKey()
	viewAnchorOffset := c.scrollOffset
	c.workspacesSnapshot = workspaces
	c.worktreesSnapshot = worktrees
	c.sessionsSnapshot = sessions
	c.workflowRunsSnapshot = workflowRuns
	c.metaSnapshot = meta
	c.showDismissedSnapshot = showDismissed
	c.sessionParents = buildSessionSidebarParents(sessions, meta)
	c.pruneExpansionOverrides(workspaces, worktrees, workflowRuns, sessions, meta)

	items := buildSidebarItemsWithRecents(workspaces, worktrees, sessions, workflowRuns, meta, showDismissed, c.recentsState, sidebarExpansionResolver{
		workspace: c.workspaceExpanded,
		worktree:  c.worktreeExpanded,
		workflow:  c.workflowExpanded,
		defaultOn: c.expandByDefault,
	})
	selectedKey := c.SelectedKey()
	c.list.SetItems(items)
	if len(items) == 0 {
		c.selectedKey = ""
		c.scrollOffset = 0
		c.syncDelegate()
		c.updateUnreadSessions(sessions, meta)
		return nil
	}
	if !sidebarItemsContainKey(items, selectedKey) {
		selectedIdx := selectSidebarIndex(items, selectedKey, activeWorkspaceID, activeWorktreeID)
		if selectedIdx < 0 || selectedIdx >= len(items) {
			selectedIdx = 0
		}
		if entry, ok := items[selectedIdx].(*sidebarItem); ok && entry != nil {
			c.selectedKey = entry.key()
		}
	}
	if !c.restoreScrollAnchor(items, viewAnchorKey, viewAnchorOffset) {
		c.clampScrollOffset()
	}
	c.syncDelegate()
	c.updateUnreadSessions(sessions, meta)
	return c.SelectedItem()
}

func (c *SidebarController) SetActive(activeWorkspaceID, activeWorktreeID string) {
	if c.delegate != nil {
		c.delegate.activeWorkspaceID = activeWorkspaceID
		c.delegate.activeWorktreeID = activeWorktreeID
	}
	c.syncDelegate()
}

func (c *SidebarController) SetProviderBadges(overrides map[string]*types.ProviderBadgeConfig) {
	if c == nil || c.delegate == nil {
		return
	}
	c.delegate.providerBadges = normalizeProviderBadgeOverrides(overrides)
}

func (c *SidebarController) SetRecentsState(state sidebarRecentsState) bool {
	if c == nil {
		return false
	}
	if c.recentsState == state {
		return false
	}
	c.recentsState = state
	c.rebuild(c.SelectedKey())
	return true
}

func (c *SidebarController) syncDelegate() {
	if c.delegate != nil {
		c.delegate.unreadSessions = c.unreadSessions
		c.delegate.selectedKey = c.selectedKey
	}
}

func (c *SidebarController) markSelectedSessionViewed() {
	if c == nil {
		return
	}
	id, marker := c.selectedSessionActivity()
	if id == "" || marker == "" {
		return
	}
	c.viewedSessionActivity[id] = marker
	delete(c.unreadSessions, id)
	c.syncDelegate()
}

func (c *SidebarController) updateUnreadSessions(sessions []*types.Session, meta map[string]*types.SessionMeta) {
	if c == nil {
		return
	}
	if c.viewedSessionActivity == nil {
		c.viewedSessionActivity = map[string]string{}
	}
	if c.unreadSessions == nil {
		c.unreadSessions = map[string]struct{}{}
	}
	if !c.unreadInitialized {
		for _, session := range sessions {
			if session == nil || session.ID == "" {
				continue
			}
			marker := sessionActivityMarker(meta[session.ID])
			if marker == "" {
				continue
			}
			c.viewedSessionActivity[session.ID] = marker
		}
		c.unreadInitialized = true
	}
	selectedID, selectedMarker := c.selectedSessionActivity()
	present := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		if session == nil || session.ID == "" {
			continue
		}
		id := session.ID
		present[id] = struct{}{}
		marker := sessionActivityMarker(meta[id])
		if id == selectedID {
			if selectedMarker != "" {
				c.viewedSessionActivity[id] = selectedMarker
			}
			delete(c.unreadSessions, id)
			continue
		}
		if marker == "" {
			delete(c.unreadSessions, id)
			continue
		}
		viewed, ok := c.viewedSessionActivity[id]
		if !ok {
			// Treat first-seen activity as already viewed to avoid unread noise on initial discovery.
			c.viewedSessionActivity[id] = marker
			delete(c.unreadSessions, id)
			continue
		}
		if viewed != marker {
			c.unreadSessions[id] = struct{}{}
		} else {
			delete(c.unreadSessions, id)
		}
	}
	for id := range c.unreadSessions {
		if _, ok := present[id]; !ok {
			delete(c.unreadSessions, id)
		}
	}
	for id := range c.viewedSessionActivity {
		if _, ok := present[id]; !ok {
			delete(c.viewedSessionActivity, id)
		}
	}
	c.syncDelegate()
}

func (c *SidebarController) selectedSessionActivity() (string, string) {
	item := c.SelectedItem()
	if item == nil || !item.isSession() || item.session == nil {
		return "", ""
	}
	return item.session.ID, sessionActivityMarker(item.meta)
}

func (c *SidebarController) setWorkspaceExpanded(id string, expanded bool) bool {
	if c.workspaceExpanded == nil {
		c.workspaceExpanded = map[string]bool{}
	}
	if current, ok := c.workspaceExpanded[id]; ok && current == expanded {
		return false
	}
	c.workspaceExpanded[id] = expanded
	return true
}

func (c *SidebarController) setWorktreeExpanded(id string, expanded bool) bool {
	if c.worktreeExpanded == nil {
		c.worktreeExpanded = map[string]bool{}
	}
	if current, ok := c.worktreeExpanded[id]; ok && current == expanded {
		return false
	}
	c.worktreeExpanded[id] = expanded
	return true
}

func (c *SidebarController) setWorkflowExpanded(id string, expanded bool) bool {
	if c.workflowExpanded == nil {
		c.workflowExpanded = map[string]bool{}
	}
	if current, ok := c.workflowExpanded[id]; ok && current == expanded {
		return false
	}
	c.workflowExpanded[id] = expanded
	return true
}

func (c *SidebarController) rebuild(selectedKey string) {
	if c == nil {
		return
	}
	viewAnchorKey := c.viewAnchorKey()
	viewAnchorOffset := c.scrollOffset
	items := buildSidebarItemsWithRecents(c.workspacesSnapshot, c.worktreesSnapshot, c.sessionsSnapshot, c.workflowRunsSnapshot, c.metaSnapshot, c.showDismissedSnapshot, c.recentsState, sidebarExpansionResolver{
		workspace: c.workspaceExpanded,
		worktree:  c.worktreeExpanded,
		workflow:  c.workflowExpanded,
		defaultOn: c.expandByDefault,
	})
	c.list.SetItems(items)
	if len(items) == 0 {
		c.selectedKey = ""
		c.scrollOffset = 0
		c.syncDelegate()
		return
	}
	activeWorkspaceID := ""
	activeWorktreeID := ""
	if c.delegate != nil {
		activeWorkspaceID = c.delegate.activeWorkspaceID
		activeWorktreeID = c.delegate.activeWorktreeID
	}
	if !sidebarItemsContainKey(items, selectedKey) {
		selectedIdx := selectSidebarIndex(items, selectedKey, activeWorkspaceID, activeWorktreeID)
		if selectedIdx < 0 || selectedIdx >= len(items) {
			selectedIdx = 0
		}
		if entry, ok := items[selectedIdx].(*sidebarItem); ok && entry != nil {
			c.selectedKey = entry.key()
		}
	} else {
		c.selectedKey = selectedKey
	}
	if !c.restoreScrollAnchor(items, viewAnchorKey, viewAnchorOffset) {
		c.clampScrollOffset()
	}
	c.syncDelegate()
	c.markSelectedSessionViewed()
}

func (c *SidebarController) pruneExpansionOverrides(workspaces []*types.Workspace, worktrees map[string][]*types.Worktree, workflowRuns []*guidedworkflows.WorkflowRun, sessions []*types.Session, meta map[string]*types.SessionMeta) {
	knownWorkspaces := map[string]struct{}{
		unassignedWorkspaceID: {},
	}
	for _, workspace := range workspaces {
		if workspace == nil || workspace.ID == "" {
			continue
		}
		knownWorkspaces[workspace.ID] = struct{}{}
	}
	for id := range c.workspaceExpanded {
		if _, ok := knownWorkspaces[id]; !ok {
			delete(c.workspaceExpanded, id)
		}
	}

	knownWorktrees := map[string]struct{}{}
	for _, entries := range worktrees {
		for _, wt := range entries {
			if wt == nil || wt.ID == "" {
				continue
			}
			knownWorktrees[wt.ID] = struct{}{}
		}
	}
	for id := range c.worktreeExpanded {
		if _, ok := knownWorktrees[id]; !ok {
			delete(c.worktreeExpanded, id)
		}
	}

	knownWorkflowRuns := map[string]struct{}{}
	for _, run := range workflowRuns {
		if run == nil {
			continue
		}
		runID := strings.TrimSpace(run.ID)
		if runID == "" {
			continue
		}
		knownWorkflowRuns[runID] = struct{}{}
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		entry := meta[session.ID]
		if entry == nil {
			continue
		}
		runID := strings.TrimSpace(entry.WorkflowRunID)
		if runID == "" {
			continue
		}
		knownWorkflowRuns[runID] = struct{}{}
	}
	for id := range c.workflowExpanded {
		if _, ok := knownWorkflowRuns[id]; !ok {
			delete(c.workflowExpanded, id)
		}
	}
}

func buildSessionSidebarParents(sessions []*types.Session, meta map[string]*types.SessionMeta) map[string]sessionSidebarParent {
	if len(sessions) == 0 {
		return map[string]sessionSidebarParent{}
	}
	out := make(map[string]sessionSidebarParent, len(sessions))
	for _, session := range sessions {
		if session == nil || session.ID == "" {
			continue
		}
		parent := sessionSidebarParent{}
		if m := meta[session.ID]; m != nil {
			parent.workspaceID = strings.TrimSpace(m.WorkspaceID)
			parent.worktreeID = strings.TrimSpace(m.WorktreeID)
			parent.workflowID = strings.TrimSpace(m.WorkflowRunID)
		}
		out[session.ID] = parent
	}
	return out
}

func cloneBoolMap(input map[string]bool) map[string]bool {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]bool, len(input))
	for key, value := range input {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *SidebarController) view() string {
	if c == nil {
		return ""
	}
	title := ""
	if c.list.ShowTitle() {
		title = c.list.Styles.TitleBar.Render(c.list.Styles.Title.Render(c.list.Title))
	}
	contentHeight := c.list.Height()
	if title != "" {
		contentHeight -= lipgloss.Height(title)
	}
	if contentHeight < 1 {
		contentHeight = 1
	}
	body := c.renderItems(contentHeight)
	if title == "" {
		return body
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, body)
}

func (c *SidebarController) renderItems(contentHeight int) string {
	items := c.list.VisibleItems()
	if len(items) == 0 {
		return lipgloss.NewStyle().Height(contentHeight).Render("")
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
	itemsOnPage := c.itemsPerView()
	if itemsOnPage <= 0 {
		itemsOnPage = len(items)
	}
	start := c.scrollOffset
	end := start + itemsOnPage
	if end > len(items) {
		end = len(items)
	}
	var b strings.Builder
	renderModel := c.list
	for i := start; i < end; i++ {
		c.delegate.Render(&b, renderModel, i, items[i])
		if i != end-1 {
			b.WriteString(strings.Repeat("\n", itemSpacing+1))
		}
	}
	itemsRendered := end - start
	if itemsRendered < itemsOnPage {
		n := (itemsOnPage - itemsRendered) * step
		if itemsRendered > 0 {
			b.WriteString(strings.Repeat("\n", n))
		}
	}
	return lipgloss.NewStyle().Height(contentHeight).Render(b.String())
}

func (c *SidebarController) itemsPerView() int {
	if c == nil {
		return 0
	}
	contentHeight := c.list.Height() - c.headerRows()
	if contentHeight < 1 {
		return 1
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
	return max(1, contentHeight/step)
}

func (c *SidebarController) maxScrollOffset(total int) int {
	itemsOnPage := c.itemsPerView()
	if itemsOnPage <= 0 || total <= itemsOnPage {
		return 0
	}
	return total - itemsOnPage
}

func (c *SidebarController) clampScrollOffset() {
	if c == nil {
		return
	}
	total := len(c.list.VisibleItems())
	maxOffset := c.maxScrollOffset(total)
	if c.scrollOffset < 0 {
		c.scrollOffset = 0
	}
	if c.scrollOffset > maxOffset {
		c.scrollOffset = maxOffset
	}
}

func (c *SidebarController) scrollTo(offset int) bool {
	if c == nil {
		return false
	}
	total := len(c.list.VisibleItems())
	maxOffset := c.maxScrollOffset(total)
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	if c.scrollOffset == offset {
		return false
	}
	c.scrollOffset = offset
	return true
}

func (c *SidebarController) selectedIndex() int {
	if c == nil || c.selectedKey == "" {
		return -1
	}
	for i, item := range c.list.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil {
			continue
		}
		if entry.key() == c.selectedKey {
			return i
		}
	}
	return -1
}

func (c *SidebarController) selectIndex(idx int) {
	items := c.list.Items()
	if len(items) == 0 {
		c.selectedKey = ""
		c.scrollOffset = 0
		c.syncDelegate()
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(items) {
		idx = len(items) - 1
	}
	entry, ok := items[idx].(*sidebarItem)
	if !ok || entry == nil {
		return
	}
	c.selectedKey = entry.key()
	c.ensureVisible(idx)
	c.syncDelegate()
	c.markSelectedSessionViewed()
}

func (c *SidebarController) ensureVisible(idx int) {
	if c == nil {
		return
	}
	itemsOnPage := c.itemsPerView()
	if itemsOnPage <= 0 {
		itemsOnPage = 1
	}
	if idx < c.scrollOffset {
		c.scrollOffset = idx
	} else if idx >= c.scrollOffset+itemsOnPage {
		c.scrollOffset = idx - itemsOnPage + 1
	}
	c.clampScrollOffset()
}

func (c *SidebarController) viewAnchorKey() string {
	if c == nil {
		return ""
	}
	items := c.list.VisibleItems()
	if c.scrollOffset < 0 || c.scrollOffset >= len(items) {
		return ""
	}
	entry, ok := items[c.scrollOffset].(*sidebarItem)
	if !ok || entry == nil {
		return ""
	}
	return entry.key()
}

func (c *SidebarController) restoreScrollAnchor(items []list.Item, anchorKey string, fallbackOffset int) bool {
	if c == nil {
		return false
	}
	if anchorKey != "" {
		for i, item := range items {
			entry, ok := item.(*sidebarItem)
			if !ok || entry == nil {
				continue
			}
			if entry.key() == anchorKey {
				c.scrollOffset = i
				c.clampScrollOffset()
				return true
			}
		}
	}
	c.scrollOffset = fallbackOffset
	return false
}

func sidebarItemsContainKey(items []list.Item, key string) bool {
	if strings.TrimSpace(key) == "" {
		return false
	}
	for _, item := range items {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil {
			continue
		}
		if entry.key() == key {
			return true
		}
	}
	return false
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
