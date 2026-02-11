package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/store"
	"control/internal/types"
)

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
			fmt.Sscanf(strings.TrimPrefix(arg, "stdout_lines="), "%d", &stdoutLines)
		case strings.HasPrefix(arg, "stderr_lines="):
			fmt.Sscanf(strings.TrimPrefix(arg, "stderr_lines="), "%d", &stderrLines)
		case strings.HasPrefix(arg, "sleep_ms="):
			fmt.Sscanf(strings.TrimPrefix(arg, "sleep_ms="), "%d", &sleepMs)
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
			fmt.Fprintln(os.Stdout, stdoutText)
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
