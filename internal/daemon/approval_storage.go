package daemon

import (
	"context"
	"encoding/json"
	"time"

	"control/internal/types"
)

type ApprovalStorage interface {
	StoreApproval(ctx context.Context, sessionID string, requestID int, method string, params json.RawMessage) error
}

type StoreApprovalStorage struct {
	stores *Stores
}

func NewStoreApprovalStorage(stores *Stores) *StoreApprovalStorage {
	return &StoreApprovalStorage{stores: stores}
}

func (s *StoreApprovalStorage) StoreApproval(ctx context.Context, sessionID string, requestID int, method string, params json.RawMessage) error {
	if s.stores == nil || s.stores.Approvals == nil {
		return nil
	}

	approval := &types.Approval{
		SessionID: sessionID,
		RequestID: requestID,
		Method:    method,
		Params:    params,
		CreatedAt: time.Now().UTC(),
	}
	_, err := s.stores.Approvals.Upsert(ctx, approval)
	return err
}

type NopApprovalStorage struct{}

func (NopApprovalStorage) StoreApproval(_ context.Context, _ string, _ int, _ string, _ json.RawMessage) error {
	return nil
}
