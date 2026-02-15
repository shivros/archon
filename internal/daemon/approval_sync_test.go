package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"control/internal/store"
	"control/internal/types"
)

func TestExtractPendingApprovalsFromCodexItemsTracksResolvedRequests(t *testing.T) {
	items := []map[string]any{
		{
			"id":     1,
			"method": "item/commandExecution/requestApproval",
			"params": map[string]any{"parsedCmd": "echo one"},
			"ts":     "2026-02-10T01:00:00Z",
		},
		{
			"id":     2,
			"method": "item/fileChange/requestApproval",
			"params": map[string]any{"reason": "write file"},
			"ts":     "2026-02-10T01:01:00Z",
		},
		{
			"method": "turn/respondToRequest",
			"params": map[string]any{
				"requestId": 1,
				"decision":  "accept",
			},
		},
	}

	approvals, authoritative := extractPendingApprovalsFromCodexItems(items, "s1")
	if !authoritative {
		t.Fatalf("expected extractor to report authoritative signal")
	}
	if len(approvals) != 1 {
		t.Fatalf("expected one pending approval, got %#v", approvals)
	}
	if approvals[0].RequestID != 2 {
		t.Fatalf("expected pending request id 2, got %#v", approvals[0])
	}
	if approvals[0].Method != "item/fileChange/requestApproval" {
		t.Fatalf("unexpected method %#v", approvals[0])
	}
	params := map[string]any{}
	if err := json.Unmarshal(approvals[0].Params, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if params["reason"] != "write file" {
		t.Fatalf("unexpected params: %#v", params)
	}
}

func TestApprovalResyncServiceSyncSessionReconcilesStore(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()

	approvalStore := store.NewFileApprovalStore(filepath.Join(base, "approvals.json"))
	sessionID := "s1"
	_, err := approvalStore.Upsert(ctx, &types.Approval{
		SessionID: sessionID,
		RequestID: 1,
		Method:    "item/commandExecution/requestApproval",
		CreatedAt: time.Date(2026, 2, 10, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("seed stale approval: %v", err)
	}

	stores := &Stores{Approvals: approvalStore}
	resync := NewApprovalResyncService(stores, nil, &fakeApprovalSyncProvider{
		provider: "fake",
		result: &ApprovalSyncResult{
			Approvals: []*types.Approval{
				{
					SessionID: sessionID,
					RequestID: 2,
					Method:    "item/fileChange/requestApproval",
					CreatedAt: time.Date(2026, 2, 10, 1, 1, 0, 0, time.UTC),
				},
			},
			Authoritative: true,
		},
	})
	session := &types.Session{ID: sessionID, Provider: "fake"}
	if err := resync.SyncSession(ctx, session, nil); err != nil {
		t.Fatalf("sync session: %v", err)
	}

	got, err := approvalStore.ListBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(got) != 1 || got[0].RequestID != 2 {
		t.Fatalf("expected only request 2 after reconcile, got %#v", got)
	}
}

func TestApprovalResyncServiceSyncSessionNonAuthoritativeDoesNotDeleteExisting(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()

	approvalStore := store.NewFileApprovalStore(filepath.Join(base, "approvals.json"))
	sessionID := "s1"
	_, err := approvalStore.Upsert(ctx, &types.Approval{
		SessionID: sessionID,
		RequestID: 1,
		Method:    "item/commandExecution/requestApproval",
		CreatedAt: time.Date(2026, 2, 10, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("seed stale approval: %v", err)
	}

	stores := &Stores{Approvals: approvalStore}
	resync := NewApprovalResyncService(stores, nil, &fakeApprovalSyncProvider{
		provider: "fake",
		result: &ApprovalSyncResult{
			Approvals: []*types.Approval{
				{
					SessionID: sessionID,
					RequestID: 2,
					Method:    "item/fileChange/requestApproval",
					CreatedAt: time.Date(2026, 2, 10, 1, 1, 0, 0, time.UTC),
				},
			},
		},
	})
	session := &types.Session{ID: sessionID, Provider: "fake"}
	if err := resync.SyncSession(ctx, session, nil); err != nil {
		t.Fatalf("sync session: %v", err)
	}

	got, err := approvalStore.ListBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected additive merge for non-authoritative sync, got %#v", got)
	}
}

func TestSessionServiceListApprovalsRunsResync(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()

	sessionStore := store.NewFileSessionIndexStore(filepath.Join(base, "sessions.json"))
	approvalStore := store.NewFileApprovalStore(filepath.Join(base, "approvals.json"))

	sessionID := "s1"
	_, err := sessionStore.UpsertRecord(ctx, &types.SessionRecord{
		Session: &types.Session{
			ID:        sessionID,
			Provider:  "fake",
			CreatedAt: time.Now().UTC(),
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	_, err = approvalStore.Upsert(ctx, &types.Approval{
		SessionID: sessionID,
		RequestID: 1,
		Method:    "item/commandExecution/requestApproval",
		CreatedAt: time.Date(2026, 2, 10, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("seed stale approval: %v", err)
	}

	stores := &Stores{
		Sessions:  sessionStore,
		Approvals: approvalStore,
	}
	service := NewSessionService(nil, stores, nil, nil)
	service.approvalSync = NewApprovalResyncService(stores, nil, &fakeApprovalSyncProvider{
		provider: "fake",
		result: &ApprovalSyncResult{
			Approvals: []*types.Approval{
				{
					SessionID: sessionID,
					RequestID: 7,
					Method:    "item/fileChange/requestApproval",
					CreatedAt: time.Date(2026, 2, 10, 1, 1, 0, 0, time.UTC),
				},
			},
			Authoritative: true,
		},
	})

	got, err := service.ListApprovals(ctx, sessionID)
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(got) != 1 || got[0].RequestID != 7 {
		t.Fatalf("expected synced request 7, got %#v", got)
	}
}

func TestOpenCodeApprovalSyncProviderSyncSessionApprovals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/permission" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"permissions": []map[string]any{
				{
					"id":        "perm-pending",
					"sessionID": "remote-session",
					"status":    "pending",
					"type":      "command",
					"title":     "Run verification suite",
					"metadata": map[string]any{
						"command": "go test ./...",
						"reason":  "Validate before merge",
						"cwd":     "/repo/worktree",
					},
					"createdAt": "2026-02-11T01:00:00Z",
				},
				{
					"id":        "perm-approved",
					"sessionID": "remote-session",
					"status":    "approved",
					"type":      "command",
					"command":   "echo done",
					"createdAt": "2026-02-11T01:01:00Z",
				},
				{
					"id":        "perm-other-session",
					"sessionID": "another-session",
					"status":    "pending",
					"type":      "command",
					"command":   "echo other",
					"createdAt": "2026-02-11T01:02:00Z",
				},
			},
		})
	}))
	defer server.Close()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENCODE_BASE_URL", server.URL)
	provider := &openCodeApprovalSyncProvider{provider: "opencode"}
	result, err := provider.SyncSessionApprovals(context.Background(), &types.Session{
		ID:       "s-open",
		Provider: "opencode",
	}, &types.SessionMeta{
		SessionID:         "s-open",
		ProviderSessionID: "remote-session",
	})
	if err != nil {
		t.Fatalf("SyncSessionApprovals: %v", err)
	}
	if result == nil || !result.Authoritative {
		t.Fatalf("expected authoritative result, got %#v", result)
	}
	if len(result.Approvals) != 1 {
		t.Fatalf("expected one pending approval, got %#v", result.Approvals)
	}
	approval := result.Approvals[0]
	if approval.SessionID != "s-open" {
		t.Fatalf("unexpected session id: %q", approval.SessionID)
	}
	if approval.Method != "item/commandExecution/requestApproval" {
		t.Fatalf("unexpected approval method: %q", approval.Method)
	}
	params := map[string]any{}
	if err := json.Unmarshal(approval.Params, &params); err != nil {
		t.Fatalf("decode approval params: %v", err)
	}
	if params["permission_id"] != "perm-pending" {
		t.Fatalf("missing permission id in params: %#v", params)
	}
	if params["title"] != "Run verification suite" {
		t.Fatalf("missing title in params: %#v", params)
	}
	if params["parsedCmd"] != "go test ./..." {
		t.Fatalf("missing parsed command in params: %#v", params)
	}
	metadata, _ := params["metadata"].(map[string]any)
	if metadata == nil || metadata["cwd"] != "/repo/worktree" {
		t.Fatalf("missing metadata in params: %#v", params)
	}
}

type fakeApprovalSyncProvider struct {
	provider string
	result   *ApprovalSyncResult
	err      error
}

func (f *fakeApprovalSyncProvider) Provider() string {
	return f.provider
}

func (f *fakeApprovalSyncProvider) SyncSessionApprovals(context.Context, *types.Session, *types.SessionMeta) (*ApprovalSyncResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return nil, nil
	}
	out := &ApprovalSyncResult{Authoritative: f.result.Authoritative}
	out.Approvals = make([]*types.Approval, 0, len(f.result.Approvals))
	for _, approval := range f.result.Approvals {
		if approval == nil {
			continue
		}
		copy := *approval
		out.Approvals = append(out.Approvals, &copy)
	}
	return out, nil
}
