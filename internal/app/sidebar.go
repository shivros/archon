package app

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"control/internal/guidedworkflows"
	"control/internal/providers"
	"control/internal/types"
)

const (
	sidebarTitleMax        = 48
	unassignedWorkspaceID  = "__unassigned__"
	unassignedWorkspaceTag = "Unassigned"
	activeDot              = "●"
	dismissedDot           = "x"
	inactiveDot            = " "
	defaultBadgeColor      = "245"
)

var defaultProviderBadges = map[string]types.ProviderBadgeConfig{
	"codex": {
		Prefix: "[CDX]",
		Color:  "15",
	},
	"claude": {
		Prefix: "[CLD]",
		Color:  "208",
	},
	"opencode": {
		Prefix: "[OPN]",
		Color:  "39",
	},
	"kilocode": {
		Prefix: "[KLO]",
		Color:  "226",
	},
	"gemini": {
		Prefix: "[GEM]",
		Color:  "45",
	},
	"custom": {
		Prefix: "[CST]",
		Color:  "250",
	},
}

type sidebarItemKind int

const (
	sidebarWorkspace sidebarItemKind = iota
	sidebarWorktree
	sidebarWorkflow
	sidebarSession
	sidebarRecentsAll
	sidebarRecentsReady
	sidebarRecentsRunning
)

type sidebarRecentsFilter string

const (
	sidebarRecentsFilterAll     sidebarRecentsFilter = "all"
	sidebarRecentsFilterReady   sidebarRecentsFilter = "ready"
	sidebarRecentsFilterRunning sidebarRecentsFilter = "running"
)

type sidebarRecentsState struct {
	Enabled      bool
	ReadyCount   int
	RunningCount int
}

type sidebarItem struct {
	kind         sidebarItemKind
	workspace    *types.Workspace
	worktree     *types.Worktree
	workflow     *guidedworkflows.WorkflowRun
	workflowID   string
	session      *types.Session
	meta         *types.SessionMeta
	recents      sidebarRecentsFilter
	recentsCount int
	sessionCount int
	depth        int
	collapsible  bool
	expanded     bool
}

func (s *sidebarItem) Title() string {
	switch s.kind {
	case sidebarRecentsAll:
		return "Recents"
	case sidebarRecentsReady:
		return "Ready"
	case sidebarRecentsRunning:
		return "Running"
	case sidebarWorkspace:
		if s.workspace == nil {
			return ""
		}
		return s.workspace.Name
	case sidebarWorktree:
		if s.worktree == nil {
			return ""
		}
		return s.worktree.Name
	case sidebarWorkflow:
		if s.workflow != nil {
			name := strings.TrimSpace(s.workflow.TemplateName)
			if name == "" {
				name = "Guided Workflow"
			}
			return name
		}
		return "Guided Workflow"
	case sidebarSession:
		return sessionTitle(s.session, s.meta)
	default:
		return ""
	}
}

func (s *sidebarItem) Description() string {
	switch s.kind {
	case sidebarSession:
		return formatSince(sessionLastActive(s.session, s.meta))
	case sidebarWorkflow:
		return workflowRunStatusText(s.workflow)
	default:
		return ""
	}
}

func (s *sidebarItem) FilterValue() string {
	return s.Title()
}

func (s *sidebarItem) key() string {
	switch s.kind {
	case sidebarRecentsAll:
		return "recents:all"
	case sidebarRecentsReady:
		return "recents:ready"
	case sidebarRecentsRunning:
		return "recents:running"
	case sidebarWorkspace:
		if s.workspace == nil {
			return "ws:"
		}
		return "ws:" + s.workspace.ID
	case sidebarWorktree:
		if s.worktree == nil {
			return "wt:"
		}
		return "wt:" + s.worktree.ID
	case sidebarSession:
		if s.session == nil {
			return "sess:"
		}
		return "sess:" + s.session.ID
	case sidebarWorkflow:
		if id := strings.TrimSpace(s.workflowID); id != "" {
			return "gwf:" + id
		}
		if s.workflow != nil {
			return "gwf:" + strings.TrimSpace(s.workflow.ID)
		}
		return "gwf:"
	default:
		return ""
	}
}

func (s *sidebarItem) workspaceID() string {
	if s.kind == sidebarSession && s.meta != nil {
		return s.meta.WorkspaceID
	}
	if s.kind == sidebarWorktree && s.worktree != nil {
		return s.worktree.WorkspaceID
	}
	if s.kind == sidebarWorkflow && s.workflow != nil {
		return strings.TrimSpace(s.workflow.WorkspaceID)
	}
	if s.kind == sidebarWorkspace && s.workspace != nil {
		return s.workspace.ID
	}
	return ""
}

func (s *sidebarItem) isSession() bool {
	return s.kind == sidebarSession && s.session != nil
}

func (s *sidebarItem) isWorkflow() bool {
	return s.kind == sidebarWorkflow && strings.TrimSpace(s.workflowRunID()) != ""
}

func (s *sidebarItem) workflowRunID() string {
	if s == nil {
		return ""
	}
	if id := strings.TrimSpace(s.workflowID); id != "" {
		return id
	}
	if s.workflow != nil {
		return strings.TrimSpace(s.workflow.ID)
	}
	return ""
}

func (s *sidebarItem) sessionProvider() string {
	if s == nil || s.session == nil {
		return ""
	}
	return s.session.Provider
}

type sidebarDelegate struct {
	activeWorkspaceID string
	activeWorktreeID  string
	selectedKey       string
	selectedKeys      map[string]struct{}
	unreadSessions    map[string]struct{}
	providerBadges    map[string]*types.ProviderBadgeConfig
	now               func() time.Time
	sessionLayout     sidebarSessionLayoutEngine
}

type sidebarSessionLayoutEngine interface {
	Layout(title, right string, width int) (string, string)
}

type sidebarRowState int

const (
	sidebarRowStateInactive sidebarRowState = iota
	sidebarRowStateActive
	sidebarRowStateDismissed
)

type defaultSidebarSessionLayoutEngine struct{}

func (defaultSidebarSessionLayoutEngine) Layout(title, right string, width int) (string, string) {
	return renderSessionColumns(title, right, width)
}

type sidebarSessionRowViewModel struct {
	Prefix        string
	BadgeText     string
	BadgeColor    string
	Title         string
	RightText     string
	TitleIsUnread bool
}

func (d *sidebarDelegate) Height() int {
	return 1
}

func (d *sidebarDelegate) Spacing() int {
	return 0
}

func (d *sidebarDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d *sidebarDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(*sidebarItem)
	if !ok {
		return
	}
	isSelected := d.selectedKey != "" && entry.key() == d.selectedKey
	isMarked := d.isSelectedKey(entry.key())
	maxWidth := m.Width()
	switch entry.kind {
	case sidebarRecentsAll:
		fmt.Fprint(w, d.renderRecentsAllRow(entry, maxWidth, isSelected, isMarked))
	case sidebarRecentsReady:
		fmt.Fprint(w, d.renderRecentsReadyRow(entry, maxWidth, isSelected, isMarked))
	case sidebarRecentsRunning:
		fmt.Fprint(w, d.renderRecentsRunningRow(entry, maxWidth, isSelected, isMarked))
	case sidebarWorkspace:
		fmt.Fprint(w, d.renderWorkspaceRow(entry, maxWidth, isSelected, isMarked))
	case sidebarWorktree:
		fmt.Fprint(w, d.renderWorktreeRow(entry, maxWidth, isSelected, isMarked))
	case sidebarWorkflow:
		fmt.Fprint(w, d.renderWorkflowRow(entry, maxWidth, isSelected, isMarked))
	case sidebarSession:
		fmt.Fprint(w, d.renderSessionRow(entry, maxWidth, isSelected, isMarked))
	}
}

func (d *sidebarDelegate) renderRecentsAllRow(entry *sidebarItem, maxWidth int, isSelected, isMarked bool) string {
	label := fmt.Sprintf("%s (%d)", entry.Title(), max(0, entry.recentsCount))
	line := truncateToWidth("• "+label, maxWidth)
	return sidebarSelectStyle(workspaceStyle, isSelected, isMarked).Render(line)
}

func (d *sidebarDelegate) renderRecentsReadyRow(entry *sidebarItem, maxWidth int, isSelected, isMarked bool) string {
	label := fmt.Sprintf("%s (%d)", entry.Title(), max(0, entry.recentsCount))
	line := truncateToWidth("  - "+label, maxWidth)
	return sidebarSelectStyle(worktreeStyle, isSelected, isMarked).Render(line)
}

func (d *sidebarDelegate) renderRecentsRunningRow(entry *sidebarItem, maxWidth int, isSelected, isMarked bool) string {
	label := fmt.Sprintf("%s (%d)", entry.Title(), max(0, entry.recentsCount))
	line := truncateToWidth("  - "+label, maxWidth)
	return sidebarSelectStyle(worktreeStyle, isSelected, isMarked).Render(line)
}

func (d *sidebarDelegate) renderWorkspaceRow(entry *sidebarItem, maxWidth int, isSelected, isMarked bool) string {
	label := entry.Title()
	indent := strings.Repeat("  ", max(0, entry.depth))
	prefix := "  "
	if entry.collapsible {
		if entry.expanded {
			prefix = "▾ "
		} else {
			prefix = "▸ "
		}
	}
	line := truncateToWidth(indent+prefix+label, maxWidth)
	baseStyle := workspaceStyle
	if entry.workspace != nil && entry.workspace.ID == d.activeWorkspaceID {
		baseStyle = workspaceActiveStyle
	}
	return sidebarSelectStyle(baseStyle, isSelected, isMarked).Render(line)
}

func (d *sidebarDelegate) renderWorktreeRow(entry *sidebarItem, maxWidth int, isSelected, isMarked bool) string {
	label := entry.Title()
	indent := strings.Repeat("  ", max(0, entry.depth))
	marker := " "
	if entry.collapsible {
		if entry.expanded {
			marker = "▾"
		} else {
			marker = "▸"
		}
	}
	line := truncateToWidth(indent+marker+" "+label, maxWidth)
	baseStyle := worktreeStyle
	if entry.worktree != nil && entry.worktree.ID == d.activeWorktreeID {
		baseStyle = worktreeActiveStyle
	}
	return sidebarSelectStyle(baseStyle, isSelected, isMarked).Render(line)
}

func (d *sidebarDelegate) renderWorkflowRow(entry *sidebarItem, maxWidth int, isSelected, isMarked bool) string {
	label := entry.Title()
	statusText := workflowRunStatusText(entry.workflow)
	if strings.TrimSpace(statusText) != "" {
		label = fmt.Sprintf("%s • %s", label, statusText)
	}
	marker := " "
	if entry.collapsible {
		if entry.expanded {
			marker = "▾"
		} else {
			marker = "▸"
		}
	}
	indent := strings.Repeat("  ", max(0, entry.depth))
	line := truncateToWidth(indent+marker+" "+indicatorForRowState(workflowRowState(entry.workflow))+" [WFL] "+label, maxWidth)
	return sidebarSelectStyle(worktreeStyle, isSelected, isMarked).Render(line)
}

func indicatorForRowState(state sidebarRowState) string {
	switch state {
	case sidebarRowStateActive:
		return activeDot
	case sidebarRowStateDismissed:
		return dismissedDot
	default:
		return inactiveDot
	}
}

func workflowRowState(run *guidedworkflows.WorkflowRun) sidebarRowState {
	if run == nil {
		return sidebarRowStateInactive
	}
	if run.DismissedAt != nil {
		return sidebarRowStateDismissed
	}
	if run.Status == guidedworkflows.WorkflowRunStatusRunning {
		return sidebarRowStateActive
	}
	return sidebarRowStateInactive
}

func sessionRowState(session *types.Session, meta *types.SessionMeta) sidebarRowState {
	if isDismissedSession(session, meta) {
		return sidebarRowStateDismissed
	}
	if session != nil && isActiveStatus(session.Status) {
		return sidebarRowStateActive
	}
	return sidebarRowStateInactive
}

func (d *sidebarDelegate) renderSessionRow(entry *sidebarItem, maxWidth int, isSelected, isMarked bool) string {
	baseStyle := sidebarSelectStyle(sessionStyle, isSelected, isMarked)
	viewModel := d.buildSessionRowViewModel(entry, maxWidth)

	rendered := baseStyle.Render(viewModel.Prefix)
	if viewModel.BadgeText != "" {
		badgeStyle := baseStyle.Copy().Foreground(lipgloss.Color(strings.TrimSpace(viewModel.BadgeColor)))
		rendered += badgeStyle.Render(viewModel.BadgeText)
		rendered += baseStyle.Render(" ")
	}
	titleStyle := baseStyle
	if viewModel.TitleIsUnread {
		titleStyle = sessionUnreadStyle
	}
	rendered += titleStyle.Render(viewModel.Title)
	if strings.TrimSpace(viewModel.RightText) != "" {
		rendered += baseStyle.Render(viewModel.RightText)
	}
	return rendered
}

func (d *sidebarDelegate) buildSessionRowViewModel(entry *sidebarItem, maxWidth int) sidebarSessionRowViewModel {
	indent := strings.Repeat("  ", max(0, entry.depth))
	prefix := indent + fmt.Sprintf(" %s ", indicatorForRowState(sessionRowState(entry.session, entry.meta)))
	badgeConfig := resolveProviderBadge(entry.sessionProvider(), d.providerBadges)
	badgeText := strings.TrimSpace(badgeConfig.Prefix)
	badgeWidth := 0
	if badgeText != "" {
		badgeWidth = ansi.StringWidth(badgeText) + ansi.StringWidth(" ")
	}
	rightText := buildSessionRightText(entry.session, entry.meta, d.nowOrDefault()())
	layout := d.layoutEngineOrDefault()
	title, right := layout.Layout(sessionTitle(entry.session, entry.meta), rightText, maxWidth-ansi.StringWidth(prefix)-badgeWidth)
	return sidebarSessionRowViewModel{
		Prefix:        prefix,
		BadgeText:     badgeText,
		BadgeColor:    strings.TrimSpace(badgeConfig.Color),
		Title:         title,
		RightText:     right,
		TitleIsUnread: entry.session != nil && d.isUnread(entry.session.ID) && d.selectedKey != entry.key(),
	}
}

func (d *sidebarDelegate) nowOrDefault() func() time.Time {
	if d != nil && d.now != nil {
		return d.now
	}
	return time.Now
}

func (d *sidebarDelegate) layoutEngineOrDefault() sidebarSessionLayoutEngine {
	if d != nil && d.sessionLayout != nil {
		return d.sessionLayout
	}
	return defaultSidebarSessionLayoutEngine{}
}

func sidebarSelectStyle(base lipgloss.Style, isSelected, isMarked bool) lipgloss.Style {
	if isSelected {
		return selectedStyle
	}
	if isMarked {
		return multiSelectStyle
	}
	return base
}

func (d *sidebarDelegate) isUnread(id string) bool {
	if d == nil || d.unreadSessions == nil {
		return false
	}
	_, ok := d.unreadSessions[id]
	return ok
}

func (d *sidebarDelegate) isSelectedKey(key string) bool {
	if d == nil || d.selectedKeys == nil || strings.TrimSpace(key) == "" {
		return false
	}
	_, ok := d.selectedKeys[key]
	return ok
}

func resolveProviderBadge(provider string, overrides map[string]*types.ProviderBadgeConfig) types.ProviderBadgeConfig {
	normalized := providers.Normalize(provider)
	badge := defaultProviderBadge(normalized)
	if override, ok := overrides[normalized]; ok && override != nil {
		if prefix := strings.TrimSpace(override.Prefix); prefix != "" {
			badge.Prefix = prefix
		}
		if color := strings.TrimSpace(override.Color); color != "" {
			badge.Color = color
		}
	}
	if strings.TrimSpace(badge.Color) == "" {
		badge.Color = defaultBadgeColor
	}
	return badge
}

func defaultProviderBadge(provider string) types.ProviderBadgeConfig {
	if badge, ok := defaultProviderBadges[provider]; ok {
		return badge
	}
	return types.ProviderBadgeConfig{
		Prefix: fallbackProviderBadgePrefix(provider),
		Color:  defaultBadgeColor,
	}
}

func fallbackProviderBadgePrefix(provider string) string {
	name := providers.Normalize(provider)
	if name == "" {
		return "[???]"
	}
	abbr := make([]rune, 0, 3)
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) {
			continue
		}
		abbr = append(abbr, unicode.ToUpper(r))
		if len(abbr) == 3 {
			break
		}
	}
	for len(abbr) < 3 {
		abbr = append(abbr, '?')
	}
	return "[" + string(abbr) + "]"
}

func normalizeProviderBadgeOverrides(overrides map[string]*types.ProviderBadgeConfig) map[string]*types.ProviderBadgeConfig {
	if len(overrides) == 0 {
		return nil
	}
	normalized := make(map[string]*types.ProviderBadgeConfig, len(overrides))
	for key, cfg := range overrides {
		provider := providers.Normalize(key)
		if provider == "" || cfg == nil {
			continue
		}
		copy := *cfg
		normalized[provider] = &copy
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

type sidebarExpansionResolver struct {
	workspace map[string]bool
	worktree  map[string]bool
	workflow  map[string]bool
	defaultOn bool
}

func (r sidebarExpansionResolver) workspaceExpanded(id string) bool {
	if value, ok := r.workspace[id]; ok {
		return value
	}
	return r.defaultOn
}

func (r sidebarExpansionResolver) worktreeExpanded(id string) bool {
	if value, ok := r.worktree[id]; ok {
		return value
	}
	return r.defaultOn
}

func (r sidebarExpansionResolver) workflowExpanded(id string) bool {
	if value, ok := r.workflow[id]; ok {
		return value
	}
	return r.defaultOn
}

func buildSidebarItems(workspaces []*types.Workspace, worktrees map[string][]*types.Worktree, sessions []*types.Session, workflows []*guidedworkflows.WorkflowRun, meta map[string]*types.SessionMeta, showDismissed bool) []list.Item {
	return buildSidebarItemsWithExpansion(workspaces, worktrees, sessions, workflows, meta, showDismissed, sidebarExpansionResolver{
		defaultOn: true,
	})
}

func buildSidebarItemsWithExpansion(workspaces []*types.Workspace, worktrees map[string][]*types.Worktree, sessions []*types.Session, workflows []*guidedworkflows.WorkflowRun, meta map[string]*types.SessionMeta, showDismissed bool, expansion sidebarExpansionResolver) []list.Item {
	return buildSidebarItemsWithRecents(workspaces, worktrees, sessions, workflows, meta, showDismissed, sidebarRecentsState{}, expansion)
}

func buildSidebarItemsWithRecents(
	workspaces []*types.Workspace,
	worktrees map[string][]*types.Worktree,
	sessions []*types.Session,
	workflows []*guidedworkflows.WorkflowRun,
	meta map[string]*types.SessionMeta,
	showDismissed bool,
	recents sidebarRecentsState,
	expansion sidebarExpansionResolver,
) []list.Item {
	return buildSidebarItemsWithOptions(workspaces, worktrees, sessions, workflows, meta, showDismissed, sidebarBuildOptions{
		recents:     recents,
		expansion:   expansion,
		sort:        defaultSidebarSortState(),
		filterQuery: "",
	})
}

type sidebarBuildOptions struct {
	recents     sidebarRecentsState
	expansion   sidebarExpansionResolver
	sort        sidebarSortState
	filterQuery string
}

func buildSidebarItemsWithOptions(
	workspaces []*types.Workspace,
	worktrees map[string][]*types.Worktree,
	sessions []*types.Session,
	workflows []*guidedworkflows.WorkflowRun,
	meta map[string]*types.SessionMeta,
	showDismissed bool,
	options sidebarBuildOptions,
) []list.Item {
	visibleSessions := filterVisibleSessions(sessions, meta, showDismissed)
	workflowByID := make(map[string]*guidedworkflows.WorkflowRun, len(workflows))
	hiddenDismissedWorkflowIDs := map[string]struct{}{}
	for _, run := range workflows {
		if run == nil {
			continue
		}
		runID := strings.TrimSpace(run.ID)
		if runID == "" {
			continue
		}
		if run.DismissedAt != nil && !showDismissed {
			hiddenDismissedWorkflowIDs[runID] = struct{}{}
			continue
		}
		workflowByID[runID] = run
	}
	workflowSessionBuckets := map[string][]*types.Session{}
	filteredSessions := make([]*types.Session, 0, len(visibleSessions))
	for _, session := range visibleSessions {
		if session == nil {
			continue
		}
		sessionMeta := meta[session.ID]
		workflowID := ""
		if sessionMeta != nil {
			workflowID = strings.TrimSpace(sessionMeta.WorkflowRunID)
		}
		if workflowID == "" {
			filteredSessions = append(filteredSessions, session)
			continue
		}
		workflowSessionBuckets[workflowID] = append(workflowSessionBuckets[workflowID], session)
	}
	knownWorkspaces := make(map[string]struct{}, len(workspaces))
	sortedWorkspaces := sortSidebarWorkspaces(workspaces, worktrees, visibleSessions, workflowMapValues(workflowByID), meta, options.sort)
	for _, workspace := range sortedWorkspaces {
		if workspace == nil {
			continue
		}
		knownWorkspaces[workspace.ID] = struct{}{}
	}
	grouped := make(map[string][]*types.Session)
	groupedWorktrees := make(map[string][]*types.Session)
	knownWorktrees := make(map[string]string)
	for wsID, entries := range worktrees {
		for _, wt := range entries {
			if wt == nil {
				continue
			}
			knownWorktrees[wt.ID] = wsID
		}
	}
	for _, session := range filteredSessions {
		workspaceID := ""
		worktreeID := ""
		if m := meta[session.ID]; m != nil {
			workspaceID = m.WorkspaceID
			worktreeID = m.WorktreeID
		}
		if workspaceID != "" {
			if _, ok := knownWorkspaces[workspaceID]; !ok {
				workspaceID = ""
			}
		}
		if worktreeID != "" {
			if _, ok := knownWorktrees[worktreeID]; !ok {
				worktreeID = ""
			}
		}
		if worktreeID != "" {
			groupedWorktrees[worktreeID] = append(groupedWorktrees[worktreeID], session)
		} else {
			grouped[workspaceID] = append(grouped[workspaceID], session)
		}
	}
	workflowByWorkspace := map[string][]*sidebarItem{}
	workflowByWorktree := map[string][]*sidebarItem{}
	for runID, run := range workflowByID {
		workspaceID := strings.TrimSpace(run.WorkspaceID)
		worktreeID := strings.TrimSpace(run.WorktreeID)
		if workspaceID != "" {
			if _, ok := knownWorkspaces[workspaceID]; !ok {
				workspaceID = ""
			}
		}
		if worktreeID != "" {
			if _, ok := knownWorktrees[worktreeID]; !ok {
				worktreeID = ""
			}
		}
		workflowItem := buildWorkflowSidebarItem(runID, run, len(workflowSessionBuckets[runID]), options.expansion)
		if worktreeID != "" {
			workflowItem.depth = 2
			workflowByWorktree[worktreeID] = append(workflowByWorktree[worktreeID], workflowItem)
			continue
		}
		workflowItem.depth = 1
		workflowByWorkspace[workspaceID] = append(workflowByWorkspace[workspaceID], workflowItem)
	}
	for runID, bucket := range workflowSessionBuckets {
		if _, exists := workflowByID[runID]; exists {
			continue
		}
		if _, hidden := hiddenDismissedWorkflowIDs[runID]; hidden {
			continue
		}
		workspaceID := ""
		worktreeID := ""
		for _, session := range bucket {
			if session == nil {
				continue
			}
			if m := meta[session.ID]; m != nil {
				if workspaceID == "" {
					workspaceID = strings.TrimSpace(m.WorkspaceID)
				}
				if worktreeID == "" {
					worktreeID = strings.TrimSpace(m.WorktreeID)
				}
			}
			if workspaceID != "" || worktreeID != "" {
				break
			}
		}
		if workspaceID != "" {
			if _, ok := knownWorkspaces[workspaceID]; !ok {
				workspaceID = ""
			}
		}
		if worktreeID != "" {
			if _, ok := knownWorktrees[worktreeID]; !ok {
				worktreeID = ""
			}
		}
		workflowItem := buildWorkflowSidebarItem(runID, nil, len(bucket), options.expansion)
		if worktreeID != "" {
			workflowItem.depth = 2
			workflowByWorktree[worktreeID] = append(workflowByWorktree[worktreeID], workflowItem)
			continue
		}
		workflowItem.depth = 1
		workflowByWorkspace[workspaceID] = append(workflowByWorkspace[workspaceID], workflowItem)
	}

	items := make([]list.Item, 0, len(workspaces)+3)
	appendSessionItem := func(session *types.Session, depth int) {
		if session == nil {
			return
		}
		items = append(items, &sidebarItem{
			kind:    sidebarSession,
			session: session,
			meta:    meta[session.ID],
			depth:   max(0, depth),
		})
	}
	if options.recents.Enabled {
		readyCount := max(0, options.recents.ReadyCount)
		runningCount := max(0, options.recents.RunningCount)
		items = append(items,
			&sidebarItem{
				kind:         sidebarRecentsAll,
				recents:      sidebarRecentsFilterAll,
				recentsCount: readyCount + runningCount,
			},
			&sidebarItem{
				kind:         sidebarRecentsReady,
				recents:      sidebarRecentsFilterReady,
				recentsCount: readyCount,
			},
			&sidebarItem{
				kind:         sidebarRecentsRunning,
				recents:      sidebarRecentsFilterRunning,
				recentsCount: runningCount,
			},
		)
	}
	for _, workspace := range sortedWorkspaces {
		if workspace == nil {
			continue
		}
		wsID := workspace.ID
		sessionsForWorkspace := grouped[wsID]
		workflowItemsForWorkspace := sortWorkflowSidebarItemsDesc(workflowByWorkspace[wsID])
		worktreesForWorkspace := worktrees[wsID]
		totalSessions := len(sessionsForWorkspace)
		for _, workflowItem := range workflowItemsForWorkspace {
			if workflowItem == nil {
				continue
			}
			totalSessions += workflowItem.sessionCount
		}
		worktreeCount := 0
		for _, wt := range worktreesForWorkspace {
			if wt == nil {
				continue
			}
			worktreeCount++
			totalSessions += len(groupedWorktrees[wt.ID])
			for _, workflowItem := range workflowByWorktree[wt.ID] {
				if workflowItem == nil {
					continue
				}
				totalSessions += workflowItem.sessionCount
			}
		}
		workspaceHasChildren := len(sessionsForWorkspace) > 0 || worktreeCount > 0 || len(workflowItemsForWorkspace) > 0
		workspaceExpanded := !workspaceHasChildren || options.expansion.workspaceExpanded(wsID)
		items = append(items, &sidebarItem{
			kind:         sidebarWorkspace,
			workspace:    workspace,
			sessionCount: totalSessions,
			depth:        0,
			collapsible:  workspaceHasChildren,
			expanded:     workspaceExpanded,
		})
		if workspaceExpanded {
			for _, workflowItem := range workflowItemsForWorkspace {
				if workflowItem == nil {
					continue
				}
				items = append(items, workflowItem)
				if workflowItem.expanded {
					for _, session := range sortSessionsDesc(workflowSessionBuckets[workflowItem.workflowRunID()]) {
						appendSessionItem(session, workflowItem.depth+1)
					}
				}
			}
			for _, session := range sortSessionsDesc(sessionsForWorkspace) {
				appendSessionItem(session, 1)
			}
			for _, wt := range worktreesForWorkspace {
				if wt == nil {
					continue
				}
				wtSessions := groupedWorktrees[wt.ID]
				wtWorkflowItems := sortWorkflowSidebarItemsDesc(workflowByWorktree[wt.ID])
				worktreeHasChildren := len(wtSessions) > 0 || len(wtWorkflowItems) > 0
				worktreeExpanded := !worktreeHasChildren || options.expansion.worktreeExpanded(wt.ID)
				items = append(items, &sidebarItem{
					kind:         sidebarWorktree,
					worktree:     wt,
					sessionCount: len(wtSessions),
					depth:        1,
					collapsible:  worktreeHasChildren,
					expanded:     worktreeExpanded,
				})
				if worktreeExpanded {
					for _, workflowItem := range wtWorkflowItems {
						if workflowItem == nil {
							continue
						}
						items = append(items, workflowItem)
						if workflowItem.expanded {
							for _, session := range sortSessionsDesc(workflowSessionBuckets[workflowItem.workflowRunID()]) {
								appendSessionItem(session, workflowItem.depth+1)
							}
						}
					}
					for _, session := range sortSessionsDesc(wtSessions) {
						appendSessionItem(session, 2)
					}
				}
				delete(groupedWorktrees, wt.ID)
				delete(workflowByWorktree, wt.ID)
			}
		}
		delete(grouped, wsID)
		delete(workflowByWorkspace, wsID)
	}

	unassignedWorkflows := sortWorkflowSidebarItemsDesc(workflowByWorkspace[""])
	if unassigned := grouped[""]; len(unassigned) > 0 || len(unassignedWorkflows) > 0 {
		ws := &types.Workspace{ID: unassignedWorkspaceID, Name: unassignedWorkspaceTag}
		workspaceHasChildren := len(unassigned) > 0 || len(unassignedWorkflows) > 0
		totalSessions := len(unassigned)
		for _, workflowItem := range unassignedWorkflows {
			if workflowItem == nil {
				continue
			}
			totalSessions += workflowItem.sessionCount
		}
		workspaceExpanded := !workspaceHasChildren || options.expansion.workspaceExpanded(unassignedWorkspaceID)
		items = append(items, &sidebarItem{
			kind:         sidebarWorkspace,
			workspace:    ws,
			sessionCount: totalSessions,
			depth:        0,
			collapsible:  workspaceHasChildren,
			expanded:     workspaceExpanded,
		})
		if workspaceExpanded {
			for _, workflowItem := range unassignedWorkflows {
				if workflowItem == nil {
					continue
				}
				items = append(items, workflowItem)
				if workflowItem.expanded {
					for _, session := range sortSessionsDesc(workflowSessionBuckets[workflowItem.workflowRunID()]) {
						appendSessionItem(session, workflowItem.depth+1)
					}
				}
			}
			for _, session := range sortSessionsDesc(unassigned) {
				appendSessionItem(session, 1)
			}
		}
	}

	return filterSidebarItems(items, options.filterQuery)
}

func buildWorkflowSidebarItem(runID string, run *guidedworkflows.WorkflowRun, sessionCount int, expansion sidebarExpansionResolver) *sidebarItem {
	runID = strings.TrimSpace(runID)
	if runID == "" && run != nil {
		runID = strings.TrimSpace(run.ID)
	}
	collapsible := sessionCount > 0
	expanded := !collapsible || expansion.workflowExpanded(runID)
	return &sidebarItem{
		kind:         sidebarWorkflow,
		workflow:     run,
		workflowID:   runID,
		sessionCount: sessionCount,
		collapsible:  collapsible,
		expanded:     expanded,
	}
}

func sortWorkflowSidebarItemsDesc(items []*sidebarItem) []*sidebarItem {
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool {
		left := workflowSidebarSortTime(items[i])
		right := workflowSidebarSortTime(items[j])
		if left.Equal(right) {
			return strings.TrimSpace(items[i].workflowRunID()) < strings.TrimSpace(items[j].workflowRunID())
		}
		return left.After(right)
	})
	return items
}

func workflowSidebarSortTime(item *sidebarItem) time.Time {
	if item == nil || item.workflow == nil {
		return time.Time{}
	}
	return workflowRunLastActivityAt(item.workflow)
}

func workflowRunLastActivityAt(run *guidedworkflows.WorkflowRun) time.Time {
	if run == nil {
		return time.Time{}
	}
	latest := run.CreatedAt
	if run.StartedAt != nil && run.StartedAt.After(latest) {
		latest = *run.StartedAt
	}
	if run.PausedAt != nil && run.PausedAt.After(latest) {
		latest = *run.PausedAt
	}
	if run.CompletedAt != nil && run.CompletedAt.After(latest) {
		latest = *run.CompletedAt
	}
	if n := len(run.AuditTrail); n > 0 {
		last := run.AuditTrail[n-1].At
		if last.After(latest) {
			latest = last
		}
	}
	return latest
}

func workflowMapValues(byID map[string]*guidedworkflows.WorkflowRun) []*guidedworkflows.WorkflowRun {
	if len(byID) == 0 {
		return nil
	}
	out := make([]*guidedworkflows.WorkflowRun, 0, len(byID))
	for _, run := range byID {
		if run == nil {
			continue
		}
		out = append(out, run)
	}
	return out
}

func sortSidebarWorkspaces(
	workspaces []*types.Workspace,
	worktrees map[string][]*types.Worktree,
	sessions []*types.Session,
	workflows []*guidedworkflows.WorkflowRun,
	meta map[string]*types.SessionMeta,
	sortState sidebarSortState,
) []*types.Workspace {
	if len(workspaces) == 0 {
		return nil
	}
	sortState.Key = parseSidebarSortKey(string(sortState.Key))
	out := make([]*types.Workspace, 0, len(workspaces))
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		out = append(out, ws)
	}
	if len(out) < 2 {
		return out
	}
	worktreeToWorkspace := map[string]string{}
	for wsID, entries := range worktrees {
		for _, wt := range entries {
			if wt == nil {
				continue
			}
			worktreeToWorkspace[strings.TrimSpace(wt.ID)] = strings.TrimSpace(wsID)
		}
	}
	activity := map[string]time.Time{}
	for _, ws := range out {
		if ws == nil {
			continue
		}
		activity[ws.ID] = ws.UpdatedAt
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		entry := meta[session.ID]
		workspaceID := ""
		if entry != nil {
			workspaceID = strings.TrimSpace(entry.WorkspaceID)
			if workspaceID == "" {
				workspaceID = strings.TrimSpace(worktreeToWorkspace[strings.TrimSpace(entry.WorktreeID)])
			}
		}
		if workspaceID == "" {
			continue
		}
		last := sessionLastActive(session, entry)
		if last == nil {
			continue
		}
		if last.After(activity[workspaceID]) {
			activity[workspaceID] = *last
		}
	}
	for _, run := range workflows {
		if run == nil {
			continue
		}
		workspaceID := strings.TrimSpace(run.WorkspaceID)
		if workspaceID == "" {
			workspaceID = strings.TrimSpace(worktreeToWorkspace[strings.TrimSpace(run.WorktreeID)])
		}
		if workspaceID == "" {
			continue
		}
		last := workflowRunLastActivityAt(run)
		if last.After(activity[workspaceID]) {
			activity[workspaceID] = last
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		wi := out[i]
		wj := out[j]
		if wi == nil || wj == nil {
			return wi != nil
		}
		ctx := sidebarSortWorkspaceContext{ActivityByWorkspaceID: activity}
		less := sidebarSortLess(sortState.Key, ctx, wi, wj)
		if sortState.Reverse {
			return sidebarSortLess(sortState.Key, ctx, wj, wi)
		}
		return less
	})
	return out
}

func filterSidebarItems(items []list.Item, query string) []list.Item {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || len(items) == 0 {
		return items
	}
	entries := make([]*sidebarItem, 0, len(items))
	for _, item := range items {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil {
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return items[:0]
	}
	include := make([]bool, len(entries))
	for i, entry := range entries {
		if entry == nil {
			continue
		}
		if entry.kind == sidebarRecentsAll || entry.kind == sidebarRecentsReady || entry.kind == sidebarRecentsRunning {
			include[i] = true
			continue
		}
		title := strings.ToLower(strings.TrimSpace(entry.Title()))
		if strings.Contains(title, query) {
			include[i] = true
		}
	}
	for i, entry := range entries {
		if entry == nil || !include[i] {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			prev := entries[j]
			if prev == nil {
				continue
			}
			if prev.depth < entry.depth {
				include[j] = true
				entry = prev
				if entry.depth == 0 {
					break
				}
			}
		}
		if !entry.collapsible {
			continue
		}
		for j := i + 1; j < len(entries); j++ {
			next := entries[j]
			if next == nil || next.depth <= entry.depth {
				break
			}
			include[j] = true
		}
	}
	out := make([]list.Item, 0, len(entries))
	for i, entry := range entries {
		if !include[i] {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func filterVisibleSessions(sessions []*types.Session, meta map[string]*types.SessionMeta, showDismissed bool) []*types.Session {
	out := make([]*types.Session, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		dismissed := isDismissedSession(session, meta[session.ID])
		if dismissed {
			if showDismissed {
				out = append(out, session)
			}
			continue
		}
		if isVisibleStatus(session.Status) {
			out = append(out, session)
		}
	}
	out = sortSessionsDesc(out)
	return out
}

func workflowRunStatusText(run *guidedworkflows.WorkflowRun) string {
	if run == nil {
		return ""
	}
	if run.DismissedAt != nil {
		return "dismissed"
	}
	switch run.Status {
	case guidedworkflows.WorkflowRunStatusCreated:
		return "created"
	case guidedworkflows.WorkflowRunStatusRunning:
		return "running"
	case guidedworkflows.WorkflowRunStatusPaused:
		return "paused"
	case guidedworkflows.WorkflowRunStatusStopped:
		return "stopped"
	case guidedworkflows.WorkflowRunStatusCompleted:
		return "completed"
	case guidedworkflows.WorkflowRunStatusFailed:
		return "failed"
	default:
		return strings.TrimSpace(string(run.Status))
	}
}

func sortSessionsDesc(sessions []*types.Session) []*types.Session {
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})
	return sessions
}

func isActiveStatus(status types.SessionStatus) bool {
	switch status {
	case types.SessionStatusCreated, types.SessionStatusStarting, types.SessionStatusRunning:
		return true
	default:
		return false
	}
}

func isVisibleStatus(status types.SessionStatus) bool {
	switch status {
	case types.SessionStatusCreated, types.SessionStatusStarting, types.SessionStatusRunning, types.SessionStatusInactive, types.SessionStatusExited:
		return true
	default:
		return false
	}
}

func isDismissedSession(session *types.Session, meta *types.SessionMeta) bool {
	if meta != nil && meta.DismissedAt != nil {
		return true
	}
	// Legacy fallback while orphaned records are being migrated.
	return session != nil && session.Status == types.SessionStatusOrphaned
}

func sessionTitle(session *types.Session, meta *types.SessionMeta) string {
	if meta != nil && strings.TrimSpace(meta.Title) != "" {
		return truncateText(cleanTitle(meta.Title), sidebarTitleMax)
	}
	if meta != nil && strings.TrimSpace(meta.InitialInput) != "" {
		return truncateText(cleanTitle(meta.InitialInput), sidebarTitleMax)
	}
	if session != nil && strings.TrimSpace(session.Title) != "" {
		return truncateText(cleanTitle(session.Title), sidebarTitleMax)
	}
	if session != nil && session.ID != "" {
		return session.ID
	}
	return ""
}

func cleanTitle(input string) string {
	if input == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(input))
	lastSpace := false
	for _, r := range input {
		if unicode.IsSpace(r) {
			if builder.Len() == 0 || lastSpace {
				continue
			}
			builder.WriteByte(' ')
			lastSpace = true
			continue
		}
		if r < 32 || r == 127 {
			continue
		}
		if r <= 126 {
			builder.WriteRune(r)
			lastSpace = false
		}
	}
	return strings.TrimSpace(builder.String())
}

func sessionLastActive(session *types.Session, meta *types.SessionMeta) *time.Time {
	if meta != nil && meta.LastActiveAt != nil {
		return meta.LastActiveAt
	}
	if session != nil && session.StartedAt != nil {
		return session.StartedAt
	}
	if session != nil && !session.CreatedAt.IsZero() {
		return &session.CreatedAt
	}
	return nil
}

func sessionActivityMarker(meta *types.SessionMeta) string {
	if meta == nil {
		return ""
	}
	if turnID := strings.TrimSpace(meta.LastTurnID); turnID != "" {
		return "turn:" + turnID
	}
	if meta.LastActiveAt != nil {
		return fmt.Sprintf("active:%d", meta.LastActiveAt.UTC().UnixNano())
	}
	return ""
}

func formatSince(last *time.Time) string {
	return formatSinceAt(last, time.Now().UTC())
}

func formatSinceAt(last *time.Time, now time.Time) string {
	if last == nil {
		return "—"
	}
	delta := now.Sub(*last)
	if delta < 0 {
		delta = 0
	}
	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh", int(delta.Hours()))
	default:
		days := int(delta.Hours() / 24)
		return fmt.Sprintf("%dd", days)
	}
}

func buildSessionRightText(session *types.Session, meta *types.SessionMeta, now time.Time) string {
	since := strings.TrimSpace(formatSinceAt(sessionLastActive(session, meta), now))
	if isDismissedSession(session, meta) {
		return "dismissed • " + since
	}
	return since
}

func renderSessionColumns(title, right string, width int) (string, string) {
	if width <= 0 {
		return "", ""
	}
	title = strings.TrimSpace(title)
	right = strings.TrimSpace(right)
	if right == "" {
		return truncateToWidth(title, width), ""
	}
	rightWidth := ansi.StringWidth(right)
	if rightWidth >= width {
		return "", truncateToWidth(right, width)
	}
	maxTitle := width - rightWidth - 1
	if maxTitle <= 0 {
		return "", strings.Repeat(" ", width-rightWidth) + right
	}
	title = truncateToWidth(title, maxTitle)
	titleWidth := ansi.StringWidth(title)
	gap := width - titleWidth - rightWidth
	if gap < 1 && titleWidth > 0 {
		title = truncateToWidth(title, max(0, maxTitle-1))
		titleWidth = ansi.StringWidth(title)
		gap = width - titleWidth - rightWidth
	}
	if gap < 0 {
		gap = 0
	}
	return title, strings.Repeat(" ", gap) + right
}

func truncateText(text string, maxLen int) string {
	text = strings.TrimSpace(text)
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	return strings.TrimSpace(text[:maxLen]) + "…"
}

func truncateToWidth(text string, width int) string {
	if width <= 0 {
		return text
	}
	if ansi.StringWidth(text) <= width {
		return text
	}
	if width == 1 {
		return "…"
	}
	return ansi.Cut(text, 0, width-1) + "…"
}

func normalizeSessionMeta(meta []*types.SessionMeta) map[string]*types.SessionMeta {
	out := make(map[string]*types.SessionMeta, len(meta))
	for _, entry := range meta {
		if entry == nil || entry.SessionID == "" {
			continue
		}
		out[entry.SessionID] = entry
	}
	return out
}
