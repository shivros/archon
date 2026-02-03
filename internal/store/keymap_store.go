package store

import (
	"context"
	"errors"
	"os"
	"sync"

	"control/internal/types"
)

type KeymapStore interface {
	Load(ctx context.Context) (*types.Keymap, error)
	Save(ctx context.Context, keymap *types.Keymap) error
}

type FileKeymapStore struct {
	path string
	mu   sync.Mutex
}

func NewFileKeymapStore(path string) *FileKeymapStore {
	return &FileKeymapStore{path: path}
}

func (s *FileKeymapStore) Load(ctx context.Context) (*types.Keymap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keymap := types.DefaultKeymap()
	err := readJSON(s.path, keymap)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return keymap, nil
		}
		return nil, err
	}
	if keymap.Bindings == nil {
		keymap.Bindings = types.DefaultKeymap().Bindings
	}
	return keymap, nil
}

func (s *FileKeymapStore) Save(ctx context.Context, keymap *types.Keymap) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if keymap == nil {
		return errors.New("keymap is required")
	}
	return writeJSONAtomic(s.path, keymap)
}
