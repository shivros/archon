package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"control/internal/config"
	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/store"
	"control/internal/types"
)

type recordGuidedWorkflowPolicyResolver struct {
	calls    int
	resolved *guidedworkflows.CheckpointPolicyOverride
}

func (r *recordGuidedWorkflowPolicyResolver) ResolvePolicyOverrides(explicit *guidedworkflows.CheckpointPolicyOverride) *guidedworkflows.CheckpointPolicyOverride {
	r.calls++
	if r.resolved != nil {
		return guidedworkflows.CloneCheckpointPolicyOverride(r.resolved)
	}
	return guidedworkflows.CloneCheckpointPolicyOverride(explicit)
}

func TestWorkflowRunEndpointsLifecycle(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:  guidedworkflows.TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		UserPrompt:  "Fix parser bug in workflow summaries",
	})
	if created.ID == "" {
		t.Fatalf("expected run id")
	}
	if created.Status != guidedworkflows.WorkflowRunStatusCreated {
		t.Fatalf("expected created status, got %q", created.Status)
	}
	if created.UserPrompt != "Fix parser bug in workflow summaries" {
		t.Fatalf("expected user prompt on created run, got %q", created.UserPrompt)
	}
	if created.DisplayUserPrompt != created.UserPrompt {
		t.Fatalf("expected display prompt on created run, got %q", created.DisplayUserPrompt)
	}

	started := postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	if started.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", started.Status)
	}
	if started.UserPrompt != created.UserPrompt {
		t.Fatalf("expected user prompt to carry into started run, got %q", started.UserPrompt)
	}
	if started.DisplayUserPrompt != created.DisplayUserPrompt {
		t.Fatalf("expected display prompt to carry into started run, got %q", started.DisplayUserPrompt)
	}

	paused := postWorkflowRunAction(t, server, created.ID, "pause", http.StatusOK)
	if paused.Status != guidedworkflows.WorkflowRunStatusPaused {
		t.Fatalf("expected paused after pause, got %q", paused.Status)
	}

	resumed := postWorkflowRunAction(t, server, created.ID, "resume", http.StatusOK)
	if resumed.Status != guidedworkflows.WorkflowRunStatusRunning && resumed.Status != guidedworkflows.WorkflowRunStatusCompleted {
		t.Fatalf("expected running/completed after resume, got %q", resumed.Status)
	}

	fetched := getWorkflowRun(t, server, created.ID, http.StatusOK)
	if fetched.ID != created.ID {
		t.Fatalf("unexpected fetched run id: %q", fetched.ID)
	}
	if fetched.UserPrompt != created.UserPrompt {
		t.Fatalf("expected user prompt on fetched run, got %q", fetched.UserPrompt)
	}
	if fetched.DisplayUserPrompt != created.DisplayUserPrompt {
		t.Fatalf("expected display prompt on fetched run, got %q", fetched.DisplayUserPrompt)
	}

	timeline := getWorkflowRunTimeline(t, server, created.ID, http.StatusOK)
	if len(timeline) == 0 {
		t.Fatalf("expected non-empty timeline")
	}
	if timeline[0].Type != "run_created" {
		t.Fatalf("expected first timeline event run_created, got %q", timeline[0].Type)
	}

	runs := getWorkflowRuns(t, server, http.StatusOK)
	if len(runs) == 0 {
		t.Fatalf("expected workflow list to include created run")
	}
	if runs[0].ID != created.ID {
		t.Fatalf("expected most-recent run first in list, got %q", runs[0].ID)
	}
	if runs[0].UserPrompt != created.UserPrompt {
		t.Fatalf("expected user prompt on listed run, got %q", runs[0].UserPrompt)
	}
	if runs[0].DisplayUserPrompt != created.DisplayUserPrompt {
		t.Fatalf("expected display prompt on listed run, got %q", runs[0].DisplayUserPrompt)
	}

	templates := getWorkflowTemplates(t, server, http.StatusOK)
	if len(templates) == 0 {
		t.Fatalf("expected template list to be non-empty")
	}
}

func TestWorkflowRunEndpointsStop(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:  guidedworkflows.TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	_ = postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	stopped := postWorkflowRunAction(t, server, created.ID, "stop", http.StatusOK)
	if stopped.Status != guidedworkflows.WorkflowRunStatusStopped {
		t.Fatalf("expected stopped status, got %q", stopped.Status)
	}
	if stopped.CompletedAt == nil {
		t.Fatalf("expected completed timestamp when stopped")
	}
	if !strings.Contains(strings.ToLower(stopped.LastError), "stopped") {
		t.Fatalf("expected stop detail in last error, got %q", stopped.LastError)
	}

	timeline := getWorkflowRunTimeline(t, server, created.ID, http.StatusOK)
	foundStopped := false
	for _, event := range timeline {
		if strings.TrimSpace(event.Type) == "run_stopped" {
			foundStopped = true
			break
		}
	}
	if !foundStopped {
		t.Fatalf("expected run_stopped event in timeline, got %#v", timeline)
	}
}

func TestWorkflowRunEndpointsStopInterruptsSessionsBestEffort(t *testing.T) {
	var logOut bytes.Buffer
	interruptor := &recordWorkflowRunSessionInterruptService{
		err: errors.New("interrupt failed"),
	}
	api := &API{
		Version:                  "test",
		WorkflowRuns:             guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		WorkflowSessionInterrupt: interruptor,
		Logger:                   logging.New(&logOut, logging.Info),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:  guidedworkflows.TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	_ = postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	stopped := postWorkflowRunAction(t, server, created.ID, "stop", http.StatusOK)
	if stopped.Status != guidedworkflows.WorkflowRunStatusStopped {
		t.Fatalf("expected stopped status, got %q", stopped.Status)
	}
	if interruptor.calls != 1 {
		t.Fatalf("expected one interrupt invocation, got %d", interruptor.calls)
	}
	if len(interruptor.runIDs) != 1 || interruptor.runIDs[0] != created.ID {
		t.Fatalf("expected interrupt call for run %q, got %#v", created.ID, interruptor.runIDs)
	}
	if !strings.Contains(logOut.String(), "msg=guided_workflow_run_session_interrupt_error") {
		t.Fatalf("expected interrupt failure telemetry log, got %q", logOut.String())
	}
}

func TestWorkflowRunEndpointsResolveDisplayPromptFromSessionMeta(t *testing.T) {
	ctx := context.Background()
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		Stores: &Stores{
			SessionMeta: metaStore,
		},
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-legacy",
	})
	if created.DisplayUserPrompt != "" {
		t.Fatalf("expected empty display prompt before metadata is available, got %q", created.DisplayUserPrompt)
	}

	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:     "sess-legacy",
		WorkflowRunID: created.ID,
		InitialInput:  "recover parser intent from legacy session",
	}); err != nil {
		t.Fatalf("seed session meta: %v", err)
	}

	fetched := getWorkflowRun(t, server, created.ID, http.StatusOK)
	if fetched.UserPrompt != "" {
		t.Fatalf("expected empty stored user prompt, got %q", fetched.UserPrompt)
	}
	if fetched.DisplayUserPrompt != "recover parser intent from legacy session" {
		t.Fatalf("expected display prompt from session metadata, got %q", fetched.DisplayUserPrompt)
	}

	runs := getWorkflowRuns(t, server, http.StatusOK)
	if len(runs) != 1 {
		t.Fatalf("expected one workflow run, got %d", len(runs))
	}
	if runs[0].DisplayUserPrompt != "recover parser intent from legacy session" {
		t.Fatalf("expected list display prompt from session metadata, got %q", runs[0].DisplayUserPrompt)
	}
}

func TestWorkflowRunEndpointsDismissAndUndismiss(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	dismissed := postWorkflowRunAction(t, server, created.ID, "dismiss", http.StatusOK)
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected dismissed_at to be set")
	}

	runs := getWorkflowRuns(t, server, http.StatusOK)
	if len(runs) != 0 {
		t.Fatalf("expected default workflow list to exclude dismissed runs, got %#v", runs)
	}

	included := getWorkflowRunsWithPath(t, server, "/v1/workflow-runs?include_dismissed=1", http.StatusOK)
	if len(included) != 1 || included[0].ID != created.ID {
		t.Fatalf("expected dismissed run in include_dismissed list, got %#v", included)
	}

	undismissed := postWorkflowRunAction(t, server, created.ID, "undismiss", http.StatusOK)
	if undismissed.DismissedAt != nil {
		t.Fatalf("expected dismissed_at to clear")
	}
}

func TestWorkflowRunEndpointsRename(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	renamed := postWorkflowRunActionWithBody(t, server, created.ID, "rename", WorkflowRunRenameRequest{
		Name: "Renamed Workflow",
	}, http.StatusOK)
	if strings.TrimSpace(renamed.TemplateName) != "Renamed Workflow" {
		t.Fatalf("expected renamed workflow template name, got %q", renamed.TemplateName)
	}
}

func TestWorkflowRunEndpointsEmitListAndFetchTelemetry(t *testing.T) {
	var logOut bytes.Buffer
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		Logger:       logging.New(&logOut, logging.Info),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	getWorkflowRuns(t, server, http.StatusOK)
	getWorkflowRun(t, server, created.ID, http.StatusOK)

	logs := logOut.String()
	if !strings.Contains(logs, "msg=guided_workflow_runs_listed") {
		t.Fatalf("expected workflow list telemetry log, got %q", logs)
	}
	if !strings.Contains(logs, "msg=guided_workflow_run_fetched") {
		t.Fatalf("expected workflow fetch telemetry log, got %q", logs)
	}
	if !strings.Contains(logs, "run_id="+created.ID) {
		t.Fatalf("expected run id in telemetry logs, got %q", logs)
	}
}

func TestWorkflowRunEndpointsDismissAndUndismissCascadeWorkflowSessions(t *testing.T) {
	ctx := context.Background()
	sessionStore := storeSessionsIndex(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	now := time.Now().UTC()
	for _, sessionID := range []string{"sess-owned", "sess-linked"} {
		_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
			Session: &types.Session{
				ID:        sessionID,
				Provider:  "codex",
				Cmd:       "codex app-server",
				Status:    types.SessionStatusInactive,
				CreatedAt: now,
			},
			Source: sessionSourceInternal,
		})
		if err != nil {
			t.Fatalf("upsert session %s: %v", sessionID, err)
		}
	}

	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		Stores: &Stores{
			Sessions:    sessionStore,
			SessionMeta: metaStore,
		},
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-owned",
	})
	for _, sessionID := range []string{"sess-owned", "sess-linked"} {
		if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
			SessionID:     sessionID,
			WorkflowRunID: created.ID,
		}); err != nil {
			t.Fatalf("upsert session meta %s: %v", sessionID, err)
		}
	}

	sessionService := NewSessionService(nil, api.Stores, nil, nil)
	before, _, err := sessionService.ListWithMetaIncludingWorkflowOwned(ctx)
	if err != nil {
		t.Fatalf("list sessions before dismiss: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("expected one canonical workflow session before dismiss, got %#v", before)
	}

	dismissed := postWorkflowRunAction(t, server, created.ID, "dismiss", http.StatusOK)
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected dismissed_at to be set")
	}
	waitForCondition(t, 3*time.Second, func() bool {
		for _, sessionID := range []string{"sess-owned", "sess-linked"} {
			meta, ok, err := metaStore.Get(ctx, sessionID)
			if err != nil || !ok || meta == nil || meta.DismissedAt == nil {
				return false
			}
		}
		return true
	}, "sessions should be dismissed with workflow")
	hidden, _, err := sessionService.ListWithMetaIncludingWorkflowOwned(ctx)
	if err != nil {
		t.Fatalf("list hidden sessions: %v", err)
	}
	if len(hidden) != 0 {
		t.Fatalf("expected workflow sessions hidden after dismiss, got %#v", hidden)
	}
	included, _, err := sessionService.ListWithMetaIncludingDismissedAndWorkflowOwned(ctx)
	if err != nil {
		t.Fatalf("list include dismissed sessions: %v", err)
	}
	if len(included) != 1 {
		t.Fatalf("expected one canonical workflow session in include dismissed list, got %#v", included)
	}

	undismissed := postWorkflowRunAction(t, server, created.ID, "undismiss", http.StatusOK)
	if undismissed.DismissedAt != nil {
		t.Fatalf("expected dismissed_at to clear")
	}
	waitForCondition(t, 3*time.Second, func() bool {
		for _, sessionID := range []string{"sess-owned", "sess-linked"} {
			meta, ok, err := metaStore.Get(ctx, sessionID)
			if err != nil || !ok || meta == nil || meta.DismissedAt != nil {
				return false
			}
		}
		return true
	}, "sessions should be undismissed with workflow")
	restored, _, err := sessionService.ListWithMetaIncludingWorkflowOwned(ctx)
	if err != nil {
		t.Fatalf("list restored sessions: %v", err)
	}
	if len(restored) != 1 {
		t.Fatalf("expected one canonical workflow session after undismiss, got %#v", restored)
	}
}

func TestWorkflowRunEndpointsDismissAndUndismissEmitSessionVisibilityTelemetry(t *testing.T) {
	ctx := context.Background()
	sessionStore := storeSessionsIndex(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	now := time.Now().UTC()
	for _, sessionID := range []string{"sess-owned", "sess-linked"} {
		_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
			Session: &types.Session{
				ID:        sessionID,
				Provider:  "codex",
				Cmd:       "codex app-server",
				Status:    types.SessionStatusInactive,
				CreatedAt: now,
			},
			Source: sessionSourceInternal,
		})
		if err != nil {
			t.Fatalf("upsert session %s: %v", sessionID, err)
		}
	}
	var logOut bytes.Buffer
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		Logger:       logging.New(&logOut, logging.Info),
		Stores: &Stores{
			Sessions:    sessionStore,
			SessionMeta: metaStore,
		},
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-owned",
	})
	for _, sessionID := range []string{"sess-owned", "sess-linked"} {
		if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
			SessionID:     sessionID,
			WorkflowRunID: created.ID,
		}); err != nil {
			t.Fatalf("upsert session meta %s: %v", sessionID, err)
		}
	}

	_ = postWorkflowRunAction(t, server, created.ID, "dismiss", http.StatusOK)
	_ = postWorkflowRunAction(t, server, created.ID, "undismiss", http.StatusOK)

	waitForCondition(t, 3*time.Second, func() bool {
		logs := logOut.String()
		return strings.Contains(logs, "msg=guided_workflow_session_visibility_sync_started") &&
			strings.Contains(logs, "msg=guided_workflow_session_visibility_sync_applied") &&
			strings.Contains(logs, "msg=guided_workflow_session_visibility_sync_completed") &&
			strings.Contains(logs, "action=dismiss") &&
			strings.Contains(logs, "action=undismiss") &&
			strings.Contains(logs, "target_source=run_session+workflow_link") &&
			strings.Contains(logs, "target_source=workflow_link")
	}, "workflow session visibility telemetry should include both dismiss and undismiss events")
}

func TestWorkflowRunEndpointsDismissSkipsSessionVisibilityWhenSessionRelinked(t *testing.T) {
	ctx := context.Background()
	sessionStore := storeSessionsIndex(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	now := time.Now().UTC()
	if _, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-shared",
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	}); err != nil {
		t.Fatalf("upsert session: %v", err)
	}

	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		Stores: &Stores{
			Sessions:    sessionStore,
			SessionMeta: metaStore,
		},
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-shared",
	})
	if _, err := metaStore.Upsert(ctx, &types.SessionMeta{
		SessionID:     "sess-shared",
		WorkflowRunID: "gwf-new-owner",
		DismissedAt:   nil,
	}); err != nil {
		t.Fatalf("upsert relinked session meta: %v", err)
	}

	dismissed := postWorkflowRunAction(t, server, created.ID, "dismiss", http.StatusOK)
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected run to be dismissed")
	}
	waitForCondition(t, 2*time.Second, func() bool {
		meta, ok, err := metaStore.Get(ctx, "sess-shared")
		if err != nil || !ok || meta == nil {
			return false
		}
		return strings.TrimSpace(meta.WorkflowRunID) == "gwf-new-owner" && meta.DismissedAt == nil
	}, "session visibility should not be overwritten when ownership changes")
}

func TestWorkflowSessionVisibilitySyncSkipsStaleLinkedSessionTargets(t *testing.T) {
	ctx := context.Background()
	baseMeta := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	if _, err := baseMeta.Upsert(ctx, &types.SessionMeta{
		SessionID:     "sess-1",
		WorkflowRunID: "gwf-new-owner",
	}); err != nil {
		t.Fatalf("seed session meta: %v", err)
	}
	metaStore := &staleListSessionMetaStore{
		base: baseMeta,
		stale: []*types.SessionMeta{
			{
				SessionID:     "sess-1",
				WorkflowRunID: "gwf-old-owner",
			},
		},
	}
	api := &API{
		Version: "test",
		Stores: &Stores{
			SessionMeta: metaStore,
		},
	}

	err := api.syncWorkflowLinkedSessionDismissal(ctx, &guidedworkflows.WorkflowRun{
		ID: "gwf-old-owner",
	}, true)
	if err != nil {
		t.Fatalf("sync workflow linked session dismissal: %v", err)
	}
	meta, ok, err := baseMeta.Get(ctx, "sess-1")
	if err != nil {
		t.Fatalf("load session meta: %v", err)
	}
	if !ok || meta == nil {
		t.Fatalf("expected session meta to exist")
	}
	if strings.TrimSpace(meta.WorkflowRunID) != "gwf-new-owner" {
		t.Fatalf("expected workflow ownership to remain unchanged, got %q", meta.WorkflowRunID)
	}
	if meta.DismissedAt != nil {
		t.Fatalf("expected stale linked target to be skipped without dismissing session")
	}
	if metaStore.upsertCalls != 0 {
		t.Fatalf("expected no writes for stale linked target, got %d", metaStore.upsertCalls)
	}
}

func TestWorkflowRunEndpointsDismissMissingRunCreatesDismissedTombstone(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	dismissed := postWorkflowRunAction(t, server, "gwf-missing", "dismiss", http.StatusOK)
	if dismissed.ID != "gwf-missing" {
		t.Fatalf("expected missing run id to be preserved, got %q", dismissed.ID)
	}
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected dismissed_at to be set for missing run tombstone")
	}

	runs := getWorkflowRuns(t, server, http.StatusOK)
	if len(runs) != 0 {
		t.Fatalf("expected default workflow list to exclude dismissed missing run, got %#v", runs)
	}

	included := getWorkflowRunsWithPath(t, server, "/v1/workflow-runs?include_dismissed=1", http.StatusOK)
	if len(included) != 1 || included[0].ID != "gwf-missing" || included[0].DismissedAt == nil {
		t.Fatalf("expected include_dismissed to include missing dismissed run, got %#v", included)
	}
}

func TestWorkflowRunEndpointsInvalidTransition(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	postWorkflowRunActionRaw(t, server, created.ID, "resume", nil, http.StatusConflict)
}

func TestWorkflowRunEndpointsResumeFailedRouteRequiresFailedRun(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	_ = postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	_ = postWorkflowRunAction(t, server, created.ID, "pause", http.StatusOK)
	postWorkflowRunActionRaw(t, server, created.ID, "resume", WorkflowRunResumeRequest{
		ResumeFailed: true,
		Message:      "resume from outage",
	}, http.StatusConflict)
}

func TestWorkflowRunEndpointsResumeIgnoresMessageWhenResumeFailedDisabled(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	_ = postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	_ = postWorkflowRunAction(t, server, created.ID, "pause", http.StatusOK)
	resumed := postWorkflowRunActionWithBody(t, server, created.ID, "resume", WorkflowRunResumeRequest{
		Message: "this should be ignored for paused resume",
	}, http.StatusOK)
	if resumed.Status != guidedworkflows.WorkflowRunStatusRunning && resumed.Status != guidedworkflows.WorkflowRunStatusCompleted {
		t.Fatalf("expected running/completed after paused resume with message, got %q", resumed.Status)
	}
}

func TestWorkflowRunEndpointsCreateWithPolicyOverrides(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	confidenceThreshold := 0.88
	pauseThreshold := 0.52
	preCommitHardGate := true
	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		PolicyOverrides: &guidedworkflows.CheckpointPolicyOverride{
			ConfidenceThreshold: &confidenceThreshold,
			PauseThreshold:      &pauseThreshold,
			HardGates: &guidedworkflows.CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	if created.Policy.ConfidenceThreshold != 0.88 {
		t.Fatalf("unexpected confidence threshold override: %v", created.Policy.ConfidenceThreshold)
	}
	if created.Policy.PauseThreshold != 0.52 {
		t.Fatalf("unexpected pause threshold override: %v", created.Policy.PauseThreshold)
	}
	if !created.Policy.HardGates.PreCommitApproval {
		t.Fatalf("expected hard gate pre_commit_approval override")
	}
}

func TestWorkflowRunEndpointsCreateUsesBaselinePolicyWithoutResolver(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})

	expected := guidedworkflows.DefaultCheckpointPolicy(guidedworkflows.DefaultCheckpointStyle)
	if created.Policy.ConfidenceThreshold != expected.ConfidenceThreshold {
		t.Fatalf("expected baseline confidence threshold %v, got %v", expected.ConfidenceThreshold, created.Policy.ConfidenceThreshold)
	}
	if created.Policy.PauseThreshold != expected.PauseThreshold {
		t.Fatalf("expected baseline pause threshold %v, got %v", expected.PauseThreshold, created.Policy.PauseThreshold)
	}
	if created.PolicyOverrides != nil {
		t.Fatalf("expected no policy overrides when resolver and request overrides are absent, got %#v", created.PolicyOverrides)
	}
}

func TestWorkflowRunEndpointsCreateUsesConfiguredResolutionBoundaryDefaults(t *testing.T) {
	coreCfg := config.DefaultCoreConfig()
	coreCfg.GuidedWorkflows.Defaults.ResolutionBoundary = "high"
	api := &API{
		Version:        "test",
		WorkflowRuns:   guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		WorkflowPolicy: newGuidedWorkflowPolicyResolver(coreCfg),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if created.Policy.ConfidenceThreshold != guidedworkflows.PolicyPresetHighConfidenceThreshold {
		t.Fatalf("expected configured high confidence threshold %v, got %v", guidedworkflows.PolicyPresetHighConfidenceThreshold, created.Policy.ConfidenceThreshold)
	}
	if created.Policy.PauseThreshold != guidedworkflows.PolicyPresetHighPauseThreshold {
		t.Fatalf("expected configured high pause threshold %v, got %v", guidedworkflows.PolicyPresetHighPauseThreshold, created.Policy.PauseThreshold)
	}
}

func TestWorkflowRunEndpointsCreateUsesInjectedPolicyResolver(t *testing.T) {
	confidence := 0.51
	pause := 0.77
	resolver := &recordGuidedWorkflowPolicyResolver{
		resolved: &guidedworkflows.CheckpointPolicyOverride{
			ConfidenceThreshold: &confidence,
			PauseThreshold:      &pause,
		},
	}
	api := &API{
		Version:        "test",
		WorkflowRuns:   guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		WorkflowPolicy: resolver,
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if resolver.calls != 1 {
		t.Fatalf("expected workflow policy resolver to be called once, got %d", resolver.calls)
	}
	if created.Policy.ConfidenceThreshold != 0.51 {
		t.Fatalf("expected resolver confidence threshold override, got %v", created.Policy.ConfidenceThreshold)
	}
	if created.Policy.PauseThreshold != 0.77 {
		t.Fatalf("expected resolver pause threshold override, got %v", created.Policy.PauseThreshold)
	}
}

func TestWorkflowRunEndpointsCreateLogsEffectiveDispatchDefaults(t *testing.T) {
	var logOut bytes.Buffer
	coreCfg := config.DefaultCoreConfig()
	coreCfg.GuidedWorkflows.Defaults.Provider = "opencode"
	coreCfg.GuidedWorkflows.Defaults.Model = "gpt-5.3-codex"
	coreCfg.GuidedWorkflows.Defaults.Access = "full_access"
	coreCfg.GuidedWorkflows.Defaults.Reasoning = "high"

	api := &API{
		Version:                  "test",
		WorkflowRuns:             guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		WorkflowDispatchDefaults: guidedWorkflowDispatchDefaultsFromCoreConfig(coreCfg),
		Logger:                   logging.New(&logOut, logging.Info),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	_ = createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})

	logs := logOut.String()
	if !strings.Contains(logs, "msg=guided_workflow_run_created") {
		t.Fatalf("expected guided_workflow_run_created log, got %q", logs)
	}
	if !strings.Contains(logs, "effective_provider=opencode") {
		t.Fatalf("expected effective provider in run creation log, got %q", logs)
	}
	if !strings.Contains(logs, "effective_model=gpt-5.3-codex") {
		t.Fatalf("expected effective model in run creation log, got %q", logs)
	}
	if !strings.Contains(logs, "effective_reasoning=high") {
		t.Fatalf("expected effective reasoning in run creation log, got %q", logs)
	}
	if !strings.Contains(logs, "effective_access=on_request") {
		t.Fatalf("expected template access precedence in run creation log, got %q", logs)
	}
}

func TestWorkflowRunEndpointsDecisionActions(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	started := postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	if started.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", started.Status)
	}

	paused := postWorkflowRunDecision(t, server, created.ID, WorkflowRunDecisionRequest{
		Action: guidedworkflows.DecisionActionPauseRun,
		Note:   "pause for review",
	}, http.StatusOK)
	if paused.Status != guidedworkflows.WorkflowRunStatusPaused {
		t.Fatalf("expected paused after pause_run decision, got %q", paused.Status)
	}

	revised := postWorkflowRunDecision(t, server, created.ID, WorkflowRunDecisionRequest{
		Action: guidedworkflows.DecisionActionRequestRevision,
		Note:   "needs revision",
	}, http.StatusOK)
	if revised.Status != guidedworkflows.WorkflowRunStatusPaused {
		t.Fatalf("expected paused after request_revision decision, got %q", revised.Status)
	}

	continued := postWorkflowRunDecision(t, server, created.ID, WorkflowRunDecisionRequest{
		Action: guidedworkflows.DecisionActionApproveContinue,
		Note:   "continue",
	}, http.StatusOK)
	if continued.Status != guidedworkflows.WorkflowRunStatusRunning && continued.Status != guidedworkflows.WorkflowRunStatusCompleted {
		t.Fatalf("expected running/completed after approve_continue decision, got %q", continued.Status)
	}
}

func TestWorkflowRunEndpointsStartPublishesDecisionNeededNotificationWhenPolicyPauses(t *testing.T) {
	notifier := &recordNotificationPublisher{}
	api := &API{
		Version: "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}, guidedworkflows.WithTemplate(guidedworkflows.WorkflowTemplate{
			ID:   "single_commit",
			Name: "Single Commit",
			Phases: []guidedworkflows.WorkflowTemplatePhase{
				{
					ID:   "phase",
					Name: "phase",
					Steps: []guidedworkflows.WorkflowTemplateStep{
						{ID: "commit", Name: "commit"},
					},
				},
			},
		})),
		Notifier: notifier,
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	preCommitHardGate := true
	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:  "single_commit",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		PolicyOverrides: &guidedworkflows.CheckpointPolicyOverride{
			HardGates: &guidedworkflows.CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	started := postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	if started.Status != guidedworkflows.WorkflowRunStatusPaused {
		t.Fatalf("expected paused on start, got %q", started.Status)
	}
	if countDecisionNeededEvents(notifier.events) != 1 {
		t.Fatalf("expected one decision-needed notification, got %d", countDecisionNeededEvents(notifier.events))
	}
	event := lastDecisionNeededEvent(notifier.events)
	if event == nil {
		t.Fatalf("expected decision-needed event payload")
	}
	if asString(event.Payload["recommended_action"]) == "" {
		t.Fatalf("expected recommended_action in notification payload")
	}
}

func TestWorkflowRunEndpointsDisabled(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	body, _ := json.Marshal(CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 500, got %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

func TestWorkflowRunCreateRejectsUnsupportedProvider(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	body, _ := json.Marshal(CreateWorkflowRunRequest{
		WorkspaceID:      "ws-1",
		WorktreeID:       "wt-1",
		SelectedProvider: "gemini",
	})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	payload, _ := io.ReadAll(resp.Body)
	if !strings.Contains(strings.ToLower(string(payload)), "not dispatchable") {
		t.Fatalf("expected unsupported provider error payload, got %s", strings.TrimSpace(string(payload)))
	}
}

func TestWorkflowTemplateEndpointMethodNotAllowed(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-templates", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("workflow template request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 405, got %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

func TestWorkflowTemplateEndpointServiceUnavailable(t *testing.T) {
	api := &API{Version: "test"}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workflow-templates", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("workflow template request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 500, got %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

func TestWorkflowTemplateEndpointReturnsMappedServiceError(t *testing.T) {
	api := &API{
		Version:           "test",
		WorkflowTemplates: stubWorkflowTemplateService{err: errors.New("boom")},
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workflow-templates", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("workflow template request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 500, got %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

func TestWorkflowTemplateEndpointPrefersExplicitTemplateService(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
		WorkflowTemplates: stubWorkflowTemplateService{templates: []guidedworkflows.WorkflowTemplate{
			{
				ID:   "explicit_template",
				Name: "Explicit Template",
				Phases: []guidedworkflows.WorkflowTemplatePhase{
					{
						ID:   "phase",
						Name: "Phase",
						Steps: []guidedworkflows.WorkflowTemplateStep{
							{ID: "step", Name: "Step", Prompt: "Prompt"},
						},
					},
				},
			},
		}},
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	templates := getWorkflowTemplates(t, server, http.StatusOK)
	if len(templates) != 1 {
		t.Fatalf("expected explicit template service payload, got %#v", templates)
	}
	if templates[0].ID != "explicit_template" {
		t.Fatalf("expected explicit template id, got %#v", templates)
	}
}

func TestWorkflowRunEndpointsMaxActiveRunsGuardrail(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}, guidedworkflows.WithMaxActiveRuns(1)),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	body, _ := json.Marshal(CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-2",
	})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409 conflict when max active runs exceeded, got %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
}

func TestWorkflowRunEndpointsTwoStepWorkflowDispatchIntegration(t *testing.T) {
	template := guidedworkflows.WorkflowTemplate{
		ID:   "gwf_two_step_integration",
		Name: "Two Step Integration",
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "overall plan prompt"},
					{ID: "step_2", Name: "Step 2", Prompt: "phase plan prompt"},
				},
			},
		},
	}
	gateway := &stubGuidedWorkflowSessionGateway{
		turnID: "turn-1",
		started: []*types.Session{
			{
				ID:        "sess-gwf",
				Provider:  "codex",
				Status:    types.SessionStatusRunning,
				CreatedAt: time.Now().UTC(),
			},
		},
	}
	metaStore := &stubGuidedWorkflowSessionMetaStore{}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway, sessionMeta: metaStore}
	runService := guidedworkflows.NewRunService(
		guidedworkflows.Config{Enabled: true},
		guidedworkflows.WithTemplate(template),
		guidedworkflows.WithStepPromptDispatcher(dispatcher),
	)

	api := &API{Version: "test", WorkflowRuns: runService}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:  "gwf_two_step_integration",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		UserPrompt:  "Fix parser bug",
	})
	started := postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	if started.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected running start status, got %q", started.Status)
	}
	if started.SessionID != "sess-gwf" {
		t.Fatalf("expected workflow run to bind created session, got %q", started.SessionID)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one workflow session start request, got %d", len(gateway.startReqs))
	}
	if len(gateway.sendCalls) != 1 {
		t.Fatalf("expected first step prompt dispatch, got %d calls", len(gateway.sendCalls))
	}
	firstText, _ := gateway.sendCalls[0].input[0]["text"].(string)
	if firstText != "Fix parser bug\n\noverall plan prompt" {
		t.Fatalf("unexpected first step prompt payload: %q", firstText)
	}

	updated, err := runService.OnTurnCompleted(context.Background(), guidedworkflows.TurnSignal{
		SessionID: "sess-gwf",
		TurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one run update after turn completion, got %d", len(updated))
	}
	if len(gateway.sendCalls) != 2 {
		t.Fatalf("expected second step prompt dispatch, got %d calls", len(gateway.sendCalls))
	}
	if gateway.sendCalls[1].sessionID != "sess-gwf" {
		t.Fatalf("expected second step on same session, got %q", gateway.sendCalls[1].sessionID)
	}
	secondText, _ := gateway.sendCalls[1].input[0]["text"].(string)
	if secondText != "phase plan prompt" {
		t.Fatalf("unexpected second step prompt payload: %q", secondText)
	}
}

func TestWorkflowRunEndpointsIntegrationUsesConfiguredDefaultsEndToEnd(t *testing.T) {
	coreCfg := config.DefaultCoreConfig()
	coreCfg.GuidedWorkflows.Defaults.Provider = "opencode"
	coreCfg.GuidedWorkflows.Defaults.Model = "gpt-5.3-codex"
	coreCfg.GuidedWorkflows.Defaults.Access = "full_access"
	coreCfg.GuidedWorkflows.Defaults.Reasoning = "extra_high"
	coreCfg.GuidedWorkflows.Defaults.ResolutionBoundary = "high"

	template := guidedworkflows.WorkflowTemplate{
		ID:   "gwf_defaults_e2e",
		Name: "Defaults E2E",
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "implement fixes"},
				},
			},
		},
	}
	gateway := &stubGuidedWorkflowSessionGateway{
		turnID: "turn-defaults-e2e",
		started: []*types.Session{
			{
				ID:        "sess-defaults-e2e",
				Provider:  "opencode",
				Status:    types.SessionStatusRunning,
				CreatedAt: time.Now().UTC(),
			},
		},
	}
	metaStore := &stubGuidedWorkflowSessionMetaStore{}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions:    gateway,
		sessionMeta: metaStore,
		defaults:    guidedWorkflowDispatchDefaultsFromCoreConfig(coreCfg),
	}
	runService := guidedworkflows.NewRunService(
		guidedworkflows.Config{Enabled: true},
		guidedworkflows.WithTemplate(template),
		guidedworkflows.WithStepPromptDispatcher(dispatcher),
	)
	api := &API{
		Version:                  "test",
		WorkflowRuns:             runService,
		WorkflowPolicy:           newGuidedWorkflowPolicyResolver(coreCfg),
		WorkflowDispatchDefaults: guidedWorkflowDispatchDefaultsFromCoreConfig(coreCfg),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:  "gwf_defaults_e2e",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		UserPrompt:  "Fix parser bug",
	})
	if created.Policy.ConfidenceThreshold != guidedworkflows.PolicyPresetHighConfidenceThreshold {
		t.Fatalf("expected resolution_boundary=high confidence threshold %v, got %v", guidedworkflows.PolicyPresetHighConfidenceThreshold, created.Policy.ConfidenceThreshold)
	}
	if created.Policy.PauseThreshold != guidedworkflows.PolicyPresetHighPauseThreshold {
		t.Fatalf("expected resolution_boundary=high pause threshold %v, got %v", guidedworkflows.PolicyPresetHighPauseThreshold, created.Policy.PauseThreshold)
	}

	started := postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	if started.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected running start status, got %q", started.Status)
	}
	if started.SessionID != "sess-defaults-e2e" {
		t.Fatalf("expected workflow run to bind created session, got %q", started.SessionID)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one workflow session start request, got %d", len(gateway.startReqs))
	}
	startReq := gateway.startReqs[0]
	if startReq.Provider != "opencode" {
		t.Fatalf("expected configured provider opencode, got %q", startReq.Provider)
	}
	if startReq.RuntimeOptions == nil {
		t.Fatalf("expected runtime options from configured defaults")
	}
	if startReq.RuntimeOptions.Model != "gpt-5.3-codex" {
		t.Fatalf("expected configured model in runtime options, got %q", startReq.RuntimeOptions.Model)
	}
	if startReq.RuntimeOptions.Access != types.AccessFull {
		t.Fatalf("expected configured access in runtime options, got %q", startReq.RuntimeOptions.Access)
	}
	if startReq.RuntimeOptions.Reasoning != types.ReasoningExtraHigh {
		t.Fatalf("expected configured reasoning in runtime options, got %q", startReq.RuntimeOptions.Reasoning)
	}
	if len(gateway.sendCalls) != 1 {
		t.Fatalf("expected one step prompt dispatch, got %d", len(gateway.sendCalls))
	}
	firstText, _ := gateway.sendCalls[0].input[0]["text"].(string)
	if firstText != "Fix parser bug\n\nimplement fixes" {
		t.Fatalf("unexpected step prompt payload: %q", firstText)
	}
}

func TestWorkflowRunEndpointsStartWrapsSessionResolutionErrors(t *testing.T) {
	template := guidedworkflows.WorkflowTemplate{
		ID:   "gwf_step_dispatch_error",
		Name: "Dispatch Error",
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase",
				Name: "phase",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "prompt 1"},
				},
			},
		},
	}
	gateway := &stubGuidedWorkflowSessionGateway{
		startErr: errors.New("codex input is required"),
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	runService := guidedworkflows.NewRunService(
		guidedworkflows.Config{Enabled: true},
		guidedworkflows.WithTemplate(template),
		guidedworkflows.WithStepPromptDispatcher(dispatcher),
	)
	api := &API{Version: "test", WorkflowRuns: runService}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		TemplateID:  "gwf_step_dispatch_error",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	resp := postWorkflowRunActionRaw(t, server, created.ID, "start", nil, http.StatusInternalServerError)
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	body := strings.ToLower(strings.TrimSpace(string(payload)))
	if !strings.Contains(body, "workflow step prompt dispatch unavailable") {
		t.Fatalf("expected wrapped step dispatch error, got %q", body)
	}
	if strings.Contains(body, "guided workflow request failed") {
		t.Fatalf("expected specific error instead of generic request failure, got %q", body)
	}
}

func TestWorkflowRunMetricsEndpoint(t *testing.T) {
	runService := guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	api := &API{
		Version:            "test",
		WorkflowRuns:       runService,
		WorkflowRunMetrics: runService,
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	started := postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	if started.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected running start status, got %q", started.Status)
	}

	metrics := getWorkflowRunMetrics(t, server, http.StatusOK)
	if !metrics.Enabled {
		t.Fatalf("expected telemetry enabled")
	}
	if metrics.RunsStarted < 1 {
		t.Fatalf("expected runs_started >= 1, got %d", metrics.RunsStarted)
	}
	if metrics.PauseRate < 0 {
		t.Fatalf("expected non-negative pause rate, got %f", metrics.PauseRate)
	}
}

func TestWorkflowRunMetricsEndpointRequiresExplicitMetricsService(t *testing.T) {
	api := &API{
		Version:      "test",
		WorkflowRuns: guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true}),
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	_ = getWorkflowRunMetrics(t, server, http.StatusInternalServerError)
}

func TestWorkflowRunMetricsResetEndpoint(t *testing.T) {
	runService := guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	api := &API{
		Version:                 "test",
		WorkflowRuns:            runService,
		WorkflowRunMetrics:      runService,
		WorkflowRunMetricsReset: runService,
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	created := createWorkflowRunViaAPI(t, server, CreateWorkflowRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	postWorkflowRunAction(t, server, created.ID, "start", http.StatusOK)
	before := getWorkflowRunMetrics(t, server, http.StatusOK)
	if before.RunsStarted < 1 {
		t.Fatalf("expected pre-reset runs_started >= 1, got %d", before.RunsStarted)
	}
	reset := postWorkflowRunMetricsReset(t, server, http.StatusOK)
	if reset.RunsStarted != 0 || reset.RunsCompleted != 0 || reset.RunsFailed != 0 || reset.PauseCount != 0 || reset.ApprovalCount != 0 {
		t.Fatalf("expected reset metrics to be zeroed, got %#v", reset)
	}
	after := getWorkflowRunMetrics(t, server, http.StatusOK)
	if after.RunsStarted != 0 || after.PauseCount != 0 || after.ApprovalCount != 0 {
		t.Fatalf("expected metrics endpoint to return reset values, got %#v", after)
	}
}

func TestWorkflowRunMetricsResetEndpointRequiresExplicitResetService(t *testing.T) {
	runService := guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	api := &API{
		Version:            "test",
		WorkflowRuns:       runService,
		WorkflowRunMetrics: runService,
	}
	server := newWorkflowRunTestServer(t, api)
	defer server.Close()

	_ = postWorkflowRunMetricsReset(t, server, http.StatusInternalServerError)
}

func TestToGuidedWorkflowServiceErrorMappings(t *testing.T) {
	if err := toGuidedWorkflowServiceError(nil); err != nil {
		t.Fatalf("expected nil")
	}
	check := func(err error, want ServiceErrorKind) {
		t.Helper()
		mapped := toGuidedWorkflowServiceError(err)
		serviceErr, ok := mapped.(*ServiceError)
		if !ok {
			t.Fatalf("expected *ServiceError, got %T", mapped)
		}
		if serviceErr.Kind != want {
			t.Fatalf("unexpected error kind: got=%s want=%s", serviceErr.Kind, want)
		}
	}
	check(guidedworkflows.ErrRunNotFound, ServiceErrorNotFound)
	check(guidedworkflows.ErrTemplateNotFound, ServiceErrorInvalid)
	check(guidedworkflows.ErrMissingContext, ServiceErrorInvalid)
	check(guidedworkflows.ErrUnsupportedProvider, ServiceErrorInvalid)
	check(guidedworkflows.ErrInvalidTransition, ServiceErrorConflict)
	check(guidedworkflows.ErrRunLimitExceeded, ServiceErrorConflict)
	check(guidedworkflows.ErrDisabled, ServiceErrorUnavailable)
	check(guidedworkflows.ErrStepDispatchDeferred, ServiceErrorConflict)
	check(guidedworkflows.ErrStepDispatch, ServiceErrorUnavailable)

	mapped := toGuidedWorkflowServiceError(fmt.Errorf("%w: turn already in progress", guidedworkflows.ErrStepDispatch))
	serviceErr, ok := mapped.(*ServiceError)
	if !ok {
		t.Fatalf("expected *ServiceError for turn conflict, got %T", mapped)
	}
	if serviceErr.Kind != ServiceErrorConflict {
		t.Fatalf("expected conflict kind for turn-in-progress dispatch error, got %s", serviceErr.Kind)
	}
}

func newWorkflowRunTestServer(t *testing.T, api *API) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workflow-runs", api.WorkflowRunsEndpoint)
	mux.HandleFunc("/v1/workflow-templates", api.WorkflowTemplatesEndpoint)
	mux.HandleFunc("/v1/workflow-runs/metrics", api.WorkflowRunMetricsEndpoint)
	mux.HandleFunc("/v1/workflow-runs/metrics/reset", api.WorkflowRunMetricsResetEndpoint)
	mux.HandleFunc("/v1/workflow-runs/", api.WorkflowRunByID)
	mux.HandleFunc("/health", api.Health)
	return httptest.NewServer(TokenAuthMiddleware("token", mux))
}

func createWorkflowRunViaAPI(t *testing.T, server *httptest.Server, reqBody CreateWorkflowRunRequest) *guidedworkflows.WorkflowRun {
	t.Helper()
	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var run guidedworkflows.WorkflowRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode create run: %v", err)
	}
	return &run
}

func postWorkflowRunAction(t *testing.T, server *httptest.Server, runID, action string, wantStatus int) *guidedworkflows.WorkflowRun {
	t.Helper()
	resp := postWorkflowRunActionRaw(t, server, runID, action, nil, wantStatus)
	defer resp.Body.Close()
	var run guidedworkflows.WorkflowRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode action run response: %v", err)
	}
	return &run
}

func postWorkflowRunActionWithBody(t *testing.T, server *httptest.Server, runID, action string, body any, wantStatus int) *guidedworkflows.WorkflowRun {
	t.Helper()
	resp := postWorkflowRunActionRaw(t, server, runID, action, body, wantStatus)
	defer resp.Body.Close()
	var run guidedworkflows.WorkflowRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode action run response: %v", err)
	}
	return &run
}

func postWorkflowRunActionRaw(t *testing.T, server *httptest.Server, runID, action string, body any, wantStatus int) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal workflow action body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs/"+runID+"/"+action, reader)
	req.Header.Set("Authorization", "Bearer token")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("workflow action request: %v", err)
	}
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("unexpected status for %s: got=%d want=%d payload=%s", action, resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	return resp
}

func postWorkflowRunDecision(t *testing.T, server *httptest.Server, runID string, decision WorkflowRunDecisionRequest, wantStatus int) *guidedworkflows.WorkflowRun {
	t.Helper()
	body, _ := json.Marshal(decision)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs/"+runID+"/decision", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("workflow decision request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status for decision: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var run guidedworkflows.WorkflowRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode decision response: %v", err)
	}
	return &run
}

func getWorkflowRun(t *testing.T, server *httptest.Server, runID string, wantStatus int) *guidedworkflows.WorkflowRun {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workflow-runs/"+runID, nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get workflow run request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected get status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var run guidedworkflows.WorkflowRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		t.Fatalf("decode workflow run: %v", err)
	}
	return &run
}

func getWorkflowRunTimeline(t *testing.T, server *httptest.Server, runID string, wantStatus int) []guidedworkflows.RunTimelineEvent {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workflow-runs/"+runID+"/timeline", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get timeline request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected timeline status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var payload struct {
		Timeline []guidedworkflows.RunTimelineEvent `json:"timeline"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode timeline payload: %v", err)
	}
	return payload.Timeline
}

func getWorkflowRuns(t *testing.T, server *httptest.Server, wantStatus int) []*guidedworkflows.WorkflowRun {
	return getWorkflowRunsWithPath(t, server, "/v1/workflow-runs", wantStatus)
}

func getWorkflowRunsWithPath(t *testing.T, server *httptest.Server, path string, wantStatus int) []*guidedworkflows.WorkflowRun {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get workflow runs request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected list status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var payload struct {
		Runs []*guidedworkflows.WorkflowRun `json:"runs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode workflow runs: %v", err)
	}
	return payload.Runs
}

func getWorkflowTemplates(t *testing.T, server *httptest.Server, wantStatus int) []guidedworkflows.WorkflowTemplate {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workflow-templates", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get workflow templates request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected template list status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var payload struct {
		Templates []guidedworkflows.WorkflowTemplate `json:"templates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode workflow templates: %v", err)
	}
	return payload.Templates
}

func getWorkflowRunMetrics(t *testing.T, server *httptest.Server, wantStatus int) guidedworkflows.RunMetricsSnapshot {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workflow-runs/metrics", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get metrics request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected metrics status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var metrics guidedworkflows.RunMetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		t.Fatalf("decode metrics payload: %v", err)
	}
	return metrics
}

func postWorkflowRunMetricsReset(t *testing.T, server *httptest.Server, wantStatus int) guidedworkflows.RunMetricsSnapshot {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workflow-runs/metrics/reset", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post metrics reset request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected metrics reset status: got=%d want=%d payload=%s", resp.StatusCode, wantStatus, strings.TrimSpace(string(payload)))
	}
	var metrics guidedworkflows.RunMetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		t.Fatalf("decode metrics reset payload: %v", err)
	}
	return metrics
}

func TestWorkflowRunServiceInterfaceCompatibility(t *testing.T) {
	var _ GuidedWorkflowRunService = guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	var _ GuidedWorkflowTemplateService = guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
}

func waitForCondition(t *testing.T, timeout time.Duration, check func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out after %s: %s", timeout, message)
}

type stubWorkflowTemplateService struct {
	templates []guidedworkflows.WorkflowTemplate
	err       error
}

type recordWorkflowRunSessionInterruptService struct {
	calls  int
	runIDs []string
	err    error
}

func (r *recordWorkflowRunSessionInterruptService) InterruptWorkflowRunSessions(_ context.Context, run *guidedworkflows.WorkflowRun) error {
	if r == nil {
		return nil
	}
	r.calls++
	if run != nil {
		r.runIDs = append(r.runIDs, strings.TrimSpace(run.ID))
	}
	return r.err
}

func (s stubWorkflowTemplateService) ListTemplates(context.Context) ([]guidedworkflows.WorkflowTemplate, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]guidedworkflows.WorkflowTemplate, len(s.templates))
	copy(out, s.templates)
	return out, nil
}

type staleListSessionMetaStore struct {
	base        SessionMetaStore
	stale       []*types.SessionMeta
	listCalls   int
	upsertCalls int
}

func (s *staleListSessionMetaStore) List(ctx context.Context) ([]*types.SessionMeta, error) {
	if s != nil && s.listCalls == 0 && len(s.stale) > 0 {
		s.listCalls++
		out := make([]*types.SessionMeta, 0, len(s.stale))
		for _, entry := range s.stale {
			if entry == nil {
				continue
			}
			copy := *entry
			out = append(out, &copy)
		}
		return out, nil
	}
	if s != nil {
		s.listCalls++
	}
	if s == nil || s.base == nil {
		return []*types.SessionMeta{}, nil
	}
	return s.base.List(ctx)
}

func (s *staleListSessionMetaStore) Get(ctx context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	if s == nil || s.base == nil {
		return nil, false, nil
	}
	return s.base.Get(ctx, sessionID)
}

func (s *staleListSessionMetaStore) Upsert(ctx context.Context, meta *types.SessionMeta) (*types.SessionMeta, error) {
	if s != nil {
		s.upsertCalls++
	}
	if s == nil || s.base == nil {
		return meta, nil
	}
	return s.base.Upsert(ctx, meta)
}

func (s *staleListSessionMetaStore) Delete(ctx context.Context, sessionID string) error {
	if s == nil || s.base == nil {
		return nil
	}
	return s.base.Delete(ctx, sessionID)
}

func TestWorkflowRunPresentationHelpersBranches(t *testing.T) {
	ctx := context.Background()
	run := &guidedworkflows.WorkflowRun{ID: "gwf-1", UserPrompt: "prompt from run"}

	api := &API{}
	presented := api.presentWorkflowRunPayload(ctx, run)
	presentedRun, ok := presented.(*guidedworkflows.WorkflowRun)
	if !ok {
		t.Fatalf("expected workflow run payload, got %T", presented)
	}
	if presentedRun.DisplayUserPrompt != "prompt from run" {
		t.Fatalf("expected display prompt from user prompt, got %q", presentedRun.DisplayUserPrompt)
	}

	presentedList := api.presentWorkflowRunPayload(ctx, []*guidedworkflows.WorkflowRun{run})
	listRuns, ok := presentedList.([]*guidedworkflows.WorkflowRun)
	if !ok || len(listRuns) != 1 {
		t.Fatalf("expected one run in presented list, got %#v", presentedList)
	}
	if listRuns[0].DisplayUserPrompt != "prompt from run" {
		t.Fatalf("expected list display prompt from user prompt, got %q", listRuns[0].DisplayUserPrompt)
	}

	raw := map[string]any{"unchanged": true}
	if got := api.presentWorkflowRunPayload(ctx, raw); !reflect.DeepEqual(got, raw) {
		t.Fatalf("expected unknown payload passthrough, got %#v", got)
	}

	var nilAPI *API
	resolver := nilAPI.workflowRunPromptResolver()
	if resolver == nil {
		t.Fatalf("expected nil API to provide default resolver")
	}
	if got := resolver.ResolveDisplayPrompt(ctx, run); got != "prompt from run" {
		t.Fatalf("expected nil API resolver to resolve user prompt, got %q", got)
	}

	if got := presentWorkflowRunWithResolver(ctx, nil, resolver); got != nil {
		t.Fatalf("expected nil run to remain nil, got %#v", got)
	}

	cloned := presentWorkflowRunWithResolver(ctx, run, nil)
	if cloned == nil || cloned == run {
		t.Fatalf("expected cloned run even without resolver, got %#v", cloned)
	}
	if cloned.DisplayUserPrompt != "" {
		t.Fatalf("expected no display prompt assignment when resolver is nil, got %q", cloned.DisplayUserPrompt)
	}
}
