package daemon

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/store"
	"control/internal/types"
)

func TestSessionServiceListWithMetaDedupesCodexAliasesWithoutStoreMutation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

	internalID := "75925eb44c64e0717f145a33"
	threadID := "019c3f57-bd61-7bd3-8188-0d00f6122bb3"
	now := time.Now().UTC()

	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        internalID,
			Provider:  "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now.Add(-2 * time.Minute),
			Title:     "Internal session",
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert internal session: %v", err)
	}
	_, err = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        threadID,
			Provider:  "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now.Add(-1 * time.Minute),
			Title:     "Thread alias",
		},
		Source: sessionSourceCodex,
	})
	if err != nil {
		t.Fatalf("upsert codex thread session: %v", err)
	}
	lastActive := now.Add(-30 * time.Second)
	_, err = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:    internalID,
		ThreadID:     threadID,
		LastActiveAt: &lastActive,
	})
	if err != nil {
		t.Fatalf("upsert internal meta: %v", err)
	}
	_, err = metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:    threadID,
		ThreadID:     threadID,
		LastActiveAt: &lastActive,
	})
	if err != nil {
		t.Fatalf("upsert codex meta: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)

	sessions, _, err := service.ListWithMeta(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 deduped session, got %d", len(sessions))
	}
	// Listing should prefer the internal canonical session without mutating IDs.
	if sessions[0].ID != internalID {
		t.Fatalf("expected internal session %q to win dedupe, got %q", internalID, sessions[0].ID)
	}
	if sessions[0].Title != "Internal session" {
		t.Fatalf("expected internal session title to be preserved, got %q", sessions[0].Title)
	}

	// listWithMeta must not rewrite store identity.
	_, internalExists, _ := sessionStore.GetRecord(ctx, internalID)
	if !internalExists {
		t.Fatalf("expected internal session record to remain")
	}
	_, aliasExists, _ := sessionStore.GetRecord(ctx, threadID)
	if !aliasExists {
		t.Fatalf("expected alias session record to remain")
	}
}

func TestSessionServiceListWithMetaNormalizesDetachedActiveSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))

	sessionID := "2524b2a8dc9079f135cdf9fe"
	createdAt := time.Now().UTC()
	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        sessionID,
			Provider:  "codex",
			Status:    types.SessionStatusRunning,
			PID:       4242,
			CreatedAt: createdAt,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert active session: %v", err)
	}

	service := NewSessionService(nil, &Stores{Sessions: sessionStore}, nil, nil)
	sessions, _, err := service.ListWithMeta(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Status != types.SessionStatusInactive {
		t.Fatalf("expected detached session to be inactive, got %s", sessions[0].Status)
	}
	if sessions[0].PID != 0 {
		t.Fatalf("expected detached session pid to be cleared, got %d", sessions[0].PID)
	}

	record, ok, err := sessionStore.GetRecord(ctx, sessionID)
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if !ok || record == nil || record.Session == nil {
		t.Fatalf("expected session record to exist")
	}
	if record.Session.Status != types.SessionStatusInactive {
		t.Fatalf("expected persisted session status to be inactive, got %s", record.Session.Status)
	}
}

func TestSessionServiceListWithMetaKeepsExitedVisible(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))

	sessionID := "sess-exited-visible"
	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        sessionID,
			Provider:  "codex",
			Status:    types.SessionStatusExited,
			CreatedAt: time.Now().UTC(),
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert exited session: %v", err)
	}

	service := NewSessionService(nil, &Stores{Sessions: sessionStore}, nil, nil)
	sessions, _, err := service.ListWithMeta(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != sessionID {
		t.Fatalf("expected exited session in default list, got %#v", sessions)
	}
}

func TestSessionServiceListWithMetaUsesDismissedMetaFilter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

	sessionID := "sess-dismissed-meta"
	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        sessionID,
			Provider:  "codex",
			Status:    types.SessionStatusExited,
			CreatedAt: time.Now().UTC(),
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	dismissedAt := time.Now().UTC().Add(-time.Minute)
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:   sessionID,
		DismissedAt: &dismissedAt,
	}); err != nil {
		t.Fatalf("upsert meta: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)

	defaultList, _, err := service.ListWithMeta(ctx)
	if err != nil {
		t.Fatalf("default list: %v", err)
	}
	if len(defaultList) != 0 {
		t.Fatalf("expected dismissed session hidden from default list, got %#v", defaultList)
	}

	includeDismissedList, _, err := service.ListWithMetaIncludingDismissed(ctx)
	if err != nil {
		t.Fatalf("include dismissed list: %v", err)
	}
	if len(includeDismissedList) != 1 || includeDismissedList[0].ID != sessionID {
		t.Fatalf("expected dismissed session in include_dismissed list, got %#v", includeDismissedList)
	}
}

func TestSessionServiceListWithMetaFiltersWorkflowOwnedByDefault(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

	sessionID := "sess-workflow-owned"
	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        sessionID,
			Provider:  "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: time.Now().UTC(),
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:     sessionID,
		WorkflowRunID: "gwf-1",
	}); err != nil {
		t.Fatalf("upsert meta: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)

	defaultList, _, err := service.ListWithMeta(ctx)
	if err != nil {
		t.Fatalf("default list: %v", err)
	}
	if len(defaultList) != 0 {
		t.Fatalf("expected workflow-owned session hidden from default list, got %#v", defaultList)
	}

	includeWorkflowOwned, _, err := service.ListWithMetaIncludingWorkflowOwned(ctx)
	if err != nil {
		t.Fatalf("include workflow-owned list: %v", err)
	}
	if len(includeWorkflowOwned) != 1 || includeWorkflowOwned[0].ID != sessionID {
		t.Fatalf("expected workflow-owned session in include_workflow_owned list, got %#v", includeWorkflowOwned)
	}
}

func TestSessionServiceListWithMetaIncludesDismissedWorkflowOwnedWhenRequested(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))

	sessionID := "sess-workflow-owned-dismissed"
	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        sessionID,
			Provider:  "codex",
			Status:    types.SessionStatusExited,
			CreatedAt: time.Now().UTC(),
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	dismissedAt := time.Now().UTC().Add(-time.Minute)
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:     sessionID,
		WorkflowRunID: "gwf-1",
		DismissedAt:   &dismissedAt,
	}); err != nil {
		t.Fatalf("upsert meta: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)

	defaultList, _, err := service.ListWithMeta(ctx)
	if err != nil {
		t.Fatalf("default list: %v", err)
	}
	if len(defaultList) != 0 {
		t.Fatalf("expected session hidden from default list, got %#v", defaultList)
	}

	combinedList, _, err := service.ListWithMetaIncludingDismissedAndWorkflowOwned(ctx)
	if err != nil {
		t.Fatalf("combined list: %v", err)
	}
	if len(combinedList) != 1 || combinedList[0].ID != sessionID {
		t.Fatalf("expected session in combined include list, got %#v", combinedList)
	}
}

func TestSessionServiceListWithMetaHidesWorkflowCodexAliasSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))
	now := time.Now().UTC()

	const (
		runID              = "gwf-1"
		internalSessionID  = "sess-internal"
		codexAliasThreadID = "thread-alias"
	)

	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        internalSessionID,
			Provider:  "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert internal session: %v", err)
	}
	_, err = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        codexAliasThreadID,
			Provider:  "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now.Add(-time.Minute),
		},
		Source: sessionSourceCodex,
	})
	if err != nil {
		t.Fatalf("upsert codex alias session: %v", err)
	}
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:     internalSessionID,
		WorkflowRunID: runID,
	}); err != nil {
		t.Fatalf("upsert internal meta: %v", err)
	}
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:     codexAliasThreadID,
		WorkflowRunID: runID,
		ThreadID:      codexAliasThreadID,
	}); err != nil {
		t.Fatalf("upsert alias meta: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:    sessionStore,
		SessionMeta: metaStore,
	}, nil, nil)

	includeWorkflowOwned, _, err := service.ListWithMetaIncludingWorkflowOwned(ctx)
	if err != nil {
		t.Fatalf("include workflow-owned list: %v", err)
	}
	if len(includeWorkflowOwned) != 1 {
		t.Fatalf("expected one canonical session, got %#v", includeWorkflowOwned)
	}
	if includeWorkflowOwned[0].ID != internalSessionID {
		t.Fatalf("expected internal canonical session %q, got %q", internalSessionID, includeWorkflowOwned[0].ID)
	}
}

func TestSessionServiceListWithMetaUsesWorkflowRunCanonicalSessionID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := t.TempDir()
	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))
	metaStore := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))
	runStore := store.NewFileWorkflowRunStore(filepath.Join(base, "workflow_runs.json"))
	now := time.Now().UTC()

	const (
		runID              = "gwf-canonical"
		oldSessionID       = "sess-old"
		canonicalSessionID = "sess-canonical"
	)

	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        oldSessionID,
			Provider:  "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now.Add(-time.Minute),
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert old session: %v", err)
	}
	_, err = sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        canonicalSessionID,
			Provider:  "codex",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert canonical session: %v", err)
	}
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:     oldSessionID,
		WorkflowRunID: runID,
	}); err != nil {
		t.Fatalf("upsert old meta: %v", err)
	}
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:     canonicalSessionID,
		WorkflowRunID: runID,
	}); err != nil {
		t.Fatalf("upsert canonical meta: %v", err)
	}
	if err := runStore.UpsertWorkflowRun(ctx, guidedworkflows.RunStatusSnapshot{
		Run: &guidedworkflows.WorkflowRun{
			ID:        runID,
			SessionID: canonicalSessionID,
			CreatedAt: now,
		},
	}); err != nil {
		t.Fatalf("upsert workflow run: %v", err)
	}

	service := NewSessionService(nil, &Stores{
		Sessions:     sessionStore,
		SessionMeta:  metaStore,
		WorkflowRuns: runStore,
	}, nil, nil)

	includeWorkflowOwned, _, err := service.ListWithMetaIncludingWorkflowOwned(ctx)
	if err != nil {
		t.Fatalf("include workflow-owned list: %v", err)
	}
	if len(includeWorkflowOwned) != 1 {
		t.Fatalf("expected one canonical workflow session, got %#v", includeWorkflowOwned)
	}
	if includeWorkflowOwned[0].ID != canonicalSessionID {
		t.Fatalf("expected canonical session %q, got %q", canonicalSessionID, includeWorkflowOwned[0].ID)
	}
}
