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

type syncSessionRecordStore interface {
	ListRecords(ctx context.Context) ([]*types.SessionRecord, error)
	UpsertRecord(ctx context.Context, record *types.SessionRecord) (*types.SessionRecord, error)
	DeleteRecord(ctx context.Context, sessionID string) error
}

type syncSessionMetaStore interface {
	List(ctx context.Context) ([]*types.SessionMeta, error)
	Upsert(ctx context.Context, meta *types.SessionMeta) (*types.SessionMeta, error)
	Delete(ctx context.Context, sessionID string) error
}

type SyncSnapshotLoader interface {
	Load(ctx context.Context) (*syncSnapshot, error)
}

type ThreadSyncPolicy interface {
	ClassifyThread(thread codexThreadSummary, cwd string, exclude []string, snapshot *syncSnapshot) threadDisposition
	ShouldSkipOverwrite(record *types.SessionRecord, meta *types.SessionMeta) bool
	ShouldRemoveStale(record *types.SessionRecord, meta *types.SessionMeta, workspaceID, worktreeID string, seen map[string]struct{}) bool
}

type SyncMetricsSink interface {
	RecordSyncPath(metric SyncPathMetric)
}

type threadDisposition int

const (
	threadDispositionProcess threadDisposition = iota
	threadDispositionOutOfScope
	threadDispositionExcluded
	threadDispositionDismissed
)

type SyncPathMetric struct {
	WorkspaceID string
	WorktreeID  string
	Cwd         string
	DurationMS  int64

	Pages                 int
	ThreadsListed         int
	ThreadsOutOfScope     int
	ThreadsExcluded       int
	ThreadsDismissed      int
	ThreadsConsidered     int
	ThreadsSkipOverwrite  int
	SessionRecordsWritten int
	SessionMetaWritten    int
	StaleRemoved          int
}

type CodexSyncer struct {
	workspaces WorkspaceStore
	worktrees  WorktreeStore
	sessions   syncSessionRecordStore
	meta       syncSessionMetaStore
	snapshots  SyncSnapshotLoader
	policy     ThreadSyncPolicy
	metrics    SyncMetricsSink
	logger     logging.Logger
}

type syncSnapshot struct {
	recordsBySessionID map[string]*types.SessionRecord
	metaBySessionID    map[string]*types.SessionMeta
	dismissedSessionID map[string]struct{}
}

type syncPathStats struct {
	pages                 int
	threadsListed         int
	threadsOutOfScope     int
	threadsExcluded       int
	threadsDismissed      int
	threadsConsidered     int
	threadsSkipOverwrite  int
	sessionRecordsWritten int
	sessionMetaWritten    int
	staleRemoved          int
}

type storeSyncSnapshotLoader struct {
	sessions syncSessionRecordStore
	meta     syncSessionMetaStore
}

type defaultThreadSyncPolicy struct{}

type logSyncMetricsSink struct {
	logger logging.Logger
}

func NewCodexSyncer(stores *Stores, logger logging.Logger) *CodexSyncer {
	if logger == nil {
		logger = logging.Nop()
	}
	syncer := &CodexSyncer{
		logger:  logger,
		policy:  &defaultThreadSyncPolicy{},
		metrics: &logSyncMetricsSink{logger: logger},
	}
	if stores == nil {
		syncer.snapshots = &storeSyncSnapshotLoader{}
		return syncer
	}
	syncer.workspaces = stores.Workspaces
	syncer.worktrees = stores.Worktrees
	syncer.sessions = stores.Sessions
	syncer.meta = stores.SessionMeta
	syncer.snapshots = &storeSyncSnapshotLoader{
		sessions: stores.Sessions,
		meta:     stores.SessionMeta,
	}
	return syncer
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
	snapshot, err := s.loadSyncSnapshot(ctx)
	if err != nil {
		return err
	}
	ws, ok, err := s.workspaces.Get(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !ok {
		return store.ErrWorkspaceNotFound
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
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		worktreePaths = append(worktreePaths, wt.Path)
	}

	if err := s.syncCodexPath(ctx, ws.RepoPath, ws.RepoPath, ws.ID, "", worktreePaths, snapshot); err != nil {
		return err
	}
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		if err := s.syncCodexPath(ctx, wt.Path, ws.RepoPath, ws.ID, wt.ID, nil, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (s *CodexSyncer) syncCodexPath(ctx context.Context, cwd, workspacePath, workspaceID, worktreeID string, exclude []string, snapshot *syncSnapshot) error {
	startedAt := time.Now()
	syncCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	codexHome := resolveCodexHome(cwd, workspacePath)
	client, err := startCodexAppServer(syncCtx, cwd, codexHome, s.logger)
	if err != nil {
		return err
	}
	defer client.Close()

	seen := make(map[string]struct{})
	stats := &syncPathStats{}
	var cursor *string
	for {
		result, err := client.ListThreads(syncCtx, cursor)
		if err != nil {
			return err
		}
		stats.pages++
		stats.threadsListed += len(result.Data)
		for _, thread := range result.Data {
			if err := s.processThread(syncCtx, thread, cwd, workspaceID, worktreeID, exclude, snapshot, seen, stats); err != nil {
				return err
			}
		}
		if result.NextCursor == nil || *result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}

	if err := s.removeStale(syncCtx, workspaceID, worktreeID, seen, snapshot, stats); err != nil {
		return err
	}
	s.recordSyncPathMetric(cwd, workspaceID, worktreeID, startedAt, stats)
	return nil
}

func (s *CodexSyncer) processThread(ctx context.Context, thread codexThreadSummary, cwd, workspaceID, worktreeID string, exclude []string, snapshot *syncSnapshot, seen map[string]struct{}, stats *syncPathStats) error {
	switch s.classifyThread(thread, cwd, exclude, snapshot) {
	case threadDispositionOutOfScope:
		stats.threadsOutOfScope++
		return nil
	case threadDispositionExcluded:
		stats.threadsExcluded++
		return nil
	case threadDispositionDismissed:
		stats.threadsDismissed++
		return nil
	default:
		stats.threadsConsidered++
	}

	seen[thread.ID] = struct{}{}
	existingMeta := snapshot.meta(thread.ID)
	if existing, ok := snapshot.record(thread.ID); ok && existing != nil && existing.Session != nil {
		// Tombstoned sessions are user-dismissed and must only be restored
		// through explicit undismiss; sync should never revive them.
		// Internal sessions are authoritative and should not be overwritten.
		if s.shouldSkipOverwrite(existing, existingMeta) {
			stats.threadsSkipOverwrite++
			if existingMeta == nil || existingMeta.DismissedAt == nil {
				meta, err := s.upsertSyncedMeta(ctx, thread, workspaceID, worktreeID, time.Now().UTC())
				if err != nil {
					return err
				}
				snapshot.upsertMeta(meta)
				stats.sessionMetaWritten++
			}
			return nil
		}
	}

	if err := s.upsertThreadSession(ctx, thread, cwd, workspaceID, worktreeID, existingMeta, snapshot); err != nil {
		return err
	}
	stats.sessionRecordsWritten++
	stats.sessionMetaWritten++
	return nil
}

func (s *CodexSyncer) upsertThreadSession(ctx context.Context, thread codexThreadSummary, cwd, workspaceID, worktreeID string, existingMeta *types.SessionMeta, snapshot *syncSnapshot) error {
	createdAt := time.Unix(thread.CreatedAt, 0).UTC()
	if thread.CreatedAt == 0 {
		createdAt = time.Now().UTC()
	}
	title := sanitizeTitle(thread.Preview)
	titleLocked := false
	if existingMeta != nil && existingMeta.TitleLocked && strings.TrimSpace(existingMeta.Title) != "" {
		title = existingMeta.Title
		titleLocked = true
	}
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
	if _, err := s.sessions.UpsertRecord(ctx, &types.SessionRecord{
		Session: session,
		Source:  sessionSourceCodex,
	}); err != nil {
		return err
	}
	snapshot.upsertRecord(&types.SessionRecord{Session: session, Source: sessionSourceCodex})

	lastActive := syncThreadLastActive(thread, createdAt)
	meta := &types.SessionMeta{
		SessionID:    thread.ID,
		WorkspaceID:  workspaceID,
		WorktreeID:   worktreeID,
		Title:        title,
		TitleLocked:  titleLocked,
		ThreadID:     thread.ID,
		LastActiveAt: &lastActive,
	}
	if _, err := s.meta.Upsert(ctx, meta); err != nil {
		return err
	}
	snapshot.upsertMeta(meta)
	return nil
}

func (s *CodexSyncer) removeStale(ctx context.Context, workspaceID, worktreeID string, seen map[string]struct{}, snapshot *syncSnapshot, stats *syncPathStats) error {
	if s.sessions == nil || s.meta == nil {
		return nil
	}
	if snapshot == nil {
		var err error
		snapshot, err = s.loadSyncSnapshot(ctx)
		if err != nil {
			return err
		}
	}
	for _, record := range snapshot.recordsBySessionID {
		if record == nil || record.Session == nil {
			continue
		}
		meta := snapshot.meta(record.Session.ID)
		if !s.shouldRemoveStale(record, meta, workspaceID, worktreeID, seen) {
			continue
		}
		_ = s.sessions.DeleteRecord(ctx, record.Session.ID)
		_ = s.meta.Delete(ctx, record.Session.ID)
		snapshot.delete(record.Session.ID)
		if stats != nil {
			stats.staleRemoved++
		}
	}
	return nil
}

func (s *CodexSyncer) classifyThread(thread codexThreadSummary, cwd string, exclude []string, snapshot *syncSnapshot) threadDisposition {
	if s == nil || s.policy == nil {
		return (&defaultThreadSyncPolicy{}).ClassifyThread(thread, cwd, exclude, snapshot)
	}
	return s.policy.ClassifyThread(thread, cwd, exclude, snapshot)
}

func (s *CodexSyncer) shouldSkipOverwrite(record *types.SessionRecord, meta *types.SessionMeta) bool {
	if s == nil || s.policy == nil {
		return (&defaultThreadSyncPolicy{}).ShouldSkipOverwrite(record, meta)
	}
	return s.policy.ShouldSkipOverwrite(record, meta)
}

func (s *CodexSyncer) shouldRemoveStale(record *types.SessionRecord, meta *types.SessionMeta, workspaceID, worktreeID string, seen map[string]struct{}) bool {
	if s == nil || s.policy == nil {
		return (&defaultThreadSyncPolicy{}).ShouldRemoveStale(record, meta, workspaceID, worktreeID, seen)
	}
	return s.policy.ShouldRemoveStale(record, meta, workspaceID, worktreeID, seen)
}

var ErrCodexSyncUnavailable = errors.New("codex sync unavailable")

func syncThreadLastActive(thread codexThreadSummary, fallback time.Time) time.Time {
	lastActive := time.Unix(thread.UpdatedAt, 0).UTC()
	if thread.UpdatedAt == 0 {
		lastActive = fallback
	}
	return lastActive
}

func (s *CodexSyncer) upsertSyncedMeta(ctx context.Context, thread codexThreadSummary, workspaceID, worktreeID string, fallback time.Time) (*types.SessionMeta, error) {
	lastActive := syncThreadLastActive(thread, fallback)
	meta := &types.SessionMeta{
		SessionID:    thread.ID,
		WorkspaceID:  workspaceID,
		WorktreeID:   worktreeID,
		ThreadID:     thread.ID,
		LastActiveAt: &lastActive,
	}
	_, err := s.meta.Upsert(ctx, meta)
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func isSyncTombstonedStatus(status types.SessionStatus) bool {
	switch status {
	case types.SessionStatusOrphaned:
		return true
	default:
		return false
	}
}

func shouldSkipSyncOverwrite(record *types.SessionRecord, meta *types.SessionMeta) bool {
	return (&defaultThreadSyncPolicy{}).ShouldSkipOverwrite(record, meta)
}

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

func (s *CodexSyncer) loadSyncSnapshot(ctx context.Context) (*syncSnapshot, error) {
	if s == nil {
		return &syncSnapshot{
			recordsBySessionID: map[string]*types.SessionRecord{},
			metaBySessionID:    map[string]*types.SessionMeta{},
			dismissedSessionID: map[string]struct{}{},
		}, nil
	}
	loader := s.snapshots
	if loader == nil {
		loader = &storeSyncSnapshotLoader{
			sessions: s.sessions,
			meta:     s.meta,
		}
	}
	return loader.Load(ctx)
}

func (s *syncSnapshot) meta(sessionID string) *types.SessionMeta {
	if s == nil {
		return nil
	}
	return s.metaBySessionID[sessionID]
}

func (s *syncSnapshot) record(sessionID string) (*types.SessionRecord, bool) {
	if s == nil {
		return nil, false
	}
	record, ok := s.recordsBySessionID[sessionID]
	return record, ok
}

func (s *syncSnapshot) isDismissed(sessionID string) bool {
	if s == nil {
		return false
	}
	_, ok := s.dismissedSessionID[sessionID]
	return ok
}

func (s *syncSnapshot) upsertMeta(meta *types.SessionMeta) {
	if s == nil || meta == nil || strings.TrimSpace(meta.SessionID) == "" {
		return
	}
	s.metaBySessionID[meta.SessionID] = meta
	if meta.DismissedAt != nil {
		s.dismissedSessionID[meta.SessionID] = struct{}{}
		return
	}
	delete(s.dismissedSessionID, meta.SessionID)
}

func (s *syncSnapshot) upsertRecord(record *types.SessionRecord) {
	if s == nil || record == nil || record.Session == nil || strings.TrimSpace(record.Session.ID) == "" {
		return
	}
	s.recordsBySessionID[record.Session.ID] = record
}

func (s *syncSnapshot) delete(sessionID string) {
	if s == nil {
		return
	}
	delete(s.recordsBySessionID, sessionID)
	delete(s.metaBySessionID, sessionID)
	delete(s.dismissedSessionID, sessionID)
}

func (s *CodexSyncer) recordSyncPathMetric(cwd, workspaceID, worktreeID string, startedAt time.Time, stats *syncPathStats) {
	if s == nil || stats == nil {
		return
	}
	if s.metrics == nil {
		s.metrics = &logSyncMetricsSink{logger: s.logger}
	}
	s.metrics.RecordSyncPath(SyncPathMetric{
		WorkspaceID: workspaceID,
		WorktreeID:  worktreeID,
		Cwd:         cwd,
		DurationMS:  time.Since(startedAt).Milliseconds(),

		Pages:                 stats.pages,
		ThreadsListed:         stats.threadsListed,
		ThreadsOutOfScope:     stats.threadsOutOfScope,
		ThreadsExcluded:       stats.threadsExcluded,
		ThreadsDismissed:      stats.threadsDismissed,
		ThreadsConsidered:     stats.threadsConsidered,
		ThreadsSkipOverwrite:  stats.threadsSkipOverwrite,
		SessionRecordsWritten: stats.sessionRecordsWritten,
		SessionMetaWritten:    stats.sessionMetaWritten,
		StaleRemoved:          stats.staleRemoved,
	})
}

func (l *storeSyncSnapshotLoader) Load(ctx context.Context) (*syncSnapshot, error) {
	if l == nil || l.sessions == nil || l.meta == nil {
		return &syncSnapshot{
			recordsBySessionID: map[string]*types.SessionRecord{},
			metaBySessionID:    map[string]*types.SessionMeta{},
			dismissedSessionID: map[string]struct{}{},
		}, nil
	}
	records, err := l.sessions.ListRecords(ctx)
	if err != nil {
		return nil, err
	}
	metaEntries, err := l.meta.List(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := &syncSnapshot{
		recordsBySessionID: make(map[string]*types.SessionRecord, len(records)),
		metaBySessionID:    make(map[string]*types.SessionMeta, len(metaEntries)),
		dismissedSessionID: map[string]struct{}{},
	}
	for _, record := range records {
		if record == nil || record.Session == nil {
			continue
		}
		snapshot.recordsBySessionID[record.Session.ID] = record
	}
	for _, meta := range metaEntries {
		if meta == nil || strings.TrimSpace(meta.SessionID) == "" {
			continue
		}
		snapshot.metaBySessionID[meta.SessionID] = meta
		if meta.DismissedAt != nil {
			snapshot.dismissedSessionID[meta.SessionID] = struct{}{}
		}
	}
	return snapshot, nil
}

func (p *defaultThreadSyncPolicy) ClassifyThread(thread codexThreadSummary, cwd string, exclude []string, snapshot *syncSnapshot) threadDisposition {
	if !pathMatchesWorkspace(thread.Cwd, cwd) {
		return threadDispositionOutOfScope
	}
	if matchesAnyPath(thread.Cwd, exclude) {
		return threadDispositionExcluded
	}
	if snapshot != nil && snapshot.isDismissed(thread.ID) {
		return threadDispositionDismissed
	}
	return threadDispositionProcess
}

func (p *defaultThreadSyncPolicy) ShouldSkipOverwrite(record *types.SessionRecord, meta *types.SessionMeta) bool {
	if record == nil || record.Session == nil {
		return false
	}
	if meta != nil && meta.DismissedAt != nil {
		return true
	}
	if isSyncTombstonedStatus(record.Session.Status) {
		return true
	}
	return strings.TrimSpace(record.Source) == sessionSourceInternal
}

func (p *defaultThreadSyncPolicy) ShouldRemoveStale(record *types.SessionRecord, meta *types.SessionMeta, workspaceID, worktreeID string, seen map[string]struct{}) bool {
	if record == nil || record.Session == nil || strings.TrimSpace(record.Source) != sessionSourceCodex {
		return false
	}
	if meta == nil || meta.DismissedAt != nil {
		return false
	}
	if meta.WorkspaceID != workspaceID || meta.WorktreeID != worktreeID {
		return false
	}
	_, ok := seen[record.Session.ID]
	return !ok
}

func (s *logSyncMetricsSink) RecordSyncPath(metric SyncPathMetric) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info("codex_sync_path",
		logging.F("workspace_id", metric.WorkspaceID),
		logging.F("worktree_id", metric.WorktreeID),
		logging.F("cwd", metric.Cwd),
		logging.F("pages", metric.Pages),
		logging.F("threads_listed", metric.ThreadsListed),
		logging.F("threads_out_of_scope", metric.ThreadsOutOfScope),
		logging.F("threads_excluded", metric.ThreadsExcluded),
		logging.F("threads_dismissed_skipped", metric.ThreadsDismissed),
		logging.F("threads_considered", metric.ThreadsConsidered),
		logging.F("threads_skip_overwrite", metric.ThreadsSkipOverwrite),
		logging.F("session_records_written", metric.SessionRecordsWritten),
		logging.F("session_meta_written", metric.SessionMetaWritten),
		logging.F("stale_removed", metric.StaleRemoved),
		logging.F("duration_ms", metric.DurationMS),
	)
}
