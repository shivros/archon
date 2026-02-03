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

const sessionIndexSchemaVersion = 1

type SessionIndexStore interface {
	ListRecords(ctx context.Context) ([]*types.SessionRecord, error)
	GetRecord(ctx context.Context, sessionID string) (*types.SessionRecord, bool, error)
	UpsertRecord(ctx context.Context, record *types.SessionRecord) (*types.SessionRecord, error)
	DeleteRecord(ctx context.Context, sessionID string) error
}

type FileSessionIndexStore struct {
	path string
	mu   sync.Mutex
}

type sessionIndexFile struct {
	Version int                    `json:"version"`
	Records []*types.SessionRecord `json:"records"`
}

func NewFileSessionIndexStore(path string) *FileSessionIndexStore {
	return &FileSessionIndexStore{path: path}
}

func (s *FileSessionIndexStore) ListRecords(ctx context.Context) ([]*types.SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []*types.SessionRecord{}, nil
		}
		return nil, err
	}
	out := make([]*types.SessionRecord, 0, len(file.Records))
	for _, record := range file.Records {
		copy := cloneRecord(record)
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Session
		right := out[j].Session
		if left == nil || right == nil {
			return left != nil
		}
		return left.CreatedAt.After(right.CreatedAt)
	})
	return out, nil
}

func (s *FileSessionIndexStore) GetRecord(ctx context.Context, sessionID string) (*types.SessionRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	for _, record := range file.Records {
		if record.Session != nil && record.Session.ID == sessionID {
			return cloneRecord(record), true, nil
		}
	}
	return nil, false, nil
}

func (s *FileSessionIndexStore) UpsertRecord(ctx context.Context, record *types.SessionRecord) (*types.SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record == nil || record.Session == nil || record.Session.ID == "" {
		return nil, errors.New("session record requires session id")
	}
	file, err := s.load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if file == nil {
		file = newSessionIndexFile()
	}

	normalized := normalizeSessionRecord(record)
	updated := false
	for i, existing := range file.Records {
		if existing.Session != nil && existing.Session.ID == normalized.Session.ID {
			file.Records[i] = normalized
			updated = true
			break
		}
	}
	if !updated {
		file.Records = append(file.Records, normalized)
	}

	if err := s.save(file); err != nil {
		return nil, err
	}
	return cloneRecord(normalized), nil
}

func (s *FileSessionIndexStore) DeleteRecord(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return err
	}
	filtered := file.Records[:0]
	found := false
	for _, record := range file.Records {
		if record.Session != nil && record.Session.ID == sessionID {
			found = true
			continue
		}
		filtered = append(filtered, record)
	}
	file.Records = filtered
	if !found {
		return errors.New("session record not found")
	}
	return s.save(file)
}

func (s *FileSessionIndexStore) load() (*sessionIndexFile, error) {
	file := newSessionIndexFile()
	if err := readJSON(s.path, file); err != nil {
		return nil, err
	}
	if file.Version == 0 {
		file.Version = sessionIndexSchemaVersion
	}
	return file, nil
}

func (s *FileSessionIndexStore) save(file *sessionIndexFile) error {
	file.Version = sessionIndexSchemaVersion
	return writeJSONAtomic(s.path, file)
}

func newSessionIndexFile() *sessionIndexFile {
	return &sessionIndexFile{
		Version: sessionIndexSchemaVersion,
		Records: []*types.SessionRecord{},
	}
}

func normalizeSessionRecord(record *types.SessionRecord) *types.SessionRecord {
	copy := cloneRecord(record)
	if copy.Session == nil {
		return copy
	}
	if copy.Session.CreatedAt.IsZero() {
		copy.Session.CreatedAt = time.Now().UTC()
	}
	return copy
}

func cloneRecord(record *types.SessionRecord) *types.SessionRecord {
	if record == nil {
		return nil
	}
	clone := *record
	if record.Session != nil {
		copySession := *record.Session
		clone.Session = &copySession
	}
	return &clone
}
