package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"control/internal/types"
)

func TestBboltRepositoryCRUD(t *testing.T) {
	repo, err := NewBboltRepository(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("NewBboltRepository: %v", err)
	}
	defer repo.Close()
	ctx := context.Background()

	state := &types.AppState{
		ActiveWorkspaceID:       "ws-1",
		ActiveWorkspaceGroupIDs: []string{"ungrouped"},
		ComposeHistory:          map[string][]string{"s1": []string{"hello"}},
	}
	if err := repo.AppState().Save(ctx, state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	loadedState, err := repo.AppState().Load(ctx)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if loadedState.ActiveWorkspaceID != "ws-1" {
		t.Fatalf("unexpected state: %#v", loadedState)
	}

	workspace, err := repo.Workspaces().Add(ctx, &types.Workspace{
		ID:       "ws-1",
		Name:     "Main",
		RepoPath: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}
	workspaces, err := repo.Workspaces().List(ctx)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].ID != workspace.ID {
		t.Fatalf("unexpected workspaces: %#v", workspaces)
	}
	group, err := repo.Groups().AddGroup(ctx, &types.WorkspaceGroup{Name: "Core"})
	if err != nil {
		t.Fatalf("add group: %v", err)
	}
	if group.ID == "" {
		t.Fatalf("expected group id")
	}
	worktree, err := repo.Worktrees().AddWorktree(ctx, workspace.ID, &types.Worktree{
		Name: "Main WT",
		Path: filepath.Join(t.TempDir(), "repo-main"),
	})
	if err != nil {
		t.Fatalf("add worktree: %v", err)
	}
	if worktree.ID == "" {
		t.Fatalf("expected worktree id")
	}
	approvals, err := repo.Approvals().ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list approvals pre-seed: %v", err)
	}
	if len(approvals) != 0 {
		t.Fatalf("expected no approvals, got %#v", approvals)
	}

	lastActive := time.Now().UTC()
	meta := &types.SessionMeta{
		SessionID:    "s1",
		WorkspaceID:  "ws-1",
		LastActiveAt: &lastActive,
	}
	if _, err := repo.SessionMeta().Upsert(ctx, meta); err != nil {
		t.Fatalf("upsert meta: %v", err)
	}
	metas, err := repo.SessionMeta().List(ctx)
	if err != nil {
		t.Fatalf("list meta: %v", err)
	}
	if len(metas) != 1 || metas[0].SessionID != "s1" {
		t.Fatalf("unexpected metas: %#v", metas)
	}

	record := &types.SessionRecord{
		Session: &types.Session{
			ID:        "s1",
			Provider:  "codex",
			Status:    types.SessionStatusRunning,
			CreatedAt: time.Now().UTC(),
		},
		Source: "internal",
	}
	if _, err := repo.SessionIndex().UpsertRecord(ctx, record); err != nil {
		t.Fatalf("upsert record: %v", err)
	}
	records, err := repo.SessionIndex().ListRecords(ctx)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 1 || records[0].Session == nil || records[0].Session.ID != "s1" {
		t.Fatalf("unexpected records: %#v", records)
	}

	note := &types.Note{
		Scope:       types.NoteScopeSession,
		SessionID:   "s1",
		Title:       "Test Note",
		Body:        "Body",
		Status:      types.NoteStatusTodo,
		WorkspaceID: "ws-1",
	}
	savedNote, err := repo.Notes().Upsert(ctx, note)
	if err != nil {
		t.Fatalf("upsert note: %v", err)
	}
	if savedNote.ID == "" {
		t.Fatalf("expected note id")
	}
	notes, err := repo.Notes().List(ctx, NoteFilter{SessionID: "s1"})
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != savedNote.ID {
		t.Fatalf("unexpected notes: %#v", notes)
	}

	approval, err := repo.Approvals().Upsert(ctx, &types.Approval{
		SessionID: "s1",
		RequestID: 7,
		Method:    "shell",
	})
	if err != nil {
		t.Fatalf("upsert approval: %v", err)
	}
	if approval.CreatedAt.IsZero() {
		t.Fatalf("expected approval created_at")
	}
	approvals, err = repo.Approvals().ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(approvals) != 1 || approvals[0].RequestID != 7 {
		t.Fatalf("unexpected approvals: %#v", approvals)
	}
}

func TestSeedRepositoryFromFiles(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	paths := RepositoryPaths{
		WorkspacesPath:   filepath.Join(base, "workspaces.json"),
		AppStatePath:     filepath.Join(base, "state.json"),
		SessionMetaPath:  filepath.Join(base, "sessions_meta.json"),
		SessionIndexPath: filepath.Join(base, "sessions_index.json"),
		ApprovalsPath:    filepath.Join(base, "approvals.json"),
		NotesPath:        filepath.Join(base, "notes.json"),
		DBPath:           filepath.Join(base, "storage.db"),
	}
	src := NewFileRepository(paths)
	defer src.Close()

	if err := src.AppState().Save(ctx, &types.AppState{ActiveWorkspaceID: "seed-ws"}); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	if _, err := src.SessionMeta().Upsert(ctx, &types.SessionMeta{SessionID: "s1", WorkspaceID: "seed-ws"}); err != nil {
		t.Fatalf("seed meta: %v", err)
	}
	if _, err := src.SessionIndex().UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: time.Now().UTC()},
		Source:  "internal",
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, err := src.Notes().Upsert(ctx, &types.Note{
		Scope:       types.NoteScopeSession,
		SessionID:   "s1",
		WorkspaceID: "seed-ws",
		Title:       "Seed",
		Body:        "Seed note",
	}); err != nil {
		t.Fatalf("seed note: %v", err)
	}
	seedWorkspace, err := src.Workspaces().Add(ctx, &types.Workspace{
		ID:       "seed-ws",
		Name:     "Seed Workspace",
		RepoPath: filepath.Join(base, "repo"),
	})
	if err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := src.Worktrees().AddWorktree(ctx, seedWorkspace.ID, &types.Worktree{
		ID:   "wt-1",
		Name: "Seed WT",
		Path: filepath.Join(base, "repo-wt"),
	}); err != nil {
		t.Fatalf("seed worktree: %v", err)
	}
	if _, err := src.Groups().AddGroup(ctx, &types.WorkspaceGroup{ID: "g-1", Name: "Core"}); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if _, err := src.Approvals().Upsert(ctx, &types.Approval{
		SessionID: "s1",
		RequestID: 9,
		Method:    "shell",
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	dst, err := OpenRepository(paths, RepositoryBackendBbolt)
	if err != nil {
		t.Fatalf("open bbolt repo: %v", err)
	}
	defer dst.Close()

	if err := SeedRepositoryFromFiles(ctx, dst, paths); err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	loadedState, err := dst.AppState().Load(ctx)
	if err != nil {
		t.Fatalf("load seeded state: %v", err)
	}
	if loadedState.ActiveWorkspaceID != "seed-ws" {
		t.Fatalf("expected seeded state, got %#v", loadedState)
	}
	if records, err := dst.SessionIndex().ListRecords(ctx); err != nil || len(records) != 1 {
		t.Fatalf("expected seeded record, got len=%d err=%v", len(records), err)
	}
	if notes, err := dst.Notes().List(ctx, NoteFilter{SessionID: "s1"}); err != nil || len(notes) != 1 {
		t.Fatalf("expected seeded note, got len=%d err=%v", len(notes), err)
	}
	if workspaces, err := dst.Workspaces().List(ctx); err != nil || len(workspaces) != 1 {
		t.Fatalf("expected seeded workspace, got len=%d err=%v", len(workspaces), err)
	}
	if worktrees, err := dst.Worktrees().ListWorktrees(ctx, "seed-ws"); err != nil || len(worktrees) != 1 {
		t.Fatalf("expected seeded worktree, got len=%d err=%v", len(worktrees), err)
	}
	if groups, err := dst.Groups().ListGroups(ctx); err != nil || len(groups) != 1 {
		t.Fatalf("expected seeded group, got len=%d err=%v", len(groups), err)
	}
	if approvals, err := dst.Approvals().ListBySession(ctx, "s1"); err != nil || len(approvals) != 1 {
		t.Fatalf("expected seeded approval, got len=%d err=%v", len(approvals), err)
	}
}
