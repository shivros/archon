package daemon

import (
	"context"
	"encoding/json"
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
