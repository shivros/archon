package store

import (
	"context"
	"errors"
	"os"
	"sort"
	"sync"
	"time"

	"control/internal/types"
)

var ErrSessionMetaNotFound = errors.New("session meta not found")

const sessionMetaSchemaVersion = 1

type SessionMetaStore interface {
	List(ctx context.Context) ([]*types.SessionMeta, error)
	Get(ctx context.Context, sessionID string) (*types.SessionMeta, bool, error)
	Upsert(ctx context.Context, meta *types.SessionMeta) (*types.SessionMeta, error)
	Delete(ctx context.Context, sessionID string) error
}

type FileSessionMetaStore struct {
	path string
	mu   sync.Mutex
}

type sessionMetaFile struct {
	Version  int                  `json:"version"`
	Sessions []*types.SessionMeta `json:"sessions"`
}

func NewFileSessionMetaStore(path string) *FileSessionMetaStore {
	return &FileSessionMetaStore{path: path}
}

func (s *FileSessionMetaStore) List(ctx context.Context) ([]*types.SessionMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrSessionMetaNotFound) {
			return []*types.SessionMeta{}, nil
		}
		return nil, err
	}
	out := make([]*types.SessionMeta, 0, len(file.Sessions))
	for _, meta := range file.Sessions {
		copy := *meta
		out = append(out, &copy)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SessionID < out[j].SessionID
	})
	return out, nil
}

func (s *FileSessionMetaStore) Get(ctx context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrSessionMetaNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	for _, meta := range file.Sessions {
		if meta.SessionID == sessionID {
			copy := *meta
			return &copy, true, nil
		}
	}
	return nil, false, nil
}

func (s *FileSessionMetaStore) Upsert(ctx context.Context, meta *types.SessionMeta) (*types.SessionMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if meta == nil || meta.SessionID == "" {
		return nil, errors.New("session meta requires session_id")
	}

	file, err := s.load()
	if err != nil && !errors.Is(err, ErrSessionMetaNotFound) {
		return nil, err
	}
	if file == nil {
		file = newSessionMetaFile()
	}

	updated := false
	for i, existing := range file.Sessions {
		if existing.SessionID == meta.SessionID {
			file.Sessions[i] = normalizeSessionMeta(meta, existing)
			updated = true
			break
		}
	}
	if !updated {
		file.Sessions = append(file.Sessions, normalizeSessionMeta(meta, nil))
	}

	if err := s.save(file); err != nil {
		return nil, err
	}
	copy := *meta
	return &copy, nil
}

func (s *FileSessionMetaStore) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return err
	}
	filtered := file.Sessions[:0]
	found := false
	for _, meta := range file.Sessions {
		if meta.SessionID == sessionID {
			found = true
			continue
		}
		filtered = append(filtered, meta)
	}
	file.Sessions = filtered
	if !found {
		return ErrSessionMetaNotFound
	}
	return s.save(file)
}

func (s *FileSessionMetaStore) load() (*sessionMetaFile, error) {
	file := newSessionMetaFile()
	err := readJSON(s.path, file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrSessionMetaNotFound
		}
		return nil, err
	}
	if file.Version == 0 {
		file.Version = sessionMetaSchemaVersion
	}
	return file, nil
}

func (s *FileSessionMetaStore) save(file *sessionMetaFile) error {
	file.Version = sessionMetaSchemaVersion
	return writeJSONAtomic(s.path, file)
}

func newSessionMetaFile() *sessionMetaFile {
	return &sessionMetaFile{
		Version:  sessionMetaSchemaVersion,
		Sessions: []*types.SessionMeta{},
	}
}

func normalizeSessionMeta(meta *types.SessionMeta, existing *types.SessionMeta) *types.SessionMeta {
	normalized := *meta
	clearDismissedAt := normalized.DismissedAt != nil && normalized.DismissedAt.IsZero()
	if clearDismissedAt {
		normalized.DismissedAt = nil
	}
	if existing != nil {
		if normalized.Title == "" {
			normalized.Title = existing.Title
		}
		if !normalized.TitleLocked {
			normalized.TitleLocked = existing.TitleLocked
		}
		if normalized.InitialInput == "" {
			normalized.InitialInput = existing.InitialInput
		}
		if !clearDismissedAt && normalized.DismissedAt == nil && existing.DismissedAt != nil {
			ts := existing.DismissedAt.UTC()
			normalized.DismissedAt = &ts
		}
		if normalized.WorkspaceID == "" {
			normalized.WorkspaceID = existing.WorkspaceID
		}
		if normalized.WorktreeID == "" {
			normalized.WorktreeID = existing.WorktreeID
		}
		if normalized.ThreadID == "" {
			normalized.ThreadID = existing.ThreadID
		}
		if normalized.ProviderSessionID == "" {
			normalized.ProviderSessionID = existing.ProviderSessionID
		}
		if normalized.LastTurnID == "" {
			normalized.LastTurnID = existing.LastTurnID
		}
		if normalized.RuntimeOptions == nil {
			normalized.RuntimeOptions = types.CloneRuntimeOptions(existing.RuntimeOptions)
		}
		if normalized.NotificationOverrides == nil {
			normalized.NotificationOverrides = types.CloneNotificationSettingsPatch(existing.NotificationOverrides)
		}
	}
	if normalized.LastActiveAt == nil {
		now := time.Now().UTC()
		normalized.LastActiveAt = &now
	}
	return &normalized
}
