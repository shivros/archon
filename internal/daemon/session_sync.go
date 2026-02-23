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
	paths      WorkspacePathResolver
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
	return NewCodexSyncerWithPathResolver(stores, logger, nil)
}

func NewCodexSyncerWithPathResolver(stores *Stores, logger logging.Logger, paths WorkspacePathResolver) *CodexSyncer {
	if logger == nil {
		logger = logging.Nop()
	}
	syncer := &CodexSyncer{
		logger:  logger,
		paths:   workspacePathResolverOrDefault(paths),
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
	resolver := workspacePathResolverOrDefault(s.paths)
	workspaceSessionPath, err := resolver.ResolveWorkspaceSessionPath(ws)
	if err != nil {
		return err
	}
	worktrees := []*types.Worktree{}
	if s.worktrees != nil {
		entries, err := s.worktrees.ListWorktrees(ctx, ws.ID)
		if err != nil {
			return err
		}
		worktrees = entries
	}

	type syncWorktreePath struct {
		worktreeID string
		path       string
	}
	worktreePaths := make([]string, 0, len(worktrees))
	worktreeSessionPaths := make([]syncWorktreePath, 0, len(worktrees))
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		path, err := resolver.ResolveWorktreeSessionPath(ws, wt)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("codex_sync_worktree_session_path_skipped",
					logging.F("workspace_id", strings.TrimSpace(ws.ID)),
					logging.F("worktree_id", strings.TrimSpace(wt.ID)),
					logging.F("worktree_path", strings.TrimSpace(wt.Path)),
					logging.F("error", err),
				)
			}
			continue
		}
		worktreePaths = append(worktreePaths, path)
		worktreeSessionPaths = append(worktreeSessionPaths, syncWorktreePath{
			worktreeID: wt.ID,
			path:       path,
		})
	}

	if err := s.syncCodexPath(ctx, workspaceSessionPath, ws.RepoPath, ws.ID, "", worktreePaths, snapshot); err != nil {
		return err
	}
	for _, worktreePath := range worktreeSessionPaths {
		if err := s.syncCodexPath(ctx, worktreePath.path, ws.RepoPath, ws.ID, worktreePath.worktreeID, nil, snapshot); err != nil {
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
	existingMeta := snapshot.meta(thread.ID)
	switch s.classifyThread(thread, cwd, exclude, snapshot) {
	case threadDispositionOutOfScope:
		stats.threadsOutOfScope++
		return nil
	case threadDispositionExcluded:
		stats.threadsExcluded++
		return nil
	case threadDispositionDismissed:
		stats.threadsDismissed++
		s.logThreadSkippedAsDismissed(thread, cwd, workspaceID, worktreeID, existingMeta)
		return nil
	default:
		stats.threadsConsidered++
	}

	seen[thread.ID] = struct{}{}
	ownerSessionID, ownerRecord, ownerMeta := resolveThreadOwnerSession(snapshot, thread.ID)
	if ownerSessionID == "" {
		// Never materialize raw codex threads as standalone sessions. Sessions
		// are internal entities and only carry a linked thread id in metadata.
		return nil
	}
	if s.shouldSkipOverwrite(ownerRecord, ownerMeta) {
		s.logThreadSkipOverwrite(thread, ownerRecord, ownerMeta)
		stats.threadsSkipOverwrite++
	}
	if ownerMeta == nil || ownerMeta.DismissedAt == nil {
		lastActive := syncThreadLastActive(thread, time.Now().UTC())
		meta, err := s.upsertSyncedMeta(ctx, ownerSessionID, thread.ID, workspaceID, worktreeID, lastActive)
		if err != nil {
			return err
		}
		snapshot.upsertMeta(meta)
		stats.sessionMetaWritten++
	}
	return nil
}

func resolveThreadOwnerSession(snapshot *syncSnapshot, threadID string) (string, *types.SessionRecord, *types.SessionMeta) {
	threadID = strings.TrimSpace(threadID)
	if snapshot == nil || threadID == "" {
		return "", nil, nil
	}
	// Fast path: session id already matches thread id (the preferred shape).
	if record, ok := snapshot.record(threadID); ok && record != nil && record.Session != nil {
		return threadID, record, snapshot.meta(threadID)
	}

	var chosenSessionID string
	var chosenRecord *types.SessionRecord
	var chosenMeta *types.SessionMeta
	for sessionID, meta := range snapshot.metaBySessionID {
		if meta == nil || strings.TrimSpace(meta.ThreadID) != threadID {
			continue
		}
		record, ok := snapshot.record(sessionID)
		if !ok || record == nil || record.Session == nil {
			continue
		}
		if chosenSessionID == "" {
			chosenSessionID = sessionID
			chosenRecord = record
			chosenMeta = meta
			continue
		}
		currentSource := strings.TrimSpace(record.Source)
		chosenSource := strings.TrimSpace(chosenRecord.Source)
		// Prefer internal records over codex-synced aliases.
		if currentSource == sessionSourceInternal && chosenSource != sessionSourceInternal {
			chosenSessionID = sessionID
			chosenRecord = record
			chosenMeta = meta
		}
	}
	return chosenSessionID, chosenRecord, chosenMeta
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
		s.logStaleSessionRemoved(record, meta, workspaceID, worktreeID)
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

func (s *CodexSyncer) upsertSyncedMeta(ctx context.Context, sessionID, threadID, workspaceID, worktreeID string, lastActive time.Time) (*types.SessionMeta, error) {
	sessionID = strings.TrimSpace(sessionID)
	threadID = strings.TrimSpace(threadID)
	if sessionID == "" || threadID == "" {
		return nil, errors.New("thread/session id required for codex sync metadata update")
	}
	meta := &types.SessionMeta{
		SessionID:    sessionID,
		WorkspaceID:  workspaceID,
		WorktreeID:   worktreeID,
		ThreadID:     threadID,
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

func (s *CodexSyncer) logThreadSkippedAsDismissed(
	thread codexThreadSummary,
	cwd string,
	workspaceID string,
	worktreeID string,
	meta *types.SessionMeta,
) {
	if s == nil || s.logger == nil {
		return
	}
	workflowRunID := ""
	dismissedAt := ""
	if meta != nil {
		workflowRunID = strings.TrimSpace(meta.WorkflowRunID)
		if meta.DismissedAt != nil {
			dismissedAt = meta.DismissedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	s.logger.Info("codex_sync_thread_skipped_dismissed",
		logging.F("thread_id", strings.TrimSpace(thread.ID)),
		logging.F("thread_cwd", strings.TrimSpace(thread.Cwd)),
		logging.F("sync_cwd", strings.TrimSpace(cwd)),
		logging.F("workspace_id", strings.TrimSpace(workspaceID)),
		logging.F("worktree_id", strings.TrimSpace(worktreeID)),
		logging.F("workflow_run_id", workflowRunID),
		logging.F("dismissed_at", dismissedAt),
	)
}

func (s *CodexSyncer) logThreadSkipOverwrite(
	thread codexThreadSummary,
	record *types.SessionRecord,
	meta *types.SessionMeta,
) {
	if s == nil || s.logger == nil {
		return
	}
	source := ""
	sessionStatus := ""
	if record != nil {
		source = strings.TrimSpace(record.Source)
		if record.Session != nil {
			sessionStatus = strings.TrimSpace(string(record.Session.Status))
		}
	}
	workflowRunID := ""
	dismissedAt := ""
	if meta != nil {
		workflowRunID = strings.TrimSpace(meta.WorkflowRunID)
		if meta.DismissedAt != nil {
			dismissedAt = meta.DismissedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	s.logger.Info("codex_sync_thread_skip_overwrite",
		logging.F("thread_id", strings.TrimSpace(thread.ID)),
		logging.F("thread_cwd", strings.TrimSpace(thread.Cwd)),
		logging.F("session_source", source),
		logging.F("session_status", sessionStatus),
		logging.F("workflow_run_id", workflowRunID),
		logging.F("dismissed_at", dismissedAt),
		logging.F("reason", syncSkipOverwriteReason(record, meta)),
	)
}

func syncSkipOverwriteReason(record *types.SessionRecord, meta *types.SessionMeta) string {
	if meta != nil && meta.DismissedAt != nil {
		return "session_dismissed"
	}
	if record != nil && record.Session != nil && isSyncTombstonedStatus(record.Session.Status) {
		return "session_tombstoned"
	}
	if record != nil && strings.TrimSpace(record.Source) == sessionSourceInternal {
		return "internal_source_authoritative"
	}
	return "policy"
}

func (s *CodexSyncer) logStaleSessionRemoved(
	record *types.SessionRecord,
	meta *types.SessionMeta,
	workspaceID string,
	worktreeID string,
) {
	if s == nil || s.logger == nil || record == nil || record.Session == nil {
		return
	}
	workflowRunID := ""
	dismissedAt := ""
	metaWorkspaceID := ""
	metaWorktreeID := ""
	if meta != nil {
		workflowRunID = strings.TrimSpace(meta.WorkflowRunID)
		metaWorkspaceID = strings.TrimSpace(meta.WorkspaceID)
		metaWorktreeID = strings.TrimSpace(meta.WorktreeID)
		if meta.DismissedAt != nil {
			dismissedAt = meta.DismissedAt.UTC().Format(time.RFC3339Nano)
		}
	}
	s.logger.Info("codex_sync_stale_session_removed",
		logging.F("session_id", strings.TrimSpace(record.Session.ID)),
		logging.F("session_source", strings.TrimSpace(record.Source)),
		logging.F("workflow_run_id", workflowRunID),
		logging.F("dismissed_at", dismissedAt),
		logging.F("meta_workspace_id", metaWorkspaceID),
		logging.F("meta_worktree_id", metaWorktreeID),
		logging.F("sync_workspace_id", strings.TrimSpace(workspaceID)),
		logging.F("sync_worktree_id", strings.TrimSpace(worktreeID)),
	)
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
	if strings.TrimSpace(meta.WorkflowRunID) != "" {
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
