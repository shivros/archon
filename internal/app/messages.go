package app

import (
	"time"

	"control/internal/guidedworkflows"
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

type workspaceGroupsMsg struct {
	groups []*types.WorkspaceGroup
	err    error
}

type appStateMsg struct {
	state *types.AppState
	err   error
}

type appStateInitialLoadMsg struct {
	state *types.AppState
	err   error
}

type appStateSavedMsg struct {
	requestSeq int
	state      *types.AppState
	err        error
}

type appStateSaveFlushMsg struct {
	requestSeq int
}

type providerOptionsMsg struct {
	provider string
	options  *types.ProviderOptionCatalog
	err      error
}

type createWorkspaceMsg struct {
	workspace *types.Workspace
	err       error
}

type createWorkspaceGroupMsg struct {
	group *types.WorkspaceGroup
	err   error
}

type updateWorkspaceGroupMsg struct {
	group *types.WorkspaceGroup
	err   error
}

type deleteWorkspaceGroupMsg struct {
	id  string
	err error
}

type assignGroupWorkspacesMsg struct {
	groupID string
	updated int
	err     error
}
type updateWorkspaceMsg struct {
	workspace *types.Workspace
	err       error
}

type updateSessionMsg struct {
	id  string
	err error
}

type deleteWorkspaceMsg struct {
	id  string
	err error
}

type worktreesMsg struct {
	workspaceID string
	worktrees   []*types.Worktree
	err         error
}

type notesMsg struct {
	scope noteScopeTarget
	notes []*types.Note
	err   error
}

type notesPanelReflowMsg struct{}

type noteCreatedMsg struct {
	note  *types.Note
	scope noteScopeTarget
	err   error
}

type notePinnedMsg struct {
	note      *types.Note
	sessionID string
	err       error
}

type noteDeletedMsg struct {
	id  string
	err error
}

type noteMovedMsg struct {
	note     *types.Note
	previous *types.Note
	err      error
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
	key   string
}

type historyMsg struct {
	id    string
	items []map[string]any
	err   error
	key   string
}

type sessionProjectionSource string

const (
	sessionProjectionSourceHistory sessionProjectionSource = "history"
	sessionProjectionSourceTail    sessionProjectionSource = "tail"
)

type sessionBlocksProjectedMsg struct {
	source        sessionProjectionSource
	id            string
	key           string
	provider      string
	blocks        []ChatBlock
	projectionSeq int
}

type recentsPreviewMsg struct {
	id       string
	revision string
	text     string
	err      error
}

type recentsTurnCompletedMsg struct {
	id           string
	expectedTurn string
	turnID       string
	err          error
}

type historyPollMsg struct {
	id        string
	key       string
	attempt   int
	minAgents int
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

type dismissMsg struct {
	id  string
	err error
}

type bulkDismissMsg struct {
	ids []string
	err error
}

type undismissMsg struct {
	id  string
	err error
}

type bulkUndismissMsg struct {
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

type updateWorktreeMsg struct {
	workspaceID string
	worktree    *types.Worktree
	err         error
}

type worktreeDeletedMsg struct {
	workspaceID string
	worktreeID  string
	err         error
}

type sendMsg struct {
	id     string
	turnID string
	text   string
	err    error
	token  int
}

type startSessionMsg struct {
	session *types.Session
	err     error
}

type approvalMsg struct {
	id        string
	requestID int
	decision  string
	err       error
}

type approvalsMsg struct {
	id        string
	approvals []*types.Approval
	err       error
}

type interruptMsg struct {
	id  string
	err error
}

type streamMsg struct {
	id     string
	ch     <-chan types.LogEvent
	cancel func()
	err    error
}

type eventsMsg struct {
	id     string
	ch     <-chan types.CodexEvent
	cancel func()
	err    error
}

type itemsStreamMsg struct {
	id     string
	ch     <-chan map[string]any
	cancel func()
	err    error
}

type selectDebounceMsg struct {
	id  string
	seq int
}

type clipboardResultMsg struct {
	success string
	err     error
}

type workflowRunCreatedMsg struct {
	run *guidedworkflows.WorkflowRun
	err error
}

type workflowRunsMsg struct {
	runs []*guidedworkflows.WorkflowRun
	err  error
}

type workflowTemplatesMsg struct {
	templates []guidedworkflows.WorkflowTemplate
	err       error
}

type workflowRunStartedMsg struct {
	run *guidedworkflows.WorkflowRun
	err error
}

type workflowRunStoppedMsg struct {
	run *guidedworkflows.WorkflowRun
	err error
}

type workflowRunResumedMsg struct {
	run *guidedworkflows.WorkflowRun
	err error
}

type workflowRunRenamedMsg struct {
	run *guidedworkflows.WorkflowRun
	err error
}

type workflowRunSnapshotMsg struct {
	run      *guidedworkflows.WorkflowRun
	timeline []guidedworkflows.RunTimelineEvent
	err      error
}

type workflowRunDecisionMsg struct {
	run *guidedworkflows.WorkflowRun
	err error
}

type workflowRunVisibilityMsg struct {
	runID     string
	run       *guidedworkflows.WorkflowRun
	err       error
	dismissed bool
}

type tickMsg time.Time
