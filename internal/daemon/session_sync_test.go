package daemon

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/logging"
	"control/internal/store"
	"control/internal/types"
)

func TestSyncSkipsRekeyedInternalSession(t *testing.T) {
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	ctx := context.Background()

	threadID := "thread-abc-123"

	// Simulate a re-keyed internal session that already uses the thread ID.
	_, _ = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:       threadID,
			Provider: "codex",
			Title:    "User Renamed",
			Status:   types.SessionStatusInactive,
		},
		Source: sessionSourceInternal,
	})
	_, _ = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   threadID,
		Title:       "User Renamed",
		TitleLocked: true,
		ThreadID:    threadID,
	})

	syncer := &CodexSyncer{
		sessions: sessionStore,
		meta:     metaStore,
	}

	// Simulate what syncCodexPath does when it encounters this thread.
	record, ok, _ := sessionStore.GetRecord(ctx, threadID)
	if !ok || record == nil || record.Session == nil {
		t.Fatalf("expected existing record")
	}
	// The syncer should detect source=internal and skip overwriting.
	if record.Source != sessionSourceInternal {
		t.Fatalf("expected source %q, got %q", sessionSourceInternal, record.Source)
	}

	// After the skip, the session record should still have the user's title.
	_ = syncer // suppress unused
	finalRecord, _, _ := sessionStore.GetRecord(ctx, threadID)
	if finalRecord.Session.Title != "User Renamed" {
		t.Fatalf("expected title preserved, got %q", finalRecord.Session.Title)
	}

	// Meta should still have the locked title.
	meta, _, _ := metaStore.Get(ctx, threadID)
	if !meta.TitleLocked {
		t.Fatalf("expected title to remain locked")
	}
}

func TestMigrateCodexDualEntries(t *testing.T) {
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	ctx := context.Background()

	internalID := "random-hex-id"
	threadID := "codex-thread-uuid"

	// Seed old-format dual entries: an internal session with a different thread ID.
	_, _ = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:       internalID,
			Provider: "codex",
			Title:    "User Title",
			Status:   types.SessionStatusInactive,
		},
		Source: sessionSourceInternal,
	})
	_, _ = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   internalID,
		Title:       "User Title",
		TitleLocked: true,
		ThreadID:    threadID,
	})

	// Seed the codex-synced duplicate.
	_, _ = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:       threadID,
			Provider: "codex",
			Title:    "Codex preview text",
			Status:   types.SessionStatusInactive,
		},
		Source: sessionSourceCodex,
	})
	_, _ = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   threadID,
		Title:       "Codex preview text",
		ThreadID:    threadID,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})

	service := &SessionService{
		stores: &Stores{
			Sessions:    sessionStore,
			SessionMeta: metaStore,
		},
	}

	service.migrateCodexDualEntries(ctx)

	// The old internal entry should be gone.
	_, oldExists, _ := sessionStore.GetRecord(ctx, internalID)
	if oldExists {
		t.Fatalf("old internal session record should be deleted")
	}
	_, oldMetaExists, _ := metaStore.Get(ctx, internalID)
	if oldMetaExists {
		t.Fatalf("old internal meta entry should be deleted")
	}

	// The thread ID entry should exist with merged data.
	record, exists, _ := sessionStore.GetRecord(ctx, threadID)
	if !exists || record.Session == nil {
		t.Fatalf("merged session record should exist under thread ID")
	}
	if record.Session.Title != "User Title" {
		t.Fatalf("merged session should have user's title, got %q", record.Session.Title)
	}
	if record.Source != sessionSourceInternal {
		t.Fatalf("merged session should have internal source, got %q", record.Source)
	}

	meta, metaExists, _ := metaStore.Get(ctx, threadID)
	if !metaExists || meta == nil {
		t.Fatalf("merged meta should exist under thread ID")
	}
	if meta.Title != "User Title" {
		t.Fatalf("merged meta should have user's title, got %q", meta.Title)
	}
	if !meta.TitleLocked {
		t.Fatalf("merged meta should have title locked")
	}
	if meta.WorkspaceID != "ws-1" {
		t.Fatalf("merged meta should carry over workspace from codex entry, got %q", meta.WorkspaceID)
	}
	if meta.WorktreeID != "wt-1" {
		t.Fatalf("merged meta should carry over worktree from codex entry, got %q", meta.WorktreeID)
	}
}

func TestMigrateSkipsAlreadyRekeyedSessions(t *testing.T) {
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	ctx := context.Background()

	threadID := "codex-thread-already-rekeyed"

	// A session that was already re-keyed: ID == ThreadID.
	_, _ = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:       threadID,
			Provider: "codex",
			Title:    "Already Good",
			Status:   types.SessionStatusInactive,
		},
		Source: sessionSourceInternal,
	})
	_, _ = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID: threadID,
		Title:     "Already Good",
		ThreadID:  threadID,
	})

	service := &SessionService{
		stores: &Stores{
			Sessions:    sessionStore,
			SessionMeta: metaStore,
		},
	}

	service.migrateCodexDualEntries(ctx)

	// Session should be untouched.
	record, exists, _ := sessionStore.GetRecord(ctx, threadID)
	if !exists || record.Session == nil {
		t.Fatalf("session should still exist")
	}
	if record.Session.Title != "Already Good" {
		t.Fatalf("session title should be unchanged, got %q", record.Session.Title)
	}
}

func TestIsSyncTombstonedStatus(t *testing.T) {
	if !isSyncTombstonedStatus(types.SessionStatusOrphaned) {
		t.Fatalf("expected orphaned status to be tombstoned")
	}
	for _, status := range []types.SessionStatus{
		types.SessionStatusCreated,
		types.SessionStatusStarting,
		types.SessionStatusRunning,
		types.SessionStatusInactive,
		types.SessionStatusExited,
		types.SessionStatusKilled,
		types.SessionStatusFailed,
	} {
		if isSyncTombstonedStatus(status) {
			t.Fatalf("expected %s not to be tombstoned", status)
		}
	}
}

func TestShouldSkipSyncOverwrite(t *testing.T) {
	tests := []struct {
		name   string
		record *types.SessionRecord
		meta   *types.SessionMeta
		want   bool
	}{
		{
			name: "nil record",
			want: false,
		},
		{
			name: "internal record should skip",
			record: &types.SessionRecord{
				Session: &types.Session{Status: types.SessionStatusInactive},
				Source:  sessionSourceInternal,
			},
			want: true,
		},
		{
			name: "orphaned codex should skip",
			record: &types.SessionRecord{
				Session: &types.Session{Status: types.SessionStatusOrphaned},
				Source:  sessionSourceCodex,
			},
			want: true,
		},
		{
			name: "exited codex should not skip",
			record: &types.SessionRecord{
				Session: &types.Session{Status: types.SessionStatusExited},
				Source:  sessionSourceCodex,
			},
			want: false,
		},
		{
			name: "dismissed meta should skip",
			record: &types.SessionRecord{
				Session: &types.Session{Status: types.SessionStatusInactive},
				Source:  sessionSourceCodex,
			},
			meta: &types.SessionMeta{
				SessionID:   "s1",
				DismissedAt: func() *time.Time { t := time.Now().UTC(); return &t }(),
			},
			want: true,
		},
		{
			name: "inactive codex should not skip",
			record: &types.SessionRecord{
				Session: &types.Session{Status: types.SessionStatusInactive},
				Source:  sessionSourceCodex,
			},
			want: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := shouldSkipSyncOverwrite(tc.record, tc.meta)
			if got != tc.want {
				t.Fatalf("shouldSkipSyncOverwrite() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoadSyncSnapshotTracksDismissedSessions(t *testing.T) {
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	ctx := context.Background()
	dismissedAt := time.Now().UTC()

	_, _ = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:       "sess-dismissed",
			Provider: "codex",
			Status:   types.SessionStatusInactive,
		},
		Source: sessionSourceCodex,
	})
	_, _ = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   "sess-dismissed",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		DismissedAt: &dismissedAt,
	})
	_, _ = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:       "sess-active",
			Provider: "codex",
			Status:   types.SessionStatusInactive,
		},
		Source: sessionSourceCodex,
	})
	_, _ = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   "sess-active",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})

	syncer := &CodexSyncer{
		sessions: sessionStore,
		meta:     metaStore,
	}

	snapshot, err := syncer.loadSyncSnapshot(ctx)
	if err != nil {
		t.Fatalf("loadSyncSnapshot() error = %v", err)
	}
	if snapshot == nil {
		t.Fatalf("expected snapshot")
	}
	if !snapshot.isDismissed("sess-dismissed") {
		t.Fatalf("expected dismissed session in snapshot set")
	}
	if snapshot.isDismissed("sess-active") {
		t.Fatalf("did not expect active session in dismissed set")
	}
	if _, ok := snapshot.record("sess-dismissed"); !ok {
		t.Fatalf("expected dismissed record in snapshot")
	}
	if snapshot.meta("sess-active") == nil {
		t.Fatalf("expected active meta in snapshot")
	}
}

func TestRemoveStaleSkipsDismissedSessions(t *testing.T) {
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	ctx := context.Background()
	dismissedAt := time.Now().UTC()

	_, _ = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:       "sess-dismissed",
			Provider: "codex",
			Status:   types.SessionStatusInactive,
		},
		Source: sessionSourceCodex,
	})
	_, _ = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   "sess-dismissed",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		DismissedAt: &dismissedAt,
	})
	_, _ = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:       "sess-stale",
			Provider: "codex",
			Status:   types.SessionStatusInactive,
		},
		Source: sessionSourceCodex,
	})
	_, _ = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   "sess-stale",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	_, _ = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:       "sess-seen",
			Provider: "codex",
			Status:   types.SessionStatusInactive,
		},
		Source: sessionSourceCodex,
	})
	_, _ = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   "sess-seen",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})

	syncer := &CodexSyncer{
		sessions: sessionStore,
		meta:     metaStore,
	}
	snapshot, err := syncer.loadSyncSnapshot(ctx)
	if err != nil {
		t.Fatalf("loadSyncSnapshot() error = %v", err)
	}
	if err := syncer.removeStale(ctx, "ws-1", "wt-1", map[string]struct{}{"sess-seen": {}}, snapshot, nil); err != nil {
		t.Fatalf("removeStale() error = %v", err)
	}

	if _, ok, err := sessionStore.GetRecord(ctx, "sess-dismissed"); err != nil || !ok {
		t.Fatalf("dismissed record should remain, ok=%v err=%v", ok, err)
	}
	if _, ok, err := metaStore.Get(ctx, "sess-dismissed"); err != nil || !ok {
		t.Fatalf("dismissed meta should remain, ok=%v err=%v", ok, err)
	}
	if _, ok, err := sessionStore.GetRecord(ctx, "sess-stale"); err != nil {
		t.Fatalf("get stale record: %v", err)
	} else if ok {
		t.Fatalf("expected stale record to be removed")
	}
	if _, ok, err := metaStore.Get(ctx, "sess-stale"); err != nil {
		t.Fatalf("get stale meta: %v", err)
	} else if ok {
		t.Fatalf("expected stale meta to be removed")
	}
	if _, ok, err := sessionStore.GetRecord(ctx, "sess-seen"); err != nil || !ok {
		t.Fatalf("seen record should remain, ok=%v err=%v", ok, err)
	}
	if _, ok, err := metaStore.Get(ctx, "sess-seen"); err != nil || !ok {
		t.Fatalf("seen meta should remain, ok=%v err=%v", ok, err)
	}
}

func TestDefaultThreadSyncPolicyClassifyThread(t *testing.T) {
	policy := &defaultThreadSyncPolicy{}
	snapshot := &syncSnapshot{
		dismissedSessionID: map[string]struct{}{
			"s-dismissed": {},
		},
	}

	tests := []struct {
		name    string
		thread  codexThreadSummary
		cwd     string
		exclude []string
		want    threadDisposition
	}{
		{
			name:   "out of scope",
			thread: codexThreadSummary{ID: "s-out", Cwd: "/tmp/other"},
			cwd:    "/tmp/repo",
			want:   threadDispositionOutOfScope,
		},
		{
			name:    "excluded path",
			thread:  codexThreadSummary{ID: "s-excluded", Cwd: "/tmp/repo/wt"},
			cwd:     "/tmp/repo",
			exclude: []string{"/tmp/repo/wt"},
			want:    threadDispositionExcluded,
		},
		{
			name:   "dismissed",
			thread: codexThreadSummary{ID: "s-dismissed", Cwd: "/tmp/repo"},
			cwd:    "/tmp/repo",
			want:   threadDispositionDismissed,
		},
		{
			name:   "process",
			thread: codexThreadSummary{ID: "s-process", Cwd: "/tmp/repo"},
			cwd:    "/tmp/repo",
			want:   threadDispositionProcess,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := policy.ClassifyThread(tc.thread, tc.cwd, tc.exclude, snapshot)
			if got != tc.want {
				t.Fatalf("ClassifyThread() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDefaultThreadSyncPolicyShouldRemoveStale(t *testing.T) {
	policy := &defaultThreadSyncPolicy{}
	record := &types.SessionRecord{
		Session: &types.Session{ID: "s1"},
		Source:  sessionSourceCodex,
	}
	meta := &types.SessionMeta{
		SessionID:   "s1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}

	if !policy.ShouldRemoveStale(record, meta, "ws-1", "wt-1", map[string]struct{}{}) {
		t.Fatalf("expected stale codex session to be removable")
	}
	if policy.ShouldRemoveStale(record, meta, "ws-2", "wt-1", map[string]struct{}{}) {
		t.Fatalf("expected workspace mismatch to block removal")
	}
	if policy.ShouldRemoveStale(record, meta, "ws-1", "wt-1", map[string]struct{}{"s1": {}}) {
		t.Fatalf("expected seen session to block removal")
	}
	dismissedAt := time.Now().UTC()
	meta.DismissedAt = &dismissedAt
	if policy.ShouldRemoveStale(record, meta, "ws-1", "wt-1", map[string]struct{}{}) {
		t.Fatalf("expected dismissed session to block removal")
	}
	meta.DismissedAt = nil
	meta.WorkflowRunID = "gwf-1"
	if policy.ShouldRemoveStale(record, meta, "ws-1", "wt-1", map[string]struct{}{}) {
		t.Fatalf("expected workflow-linked session to block stale removal")
	}
}

type captureSyncMetricsSink struct {
	metrics []SyncPathMetric
}

func (s *captureSyncMetricsSink) RecordSyncPath(metric SyncPathMetric) {
	s.metrics = append(s.metrics, metric)
}

func TestRecordSyncPathMetricUsesInjectedSink(t *testing.T) {
	sink := &captureSyncMetricsSink{}
	syncer := &CodexSyncer{
		metrics: sink,
	}
	startedAt := time.Now().Add(-25 * time.Millisecond)
	syncer.recordSyncPathMetric("/tmp/repo", "ws-1", "wt-1", startedAt, &syncPathStats{
		pages:                 2,
		threadsListed:         10,
		threadsDismissed:      5,
		threadsConsidered:     5,
		sessionRecordsWritten: 2,
		sessionMetaWritten:    3,
		staleRemoved:          1,
	})
	if len(sink.metrics) != 1 {
		t.Fatalf("expected one metric call, got %d", len(sink.metrics))
	}
	metric := sink.metrics[0]
	if metric.WorkspaceID != "ws-1" || metric.WorktreeID != "wt-1" {
		t.Fatalf("unexpected target in metric: %#v", metric)
	}
	if metric.ThreadsDismissed != 5 {
		t.Fatalf("expected dismissed count 5, got %d", metric.ThreadsDismissed)
	}
	if metric.DurationMS < 0 {
		t.Fatalf("expected non-negative duration, got %d", metric.DurationMS)
	}
}

func TestProcessThreadLogsDismissedSkipTelemetry(t *testing.T) {
	var logOut bytes.Buffer
	dismissedAt := time.Now().UTC().Add(-time.Minute)
	syncer := &CodexSyncer{
		logger: logging.New(&logOut, logging.Info),
		policy: &defaultThreadSyncPolicy{},
	}
	snapshot := &syncSnapshot{
		recordsBySessionID: map[string]*types.SessionRecord{},
		metaBySessionID: map[string]*types.SessionMeta{
			"s-dismissed": {
				SessionID:     "s-dismissed",
				WorkflowRunID: "gwf-1",
				DismissedAt:   &dismissedAt,
			},
		},
		dismissedSessionID: map[string]struct{}{
			"s-dismissed": {},
		},
	}
	stats := &syncPathStats{}
	err := syncer.processThread(
		context.Background(),
		codexThreadSummary{ID: "s-dismissed", Cwd: "/tmp/repo"},
		"/tmp/repo",
		"ws-1",
		"wt-1",
		nil,
		snapshot,
		map[string]struct{}{},
		stats,
	)
	if err != nil {
		t.Fatalf("processThread: %v", err)
	}
	if stats.threadsDismissed != 1 {
		t.Fatalf("expected dismissed counter increment, got %d", stats.threadsDismissed)
	}
	logs := logOut.String()
	if !strings.Contains(logs, "msg=codex_sync_thread_skipped_dismissed") {
		t.Fatalf("expected dismissed skip telemetry, got %q", logs)
	}
	if !strings.Contains(logs, "thread_id=s-dismissed") {
		t.Fatalf("expected thread id in dismissed telemetry, got %q", logs)
	}
	if !strings.Contains(logs, "workflow_run_id=gwf-1") {
		t.Fatalf("expected workflow run id in dismissed telemetry, got %q", logs)
	}
}

func TestLogStaleSessionRemovedIncludesWorkflowTelemetry(t *testing.T) {
	var logOut bytes.Buffer
	syncer := &CodexSyncer{
		logger: logging.New(&logOut, logging.Info),
	}
	dismissedAt := time.Now().UTC().Add(-2 * time.Minute)
	syncer.logStaleSessionRemoved(
		&types.SessionRecord{
			Session: &types.Session{ID: "sess-stale"},
			Source:  sessionSourceCodex,
		},
		&types.SessionMeta{
			SessionID:     "sess-stale",
			WorkspaceID:   "ws-1",
			WorktreeID:    "wt-1",
			WorkflowRunID: "gwf-1",
			DismissedAt:   &dismissedAt,
		},
		"ws-1",
		"wt-1",
	)
	logs := logOut.String()
	if !strings.Contains(logs, "msg=codex_sync_stale_session_removed") {
		t.Fatalf("expected stale removal telemetry, got %q", logs)
	}
	if !strings.Contains(logs, "session_id=sess-stale") {
		t.Fatalf("expected session id in stale removal telemetry, got %q", logs)
	}
	if !strings.Contains(logs, "workflow_run_id=gwf-1") {
		t.Fatalf("expected workflow run id in stale removal telemetry, got %q", logs)
	}
}

func TestNewCodexSyncerWithPathResolverUsesInjectedResolver(t *testing.T) {
	resolver := &stubWorkspacePathResolver{workspacePath: "/tmp/ws", worktreePath: "/tmp/wt"}
	syncer := NewCodexSyncerWithPathResolver(nil, nil, resolver)
	if syncer == nil {
		t.Fatalf("expected syncer")
	}
	if syncer.paths != resolver {
		t.Fatalf("expected injected resolver to be used")
	}
}

func TestNewCodexSyncerProvidesDefaults(t *testing.T) {
	syncer := NewCodexSyncer(nil, nil)
	if syncer == nil {
		t.Fatalf("expected syncer")
	}
	if syncer.paths == nil {
		t.Fatalf("expected default path resolver")
	}
}

func TestNewCodexSyncerWithPathResolverDefaultsWhenNil(t *testing.T) {
	syncer := NewCodexSyncerWithPathResolver(nil, nil, nil)
	if syncer == nil {
		t.Fatalf("expected syncer")
	}
	if syncer.paths == nil {
		t.Fatalf("expected default path resolver")
	}
}

func TestCodexSyncerSyncWorkspaceReturnsResolverError(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	workspaceStore := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

	repoDir := filepath.Join(base, "repo")
	if err := ensureDir(repoDir); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	ws, err := workspaceStore.Add(ctx, &types.Workspace{RepoPath: repoDir})
	if err != nil {
		t.Fatalf("add workspace: %v", err)
	}

	resolver := &stubWorkspacePathResolver{
		validateErr:   nil,
		workspacePath: "",
		worktreePath:  "",
	}
	syncer := &CodexSyncer{
		workspaces: workspaceStore,
		worktrees:  workspaceStore,
		sessions:   sessionStore,
		meta:       metaStore,
		snapshots: &storeSyncSnapshotLoader{
			sessions: sessionStore,
			meta:     metaStore,
		},
		paths: resolver,
	}
	if err := syncer.SyncWorkspace(ctx, ws.ID); err == nil {
		t.Fatalf("expected resolver error")
	}
}

func TestCodexSyncerSyncAllReturnsListError(t *testing.T) {
	syncer := &CodexSyncer{
		workspaces: &syncWorkspaceListErrorStore{err: errors.New("list failed")},
	}
	err := syncer.SyncAll(context.Background())
	if err == nil || !strings.Contains(err.Error(), "list failed") {
		t.Fatalf("expected list error, got %v", err)
	}
}

type syncWorkspaceListErrorStore struct {
	err error
}

func (s *syncWorkspaceListErrorStore) List(context.Context) ([]*types.Workspace, error) {
	return nil, s.err
}

func (s *syncWorkspaceListErrorStore) Get(context.Context, string) (*types.Workspace, bool, error) {
	return nil, false, nil
}

func (s *syncWorkspaceListErrorStore) Add(context.Context, *types.Workspace) (*types.Workspace, error) {
	return nil, nil
}

func (s *syncWorkspaceListErrorStore) Update(context.Context, *types.Workspace) (*types.Workspace, error) {
	return nil, nil
}

func (s *syncWorkspaceListErrorStore) Delete(context.Context, string) error {
	return nil
}
