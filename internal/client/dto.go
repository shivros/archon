package client

import (
	"control/internal/guidedworkflows"
	"control/internal/types"
)

type SessionsResponse struct {
	Sessions []*types.Session `json:"sessions"`
}

type SessionsWithMetaResponse struct {
	Sessions    []*types.Session     `json:"sessions"`
	SessionMeta []*types.SessionMeta `json:"session_meta"`
}

type WorkspacesResponse struct {
	Workspaces []*types.Workspace `json:"workspaces"`
}

type WorkspaceGroupsResponse struct {
	Groups []*types.WorkspaceGroup `json:"groups"`
}

type WorktreesResponse struct {
	Worktrees []*types.Worktree `json:"worktrees"`
}

type NotesResponse struct {
	Notes []*types.Note `json:"notes"`
}

type ListNotesRequest struct {
	Scope       types.NoteScope
	WorkspaceID string
	WorktreeID  string
	SessionID   string
}

type AvailableWorktreesResponse struct {
	Worktrees []*types.GitWorktree `json:"worktrees"`
}

type CreateWorktreeRequest struct {
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Name   string `json:"name,omitempty"`
}

type StartSessionRequest struct {
	Provider              string                           `json:"provider"`
	Cmd                   string                           `json:"cmd,omitempty"`
	Cwd                   string                           `json:"cwd,omitempty"`
	Args                  []string                         `json:"args,omitempty"`
	Env                   []string                         `json:"env,omitempty"`
	Title                 string                           `json:"title,omitempty"`
	Tags                  []string                         `json:"tags,omitempty"`
	WorkspaceID           string                           `json:"workspace_id,omitempty"`
	WorktreeID            string                           `json:"worktree_id,omitempty"`
	Text                  string                           `json:"text,omitempty"`
	RuntimeOptions        *types.SessionRuntimeOptions     `json:"runtime_options,omitempty"`
	NotificationOverrides *types.NotificationSettingsPatch `json:"notification_overrides,omitempty"`
}

type UpdateSessionRequest struct {
	Title                 string                           `json:"title,omitempty"`
	RuntimeOptions        *types.SessionRuntimeOptions     `json:"runtime_options,omitempty"`
	NotificationOverrides *types.NotificationSettingsPatch `json:"notification_overrides,omitempty"`
}

type ProviderOptionsResponse struct {
	Options *types.ProviderOptionCatalog `json:"options"`
}

type PinSessionNoteRequest struct {
	Scope         types.NoteScope  `json:"scope,omitempty"`
	WorkspaceID   string           `json:"workspace_id,omitempty"`
	WorktreeID    string           `json:"worktree_id,omitempty"`
	Title         string           `json:"title,omitempty"`
	Body          string           `json:"body,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
	Status        types.NoteStatus `json:"status,omitempty"`
	SourceBlockID string           `json:"source_block_id,omitempty"`
	SourceRole    string           `json:"source_role,omitempty"`
	SourceSnippet string           `json:"source_snippet,omitempty"`
}

type TailItemsResponse struct {
	Items []map[string]any `json:"items"`
}

type SendSessionRequest struct {
	Text  string           `json:"text,omitempty"`
	Input []map[string]any `json:"input,omitempty"`
}

type SendSessionResponse struct {
	OK     bool   `json:"ok"`
	TurnID string `json:"turn_id,omitempty"`
}

type ApproveSessionRequest struct {
	RequestID      int            `json:"request_id"`
	Decision       string         `json:"decision"`
	Responses      []string       `json:"responses,omitempty"`
	AcceptSettings map[string]any `json:"accept_settings,omitempty"`
}

type ApprovalsResponse struct {
	Approvals []*types.Approval `json:"approvals"`
}

type HealthResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	PID     int    `json:"pid"`
}

type CreateWorkflowRunRequest struct {
	TemplateID      string                                    `json:"template_id,omitempty"`
	WorkspaceID     string                                    `json:"workspace_id,omitempty"`
	WorktreeID      string                                    `json:"worktree_id,omitempty"`
	SessionID       string                                    `json:"session_id,omitempty"`
	TaskID          string                                    `json:"task_id,omitempty"`
	UserPrompt      string                                    `json:"user_prompt,omitempty"`
	PolicyOverrides *guidedworkflows.CheckpointPolicyOverride `json:"policy_overrides,omitempty"`
}

type WorkflowRunDecisionRequest struct {
	Action     guidedworkflows.DecisionAction `json:"action"`
	DecisionID string                         `json:"decision_id,omitempty"`
	Note       string                         `json:"note,omitempty"`
}

type WorkflowRunTimelineResponse struct {
	Timeline []guidedworkflows.RunTimelineEvent `json:"timeline"`
}

type WorkflowRunsResponse struct {
	Runs []*guidedworkflows.WorkflowRun `json:"runs"`
}
