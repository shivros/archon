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
		if id := strings.TrimSpace(s.workflowID); id != "" {
			return "Guided Workflow " + id
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
	unreadSessions    map[string]struct{}
	providerBadges    map[string]*types.ProviderBadgeConfig
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
	maxWidth := m.Width()
	switch entry.kind {
	case sidebarRecentsAll:
		label := entry.Title()
		total := max(0, entry.recentsCount)
		label = fmt.Sprintf("%s (%d)", label, total)
		line := "• " + label
		line = truncateToWidth(line, maxWidth)
		style := workspaceStyle
		if isSelected {
			style = selectedStyle
		}
		fmt.Fprint(w, style.Render(line))
	case sidebarRecentsReady:
		label := fmt.Sprintf("%s (%d)", entry.Title(), max(0, entry.recentsCount))
		line := "  - " + label
		line = truncateToWidth(line, maxWidth)
		style := worktreeStyle
		if isSelected {
			style = selectedStyle
		}
		fmt.Fprint(w, style.Render(line))
	case sidebarRecentsRunning:
		label := fmt.Sprintf("%s (%d)", entry.Title(), max(0, entry.recentsCount))
		line := "  - " + label
		line = truncateToWidth(line, maxWidth)
		style := worktreeStyle
		if isSelected {
			style = selectedStyle
		}
		fmt.Fprint(w, style.Render(line))
	case sidebarWorkspace:
		label := entry.Title()
		if entry.sessionCount > 0 {
			label = fmt.Sprintf("%s (%d)", label, entry.sessionCount)
		}
		indent := strings.Repeat("  ", max(0, entry.depth))
		prefix := "  "
		if entry.collapsible {
			if entry.expanded {
				prefix = "▾ "
			} else {
				prefix = "▸ "
			}
		}
		label = indent + prefix + label
		label = truncateToWidth(label, maxWidth)
		style := workspaceStyle
		if entry.workspace != nil && entry.workspace.ID == d.activeWorkspaceID {
			style = workspaceActiveStyle
		}
		if isSelected {
			style = selectedStyle
		}
		fmt.Fprint(w, style.Render(label))
	case sidebarWorktree:
		label := entry.Title()
		if entry.sessionCount > 0 {
			label = fmt.Sprintf("%s (%d)", label, entry.sessionCount)
		}
		indent := strings.Repeat("  ", max(0, entry.depth))
		marker := " "
		if entry.collapsible {
			if entry.expanded {
				marker = "▾"
			} else {
				marker = "▸"
			}
		}
		line := indent + marker + " " + label
		line = truncateToWidth(line, maxWidth)
		style := worktreeStyle
		if entry.worktree != nil && entry.worktree.ID == d.activeWorktreeID {
			style = worktreeActiveStyle
		}
		if isSelected {
			style = selectedStyle
		}
		fmt.Fprint(w, style.Render(line))
	case sidebarWorkflow:
		label := entry.Title()
		if entry.workflow != nil {
			label = fmt.Sprintf("%s (%s)", label, strings.TrimSpace(entry.workflow.ID))
		}
		if entry.sessionCount > 0 {
			label = fmt.Sprintf("%s • %d sessions", label, entry.sessionCount)
		}
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
		line := indent + marker + " [WFL] " + label
		line = truncateToWidth(line, maxWidth)
		style := worktreeStyle
		if isSelected {
			style = selectedStyle
		}
		fmt.Fprint(w, style.Render(line))
	case sidebarSession:
		title := sessionTitle(entry.session, entry.meta)
		since := formatSince(sessionLastActive(entry.session, entry.meta))
		indicator := inactiveDot
		if entry.session != nil && isActiveStatus(entry.session.Status) {
			indicator = activeDot
		}
		if isDismissedSession(entry.session, entry.meta) {
			indicator = dismissedDot
		}
		badgeConfig := resolveProviderBadge(entry.sessionProvider(), d.providerBadges)
		badgeText := strings.TrimSpace(badgeConfig.Prefix)
		indent := strings.Repeat("  ", max(0, entry.depth))
		prefix := indent + fmt.Sprintf(" %s ", indicator)
		if badgeText != "" {
			prefix += badgeText + " "
		}
		suffix := ""
		if strings.TrimSpace(since) != "" {
			suffix = fmt.Sprintf(" • %s", since)
		}
		if isDismissedSession(entry.session, entry.meta) {
			suffix += " • dismissed"
		}
		available := maxWidth - ansi.StringWidth(prefix) - ansi.StringWidth(suffix)
		if available <= 0 {
			title = ""
		} else {
			title = truncateToWidth(title, available)
		}
		main := title + suffix
		if ansi.StringWidth(prefix)+ansi.StringWidth(main) > maxWidth {
			mainWidth := maxWidth - ansi.StringWidth(prefix)
			if mainWidth <= 0 {
				title = ""
				suffix = ""
			} else {
				titleWidth := ansi.StringWidth(title)
				if titleWidth > mainWidth {
					title = truncateToWidth(title, mainWidth)
					suffix = ""
				} else {
					suffix = truncateToWidth(suffix, mainWidth-titleWidth)
				}
			}
		}
		style := sessionStyle
		if isSelected {
			style = selectedStyle
		}
		titleStyle := style
		if entry.session != nil && d.isUnread(entry.session.ID) && !isSelected {
			titleStyle = sessionUnreadStyle
		}

		rendered := style.Render(indent + fmt.Sprintf(" %s ", indicator))
		if badgeText != "" {
			badgeStyle := style.Copy().Foreground(lipgloss.Color(strings.TrimSpace(badgeConfig.Color)))
			rendered += badgeStyle.Render(badgeText)
			rendered += style.Render(" ")
		}
		rendered += titleStyle.Render(title)
		rendered += style.Render(suffix)
		fmt.Fprint(w, rendered)
	}
}

func (d *sidebarDelegate) isUnread(id string) bool {
	if d == nil || d.unreadSessions == nil {
		return false
	}
	_, ok := d.unreadSessions[id]
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
	for _, workspace := range workspaces {
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
		workflowItem := buildWorkflowSidebarItem(runID, run, len(workflowSessionBuckets[runID]), expansion)
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
		workflowItem := buildWorkflowSidebarItem(runID, nil, len(bucket), expansion)
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
	if recents.Enabled {
		readyCount := max(0, recents.ReadyCount)
		runningCount := max(0, recents.RunningCount)
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
	for _, workspace := range workspaces {
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
		workspaceExpanded := !workspaceHasChildren || expansion.workspaceExpanded(wsID)
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
				worktreeExpanded := !worktreeHasChildren || expansion.worktreeExpanded(wt.ID)
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
		workspaceExpanded := !workspaceHasChildren || expansion.workspaceExpanded(unassignedWorkspaceID)
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

	return items
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
	if last == nil {
		return "—"
	}
	delta := time.Since(*last)
	if delta < 0 {
		delta = 0
	}
	switch {
	case delta < time.Minute:
		return "just now"
	case delta < time.Hour:
		return fmt.Sprintf("%dm ago", int(delta.Minutes()))
	case delta < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(delta.Hours()))
	default:
		days := int(delta.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
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
