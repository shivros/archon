package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"control/internal/types"
)

var (
	bucketAppState     = []byte("app_state")
	bucketSessionMeta  = []byte("session_meta")
	bucketSessionIndex = []byte("session_index")
	bucketNotes        = []byte("notes")
	keyAppState        = []byte("state")
)

type bboltRepository struct {
	db       *bolt.DB
	appState AppStateStore
	meta     SessionMetaStore
	sessions SessionIndexStore
	notes    NoteStore
}

func NewBboltRepository(path string) (Repository, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("repository db path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, err
	}
	if err := initBboltSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	repo := &bboltRepository{db: db}
	repo.appState = &bboltAppStateStore{db: db}
	repo.meta = &bboltSessionMetaStore{db: db}
	repo.sessions = &bboltSessionIndexStore{db: db}
	repo.notes = &bboltNoteStore{db: db}
	return repo, nil
}

func (r *bboltRepository) AppState() AppStateStore {
	return r.appState
}

func (r *bboltRepository) SessionMeta() SessionMetaStore {
	return r.meta
}

func (r *bboltRepository) SessionIndex() SessionIndexStore {
	return r.sessions
}

func (r *bboltRepository) Notes() NoteStore {
	return r.notes
}

func (r *bboltRepository) Backend() string {
	return RepositoryBackendBbolt
}

func (r *bboltRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func initBboltSchema(db *bolt.DB) error {
	return db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucketAppState); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketSessionMeta); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketSessionIndex); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketNotes); err != nil {
			return err
		}
		return nil
	})
}

type bboltAppStateStore struct {
	db *bolt.DB
}

func (s *bboltAppStateStore) Load(ctx context.Context) (*types.AppState, error) {
	state := &types.AppState{}
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAppState)
		if b == nil {
			return nil
		}
		raw := b.Get(keyAppState)
		if len(raw) == 0 {
			return nil
		}
		return json.Unmarshal(raw, state)
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}

func (s *bboltAppStateStore) Save(ctx context.Context, state *types.AppState) error {
	if state == nil {
		return errors.New("state is required")
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketAppState)
		if b == nil {
			return errors.New("app state bucket missing")
		}
		return b.Put(keyAppState, raw)
	})
}

type bboltSessionMetaStore struct {
	db *bolt.DB
	mu sync.Mutex
}

func (s *bboltSessionMetaStore) List(ctx context.Context) ([]*types.SessionMeta, error) {
	out := make([]*types.SessionMeta, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionMeta)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var meta types.SessionMeta
			if err := json.Unmarshal(v, &meta); err != nil {
				return err
			}
			copyMeta := meta
			out = append(out, &copyMeta)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SessionID < out[j].SessionID
	})
	return out, nil
}

func (s *bboltSessionMetaStore) Get(ctx context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	var (
		out *types.SessionMeta
		ok  bool
	)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionMeta)
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(sessionID))
		if len(raw) == 0 {
			return nil
		}
		var meta types.SessionMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			return err
		}
		copyMeta := meta
		out = &copyMeta
		ok = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, ok, nil
}

func (s *bboltSessionMetaStore) Upsert(ctx context.Context, meta *types.SessionMeta) (*types.SessionMeta, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if meta == nil || meta.SessionID == "" {
		return nil, errors.New("session meta requires session_id")
	}
	normalized, err := s.normalizeUpsert(ctx, meta)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionMeta)
		if b == nil {
			return errors.New("session meta bucket missing")
		}
		return b.Put([]byte(normalized.SessionID), raw)
	}); err != nil {
		return nil, err
	}
	copyMeta := *normalized
	return &copyMeta, nil
}

func (s *bboltSessionMetaStore) normalizeUpsert(ctx context.Context, meta *types.SessionMeta) (*types.SessionMeta, error) {
	existing, _, err := s.Get(ctx, meta.SessionID)
	if err != nil {
		return nil, err
	}
	return normalizeSessionMeta(meta, existing), nil
}

func (s *bboltSessionMetaStore) Delete(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionMeta)
		if b == nil {
			return errors.New("session meta bucket missing")
		}
		key := []byte(sessionID)
		if b.Get(key) == nil {
			return ErrSessionMetaNotFound
		}
		return b.Delete(key)
	})
}

type bboltSessionIndexStore struct {
	db *bolt.DB
	mu sync.Mutex
}

func (s *bboltSessionIndexStore) ListRecords(ctx context.Context) ([]*types.SessionRecord, error) {
	out := make([]*types.SessionRecord, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionIndex)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var record types.SessionRecord
			if err := json.Unmarshal(v, &record); err != nil {
				return err
			}
			out = append(out, cloneRecord(&record))
			return nil
		})
	})
	if err != nil {
		return nil, err
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

func (s *bboltSessionIndexStore) GetRecord(ctx context.Context, sessionID string) (*types.SessionRecord, bool, error) {
	var (
		record *types.SessionRecord
		ok     bool
	)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionIndex)
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(sessionID))
		if len(raw) == 0 {
			return nil
		}
		var item types.SessionRecord
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		record = cloneRecord(&item)
		ok = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return record, ok, nil
}

func (s *bboltSessionIndexStore) UpsertRecord(ctx context.Context, record *types.SessionRecord) (*types.SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if record == nil || record.Session == nil || record.Session.ID == "" {
		return nil, errors.New("session record requires session id")
	}
	normalized := normalizeSessionRecord(record)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionIndex)
		if b == nil {
			return errors.New("session index bucket missing")
		}
		return b.Put([]byte(normalized.Session.ID), raw)
	}); err != nil {
		return nil, err
	}
	return cloneRecord(normalized), nil
}

func (s *bboltSessionIndexStore) DeleteRecord(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessionIndex)
		if b == nil {
			return errors.New("session index bucket missing")
		}
		key := []byte(sessionID)
		if b.Get(key) == nil {
			return errors.New("session record not found")
		}
		return b.Delete(key)
	})
}

type bboltNoteStore struct {
	db *bolt.DB
	mu sync.Mutex
}

func (s *bboltNoteStore) List(ctx context.Context, filter NoteFilter) ([]*types.Note, error) {
	out := make([]*types.Note, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNotes)
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			var note types.Note
			if err := json.Unmarshal(v, &note); err != nil {
				return err
			}
			if !matchesNoteFilter(&note, filter) {
				return nil
			}
			out = append(out, cloneNote(&note))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *bboltNoteStore) Get(ctx context.Context, id string) (*types.Note, bool, error) {
	var (
		note *types.Note
		ok   bool
	)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNotes)
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(id))
		if len(raw) == 0 {
			return nil
		}
		var item types.Note
		if err := json.Unmarshal(raw, &item); err != nil {
			return err
		}
		note = cloneNote(&item)
		ok = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return note, ok, nil
}

func (s *bboltNoteStore) Upsert(ctx context.Context, note *types.Note) (*types.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if note == nil {
		return nil, errors.New("note is required")
	}
	existing, _, err := s.Get(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeNote(note, existing)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNotes)
		if b == nil {
			return errors.New("notes bucket missing")
		}
		return b.Put([]byte(normalized.ID), raw)
	}); err != nil {
		return nil, err
	}
	return cloneNote(normalized), nil
}

func (s *bboltNoteStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNotes)
		if b == nil {
			return errors.New("notes bucket missing")
		}
		key := []byte(id)
		if b.Get(key) == nil {
			return ErrNoteNotFound
		}
		return b.Delete(key)
	})
}
