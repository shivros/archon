package app

import (
	"context"

	"control/internal/client"
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
	WorktreeDeleteAPI
}

type SessionListWithMetaAPI interface {
	ListSessionsWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error)
}

type SessionTailAPI interface {
	TailItems(ctx context.Context, id string, lines int) (*client.TailItemsResponse, error)
}

type SessionHistoryAPI interface {
	History(ctx context.Context, id string, lines int) (*client.TailItemsResponse, error)
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
	SessionTailAPI
	SessionHistoryAPI
	SessionTailStreamAPI
	SessionEventStreamAPI
	SessionItemsStreamAPI
	SessionKillAPI
	SessionMarkExitedAPI
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

func (a *ClientAPI) DeleteWorktree(ctx context.Context, workspaceID, worktreeID string) error {
	return a.client.DeleteWorktree(ctx, workspaceID, worktreeID)
}

func (a *ClientAPI) ListSessionsWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return a.client.ListSessionsWithMeta(ctx)
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
