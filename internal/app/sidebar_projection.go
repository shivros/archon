package app

import (
	"strings"

	"control/internal/types"
)

type SidebarProjectionInput struct {
	Workspaces         []*types.Workspace
	Worktrees          map[string][]*types.Worktree
	Sessions           []*types.Session
	SessionMeta        map[string]*types.SessionMeta
	ActiveWorkspaceIDs []string
}

type SidebarProjection struct {
	Workspaces []*types.Workspace
	Sessions   []*types.Session
}

type SidebarProjectionBuilder interface {
	Build(input SidebarProjectionInput) SidebarProjection
}

type defaultSidebarProjectionBuilder struct{}

func NewDefaultSidebarProjectionBuilder() SidebarProjectionBuilder {
	return defaultSidebarProjectionBuilder{}
}

func (defaultSidebarProjectionBuilder) Build(input SidebarProjectionInput) SidebarProjection {
	selected := selectedWorkspaceGroups(input.ActiveWorkspaceIDs)
	if len(selected) == 0 {
		return SidebarProjection{
			Workspaces: []*types.Workspace{},
			Sessions:   []*types.Session{},
		}
	}
	workspaces := filterWorkspacesForSidebar(input.Workspaces, selected)
	sessions := filterSessionsForSidebar(input.Sessions, input.SessionMeta, input.Worktrees, workspaces, selected)
	return SidebarProjection{
		Workspaces: workspaces,
		Sessions:   sessions,
	}
}

func selectedWorkspaceGroups(ids []string) map[string]bool {
	selected := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		selected[id] = true
	}
	return selected
}

func filterWorkspacesForSidebar(workspaces []*types.Workspace, selected map[string]bool) []*types.Workspace {
	if len(selected) == 0 {
		return []*types.Workspace{}
	}
	out := make([]*types.Workspace, 0, len(workspaces))
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		groupIDs := ws.GroupIDs
		if len(groupIDs) == 0 {
			if selected["ungrouped"] {
				out = append(out, ws)
			}
			continue
		}
		for _, id := range groupIDs {
			if selected[id] {
				out = append(out, ws)
				break
			}
		}
	}
	return out
}

func filterSessionsForSidebar(
	sessions []*types.Session,
	meta map[string]*types.SessionMeta,
	worktrees map[string][]*types.Worktree,
	workspaces []*types.Workspace,
	selected map[string]bool,
) []*types.Session {
	if len(selected) == 0 {
		return []*types.Session{}
	}
	visibleWorkspaces := map[string]struct{}{}
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		visibleWorkspaces[ws.ID] = struct{}{}
	}
	visibleWorktrees := map[string]struct{}{}
	for wsID := range visibleWorkspaces {
		for _, wt := range worktrees[wsID] {
			if wt == nil {
				continue
			}
			visibleWorktrees[wt.ID] = struct{}{}
		}
	}
	out := make([]*types.Session, 0, len(sessions))
	for _, session := range sessions {
		if session == nil {
			continue
		}
		entry := meta[session.ID]
		workspaceID := ""
		worktreeID := ""
		if entry != nil {
			workspaceID = strings.TrimSpace(entry.WorkspaceID)
			worktreeID = strings.TrimSpace(entry.WorktreeID)
		}
		if worktreeID != "" {
			if _, ok := visibleWorktrees[worktreeID]; ok {
				out = append(out, session)
			}
			continue
		}
		if workspaceID != "" {
			if _, ok := visibleWorkspaces[workspaceID]; ok {
				out = append(out, session)
			}
			continue
		}
		if selected["ungrouped"] {
			out = append(out, session)
		}
	}
	return out
}

func WithSidebarProjectionBuilder(builder SidebarProjectionBuilder) ModelOption {
	return func(m *Model) {
		if m == nil || builder == nil {
			return
		}
		m.sidebarProjectionBuilder = builder
	}
}
