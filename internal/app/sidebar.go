package app

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"control/internal/types"
)

const (
	sidebarTitleMax        = 48
	unassignedWorkspaceID  = "__unassigned__"
	unassignedWorkspaceTag = "Unassigned"
	activeDot              = "●"
	inactiveDot            = " "
)

type sidebarItemKind int

const (
	sidebarWorkspace sidebarItemKind = iota
	sidebarWorktree
	sidebarSession
)

type sidebarItem struct {
	kind         sidebarItemKind
	workspace    *types.Workspace
	worktree     *types.Worktree
	session      *types.Session
	meta         *types.SessionMeta
	sessionCount int
}

func (s *sidebarItem) Title() string {
	switch s.kind {
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
	case sidebarSession:
		return sessionTitle(s.session, s.meta)
	default:
		return ""
	}
}

func (s *sidebarItem) Description() string {
	if s.kind != sidebarSession {
		return ""
	}
	return formatSince(sessionLastActive(s.session, s.meta))
}

func (s *sidebarItem) FilterValue() string {
	return s.Title()
}

func (s *sidebarItem) key() string {
	switch s.kind {
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
	if s.kind == sidebarWorkspace && s.workspace != nil {
		return s.workspace.ID
	}
	return ""
}

func (s *sidebarItem) isSession() bool {
	return s.kind == sidebarSession && s.session != nil
}

type sidebarDelegate struct {
	activeWorkspaceID string
	activeWorktreeID  string
	selectedSessions  map[string]struct{}
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
	isSelected := index == m.Index()
	switch entry.kind {
	case sidebarWorkspace:
		label := entry.Title()
		if entry.sessionCount > 0 {
			label = fmt.Sprintf("%s (%d)", label, entry.sessionCount)
		}
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
		line := "  ↳ " + label
		style := worktreeStyle
		if entry.worktree != nil && entry.worktree.ID == d.activeWorktreeID {
			style = worktreeActiveStyle
		}
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
		line := fmt.Sprintf(" %s %s • %s", indicator, title, since)
		style := sessionStyle
		if indicator == activeDot {
			style = activeSessionStyle
		}
		if entry.session != nil && d.isSelected(entry.session.ID) {
			style = sessionSelectedStyle
		}
		if isSelected {
			style = selectedStyle
		}
		fmt.Fprint(w, style.Render(line))
	}
}

func (d *sidebarDelegate) isSelected(id string) bool {
	if d == nil || d.selectedSessions == nil {
		return false
	}
	_, ok := d.selectedSessions[id]
	return ok
}

func buildSidebarItems(workspaces []*types.Workspace, worktrees map[string][]*types.Worktree, sessions []*types.Session, meta map[string]*types.SessionMeta) []list.Item {
	visibleSessions := filterVisibleSessions(sessions)
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
	for _, session := range visibleSessions {
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

	items := make([]list.Item, 0)
	for _, workspace := range workspaces {
		wsID := workspace.ID
		sessionsForWorkspace := grouped[wsID]
		worktreesForWorkspace := worktrees[wsID]
		totalSessions := len(sessionsForWorkspace)
		for _, wt := range worktreesForWorkspace {
			if wt == nil {
				continue
			}
			totalSessions += len(groupedWorktrees[wt.ID])
		}
		items = append(items, &sidebarItem{
			kind:         sidebarWorkspace,
			workspace:    workspace,
			sessionCount: totalSessions,
		})
		for _, session := range sortSessionsDesc(sessionsForWorkspace) {
			items = append(items, &sidebarItem{
				kind:    sidebarSession,
				session: session,
				meta:    meta[session.ID],
			})
		}
		for _, wt := range worktreesForWorkspace {
			if wt == nil {
				continue
			}
			wtSessions := groupedWorktrees[wt.ID]
			items = append(items, &sidebarItem{
				kind:         sidebarWorktree,
				worktree:     wt,
				sessionCount: len(wtSessions),
			})
			for _, session := range sortSessionsDesc(wtSessions) {
				items = append(items, &sidebarItem{
					kind:    sidebarSession,
					session: session,
					meta:    meta[session.ID],
				})
			}
			delete(groupedWorktrees, wt.ID)
		}
		delete(grouped, wsID)
	}

	if unassigned := grouped[""]; len(unassigned) > 0 {
		ws := &types.Workspace{ID: unassignedWorkspaceID, Name: unassignedWorkspaceTag}
		items = append(items, &sidebarItem{
			kind:         sidebarWorkspace,
			workspace:    ws,
			sessionCount: len(unassigned),
		})
		for _, session := range sortSessionsDesc(unassigned) {
			items = append(items, &sidebarItem{
				kind:    sidebarSession,
				session: session,
				meta:    meta[session.ID],
			})
		}
	}

	return items
}

func filterVisibleSessions(sessions []*types.Session) []*types.Session {
	out := make([]*types.Session, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if isVisibleStatus(session.Status) {
			out = append(out, session)
		}
	}
	out = sortSessionsDesc(out)
	return out
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
	case types.SessionStatusCreated, types.SessionStatusStarting, types.SessionStatusRunning, types.SessionStatusInactive:
		return true
	default:
		return false
	}
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
