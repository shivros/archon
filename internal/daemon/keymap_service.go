package daemon

import (
	"context"

	"control/internal/types"
)

type KeymapService struct {
	store KeymapStore
}

func NewKeymapService(stores *Stores) *KeymapService {
	if stores == nil {
		return &KeymapService{}
	}
	return &KeymapService{store: stores.Keymap}
}

func (s *KeymapService) Get(ctx context.Context) (*types.Keymap, error) {
	if s.store == nil {
		return nil, unavailableError("keymap store not available", nil)
	}
	return s.store.Load(ctx)
}

func (s *KeymapService) Update(ctx context.Context, keymap *types.Keymap) error {
	if s.store == nil {
		return unavailableError("keymap store not available", nil)
	}
	if keymap == nil {
		return invalidError("keymap payload is required", nil)
	}
	return s.store.Save(ctx, keymap)
}
