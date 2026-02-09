package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"control/internal/types"
)

var ErrNoteNotFound = errors.New("note not found")

const noteSchemaVersion = 1

type NoteFilter struct {
	Scope       types.NoteScope
	WorkspaceID string
	WorktreeID  string
	SessionID   string
}

type NoteStore interface {
	List(ctx context.Context, filter NoteFilter) ([]*types.Note, error)
	Get(ctx context.Context, id string) (*types.Note, bool, error)
	Upsert(ctx context.Context, note *types.Note) (*types.Note, error)
	Delete(ctx context.Context, id string) error
}

type FileNoteStore struct {
	path string
	mu   sync.Mutex
}

type noteFile struct {
	Version int           `json:"version"`
	Notes   []*types.Note `json:"notes"`
}

func NewFileNoteStore(path string) *FileNoteStore {
	return &FileNoteStore{path: path}
}

func (s *FileNoteStore) List(ctx context.Context, filter NoteFilter) ([]*types.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			return []*types.Note{}, nil
		}
		return nil, err
	}

	out := make([]*types.Note, 0, len(file.Notes))
	for _, note := range file.Notes {
		if !matchesNoteFilter(note, filter) {
			continue
		}
		out = append(out, cloneNote(note))
	}
	// Newest first for easier note triage UX.
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *FileNoteStore) Get(ctx context.Context, id string) (*types.Note, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrNoteNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	for _, note := range file.Notes {
		if note.ID == id {
			return cloneNote(note), true, nil
		}
	}
	return nil, false, nil
}

func (s *FileNoteStore) Upsert(ctx context.Context, note *types.Note) (*types.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if note == nil {
		return nil, errors.New("note is required")
	}

	file, err := s.load()
	if err != nil && !errors.Is(err, ErrNoteNotFound) {
		return nil, err
	}
	if file == nil {
		file = newNoteFile()
	}

	normalized, err := normalizeNote(note, nil)
	if err != nil {
		return nil, err
	}
	updated := false
	for i, existing := range file.Notes {
		if existing.ID != normalized.ID {
			continue
		}
		normalized, err = normalizeNote(note, existing)
		if err != nil {
			return nil, err
		}
		file.Notes[i] = normalized
		updated = true
		break
	}
	if !updated {
		file.Notes = append(file.Notes, normalized)
	}

	if err := s.save(file); err != nil {
		return nil, err
	}
	return cloneNote(normalized), nil
}

func (s *FileNoteStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return err
	}
	filtered := file.Notes[:0]
	found := false
	for _, note := range file.Notes {
		if note.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, note)
	}
	file.Notes = filtered
	if !found {
		return ErrNoteNotFound
	}
	return s.save(file)
}

func matchesNoteFilter(note *types.Note, filter NoteFilter) bool {
	if note == nil {
		return false
	}
	if filter.Scope != "" && note.Scope != filter.Scope {
		return false
	}
	if strings.TrimSpace(filter.WorkspaceID) != "" && note.WorkspaceID != strings.TrimSpace(filter.WorkspaceID) {
		return false
	}
	if strings.TrimSpace(filter.WorktreeID) != "" && note.WorktreeID != strings.TrimSpace(filter.WorktreeID) {
		return false
	}
	if strings.TrimSpace(filter.SessionID) != "" && note.SessionID != strings.TrimSpace(filter.SessionID) {
		return false
	}
	return true
}

func (s *FileNoteStore) load() (*noteFile, error) {
	file := newNoteFile()
	if err := readJSON(s.path, file); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoteNotFound
		}
		return nil, err
	}
	if file.Version == 0 {
		file.Version = noteSchemaVersion
	}
	if file.Notes == nil {
		file.Notes = []*types.Note{}
	}
	return file, nil
}

func (s *FileNoteStore) save(file *noteFile) error {
	file.Version = noteSchemaVersion
	return writeJSONAtomic(s.path, file)
}

func newNoteFile() *noteFile {
	return &noteFile{Version: noteSchemaVersion, Notes: []*types.Note{}}
}

func normalizeNote(note *types.Note, existing *types.Note) (*types.Note, error) {
	normalized := *note
	if strings.TrimSpace(normalized.ID) == "" {
		normalized.ID = newNoteID()
	}
	if existing != nil {
		normalized.ID = existing.ID
		normalized.CreatedAt = existing.CreatedAt
	} else if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = time.Now().UTC()
	}
	if normalized.UpdatedAt.IsZero() || existing != nil {
		normalized.UpdatedAt = time.Now().UTC()
	}
	if normalized.Tags == nil {
		normalized.Tags = []string{}
	}
	if normalized.Source != nil {
		sourceCopy := *normalized.Source
		normalized.Source = &sourceCopy
	}
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = time.Now().UTC()
	}
	return &normalized, nil
}

func cloneNote(note *types.Note) *types.Note {
	if note == nil {
		return nil
	}
	copy := *note
	if note.Tags != nil {
		copy.Tags = append([]string(nil), note.Tags...)
	}
	if note.Source != nil {
		source := *note.Source
		copy.Source = &source
	}
	return &copy
}

func newNoteID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "note" + time.Now().UTC().Format("20060102150405")
	}
	return "note_" + hex.EncodeToString(buf)
}
