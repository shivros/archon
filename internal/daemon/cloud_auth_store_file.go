package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type fileCloudAuthStore struct {
	path string
	mu   sync.Mutex
}

func newFileCloudAuthStore(path string) *fileCloudAuthStore {
	return &fileCloudAuthStore{path: strings.TrimSpace(path)}
}

func (s *fileCloudAuthStore) Load(context.Context) (*CloudAuthState, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return &CloudAuthState{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CloudAuthState{}, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return &CloudAuthState{}, nil
	}
	var state CloudAuthState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *fileCloudAuthStore) Save(_ context.Context, state *CloudAuthState) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return errors.New("cloud auth store path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	if state == nil {
		state = &CloudAuthState{}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.path, data, 0o600)
}
