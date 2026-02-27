package daemon

import (
	"context"
	"encoding/json"
	"time"

	"control/internal/types"
)

type ApprovalStorage interface {
	StoreApproval(ctx context.Context, sessionID string, requestID int, method string, params json.RawMessage) error
	GetApproval(ctx context.Context, sessionID string, requestID int) (*types.Approval, bool, error)
	DeleteApproval(ctx context.Context, sessionID string, requestID int) error
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

func (s *StoreApprovalStorage) GetApproval(ctx context.Context, sessionID string, requestID int) (*types.Approval, bool, error) {
	if s.stores == nil || s.stores.Approvals == nil {
		return nil, false, nil
	}
	return s.stores.Approvals.Get(ctx, sessionID, requestID)
}

func (s *StoreApprovalStorage) DeleteApproval(ctx context.Context, sessionID string, requestID int) error {
	if s.stores == nil || s.stores.Approvals == nil {
		return nil
	}
	return s.stores.Approvals.Delete(ctx, sessionID, requestID)
}

type NopApprovalStorage struct{}

func (NopApprovalStorage) StoreApproval(_ context.Context, _ string, _ int, _ string, _ json.RawMessage) error {
	return nil
}

func (NopApprovalStorage) GetApproval(_ context.Context, _ string, _ int) (*types.Approval, bool, error) {
	return nil, false, nil
}

func (NopApprovalStorage) DeleteApproval(_ context.Context, _ string, _ int) error {
	return nil
}
