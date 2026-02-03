package daemon

import (
	"context"

	"control/internal/types"
)

type AppStateService struct {
	store AppStateStore
}

func NewAppStateService(stores *Stores) *AppStateService {
	if stores == nil {
		return &AppStateService{}
	}
	return &AppStateService{store: stores.AppState}
}

func (s *AppStateService) Get(ctx context.Context) (*types.AppState, error) {
	if s.store == nil {
		return nil, unavailableError("state store not available", nil)
	}
	return s.store.Load(ctx)
}

func (s *AppStateService) Update(ctx context.Context, state *types.AppState) error {
	if s.store == nil {
		return unavailableError("state store not available", nil)
	}
	if state == nil {
		return invalidError("state payload is required", nil)
	}
	return s.store.Save(ctx, state)
}
