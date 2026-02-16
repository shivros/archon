package store

import (
	"context"
	"errors"
	"strings"

	"control/internal/types"
)

const (
	RepositoryBackendFile  = "file"
	RepositoryBackendBbolt = "bbolt"
)

type Repository interface {
	AppState() AppStateStore
	SessionMeta() SessionMetaStore
	SessionIndex() SessionIndexStore
	Notes() NoteStore
	Backend() string
	Close() error
}

type RepositoryPaths struct {
	AppStatePath     string
	SessionMetaPath  string
	SessionIndexPath string
	NotesPath        string
	DBPath           string
}

type fileRepository struct {
	appState AppStateStore
	meta     SessionMetaStore
	sessions SessionIndexStore
	notes    NoteStore
}

func NewFileRepository(paths RepositoryPaths) Repository {
	return &fileRepository{
		appState: NewFileAppStateStore(paths.AppStatePath),
		meta:     NewFileSessionMetaStore(paths.SessionMetaPath),
		sessions: NewFileSessionIndexStore(paths.SessionIndexPath),
		notes:    NewFileNoteStore(paths.NotesPath),
	}
}

func (r *fileRepository) AppState() AppStateStore {
	return r.appState
}

func (r *fileRepository) SessionMeta() SessionMetaStore {
	return r.meta
}

func (r *fileRepository) SessionIndex() SessionIndexStore {
	return r.sessions
}

func (r *fileRepository) Notes() NoteStore {
	return r.notes
}

func (r *fileRepository) Backend() string {
	return RepositoryBackendFile
}

func (r *fileRepository) Close() error {
	return nil
}

func OpenRepository(paths RepositoryPaths, backend string) (Repository, error) {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", RepositoryBackendBbolt:
		if strings.TrimSpace(paths.DBPath) == "" {
			return nil, errors.New("db path is required for bbolt repository")
		}
		return NewBboltRepository(paths.DBPath)
	case RepositoryBackendFile:
		return NewFileRepository(paths), nil
	default:
		return nil, errors.New("unsupported repository backend: " + backend)
	}
}

// SeedRepositoryFromFiles migrates file-backed metadata into dst when dst is empty.
// This keeps startup backward-compatible for existing users while switching the
// hot path to transactional storage.
func SeedRepositoryFromFiles(ctx context.Context, dst Repository, paths RepositoryPaths) error {
	if dst == nil || dst.Backend() == RepositoryBackendFile {
		return nil
	}
	src := NewFileRepository(paths)
	defer src.Close()

	if err := seedAppState(ctx, dst.AppState(), src.AppState()); err != nil {
		return err
	}
	if err := seedSessionMeta(ctx, dst.SessionMeta(), src.SessionMeta()); err != nil {
		return err
	}
	if err := seedSessionIndex(ctx, dst.SessionIndex(), src.SessionIndex()); err != nil {
		return err
	}
	if err := seedNotes(ctx, dst.Notes(), src.Notes()); err != nil {
		return err
	}
	return nil
}

func seedAppState(ctx context.Context, dst AppStateStore, src AppStateStore) error {
	if dst == nil || src == nil {
		return nil
	}
	current, err := dst.Load(ctx)
	if err != nil {
		return err
	}
	if !isZeroAppState(current) {
		return nil
	}
	legacy, err := src.Load(ctx)
	if err != nil {
		return err
	}
	if isZeroAppState(legacy) {
		return nil
	}
	return dst.Save(ctx, legacy)
}

func seedSessionMeta(ctx context.Context, dst SessionMetaStore, src SessionMetaStore) error {
	if dst == nil || src == nil {
		return nil
	}
	current, err := dst.List(ctx)
	if err != nil {
		return err
	}
	if len(current) > 0 {
		return nil
	}
	legacy, err := src.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range legacy {
		if _, err := dst.Upsert(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func seedSessionIndex(ctx context.Context, dst SessionIndexStore, src SessionIndexStore) error {
	if dst == nil || src == nil {
		return nil
	}
	current, err := dst.ListRecords(ctx)
	if err != nil {
		return err
	}
	if len(current) > 0 {
		return nil
	}
	legacy, err := src.ListRecords(ctx)
	if err != nil {
		return err
	}
	for _, item := range legacy {
		if _, err := dst.UpsertRecord(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func seedNotes(ctx context.Context, dst NoteStore, src NoteStore) error {
	if dst == nil || src == nil {
		return nil
	}
	current, err := dst.List(ctx, NoteFilter{})
	if err != nil {
		return err
	}
	if len(current) > 0 {
		return nil
	}
	legacy, err := src.List(ctx, NoteFilter{})
	if err != nil {
		return err
	}
	for _, item := range legacy {
		if _, err := dst.Upsert(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func isZeroAppState(state *types.AppState) bool {
	if state == nil {
		return true
	}
	if strings.TrimSpace(state.ActiveWorkspaceID) != "" || strings.TrimSpace(state.ActiveWorktreeID) != "" {
		return false
	}
	if state.SidebarCollapsed {
		return false
	}
	return len(state.ActiveWorkspaceGroupIDs) == 0 &&
		len(state.ComposeHistory) == 0 &&
		len(state.ComposeDrafts) == 0 &&
		len(state.NoteDrafts) == 0 &&
		len(state.ComposeDefaultsByProvider) == 0 &&
		len(state.ProviderBadges) == 0
}
