package store

import (
	"context"
	"errors"
	"os"
	"sync"

	"control/internal/types"
)

type AppStateStore interface {
	Load(ctx context.Context) (*types.AppState, error)
	Save(ctx context.Context, state *types.AppState) error
}

type FileAppStateStore struct {
	path string
	mu   sync.Mutex
}

func NewFileAppStateStore(path string) *FileAppStateStore {
	return &FileAppStateStore{path: path}
}

func (s *FileAppStateStore) Load(ctx context.Context) (*types.AppState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := &types.AppState{}
	err := readJSON(s.path, state)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return nil, err
	}
	return state, nil
}

func (s *FileAppStateStore) Save(ctx context.Context, state *types.AppState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state == nil {
		return errors.New("state is required")
	}
	return writeJSONAtomic(s.path, state)
}
