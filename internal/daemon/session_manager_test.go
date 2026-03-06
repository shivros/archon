package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"control/internal/store"
	"control/internal/types"
)

type captureSessionMetadataPublisher struct {
	mu     sync.Mutex
	events []types.MetadataEvent
}

func (c *captureSessionMetadataPublisher) PublishMetadataEvent(event types.MetadataEvent) {
	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
}

func (c *captureSessionMetadataPublisher) Events() []types.MetadataEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]types.MetadataEvent, len(c.events))
	copy(out, c.events)
	return out
}

func TestSessionManagerStartAndTail(t *testing.T) {
	manager := newTestManager(t)

	cfg := StartSessionConfig{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=hello", "stderr=oops", "stdout_lines=2", "stderr_lines=1", "sleep_ms=50", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
		Title:    "demo",
		Tags:     []string{"test"},
	}

	session, err := manager.StartSession(cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected session id")
	}
	if session.Status != types.SessionStatusRunning && session.Status != types.SessionStatusExited {
		t.Fatalf("unexpected status: %s", session.Status)
	}

	waitForStatus(t, manager, session.ID, types.SessionStatusExited, 2*time.Second)

	lines, truncated, order, err := manager.TailSession(session.ID, "combined", 10)
	if err != nil {
		t.Fatalf("TailSession: %v", err)
	}
	if truncated {
		t.Fatalf("expected not truncated")
	}
	if order != "stdout_then_stderr" {
		t.Fatalf("unexpected order: %s", order)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "hello") {
		t.Fatalf("expected stdout in tail")
	}
	if !strings.Contains(joined, "oops") {
		t.Fatalf("expected stderr in tail")
	}

	sessionDir := filepath.Join(manager.baseDir, session.ID)
	stdoutData, err := os.ReadFile(filepath.Join(sessionDir, "stdout.log"))
	if err != nil {
		t.Fatalf("read stdout log: %v", err)
	}
	stderrData, err := os.ReadFile(filepath.Join(sessionDir, "stderr.log"))
	if err != nil {
		t.Fatalf("read stderr log: %v", err)
	}
	if !strings.Contains(string(stdoutData), "hello") {
		t.Fatalf("stdout log missing output")
	}
	if !strings.Contains(string(stderrData), "oops") {
		t.Fatalf("stderr log missing output")
	}
}

func TestSessionManagerKill(t *testing.T) {
	manager := newTestManager(t)

	cfg := StartSessionConfig{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=sleeping", "sleep_ms=2000", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
	}

	session, err := manager.StartSession(cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	if err := manager.KillSession(session.ID); err != nil {
		t.Fatalf("KillSession: %v", err)
	}

	waitForStatus(t, manager, session.ID, types.SessionStatusKilled, 3*time.Second)
}

func TestSessionManagerPublishesSessionExitNotification(t *testing.T) {
	manager := newTestManager(t)
	notifier := newTestNotificationPublisher()
	manager.SetNotificationPublisher(notifier)

	cfg := StartSessionConfig{
		Provider:    "custom",
		Cmd:         os.Args[0],
		Args:        helperArgs("stdout=done", "sleep_ms=20", "exit=0"),
		Env:         []string{"GO_WANT_HELPER_PROCESS=1"},
		WorkspaceID: "ws-test",
		WorktreeID:  "wt-test",
		Title:       "session title",
	}

	session, err := manager.StartSession(cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	waitForStatus(t, manager, session.ID, types.SessionStatusExited, 2*time.Second)

	event, ok := notifier.WaitForEvent(2 * time.Second)
	if !ok {
		t.Fatalf("expected notification event")
	}
	if event.Trigger != types.NotificationTriggerSessionExited {
		t.Fatalf("unexpected trigger: %q", event.Trigger)
	}
	if event.SessionID != session.ID {
		t.Fatalf("unexpected session id: %q", event.SessionID)
	}
	if event.WorkspaceID != "ws-test" || event.WorktreeID != "wt-test" {
		t.Fatalf("unexpected workspace/worktree ids: %q/%q", event.WorkspaceID, event.WorktreeID)
	}
}

func TestTailInvalidStream(t *testing.T) {
	manager := newTestManager(t)

	cfg := StartSessionConfig{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=ok", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
	}

	session, err := manager.StartSession(cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	waitForStatus(t, manager, session.ID, types.SessionStatusExited, 2*time.Second)

	_, _, _, err = manager.TailSession(session.ID, "weird", 10)
	if err == nil {
		t.Fatalf("expected error for invalid stream")
	}
}

func TestUpsertSessionMetaPreservesRenamedTitle(t *testing.T) {
	manager := newTestManager(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	manager.SetMetaStore(metaStore)

	sessionID := "sess-rename"
	cfg := StartSessionConfig{
		Title:        "Default Title",
		InitialInput: "Initial prompt",
	}

	manager.upsertSessionMeta(cfg, sessionID, types.SessionStatusRunning)
	if err := manager.UpdateSessionTitle(sessionID, "Renamed Title"); err != nil {
		t.Fatalf("UpdateSessionTitle: %v", err)
	}
	manager.upsertSessionMeta(cfg, sessionID, types.SessionStatusExited)

	meta, ok, err := metaStore.Get(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("meta get: %v", err)
	}
	if !ok || meta == nil {
		t.Fatalf("expected session meta")
	}
	if meta.Title != "Renamed Title" {
		t.Fatalf("expected renamed title to persist, got %q", meta.Title)
	}
	if !meta.TitleLocked {
		t.Fatalf("expected renamed title to remain locked")
	}
}

func TestUpsertSessionMetaMigratesLegacyCustomTitleToLocked(t *testing.T) {
	manager := newTestManager(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	manager.SetMetaStore(metaStore)

	sessionID := "sess-legacy"
	if _, err := metaStore.Upsert(context.Background(), &types.SessionMeta{
		SessionID: sessionID,
		Title:     "Legacy Custom Title",
	}); err != nil {
		t.Fatalf("seed meta: %v", err)
	}

	cfg := StartSessionConfig{
		Title:        "Default Title",
		InitialInput: "Initial prompt",
	}
	manager.upsertSessionMeta(cfg, sessionID, types.SessionStatusRunning)

	meta, ok, err := metaStore.Get(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("meta get: %v", err)
	}
	if !ok || meta == nil {
		t.Fatalf("expected session meta")
	}
	if meta.Title != "Legacy Custom Title" {
		t.Fatalf("expected legacy custom title to persist, got %q", meta.Title)
	}
	if !meta.TitleLocked {
		t.Fatalf("expected legacy custom title to be migrated to locked")
	}
}

func TestUpdateSessionTitleUpdatesLiveSession(t *testing.T) {
	manager := newTestManager(t)
	cfg := StartSessionConfig{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=hello", "sleep_ms=200", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
		Title:    "before",
	}
	session, err := manager.StartSession(cfg)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if err := manager.UpdateSessionTitle(session.ID, "after"); err != nil {
		t.Fatalf("UpdateSessionTitle: %v", err)
	}
	updated, ok := manager.GetSession(session.ID)
	if !ok || updated == nil {
		t.Fatalf("expected live session")
	}
	if updated.Title != "after" {
		t.Fatalf("expected live session title to update, got %q", updated.Title)
	}
	waitForStatus(t, manager, session.ID, types.SessionStatusExited, 2*time.Second)
}

func TestUpdateSessionTitlePublishesMetadataEvent(t *testing.T) {
	manager := newTestManager(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	manager.SetMetaStore(metaStore)
	manager.SetSessionStore(sessionStore)
	publisher := &captureSessionMetadataPublisher{}
	manager.SetMetadataEventPublisher(publisher)

	now := time.Now().UTC()
	_, err := sessionStore.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s1",
			Provider:  "custom",
			Title:     "before",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	if err := manager.UpdateSessionTitle("s1", "after"); err != nil {
		t.Fatalf("UpdateSessionTitle: %v", err)
	}
	events := publisher.Events()
	if len(events) != 1 {
		t.Fatalf("expected one metadata event, got %d", len(events))
	}
	if events[0].Type != types.MetadataEventTypeSessionUpdated {
		t.Fatalf("unexpected event type: %q", events[0].Type)
	}
	if events[0].Session == nil || events[0].Session.ID != "s1" || events[0].Session.Title != "after" {
		t.Fatalf("unexpected session payload: %#v", events[0].Session)
	}
}

func TestUpdateSessionTitleSkipsNoOpMetadataPublish(t *testing.T) {
	manager := newTestManager(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	manager.SetMetaStore(metaStore)
	manager.SetSessionStore(sessionStore)
	publisher := &captureSessionMetadataPublisher{}
	manager.SetMetadataEventPublisher(publisher)

	now := time.Now().UTC()
	_, err := sessionStore.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s1",
			Provider:  "custom",
			Title:     "same",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	if err := manager.UpdateSessionTitle("s1", "same"); err != nil {
		t.Fatalf("UpdateSessionTitle: %v", err)
	}
	if got := len(publisher.Events()); got != 0 {
		t.Fatalf("expected no metadata event for no-op title update, got %d", got)
	}
}

func TestUpdateSessionTitlePublishesCanonicalTitleValue(t *testing.T) {
	manager := newTestManager(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	manager.SetMetaStore(metaStore)
	manager.SetSessionStore(sessionStore)
	publisher := &captureSessionMetadataPublisher{}
	manager.SetMetadataEventPublisher(publisher)

	now := time.Now().UTC()
	_, err := sessionStore.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "s1",
			Provider:  "custom",
			Title:     "before",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}

	if err := manager.UpdateSessionTitle("s1", "  after  "); err != nil {
		t.Fatalf("UpdateSessionTitle: %v", err)
	}
	events := publisher.Events()
	if len(events) != 1 {
		t.Fatalf("expected one metadata event, got %d", len(events))
	}
	if got := events[0].Session.Title; got != "after" {
		t.Fatalf("expected canonical emitted title 'after', got %q", got)
	}
}

func TestUpdateGeneratedSessionTitleDoesNotLock(t *testing.T) {
	manager := newTestManager(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	manager.SetMetaStore(metaStore)
	manager.SetSessionStore(sessionStore)
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-generated",
			Provider:  "custom",
			Title:     "Fallback",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	_, err = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:    "sess-generated",
		Title:        "Fallback",
		TitleLocked:  false,
		LastActiveAt: &now,
	})
	if err != nil {
		t.Fatalf("seed meta: %v", err)
	}

	if err := manager.UpdateGeneratedSessionTitle("sess-generated", "AI Generated"); err != nil {
		t.Fatalf("UpdateGeneratedSessionTitle: %v", err)
	}
	meta, ok, err := metaStore.Get(ctx, "sess-generated")
	if err != nil {
		t.Fatalf("meta get: %v", err)
	}
	if !ok || meta == nil {
		t.Fatalf("expected meta record")
	}
	if meta.Title != "AI Generated" {
		t.Fatalf("expected updated title, got %q", meta.Title)
	}
	if meta.TitleLocked {
		t.Fatalf("expected generated update to keep title unlocked")
	}
}

type failingTitleSessionIndexStore struct {
	getErr    error
	upsertErr error
	record    *types.SessionRecord
}

func (s *failingTitleSessionIndexStore) ListRecords(context.Context) ([]*types.SessionRecord, error) {
	if s.record == nil {
		return []*types.SessionRecord{}, nil
	}
	return []*types.SessionRecord{s.record}, nil
}

func (s *failingTitleSessionIndexStore) GetRecord(context.Context, string) (*types.SessionRecord, bool, error) {
	if s.getErr != nil {
		return nil, false, s.getErr
	}
	if s.record == nil {
		return nil, false, nil
	}
	return s.record, true, nil
}

func (s *failingTitleSessionIndexStore) UpsertRecord(_ context.Context, record *types.SessionRecord) (*types.SessionRecord, error) {
	if s.upsertErr != nil {
		return nil, s.upsertErr
	}
	s.record = record
	return record, nil
}

func (s *failingTitleSessionIndexStore) DeleteRecord(context.Context, string) error { return nil }

type failingTitleSessionMetaStore struct {
	getErr    error
	upsertErr error
	entry     *types.SessionMeta
}

func (s *failingTitleSessionMetaStore) List(context.Context) ([]*types.SessionMeta, error) {
	if s.entry == nil {
		return []*types.SessionMeta{}, nil
	}
	return []*types.SessionMeta{s.entry}, nil
}

func (s *failingTitleSessionMetaStore) Get(_ context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	if s.getErr != nil {
		return nil, false, s.getErr
	}
	if s.entry == nil || s.entry.SessionID != sessionID {
		return nil, false, nil
	}
	return s.entry, true, nil
}

func (s *failingTitleSessionMetaStore) Upsert(_ context.Context, meta *types.SessionMeta) (*types.SessionMeta, error) {
	if s.upsertErr != nil {
		return nil, s.upsertErr
	}
	s.entry = meta
	return meta, nil
}

func (s *failingTitleSessionMetaStore) Delete(context.Context, string) error { return nil }

func TestUpdateSessionTitleReturnsSessionStoreGetError(t *testing.T) {
	manager := newTestManager(t)
	manager.SetSessionStore(&failingTitleSessionIndexStore{getErr: errors.New("get failed")})
	err := manager.UpdateSessionTitle("s1", "after")
	if err == nil || !strings.Contains(err.Error(), "get failed") {
		t.Fatalf("expected session store get error, got %v", err)
	}
}

func TestUpdateSessionTitleReturnsMetaStoreGetError(t *testing.T) {
	manager := newTestManager(t)
	manager.SetMetaStore(&failingTitleSessionMetaStore{getErr: errors.New("meta get failed")})
	err := manager.UpdateSessionTitle("s1", "after")
	if err == nil || !strings.Contains(err.Error(), "meta get failed") {
		t.Fatalf("expected meta store get error, got %v", err)
	}
}

func TestUpdateSessionTitleReturnsMetaStoreUpsertError(t *testing.T) {
	manager := newTestManager(t)
	manager.SetMetaStore(&failingTitleSessionMetaStore{upsertErr: errors.New("upsert failed")})
	err := manager.UpdateSessionTitle("s1", "after")
	if err == nil || !strings.Contains(err.Error(), "upsert failed") {
		t.Fatalf("expected meta store upsert error, got %v", err)
	}
}

func TestUpdateSessionTitleReturnsSessionStoreUpsertError(t *testing.T) {
	manager := newTestManager(t)
	manager.SetMetaStore(&failingTitleSessionMetaStore{})
	manager.SetSessionStore(&failingTitleSessionIndexStore{
		record: &types.SessionRecord{
			Session: &types.Session{ID: "s1", Title: "before"},
			Source:  sessionSourceInternal,
		},
		upsertErr: errors.New("session upsert failed"),
	})
	err := manager.UpdateSessionTitle("s1", "after")
	if err == nil || !strings.Contains(err.Error(), "session upsert failed") {
		t.Fatalf("expected session store upsert error, got %v", err)
	}
}

func TestUpdateSessionTitlePublishesWhenNoCurrentCanonical(t *testing.T) {
	manager := newTestManager(t)
	manager.SetMetaStore(&failingTitleSessionMetaStore{})
	publisher := &captureSessionMetadataPublisher{}
	manager.SetMetadataEventPublisher(publisher)
	if err := manager.UpdateSessionTitle("s1", "after"); err != nil {
		t.Fatalf("UpdateSessionTitle: %v", err)
	}
	events := publisher.Events()
	if len(events) != 1 {
		t.Fatalf("expected metadata event publish for missing canonical title, got %d", len(events))
	}
	if events[0].Session == nil || events[0].Session.Title != "after" {
		t.Fatalf("unexpected metadata session payload: %#v", events[0].Session)
	}
}

func TestUpdateSessionTitleNoStoresNoop(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.UpdateSessionTitle("s1", "after"); err != nil {
		t.Fatalf("expected nil error when stores are unavailable, got %v", err)
	}
}

func TestRekeySessionMigratesStores(t *testing.T) {
	manager := newTestManager(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	manager.SetMetaStore(metaStore)
	manager.SetSessionStore(sessionStore)
	ctx := context.Background()

	oldID := "random-internal-id"
	newID := "codex-thread-uuid"

	// Seed initial data under the old ID.
	session := &types.Session{
		ID:       oldID,
		Provider: "codex",
		Title:    "My Session",
		Status:   types.SessionStatusRunning,
	}
	state := &sessionRuntime{session: session, done: make(chan struct{})}
	manager.mu.Lock()
	manager.sessions[oldID] = state
	manager.mu.Unlock()

	_, _ = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   oldID,
		Title:       "My Session",
		TitleLocked: true,
		ThreadID:    newID,
	})
	_, _ = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: session,
		Source:  sessionSourceInternal,
	})

	// Create the old log directory.
	oldDir := filepath.Join(manager.baseDir, oldID)
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Perform re-key.
	manager.mu.Lock()
	manager.rekeySession(oldID, newID, state)
	manager.mu.Unlock()

	// In-memory map should be re-keyed.
	if _, ok := manager.sessions[oldID]; ok {
		t.Fatalf("old ID should be removed from sessions map")
	}
	if _, ok := manager.sessions[newID]; !ok {
		t.Fatalf("new ID should be in sessions map")
	}
	if session.ID != newID {
		t.Fatalf("session.ID should be updated to %q, got %q", newID, session.ID)
	}

	// Meta store should have the new entry, not the old one.
	_, oldExists, _ := metaStore.Get(ctx, oldID)
	if oldExists {
		t.Fatalf("old meta entry should be deleted")
	}
	newMeta, newExists, _ := metaStore.Get(ctx, newID)
	if !newExists || newMeta == nil {
		t.Fatalf("new meta entry should exist")
	}
	if newMeta.Title != "My Session" {
		t.Fatalf("meta title should be preserved, got %q", newMeta.Title)
	}
	if !newMeta.TitleLocked {
		t.Fatalf("meta title_locked should be preserved")
	}
	if newMeta.ThreadID != newID {
		t.Fatalf("meta thread_id should be %q, got %q", newID, newMeta.ThreadID)
	}

	// Session index should have the new entry, not the old one.
	_, oldRecordExists, _ := sessionStore.GetRecord(ctx, oldID)
	if oldRecordExists {
		t.Fatalf("old session record should be deleted")
	}
	newRecord, newRecordExists, _ := sessionStore.GetRecord(ctx, newID)
	if !newRecordExists || newRecord.Session == nil {
		t.Fatalf("new session record should exist")
	}
	if newRecord.Session.Title != "My Session" {
		t.Fatalf("session record title should be preserved, got %q", newRecord.Session.Title)
	}

	// Log directory should be renamed.
	newDir := filepath.Join(manager.baseDir, newID)
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("new log directory should exist: %v", err)
	}
	if _, err := os.Stat(oldDir); err == nil {
		t.Fatalf("old log directory should be gone")
	}
}

func TestResolveProviderCustomPath(t *testing.T) {
	provider, err := ResolveProvider("custom", os.Args[0])
	if err != nil {
		t.Fatalf("ResolveProvider: %v", err)
	}
	if provider.Command() == "" {
		t.Fatalf("expected command")
	}
}

func TestResolveProviderUnknown(t *testing.T) {
	_, err := ResolveProvider("nope", "")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func newTestManager(t *testing.T) *SessionManager {
	t.Helper()
	baseDir := t.TempDir()
	manager, err := NewSessionManager(baseDir)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	return manager
}

func helperArgs(args ...string) []string {
	return append([]string{"-test.run=TestHelperProcess", "--"}, args...)
}

func waitForStatus(t *testing.T, manager *SessionManager, id string, status types.SessionStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session, ok := manager.GetSession(id)
		if ok && session.Status == status {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	session, _ := manager.GetSession(id)
	if session == nil {
		t.Fatalf("session not found while waiting for status")
	}
	t.Fatalf("timeout waiting for status %s; got %s", status, session.Status)
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep >= 0 {
		args = args[sep+1:]
	} else {
		args = []string{}
	}

	stdoutText := ""
	stderrText := ""
	stdoutLines := 0
	stderrLines := 0
	var sleepMs int
	exitCode := 0
	argsFile := ""

	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "stdout="):
			stdoutText = strings.TrimPrefix(arg, "stdout=")
			if stdoutLines == 0 {
				stdoutLines = 1
			}
		case strings.HasPrefix(arg, "stderr="):
			stderrText = strings.TrimPrefix(arg, "stderr=")
			if stderrLines == 0 {
				stderrLines = 1
			}
		case strings.HasPrefix(arg, "stdout_lines="):
			_, _ = fmt.Sscanf(strings.TrimPrefix(arg, "stdout_lines="), "%d", &stdoutLines)
		case strings.HasPrefix(arg, "stderr_lines="):
			_, _ = fmt.Sscanf(strings.TrimPrefix(arg, "stderr_lines="), "%d", &stderrLines)
		case strings.HasPrefix(arg, "sleep_ms="):
			_, _ = fmt.Sscanf(strings.TrimPrefix(arg, "sleep_ms="), "%d", &sleepMs)
		case strings.HasPrefix(arg, "exit="):
			fmt.Sscanf(strings.TrimPrefix(arg, "exit="), "%d", &exitCode)
		case strings.HasPrefix(arg, "args_file="):
			argsFile = strings.TrimPrefix(arg, "args_file=")
		}
	}

	if strings.TrimSpace(argsFile) != "" {
		_ = os.WriteFile(argsFile, []byte(strings.Join(args, "\n")), 0o600)
	}

	for i := 0; i < stdoutLines; i++ {
		if stdoutText != "" {
			_, _ = fmt.Fprintln(os.Stdout, stdoutText)
		}
	}
	for i := 0; i < stderrLines; i++ {
		if stderrText != "" {
			fmt.Fprintln(os.Stderr, stderrText)
		}
	}
	if sleepMs > 0 {
		time.Sleep(time.Duration(sleepMs) * time.Millisecond)
	}
	os.Exit(exitCode)
}

type testNotificationPublisher struct {
	ch chan types.NotificationEvent
}

func newTestNotificationPublisher() *testNotificationPublisher {
	return &testNotificationPublisher{ch: make(chan types.NotificationEvent, 8)}
}

func (p *testNotificationPublisher) Publish(event types.NotificationEvent) {
	if p == nil {
		return
	}
	select {
	case p.ch <- event:
	default:
	}
}

func (p *testNotificationPublisher) WaitForEvent(timeout time.Duration) (types.NotificationEvent, bool) {
	if p == nil {
		return types.NotificationEvent{}, false
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case event := <-p.ch:
		return event, true
	case <-timer.C:
		return types.NotificationEvent{}, false
	}
}
