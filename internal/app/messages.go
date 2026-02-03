package app

import (
	"time"

	"control/internal/types"
)

type sessionsWithMetaMsg struct {
	sessions []*types.Session
	meta     []*types.SessionMeta
	err      error
}

type workspacesMsg struct {
	workspaces []*types.Workspace
	err        error
}

type appStateMsg struct {
	state *types.AppState
	err   error
}

type appStateSavedMsg struct {
	state *types.AppState
	err   error
}

type createWorkspaceMsg struct {
	workspace *types.Workspace
	err       error
}

type worktreesMsg struct {
	workspaceID string
	worktrees   []*types.Worktree
	err         error
}

type availableWorktreesMsg struct {
	workspaceID   string
	workspacePath string
	worktrees     []*types.GitWorktree
	err           error
}

type tailMsg struct {
	id    string
	items []map[string]any
	err   error
}

type killMsg struct {
	id  string
	err error
}

type exitMsg struct {
	id  string
	err error
}

type bulkExitMsg struct {
	ids []string
	err error
}

type createWorktreeMsg struct {
	workspaceID string
	worktree    *types.Worktree
	err         error
}

type addWorktreeMsg struct {
	workspaceID string
	worktree    *types.Worktree
	err         error
}

type streamMsg struct {
	id     string
	ch     <-chan types.LogEvent
	cancel func()
	err    error
}

type tickMsg time.Time
