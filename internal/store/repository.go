package store

import (
	"context"
	"errors"
	"strings"

	"control/internal/types"
)

const (
	RepositoryBackendFile  = "file"
	RepositoryBackendBbolt = "bbolt"
)

type Repository interface {
	Workspaces() WorkspaceStore
	Worktrees() WorktreeStore
	Groups() WorkspaceGroupStore
	WorkflowTemplates() WorkflowTemplateStore
	AppState() AppStateStore
	SessionMeta() SessionMetaStore
	SessionIndex() SessionIndexStore
	Approvals() ApprovalStore
	Notes() NoteStore
	Backend() string
	Close() error
}

type RepositoryPaths struct {
	WorkspacesPath        string
	WorkflowTemplatesPath string
	AppStatePath          string
	SessionMetaPath       string
	SessionIndexPath      string
	ApprovalsPath         string
	NotesPath             string
	DBPath                string
}

type fileRepository struct {
	workspaces        WorkspaceStore
	worktrees         WorktreeStore
	groups            WorkspaceGroupStore
	workflowTemplates WorkflowTemplateStore
	appState          AppStateStore
	meta              SessionMetaStore
	sessions          SessionIndexStore
	approvals         ApprovalStore
	notes             NoteStore
}

func NewFileRepository(paths RepositoryPaths) Repository {
	workspaces := NewFileWorkspaceStore(paths.WorkspacesPath)
	return &fileRepository{
		workspaces:        workspaces,
		worktrees:         workspaces,
		groups:            workspaces,
		workflowTemplates: NewFileWorkflowTemplateStore(paths.WorkflowTemplatesPath),
		appState:          NewFileAppStateStore(paths.AppStatePath),
		meta:              NewFileSessionMetaStore(paths.SessionMetaPath),
		sessions:          NewFileSessionIndexStore(paths.SessionIndexPath),
		approvals:         NewFileApprovalStore(paths.ApprovalsPath),
		notes:             NewFileNoteStore(paths.NotesPath),
	}
}

func (r *fileRepository) Workspaces() WorkspaceStore {
	return r.workspaces
}

func (r *fileRepository) Worktrees() WorktreeStore {
	return r.worktrees
}

func (r *fileRepository) Groups() WorkspaceGroupStore {
	return r.groups
}

func (r *fileRepository) WorkflowTemplates() WorkflowTemplateStore {
	return r.workflowTemplates
}

func (r *fileRepository) AppState() AppStateStore {
	return r.appState
}

func (r *fileRepository) SessionMeta() SessionMetaStore {
	return r.meta
}

func (r *fileRepository) SessionIndex() SessionIndexStore {
	return r.sessions
}

func (r *fileRepository) Approvals() ApprovalStore {
	return r.approvals
}

func (r *fileRepository) Notes() NoteStore {
	return r.notes
}

func (r *fileRepository) Backend() string {
	return RepositoryBackendFile
}

func (r *fileRepository) Close() error {
	return nil
}

func OpenRepository(paths RepositoryPaths, backend string) (Repository, error) {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", RepositoryBackendBbolt:
		if strings.TrimSpace(paths.DBPath) == "" {
			return nil, errors.New("db path is required for bbolt repository")
		}
		return NewBboltRepository(paths.DBPath)
	case RepositoryBackendFile:
		return NewFileRepository(paths), nil
	default:
		return nil, errors.New("unsupported repository backend: " + backend)
	}
}

// SeedRepositoryFromFiles migrates file-backed metadata into dst when dst is empty.
// This keeps startup backward-compatible for existing users while switching the
// hot path to transactional storage.
func SeedRepositoryFromFiles(ctx context.Context, dst Repository, paths RepositoryPaths) error {
	if dst == nil || dst.Backend() == RepositoryBackendFile {
		return nil
	}
	src := NewFileRepository(paths)
	defer src.Close()

	if err := seedAppState(ctx, dst.AppState(), src.AppState()); err != nil {
		return err
	}
	if err := seedWorkflowTemplates(ctx, dst.WorkflowTemplates(), src.WorkflowTemplates()); err != nil {
		return err
	}
	if err := seedSessionMeta(ctx, dst.SessionMeta(), src.SessionMeta()); err != nil {
		return err
	}
	if err := seedSessionIndex(ctx, dst.SessionIndex(), src.SessionIndex()); err != nil {
		return err
	}
	if err := seedApprovals(ctx, dst.Approvals(), src.Approvals(), dst.SessionIndex(), src.SessionIndex()); err != nil {
		return err
	}
	if err := seedWorkspaceGroups(ctx, dst.Groups(), src.Groups()); err != nil {
		return err
	}
	seededWorkspaces, err := seedWorkspaces(ctx, dst.Workspaces(), src.Workspaces())
	if err != nil {
		return err
	}
	if seededWorkspaces {
		if err := seedWorktrees(ctx, dst.Worktrees(), src.Worktrees(), src.Workspaces()); err != nil {
			return err
		}
	}
	if err := seedNotes(ctx, dst.Notes(), src.Notes()); err != nil {
		return err
	}
	return nil
}

func seedWorkflowTemplates(ctx context.Context, dst WorkflowTemplateStore, src WorkflowTemplateStore) error {
	if dst == nil || src == nil {
		return nil
	}
	current, err := dst.ListWorkflowTemplates(ctx)
	if err != nil {
		return err
	}
	if len(current) > 0 {
		return nil
	}
	legacy, err := src.ListWorkflowTemplates(ctx)
	if err != nil {
		return err
	}
	for _, item := range legacy {
		if _, err := dst.UpsertWorkflowTemplate(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func seedAppState(ctx context.Context, dst AppStateStore, src AppStateStore) error {
	if dst == nil || src == nil {
		return nil
	}
	current, err := dst.Load(ctx)
	if err != nil {
		return err
	}
	if !isZeroAppState(current) {
		return nil
	}
	legacy, err := src.Load(ctx)
	if err != nil {
		return err
	}
	if isZeroAppState(legacy) {
		return nil
	}
	return dst.Save(ctx, legacy)
}

func seedSessionMeta(ctx context.Context, dst SessionMetaStore, src SessionMetaStore) error {
	if dst == nil || src == nil {
		return nil
	}
	current, err := dst.List(ctx)
	if err != nil {
		return err
	}
	if len(current) > 0 {
		return nil
	}
	legacy, err := src.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range legacy {
		if _, err := dst.Upsert(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func seedSessionIndex(ctx context.Context, dst SessionIndexStore, src SessionIndexStore) error {
	if dst == nil || src == nil {
		return nil
	}
	current, err := dst.ListRecords(ctx)
	if err != nil {
		return err
	}
	if len(current) > 0 {
		return nil
	}
	legacy, err := src.ListRecords(ctx)
	if err != nil {
		return err
	}
	for _, item := range legacy {
		if _, err := dst.UpsertRecord(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func seedNotes(ctx context.Context, dst NoteStore, src NoteStore) error {
	if dst == nil || src == nil {
		return nil
	}
	current, err := dst.List(ctx, NoteFilter{})
	if err != nil {
		return err
	}
	if len(current) > 0 {
		return nil
	}
	legacy, err := src.List(ctx, NoteFilter{})
	if err != nil {
		return err
	}
	for _, item := range legacy {
		if _, err := dst.Upsert(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func seedApprovals(ctx context.Context, dst ApprovalStore, src ApprovalStore, dstSessions SessionIndexStore, srcSessions SessionIndexStore) error {
	if dst == nil || src == nil || dstSessions == nil || srcSessions == nil {
		return nil
	}
	sessionIDs := map[string]struct{}{}
	srcRecords, err := srcSessions.ListRecords(ctx)
	if err != nil {
		return err
	}
	for _, record := range srcRecords {
		if record == nil || record.Session == nil || strings.TrimSpace(record.Session.ID) == "" {
			continue
		}
		sessionIDs[record.Session.ID] = struct{}{}
	}
	dstRecords, err := dstSessions.ListRecords(ctx)
	if err != nil {
		return err
	}
	for _, record := range dstRecords {
		if record == nil || record.Session == nil || strings.TrimSpace(record.Session.ID) == "" {
			continue
		}
		sessionIDs[record.Session.ID] = struct{}{}
	}
	for sessionID := range sessionIDs {
		current, err := dst.ListBySession(ctx, sessionID)
		if err != nil {
			return err
		}
		if len(current) > 0 {
			continue
		}
		legacy, err := src.ListBySession(ctx, sessionID)
		if err != nil {
			return err
		}
		for _, approval := range legacy {
			if _, err := dst.Upsert(ctx, approval); err != nil {
				return err
			}
		}
	}
	return nil
}

func seedWorkspaceGroups(ctx context.Context, dst WorkspaceGroupStore, src WorkspaceGroupStore) error {
	if dst == nil || src == nil {
		return nil
	}
	current, err := dst.ListGroups(ctx)
	if err != nil {
		return err
	}
	if len(current) > 0 {
		return nil
	}
	legacy, err := src.ListGroups(ctx)
	if err != nil {
		return err
	}
	for _, item := range legacy {
		if _, err := dst.AddGroup(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func seedWorkspaces(ctx context.Context, dst WorkspaceStore, src WorkspaceStore) (bool, error) {
	if dst == nil || src == nil {
		return false, nil
	}
	current, err := dst.List(ctx)
	if err != nil {
		return false, err
	}
	if len(current) > 0 {
		return false, nil
	}
	legacy, err := src.List(ctx)
	if err != nil {
		return false, err
	}
	for _, item := range legacy {
		if _, err := dst.Add(ctx, item); err != nil {
			return false, err
		}
	}
	return true, nil
}

func seedWorktrees(ctx context.Context, dst WorktreeStore, src WorktreeStore, srcWorkspaces WorkspaceStore) error {
	if dst == nil || src == nil || srcWorkspaces == nil {
		return nil
	}
	workspaces, err := srcWorkspaces.List(ctx)
	if err != nil {
		return err
	}
	for _, ws := range workspaces {
		worktrees, err := src.ListWorktrees(ctx, ws.ID)
		if err != nil {
			return err
		}
		for _, wt := range worktrees {
			if _, err := dst.AddWorktree(ctx, ws.ID, wt); err != nil {
				return err
			}
		}
	}
	return nil
}

func isZeroAppState(state *types.AppState) bool {
	if state == nil {
		return true
	}
	if strings.TrimSpace(state.ActiveWorkspaceID) != "" || strings.TrimSpace(state.ActiveWorktreeID) != "" {
		return false
	}
	if state.SidebarCollapsed {
		return false
	}
	return len(state.ActiveWorkspaceGroupIDs) == 0 &&
		len(state.SidebarWorkspaceExpanded) == 0 &&
		len(state.SidebarWorktreeExpanded) == 0 &&
		len(state.ComposeHistory) == 0 &&
		len(state.ComposeDrafts) == 0 &&
		len(state.NoteDrafts) == 0 &&
		len(state.ComposeDefaultsByProvider) == 0 &&
		len(state.ProviderBadges) == 0 &&
		isZeroRecentsState(state.Recents) &&
		isZeroGuidedWorkflowTelemetryState(state.GuidedWorkflowTelemetry)
}

func isZeroRecentsState(state *types.AppStateRecents) bool {
	if state == nil {
		return true
	}
	return len(state.Running) == 0 &&
		len(state.Ready) == 0 &&
		len(state.ReadyQueue) == 0 &&
		len(state.DismissedTurn) == 0
}

func isZeroGuidedWorkflowTelemetryState(state *types.GuidedWorkflowTelemetryState) bool {
	if state == nil {
		return true
	}
	return state.CapturedAt.IsZero() &&
		state.RunsStarted == 0 &&
		state.RunsCompleted == 0 &&
		state.RunsFailed == 0 &&
		state.PauseCount == 0 &&
		state.PauseRate == 0 &&
		state.ApprovalCount == 0 &&
		state.ApprovalLatencyAvgMS == 0 &&
		state.ApprovalLatencyMaxMS == 0 &&
		len(state.InterventionCauses) == 0
}
