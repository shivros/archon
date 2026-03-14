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

	"control/internal/providers"
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

type stubThreadProvider struct {
	command  string
	threadID string
}

func (p *stubThreadProvider) Name() string {
	return "custom"
}

func (p *stubThreadProvider) Command() string {
	return p.command
}

func (p *stubThreadProvider) Start(cfg StartSessionConfig, _ ProviderSink, _ ProviderItemSink) (*providerProcess, error) {
	if cfg.OnProviderSessionID != nil {
		cfg.OnProviderSessionID(p.threadID)
	}
	return &providerProcess{
		ThreadID: p.threadID,
		Send: func([]byte) error {
			return nil
		},
		Interrupt: func() error {
			return nil
		},
		Wait: func() error {
			return nil
		},
	}, nil
}

func installStubCustomProvider(t *testing.T, provider Provider) {
	t.Helper()
	originalFactory := providerFactories[providers.RuntimeCustom]
	providerFactories[providers.RuntimeCustom] = func(_ providers.Definition, _ string) (Provider, error) {
		return provider, nil
	}
	t.Cleanup(func() {
		providerFactories[providers.RuntimeCustom] = originalFactory
	})
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

func TestStartSessionKeepsInternalIDWhenProviderReportsRemoteThread(t *testing.T) {
	manager := newTestManager(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	manager.SetMetaStore(metaStore)
	manager.SetSessionStore(sessionStore)
	ctx := context.Background()

	remoteThreadID := "remote-thread-123"
	installStubCustomProvider(t, &stubThreadProvider{
		command:  os.Args[0],
		threadID: remoteThreadID,
	})

	session, err := manager.StartSession(StartSessionConfig{
		Provider: "custom",
		Cmd:      os.Args[0],
		Title:    "Immutable session",
	})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected internal session id")
	}
	if session.ID == remoteThreadID {
		t.Fatalf("session id should remain internal, got remote thread id %q", session.ID)
	}

	waitForStatus(t, manager, session.ID, types.SessionStatusExited, 2*time.Second)

	manager.mu.Lock()
	_, hasInternalRuntime := manager.sessions[session.ID]
	_, hasRemoteRuntime := manager.sessions[remoteThreadID]
	manager.mu.Unlock()
	if !hasInternalRuntime {
		t.Fatalf("expected runtime state to remain keyed by internal session id")
	}
	if hasRemoteRuntime {
		t.Fatalf("did not expect runtime state under remote thread id")
	}

	meta, metaExists, err := metaStore.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("meta get: %v", err)
	}
	if !metaExists || meta == nil {
		t.Fatalf("expected meta entry for internal session id")
	}
	if meta.ThreadID != remoteThreadID {
		t.Fatalf("expected thread id %q, got %q", remoteThreadID, meta.ThreadID)
	}
	if meta.ProviderSessionID != remoteThreadID {
		t.Fatalf("expected provider session id %q, got %q", remoteThreadID, meta.ProviderSessionID)
	}
	if _, remoteMetaExists, _ := metaStore.Get(ctx, remoteThreadID); remoteMetaExists {
		t.Fatalf("did not expect meta entry under remote thread id")
	}

	record, recordExists, err := sessionStore.GetRecord(ctx, session.ID)
	if err != nil {
		t.Fatalf("record get: %v", err)
	}
	if !recordExists || record == nil || record.Session == nil {
		t.Fatalf("expected session record for internal session id")
	}
	if record.Session.ID != session.ID {
		t.Fatalf("expected persisted session id %q, got %q", session.ID, record.Session.ID)
	}
	if _, remoteRecordExists, _ := sessionStore.GetRecord(ctx, remoteThreadID); remoteRecordExists {
		t.Fatalf("did not expect session record under remote thread id")
	}
}

func TestResumeSessionKeepsInternalIDWhenProviderReportsRemoteThread(t *testing.T) {
	manager := newTestManager(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(t.TempDir(), "sessions_index.json"))
	manager.SetMetaStore(metaStore)
	manager.SetSessionStore(sessionStore)
	ctx := context.Background()

	internalSessionID := "sess-internal-123"
	remoteThreadID := "remote-thread-456"
	installStubCustomProvider(t, &stubThreadProvider{
		command:  os.Args[0],
		threadID: remoteThreadID,
	})

	existing := &types.Session{
		ID:        internalSessionID,
		Provider:  "custom",
		Cmd:       os.Args[0],
		Status:    types.SessionStatusInactive,
		CreatedAt: time.Now().UTC(),
	}

	session, err := manager.ResumeSession(StartSessionConfig{
		Provider: "custom",
		Cmd:      os.Args[0],
	}, existing)
	if err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if session.ID != internalSessionID {
		t.Fatalf("expected resumed session id %q, got %q", internalSessionID, session.ID)
	}
	if existing.ID != internalSessionID {
		t.Fatalf("expected original session id to remain %q, got %q", internalSessionID, existing.ID)
	}

	waitForStatus(t, manager, internalSessionID, types.SessionStatusExited, 2*time.Second)

	manager.mu.Lock()
	_, hasInternalRuntime := manager.sessions[internalSessionID]
	_, hasRemoteRuntime := manager.sessions[remoteThreadID]
	manager.mu.Unlock()
	if !hasInternalRuntime {
		t.Fatalf("expected runtime state under internal session id")
	}
	if hasRemoteRuntime {
		t.Fatalf("did not expect runtime state under remote thread id")
	}

	meta, metaExists, err := metaStore.Get(ctx, internalSessionID)
	if err != nil {
		t.Fatalf("meta get: %v", err)
	}
	if !metaExists || meta == nil {
		t.Fatalf("expected meta entry for resumed session")
	}
	if meta.ThreadID != remoteThreadID {
		t.Fatalf("expected thread id %q, got %q", remoteThreadID, meta.ThreadID)
	}
	if meta.ProviderSessionID != remoteThreadID {
		t.Fatalf("expected provider session id %q, got %q", remoteThreadID, meta.ProviderSessionID)
	}
	if _, remoteMetaExists, _ := metaStore.Get(ctx, remoteThreadID); remoteMetaExists {
		t.Fatalf("did not expect meta entry under remote thread id")
	}

	record, recordExists, err := sessionStore.GetRecord(ctx, internalSessionID)
	if err != nil {
		t.Fatalf("record get: %v", err)
	}
	if !recordExists || record == nil || record.Session == nil {
		t.Fatalf("expected resumed session record")
	}
	if record.Session.ID != internalSessionID {
		t.Fatalf("expected persisted session id %q, got %q", internalSessionID, record.Session.ID)
	}
	if _, remoteRecordExists, _ := sessionStore.GetRecord(ctx, remoteThreadID); remoteRecordExists {
		t.Fatalf("did not expect session record under remote thread id")
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
			_, _ = fmt.Sscanf(strings.TrimPrefix(arg, "exit="), "%d", &exitCode)
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
