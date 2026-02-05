package daemon

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"control/internal/logging"
	"control/internal/store"
	"control/internal/types"
)

type SessionSyncer interface {
	SyncAll(ctx context.Context) error
	SyncWorkspace(ctx context.Context, workspaceID string) error
}

type CodexSyncer struct {
	workspaces WorkspaceStore
	worktrees  WorktreeStore
	sessions   SessionIndexStore
	meta       SessionMetaStore
	logger     logging.Logger
}

func NewCodexSyncer(stores *Stores, logger logging.Logger) *CodexSyncer {
	if logger == nil {
		logger = logging.Nop()
	}
	if stores == nil {
		return &CodexSyncer{logger: logger}
	}
	return &CodexSyncer{
		workspaces: stores.Workspaces,
		worktrees:  stores.Worktrees,
		sessions:   stores.Sessions,
		meta:       stores.SessionMeta,
		logger:     logger,
	}
}

func (s *CodexSyncer) SyncAll(ctx context.Context) error {
	if s.workspaces == nil {
		return nil
	}
	workspaces, err := s.workspaces.List(ctx)
	if err != nil {
		return err
	}
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		if err := s.SyncWorkspace(ctx, ws.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *CodexSyncer) SyncWorkspace(ctx context.Context, workspaceID string) error {
	if s.workspaces == nil || s.sessions == nil || s.meta == nil {
		return nil
	}
	ws, ok, err := s.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !ok {
		return store.ErrWorkspaceNotFound
	}
	if ws.Provider != "codex" {
		return nil
	}
	worktrees := []*types.Worktree{}
	if s.worktrees != nil {
		entries, err := s.worktrees.ListWorktrees(ctx, ws.ID)
		if err != nil {
			return err
		}
		worktrees = entries
	}

	worktreePaths := make([]string, 0, len(worktrees))
	worktreeIDs := make(map[string]string, len(worktrees))
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		worktreePaths = append(worktreePaths, wt.Path)
		worktreeIDs[wt.Path] = wt.ID
	}

	if err := s.syncCodexPath(ctx, ws.RepoPath, ws.RepoPath, ws.ID, "", worktreePaths); err != nil {
		return err
	}
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		if err := s.syncCodexPath(ctx, wt.Path, ws.RepoPath, ws.ID, wt.ID, nil); err != nil {
			return err
		}
	}
	return nil
}

func (s *CodexSyncer) syncCodexPath(ctx context.Context, cwd, workspacePath, workspaceID, worktreeID string, exclude []string) error {
	syncCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	codexHome := resolveCodexHome(cwd, workspacePath)
	client, err := startCodexAppServer(syncCtx, cwd, codexHome, s.logger)
	if err != nil {
		return err
	}
	defer client.Close()

	seen := make(map[string]struct{})
	var cursor *string
	for {
		result, err := client.ListThreads(syncCtx, cursor)
		if err != nil {
			return err
		}
		for _, thread := range result.Data {
			if !pathMatchesWorkspace(thread.Cwd, cwd) {
				continue
			}
			if matchesAnyPath(thread.Cwd, exclude) {
				continue
			}
			seen[thread.ID] = struct{}{}
			if existing, ok, err := s.sessions.GetRecord(ctx, thread.ID); err == nil && ok {
				if existing.Session != nil && existing.Session.Status == types.SessionStatusExited {
					continue
				}
			}
			createdAt := time.Unix(thread.CreatedAt, 0).UTC()
			if thread.CreatedAt == 0 {
				createdAt = time.Now().UTC()
			}
			title := sanitizeTitle(thread.Preview)
			sessionCwd := cwd
			if strings.TrimSpace(thread.Cwd) != "" {
				sessionCwd = thread.Cwd
			}
			session := &types.Session{
				ID:        thread.ID,
				Provider:  "codex",
				Cwd:       sessionCwd,
				Cmd:       "codex app-server",
				Status:    types.SessionStatusInactive,
				CreatedAt: createdAt,
				Title:     title,
			}
			_, err := s.sessions.UpsertRecord(ctx, &types.SessionRecord{
				Session: session,
				Source:  sessionSourceCodex,
			})
			if err != nil {
				return err
			}
			lastActive := time.Unix(thread.UpdatedAt, 0).UTC()
			if thread.UpdatedAt == 0 {
				lastActive = createdAt
			}
			meta := &types.SessionMeta{
				SessionID:    thread.ID,
				WorkspaceID:  workspaceID,
				WorktreeID:   worktreeID,
				Title:        title,
				ThreadID:     thread.ID,
				LastActiveAt: &lastActive,
			}
			if _, err := s.meta.Upsert(ctx, meta); err != nil {
				return err
			}
		}
		if result.NextCursor == nil || *result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}

	return s.removeStale(syncCtx, workspaceID, worktreeID, seen)
}

func (s *CodexSyncer) removeStale(ctx context.Context, workspaceID, worktreeID string, seen map[string]struct{}) error {
	if s.sessions == nil || s.meta == nil {
		return nil
	}
	records, err := s.sessions.ListRecords(ctx)
	if err != nil {
		return err
	}
	metaEntries, err := s.meta.List(ctx)
	if err != nil {
		return err
	}
	metaBySession := make(map[string]*types.SessionMeta, len(metaEntries))
	for _, entry := range metaEntries {
		if entry == nil {
			continue
		}
		metaBySession[entry.SessionID] = entry
	}
	for _, record := range records {
		if record == nil || record.Session == nil || record.Source != sessionSourceCodex {
			continue
		}
		meta := metaBySession[record.Session.ID]
		if meta == nil {
			continue
		}
		if meta.WorkspaceID != workspaceID || meta.WorktreeID != worktreeID {
			continue
		}
		if _, ok := seen[record.Session.ID]; ok {
			continue
		}
		_ = s.sessions.DeleteRecord(ctx, record.Session.ID)
		_ = s.meta.Delete(ctx, record.Session.ID)
	}
	return nil
}

var ErrCodexSyncUnavailable = errors.New("codex sync unavailable")

func pathMatchesWorkspace(cwd, root string) bool {
	cwd = strings.TrimSpace(cwd)
	root = strings.TrimSpace(root)
	if cwd == "" || root == "" {
		return false
	}
	cleanCwd := filepath.Clean(cwd)
	cleanRoot := filepath.Clean(root)
	if cleanCwd == cleanRoot {
		return true
	}
	if strings.HasPrefix(cleanCwd, cleanRoot+string(filepath.Separator)) {
		return true
	}
	return false
}

func matchesAnyPath(cwd string, roots []string) bool {
	for _, root := range roots {
		if pathMatchesWorkspace(cwd, root) {
			return true
		}
	}
	return false
}
