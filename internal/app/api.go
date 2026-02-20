package app

import (
	"context"
	"errors"

	"control/internal/client"
	"control/internal/guidedworkflows"
	"control/internal/types"
)

type WorkspaceListAPI interface {
	ListWorkspaces(ctx context.Context) ([]*types.Workspace, error)
}

type WorkspaceCreateAPI interface {
	CreateWorkspace(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error)
}

type WorkspaceUpdateAPI interface {
	UpdateWorkspace(ctx context.Context, id string, workspace *types.Workspace) (*types.Workspace, error)
}

type WorkspaceDeleteAPI interface {
	DeleteWorkspace(ctx context.Context, id string) error
}

type WorkspaceGroupListAPI interface {
	ListWorkspaceGroups(ctx context.Context) ([]*types.WorkspaceGroup, error)
}

type WorkspaceGroupCreateAPI interface {
	CreateWorkspaceGroup(ctx context.Context, group *types.WorkspaceGroup) (*types.WorkspaceGroup, error)
}

type WorkspaceGroupUpdateAPI interface {
	UpdateWorkspaceGroup(ctx context.Context, id string, group *types.WorkspaceGroup) (*types.WorkspaceGroup, error)
}

type WorkspaceGroupDeleteAPI interface {
	DeleteWorkspaceGroup(ctx context.Context, id string) error
}

type WorktreeListAPI interface {
	ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error)
}

type AvailableWorktreeListAPI interface {
	ListAvailableWorktrees(ctx context.Context, workspaceID string) ([]*types.GitWorktree, error)
}

type WorktreeAddAPI interface {
	AddWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error)
}

type WorktreeCreateAPI interface {
	CreateWorktree(ctx context.Context, workspaceID string, req client.CreateWorktreeRequest) (*types.Worktree, error)
}

type WorktreeUpdateAPI interface {
	UpdateWorktree(ctx context.Context, workspaceID, worktreeID string, worktree *types.Worktree) (*types.Worktree, error)
}

type WorktreeDeleteAPI interface {
	DeleteWorktree(ctx context.Context, workspaceID, worktreeID string) error
}

type WorkspaceAPI interface {
	WorkspaceListAPI
	WorkspaceCreateAPI
	WorkspaceUpdateAPI
	WorkspaceDeleteAPI
	WorkspaceGroupListAPI
	WorkspaceGroupCreateAPI
	WorkspaceGroupUpdateAPI
	WorkspaceGroupDeleteAPI
	WorktreeListAPI
	AvailableWorktreeListAPI
	WorktreeAddAPI
	WorktreeCreateAPI
	WorktreeUpdateAPI
	WorktreeDeleteAPI
}

type SessionListWithMetaAPI interface {
	ListSessionsWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error)
}

type SessionListWithMetaIncludeDismissedAPI interface {
	ListSessionsWithMetaIncludeDismissed(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error)
}

type SessionListWithMetaIncludeWorkflowOwnedAPI interface {
	ListSessionsWithMetaIncludeWorkflowOwned(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error)
}

type SessionListWithMetaRefreshAPI interface {
	ListSessionsWithMetaRefresh(ctx context.Context, workspaceID string, includeDismissed bool) ([]*types.Session, []*types.SessionMeta, error)
}

type SessionListWithMetaRefreshWithOptionsAPI interface {
	ListSessionsWithMetaRefreshWithOptions(ctx context.Context, workspaceID string, includeDismissed bool, includeWorkflowOwned bool) ([]*types.Session, []*types.SessionMeta, error)
}

type SessionProviderOptionsAPI interface {
	GetProviderOptions(ctx context.Context, provider string) (*types.ProviderOptionCatalog, error)
}

type SessionTailAPI interface {
	TailItems(ctx context.Context, id string, lines int) (*client.TailItemsResponse, error)
}

type SessionHistoryAPI interface {
	History(ctx context.Context, id string, lines int) (*client.TailItemsResponse, error)
}

type SessionSelectionAPI interface {
	SessionListWithMetaAPI
}

type SessionTailStreamAPI interface {
	TailStream(ctx context.Context, id, stream string) (<-chan types.LogEvent, func(), error)
}

type SessionEventStreamAPI interface {
	EventStream(ctx context.Context, id string) (<-chan types.CodexEvent, func(), error)
}

type SessionItemsStreamAPI interface {
	ItemsStream(ctx context.Context, id string) (<-chan map[string]any, func(), error)
}

type SessionKillAPI interface {
	KillSession(ctx context.Context, id string) error
}

type SessionMarkExitedAPI interface {
	MarkSessionExited(ctx context.Context, id string) error
}

type SessionDismissAPI interface {
	DismissSession(ctx context.Context, id string) error
}

type SessionUndismissAPI interface {
	UndismissSession(ctx context.Context, id string) error
}

type SessionUpdateAPI interface {
	UpdateSession(ctx context.Context, id string, req client.UpdateSessionRequest) error
}

type SessionSendAPI interface {
	SendMessage(ctx context.Context, id string, req client.SendSessionRequest) (*client.SendSessionResponse, error)
}

type SessionApproveAPI interface {
	ApproveSession(ctx context.Context, id string, req client.ApproveSessionRequest) error
}

type SessionApprovalsAPI interface {
	ListApprovals(ctx context.Context, id string) ([]*types.Approval, error)
}

type SessionInterruptAPI interface {
	InterruptSession(ctx context.Context, id string) error
}

type WorkspaceSessionStartAPI interface {
	StartWorkspaceSession(ctx context.Context, workspaceID, worktreeID string, req client.StartSessionRequest) (*types.Session, error)
}

type NoteListAPI interface {
	ListNotes(ctx context.Context, req client.ListNotesRequest) ([]*types.Note, error)
}

type NoteCreateAPI interface {
	CreateNote(ctx context.Context, note *types.Note) (*types.Note, error)
}

type NoteUpdateAPI interface {
	UpdateNote(ctx context.Context, id string, note *types.Note) (*types.Note, error)
}

type NoteDeleteAPI interface {
	DeleteNote(ctx context.Context, id string) error
}

type SessionPinAPI interface {
	PinSessionMessage(ctx context.Context, sessionID string, req client.PinSessionNoteRequest) (*types.Note, error)
}

type NotesAPI interface {
	NoteListAPI
	NoteCreateAPI
	NoteUpdateAPI
	NoteDeleteAPI
	SessionPinAPI
}

type SessionAPI interface {
	SessionListWithMetaAPI
	SessionProviderOptionsAPI
	SessionTailAPI
	SessionHistoryAPI
	SessionTailStreamAPI
	SessionEventStreamAPI
	SessionItemsStreamAPI
	SessionKillAPI
	SessionMarkExitedAPI
	SessionDismissAPI
	SessionUndismissAPI
	SessionUpdateAPI
	SessionSendAPI
	SessionApproveAPI
	SessionApprovalsAPI
	SessionInterruptAPI
	WorkspaceSessionStartAPI
	SessionPinAPI
}

type SessionChatAPI interface {
	SessionSendAPI
	SessionEventStreamAPI
}

type AppStateGetAPI interface {
	GetAppState(ctx context.Context) (*types.AppState, error)
}

type AppStateUpdateAPI interface {
	UpdateAppState(ctx context.Context, state *types.AppState) (*types.AppState, error)
}

type StateAPI interface {
	AppStateGetAPI
	AppStateUpdateAPI
}

type GuidedWorkflowTemplateAPI interface {
	ListWorkflowTemplates(ctx context.Context) ([]guidedworkflows.WorkflowTemplate, error)
}

type GuidedWorkflowRunAPI interface {
	ListWorkflowRuns(ctx context.Context) ([]*guidedworkflows.WorkflowRun, error)
	ListWorkflowRunsWithOptions(ctx context.Context, includeDismissed bool) ([]*guidedworkflows.WorkflowRun, error)
	CreateWorkflowRun(ctx context.Context, req client.CreateWorkflowRunRequest) (*guidedworkflows.WorkflowRun, error)
	StartWorkflowRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	DismissWorkflowRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	UndismissWorkflowRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	DecideWorkflowRun(ctx context.Context, runID string, req client.WorkflowRunDecisionRequest) (*guidedworkflows.WorkflowRun, error)
	GetWorkflowRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error)
	GetWorkflowRunTimeline(ctx context.Context, runID string) ([]guidedworkflows.RunTimelineEvent, error)
}

type GuidedWorkflowAPI interface {
	GuidedWorkflowTemplateAPI
	GuidedWorkflowRunAPI
}

type ClientAPI struct {
	client *client.Client
}

func NewClientAPI(client *client.Client) *ClientAPI {
	return &ClientAPI{client: client}
}

func (a *ClientAPI) ListWorkspaces(ctx context.Context) ([]*types.Workspace, error) {
	return a.client.ListWorkspaces(ctx)
}

func (a *ClientAPI) CreateWorkspace(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error) {
	return a.client.CreateWorkspace(ctx, workspace)
}

func (a *ClientAPI) UpdateWorkspace(ctx context.Context, id string, workspace *types.Workspace) (*types.Workspace, error) {
	return a.client.UpdateWorkspace(ctx, id, workspace)
}

func (a *ClientAPI) DeleteWorkspace(ctx context.Context, id string) error {
	return a.client.DeleteWorkspace(ctx, id)
}

func (a *ClientAPI) ListWorkspaceGroups(ctx context.Context) ([]*types.WorkspaceGroup, error) {
	return a.client.ListWorkspaceGroups(ctx)
}

func (a *ClientAPI) CreateWorkspaceGroup(ctx context.Context, group *types.WorkspaceGroup) (*types.WorkspaceGroup, error) {
	return a.client.CreateWorkspaceGroup(ctx, group)
}

func (a *ClientAPI) UpdateWorkspaceGroup(ctx context.Context, id string, group *types.WorkspaceGroup) (*types.WorkspaceGroup, error) {
	return a.client.UpdateWorkspaceGroup(ctx, id, group)
}

func (a *ClientAPI) DeleteWorkspaceGroup(ctx context.Context, id string) error {
	return a.client.DeleteWorkspaceGroup(ctx, id)
}

func (a *ClientAPI) ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error) {
	return a.client.ListWorktrees(ctx, workspaceID)
}

func (a *ClientAPI) ListAvailableWorktrees(ctx context.Context, workspaceID string) ([]*types.GitWorktree, error) {
	return a.client.ListAvailableWorktrees(ctx, workspaceID)
}

func (a *ClientAPI) AddWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error) {
	return a.client.AddWorktree(ctx, workspaceID, worktree)
}

func (a *ClientAPI) CreateWorktree(ctx context.Context, workspaceID string, req client.CreateWorktreeRequest) (*types.Worktree, error) {
	return a.client.CreateWorktree(ctx, workspaceID, req)
}

func (a *ClientAPI) UpdateWorktree(ctx context.Context, workspaceID, worktreeID string, worktree *types.Worktree) (*types.Worktree, error) {
	return a.client.UpdateWorktree(ctx, workspaceID, worktreeID, worktree)
}

func (a *ClientAPI) DeleteWorktree(ctx context.Context, workspaceID, worktreeID string) error {
	return a.client.DeleteWorktree(ctx, workspaceID, worktreeID)
}

func (a *ClientAPI) ListSessionsWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return a.client.ListSessionsWithMeta(ctx)
}

func (a *ClientAPI) ListSessionsWithMetaIncludeDismissed(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return a.client.ListSessionsWithMetaIncludeDismissed(ctx)
}

func (a *ClientAPI) ListSessionsWithMetaIncludeWorkflowOwned(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return a.client.ListSessionsWithMetaIncludeWorkflowOwned(ctx)
}

func (a *ClientAPI) ListSessionsWithMetaRefresh(ctx context.Context, workspaceID string, includeDismissed bool) ([]*types.Session, []*types.SessionMeta, error) {
	return a.client.ListSessionsWithMetaRefresh(ctx, workspaceID, includeDismissed)
}

func (a *ClientAPI) ListSessionsWithMetaRefreshWithOptions(ctx context.Context, workspaceID string, includeDismissed bool, includeWorkflowOwned bool) ([]*types.Session, []*types.SessionMeta, error) {
	return a.client.ListSessionsWithMetaRefreshWithOptions(ctx, workspaceID, includeDismissed, includeWorkflowOwned)
}

func (a *ClientAPI) GetProviderOptions(ctx context.Context, provider string) (*types.ProviderOptionCatalog, error) {
	return a.client.GetProviderOptions(ctx, provider)
}

func (a *ClientAPI) TailItems(ctx context.Context, id string, lines int) (*client.TailItemsResponse, error) {
	return a.client.TailItems(ctx, id, lines)
}

func (a *ClientAPI) History(ctx context.Context, id string, lines int) (*client.TailItemsResponse, error) {
	return a.client.History(ctx, id, lines)
}

func (a *ClientAPI) TailStream(ctx context.Context, id, stream string) (<-chan types.LogEvent, func(), error) {
	return a.client.TailStream(ctx, id, stream)
}

func (a *ClientAPI) EventStream(ctx context.Context, id string) (<-chan types.CodexEvent, func(), error) {
	return a.client.EventStream(ctx, id)
}

func (a *ClientAPI) ItemsStream(ctx context.Context, id string) (<-chan map[string]any, func(), error) {
	return a.client.ItemsStream(ctx, id)
}

func (a *ClientAPI) KillSession(ctx context.Context, id string) error {
	return a.client.KillSession(ctx, id)
}

func (a *ClientAPI) MarkSessionExited(ctx context.Context, id string) error {
	return a.client.MarkSessionExited(ctx, id)
}

func (a *ClientAPI) DismissSession(ctx context.Context, id string) error {
	return a.client.DismissSession(ctx, id)
}

func (a *ClientAPI) UndismissSession(ctx context.Context, id string) error {
	return a.client.UndismissSession(ctx, id)
}

func (a *ClientAPI) UpdateSession(ctx context.Context, id string, req client.UpdateSessionRequest) error {
	return a.client.UpdateSession(ctx, id, req)
}

func (a *ClientAPI) SendMessage(ctx context.Context, id string, req client.SendSessionRequest) (*client.SendSessionResponse, error) {
	return a.client.SendMessage(ctx, id, req)
}

func (a *ClientAPI) ApproveSession(ctx context.Context, id string, req client.ApproveSessionRequest) error {
	return a.client.ApproveSession(ctx, id, req)
}

func (a *ClientAPI) ListApprovals(ctx context.Context, id string) ([]*types.Approval, error) {
	return a.client.ListApprovals(ctx, id)
}

func (a *ClientAPI) InterruptSession(ctx context.Context, id string) error {
	return a.client.InterruptSession(ctx, id)
}

func (a *ClientAPI) StartWorkspaceSession(ctx context.Context, workspaceID, worktreeID string, req client.StartSessionRequest) (*types.Session, error) {
	return a.client.StartWorkspaceSession(ctx, workspaceID, worktreeID, req)
}

func (a *ClientAPI) ListNotes(ctx context.Context, req client.ListNotesRequest) ([]*types.Note, error) {
	return a.client.ListNotes(ctx, req)
}

func (a *ClientAPI) CreateNote(ctx context.Context, note *types.Note) (*types.Note, error) {
	return a.client.CreateNote(ctx, note)
}

func (a *ClientAPI) UpdateNote(ctx context.Context, id string, note *types.Note) (*types.Note, error) {
	return a.client.UpdateNote(ctx, id, note)
}

func (a *ClientAPI) DeleteNote(ctx context.Context, id string) error {
	return a.client.DeleteNote(ctx, id)
}

func (a *ClientAPI) PinSessionMessage(ctx context.Context, sessionID string, req client.PinSessionNoteRequest) (*types.Note, error) {
	return a.client.PinSessionMessage(ctx, sessionID, req)
}

func (a *ClientAPI) GetAppState(ctx context.Context) (*types.AppState, error) {
	return a.client.GetAppState(ctx)
}

func (a *ClientAPI) UpdateAppState(ctx context.Context, state *types.AppState) (*types.AppState, error) {
	return a.client.UpdateAppState(ctx, state)
}

func (a *ClientAPI) CreateWorkflowRun(ctx context.Context, req client.CreateWorkflowRunRequest) (*guidedworkflows.WorkflowRun, error) {
	return a.client.CreateWorkflowRun(ctx, req)
}

func (a *ClientAPI) ListWorkflowTemplates(ctx context.Context) ([]guidedworkflows.WorkflowTemplate, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("client is unavailable")
	}
	return a.client.ListWorkflowTemplates(ctx)
}

func (a *ClientAPI) ListWorkflowRuns(ctx context.Context) ([]*guidedworkflows.WorkflowRun, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("client is unavailable")
	}
	return a.client.ListWorkflowRuns(ctx)
}

func (a *ClientAPI) ListWorkflowRunsWithOptions(ctx context.Context, includeDismissed bool) ([]*guidedworkflows.WorkflowRun, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("client is unavailable")
	}
	return a.client.ListWorkflowRunsWithOptions(ctx, includeDismissed)
}

func (a *ClientAPI) StartWorkflowRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error) {
	return a.client.StartWorkflowRun(ctx, runID)
}

func (a *ClientAPI) DismissWorkflowRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error) {
	return a.client.DismissWorkflowRun(ctx, runID)
}

func (a *ClientAPI) UndismissWorkflowRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error) {
	return a.client.UndismissWorkflowRun(ctx, runID)
}

func (a *ClientAPI) DecideWorkflowRun(ctx context.Context, runID string, req client.WorkflowRunDecisionRequest) (*guidedworkflows.WorkflowRun, error) {
	return a.client.DecideWorkflowRun(ctx, runID, req)
}

func (a *ClientAPI) GetWorkflowRun(ctx context.Context, runID string) (*guidedworkflows.WorkflowRun, error) {
	return a.client.GetWorkflowRun(ctx, runID)
}

func (a *ClientAPI) GetWorkflowRunTimeline(ctx context.Context, runID string) ([]guidedworkflows.RunTimelineEvent, error) {
	return a.client.GetWorkflowRunTimeline(ctx, runID)
}
