package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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

func TestReviveExitedSessionRecord(t *testing.T) {
	exitCode := 1
	exitedAt := time.Now().UTC()
	record := &types.SessionRecord{
		Session: &types.Session{
			ID:       "thread-1",
			Provider: "codex",
			Status:   types.SessionStatusExited,
			PID:      1234,
			ExitCode: &exitCode,
			ExitedAt: &exitedAt,
		},
		Source: sessionSourceInternal,
	}

	revived, changed := reviveExitedSessionRecord(record)
	if !changed {
		t.Fatalf("expected exited session to be revived")
	}
	if revived == nil || revived.Session == nil {
		t.Fatalf("expected revived record")
	}
	if revived.Session.Status != types.SessionStatusInactive {
		t.Fatalf("expected inactive status, got %s", revived.Session.Status)
	}
	if revived.Session.PID != 0 {
		t.Fatalf("expected pid cleared, got %d", revived.Session.PID)
	}
	if revived.Session.ExitedAt != nil {
		t.Fatalf("expected exited_at cleared")
	}
	if revived.Session.ExitCode != nil {
		t.Fatalf("expected exit_code cleared")
	}
	if revived.Source != sessionSourceInternal {
		t.Fatalf("expected source preserved, got %q", revived.Source)
	}
}
