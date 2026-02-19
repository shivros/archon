package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

var (
	bucketAppState          = []byte("app_state")
	bucketSessionMeta       = []byte("session_meta")
	bucketSessionIndex      = []byte("session_index")
	bucketWorkspaces        = []byte("workspaces")
	bucketWorktrees         = []byte("worktrees")
	bucketGroups            = []byte("workspace_groups")
	bucketWorkflowTemplates = []byte("workflow_templates")
	bucketWorkflowRuns      = []byte("workflow_runs")
	bucketApprovals         = []byte("approvals")
	bucketNotes             = []byte("notes")
	keyAppState             = []byte("state")
)

type bboltRepository struct {
	db                *bolt.DB
	workspaces        WorkspaceStore
	worktrees         WorktreeStore
	groups            WorkspaceGroupStore
	workflowTemplates WorkflowTemplateStore
	appState          AppStateStore
	meta              SessionMetaStore
	sessions          SessionIndexStore
	approvals         ApprovalStore
	notes             NoteStore
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
	workspaceStore := &bboltWorkspaceStore{db: db}
	repo.workspaces = workspaceStore
	repo.worktrees = workspaceStore
	repo.groups = workspaceStore
	repo.workflowTemplates = &bboltWorkflowTemplateStore{db: db}
	repo.appState = &bboltAppStateStore{db: db}
	repo.meta = &bboltSessionMetaStore{db: db}
	repo.sessions = &bboltSessionIndexStore{db: db}
	repo.approvals = &bboltApprovalStore{db: db}
	repo.notes = &bboltNoteStore{db: db}
	return repo, nil
}

func (r *bboltRepository) Workspaces() WorkspaceStore {
	return r.workspaces
}

func (r *bboltRepository) Worktrees() WorktreeStore {
	return r.worktrees
}

func (r *bboltRepository) Groups() WorkspaceGroupStore {
	return r.groups
}

func (r *bboltRepository) WorkflowTemplates() WorkflowTemplateStore {
	return r.workflowTemplates
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

func (r *bboltRepository) Approvals() ApprovalStore {
	return r.approvals
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
		if _, err := tx.CreateBucketIfNotExists(bucketWorkspaces); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketWorktrees); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketGroups); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketWorkflowTemplates); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketWorkflowRuns); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketApprovals); err != nil {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists(bucketNotes); err != nil {
			return err
		}
		return nil
	})
}

type bboltWorkspaceStore struct {
	db *bolt.DB
	mu sync.Mutex
}

func (s *bboltWorkspaceStore) List(ctx context.Context) ([]*types.Workspace, error) {
	out := make([]*types.Workspace, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkspaces)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var ws types.Workspace
			if err := json.Unmarshal(v, &ws); err != nil {
				return err
			}
			out = append(out, cloneWorkspace(&ws))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *bboltWorkspaceStore) Get(ctx context.Context, id string) (*types.Workspace, bool, error) {
	var (
		out *types.Workspace
		ok  bool
	)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkspaces)
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(id))
		if len(raw) == 0 {
			return nil
		}
		var ws types.Workspace
		if err := json.Unmarshal(raw, &ws); err != nil {
			return err
		}
		out = cloneWorkspace(&ws)
		ok = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, ok, nil
}

func (s *bboltWorkspaceStore) Add(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(ws)
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkspaces)
		if b == nil {
			return errors.New("workspaces bucket missing")
		}
		key := []byte(ws.ID)
		if b.Get(key) != nil {
			return errors.New("workspace already exists")
		}
		return b.Put(key, raw)
	}); err != nil {
		return nil, err
	}
	return cloneWorkspace(ws), nil
}

func (s *bboltWorkspaceStore) Update(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkspaces)
		if b == nil {
			return errors.New("workspaces bucket missing")
		}
		key := []byte(ws.ID)
		current := b.Get(key)
		if len(current) == 0 {
			return ErrWorkspaceNotFound
		}
		var existing types.Workspace
		if err := json.Unmarshal(current, &existing); err != nil {
			return err
		}
		ws.CreatedAt = existing.CreatedAt
		ws.UpdatedAt = time.Now().UTC()
		raw, err := json.Marshal(ws)
		if err != nil {
			return err
		}
		return b.Put(key, raw)
	}); err != nil {
		return nil, err
	}
	return cloneWorkspace(ws), nil
}

func (s *bboltWorkspaceStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		workspaces := tx.Bucket(bucketWorkspaces)
		worktrees := tx.Bucket(bucketWorktrees)
		if workspaces == nil || worktrees == nil {
			return errors.New("workspace buckets missing")
		}
		key := []byte(id)
		if workspaces.Get(key) == nil {
			return ErrWorkspaceNotFound
		}
		if err := workspaces.Delete(key); err != nil {
			return err
		}
		c := worktrees.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var wt types.Worktree
			if err := json.Unmarshal(v, &wt); err != nil {
				return err
			}
			if wt.WorkspaceID == id {
				if err := c.Delete(); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *bboltWorkspaceStore) ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error) {
	out := make([]*types.Worktree, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorktrees)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var wt types.Worktree
			if err := json.Unmarshal(v, &wt); err != nil {
				return err
			}
			if wt.WorkspaceID != workspaceID {
				return nil
			}
			out = append(out, cloneWorktree(&wt))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *bboltWorkspaceStore) AddWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := normalizeWorktree(workspaceID, worktree)
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		workspaces := tx.Bucket(bucketWorkspaces)
		worktrees := tx.Bucket(bucketWorktrees)
		if workspaces == nil || worktrees == nil {
			return errors.New("workspace buckets missing")
		}
		if workspaces.Get([]byte(workspaceID)) == nil {
			return ErrWorkspaceNotFound
		}
		c := worktrees.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var existing types.Worktree
			if err := json.Unmarshal(v, &existing); err != nil {
				return err
			}
			if existing.ID == wt.ID {
				return errors.New("worktree already exists")
			}
			if existing.WorkspaceID == workspaceID && strings.EqualFold(existing.Path, wt.Path) {
				return errors.New("worktree path already added")
			}
		}
		raw, err := json.Marshal(wt)
		if err != nil {
			return err
		}
		return worktrees.Put([]byte(wt.ID), raw)
	}); err != nil {
		return nil, err
	}
	return cloneWorktree(wt), nil
}

func (s *bboltWorkspaceStore) UpdateWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	wt, err := normalizeWorktree(workspaceID, worktree)
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		workspaces := tx.Bucket(bucketWorkspaces)
		worktrees := tx.Bucket(bucketWorktrees)
		if workspaces == nil || worktrees == nil {
			return errors.New("workspace buckets missing")
		}
		if workspaces.Get([]byte(workspaceID)) == nil {
			return ErrWorkspaceNotFound
		}
		key := []byte(wt.ID)
		current := worktrees.Get(key)
		if len(current) == 0 {
			return ErrWorktreeNotFound
		}
		var existing types.Worktree
		if err := json.Unmarshal(current, &existing); err != nil {
			return err
		}
		if existing.WorkspaceID != workspaceID {
			return ErrWorktreeNotFound
		}
		c := worktrees.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var candidate types.Worktree
			if err := json.Unmarshal(v, &candidate); err != nil {
				return err
			}
			if candidate.ID == wt.ID {
				continue
			}
			if candidate.WorkspaceID == workspaceID && strings.EqualFold(candidate.Path, wt.Path) {
				return errors.New("worktree path already added")
			}
		}
		wt.CreatedAt = existing.CreatedAt
		wt.UpdatedAt = time.Now().UTC()
		raw, err := json.Marshal(wt)
		if err != nil {
			return err
		}
		return worktrees.Put(key, raw)
	}); err != nil {
		return nil, err
	}
	return cloneWorktree(wt), nil
}

func (s *bboltWorkspaceStore) DeleteWorktree(ctx context.Context, workspaceID, worktreeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorktrees)
		if b == nil {
			return errors.New("worktrees bucket missing")
		}
		key := []byte(worktreeID)
		raw := b.Get(key)
		if len(raw) == 0 {
			return ErrWorktreeNotFound
		}
		var existing types.Worktree
		if err := json.Unmarshal(raw, &existing); err != nil {
			return err
		}
		if existing.WorkspaceID != workspaceID {
			return ErrWorktreeNotFound
		}
		return b.Delete(key)
	})
}

func (s *bboltWorkspaceStore) ListGroups(ctx context.Context) ([]*types.WorkspaceGroup, error) {
	out := make([]*types.WorkspaceGroup, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketGroups)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var group types.WorkspaceGroup
			if err := json.Unmarshal(v, &group); err != nil {
				return err
			}
			out = append(out, cloneWorkspaceGroup(&group))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *bboltWorkspaceStore) GetGroup(ctx context.Context, id string) (*types.WorkspaceGroup, bool, error) {
	var (
		out *types.WorkspaceGroup
		ok  bool
	)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketGroups)
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(id))
		if len(raw) == 0 {
			return nil
		}
		var group types.WorkspaceGroup
		if err := json.Unmarshal(raw, &group); err != nil {
			return err
		}
		out = cloneWorkspaceGroup(&group)
		ok = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, ok, nil
}

func (s *bboltWorkspaceStore) AddGroup(ctx context.Context, group *types.WorkspaceGroup) (*types.WorkspaceGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized, err := normalizeWorkspaceGroup(group)
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketGroups)
		if b == nil {
			return errors.New("workspace groups bucket missing")
		}
		if b.Get([]byte(normalized.ID)) != nil {
			return errors.New("workspace group already exists")
		}
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var existing types.WorkspaceGroup
			if err := json.Unmarshal(v, &existing); err != nil {
				return err
			}
			if strings.EqualFold(existing.Name, normalized.Name) {
				return errors.New("workspace group name already exists")
			}
		}
		raw, err := json.Marshal(normalized)
		if err != nil {
			return err
		}
		return b.Put([]byte(normalized.ID), raw)
	}); err != nil {
		return nil, err
	}
	return cloneWorkspaceGroup(normalized), nil
}

func (s *bboltWorkspaceStore) UpdateGroup(ctx context.Context, group *types.WorkspaceGroup) (*types.WorkspaceGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized, err := normalizeWorkspaceGroup(group)
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketGroups)
		if b == nil {
			return errors.New("workspace groups bucket missing")
		}
		key := []byte(normalized.ID)
		current := b.Get(key)
		if len(current) == 0 {
			return ErrWorkspaceGroupNotFound
		}
		var existing types.WorkspaceGroup
		if err := json.Unmarshal(current, &existing); err != nil {
			return err
		}
		normalized.CreatedAt = existing.CreatedAt
		normalized.UpdatedAt = time.Now().UTC()
		raw, err := json.Marshal(normalized)
		if err != nil {
			return err
		}
		return b.Put(key, raw)
	}); err != nil {
		return nil, err
	}
	return cloneWorkspaceGroup(normalized), nil
}

func (s *bboltWorkspaceStore) DeleteGroup(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketGroups)
		if b == nil {
			return errors.New("workspace groups bucket missing")
		}
		key := []byte(id)
		if b.Get(key) == nil {
			return ErrWorkspaceGroupNotFound
		}
		return b.Delete(key)
	})
}

type bboltApprovalStore struct {
	db *bolt.DB
	mu sync.Mutex
}

func (s *bboltApprovalStore) ListBySession(ctx context.Context, sessionID string) ([]*types.Approval, error) {
	out := make([]*types.Approval, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketApprovals)
		if b == nil {
			return nil
		}
		prefix := approvalSessionPrefix(sessionID)
		c := b.Cursor()
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			var approval types.Approval
			if err := json.Unmarshal(v, &approval); err != nil {
				return err
			}
			out = append(out, cloneApproval(&approval))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *bboltApprovalStore) Get(ctx context.Context, sessionID string, requestID int) (*types.Approval, bool, error) {
	var (
		out *types.Approval
		ok  bool
	)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketApprovals)
		if b == nil {
			return nil
		}
		raw := b.Get(approvalKey(sessionID, requestID))
		if len(raw) == 0 {
			return nil
		}
		var approval types.Approval
		if err := json.Unmarshal(raw, &approval); err != nil {
			return err
		}
		out = cloneApproval(&approval)
		ok = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, ok, nil
}

func (s *bboltApprovalStore) Upsert(ctx context.Context, approval *types.Approval) (*types.Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if approval == nil || approval.SessionID == "" || approval.RequestID < 0 {
		return nil, errors.New("approval requires session_id and request_id")
	}
	var normalized *types.Approval
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketApprovals)
		if b == nil {
			return errors.New("approvals bucket missing")
		}
		key := approvalKey(approval.SessionID, approval.RequestID)
		var existing *types.Approval
		if raw := b.Get(key); len(raw) > 0 {
			var item types.Approval
			if err := json.Unmarshal(raw, &item); err != nil {
				return err
			}
			existing = &item
		}
		normalized = normalizeApproval(approval, existing)
		raw, err := json.Marshal(normalized)
		if err != nil {
			return err
		}
		return b.Put(key, raw)
	}); err != nil {
		return nil, err
	}
	return cloneApproval(normalized), nil
}

func (s *bboltApprovalStore) Delete(ctx context.Context, sessionID string, requestID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketApprovals)
		if b == nil {
			return errors.New("approvals bucket missing")
		}
		key := approvalKey(sessionID, requestID)
		if b.Get(key) == nil {
			return ErrApprovalNotFound
		}
		return b.Delete(key)
	})
}

func (s *bboltApprovalStore) DeleteSession(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketApprovals)
		if b == nil {
			return errors.New("approvals bucket missing")
		}
		prefix := approvalSessionPrefix(sessionID)
		found := false
		c := b.Cursor()
		for k, _ := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, _ = c.Next() {
			found = true
			if err := c.Delete(); err != nil {
				return err
			}
		}
		if !found {
			return ErrApprovalNotFound
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

type bboltWorkflowTemplateStore struct {
	db *bolt.DB
	mu sync.Mutex
}

func (s *bboltWorkflowTemplateStore) ListWorkflowTemplates(ctx context.Context) ([]guidedworkflows.WorkflowTemplate, error) {
	out := make([]guidedworkflows.WorkflowTemplate, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkflowTemplates)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var template guidedworkflows.WorkflowTemplate
			if err := json.Unmarshal(v, &template); err != nil {
				return err
			}
			out = append(out, cloneWorkflowTemplate(template))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *bboltWorkflowTemplateStore) GetWorkflowTemplate(ctx context.Context, templateID string) (*guidedworkflows.WorkflowTemplate, bool, error) {
	id := strings.TrimSpace(templateID)
	if id == "" {
		return nil, false, nil
	}
	var (
		out *guidedworkflows.WorkflowTemplate
		ok  bool
	)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkflowTemplates)
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(id))
		if len(raw) == 0 {
			return nil
		}
		var template guidedworkflows.WorkflowTemplate
		if err := json.Unmarshal(raw, &template); err != nil {
			return err
		}
		copyTemplate := cloneWorkflowTemplate(template)
		out = &copyTemplate
		ok = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, ok, nil
}

func (s *bboltWorkflowTemplateStore) UpsertWorkflowTemplate(ctx context.Context, template guidedworkflows.WorkflowTemplate) (*guidedworkflows.WorkflowTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized, err := normalizeWorkflowTemplate(template)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkflowTemplates)
		if b == nil {
			return errors.New("workflow templates bucket missing")
		}
		return b.Put([]byte(normalized.ID), raw)
	}); err != nil {
		return nil, err
	}
	out := cloneWorkflowTemplate(normalized)
	return &out, nil
}

func (s *bboltWorkflowTemplateStore) DeleteWorkflowTemplate(ctx context.Context, templateID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(templateID)
	if id == "" {
		return ErrWorkflowTemplateNotFound
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketWorkflowTemplates)
		if b == nil {
			return errors.New("workflow templates bucket missing")
		}
		key := []byte(id)
		if b.Get(key) == nil {
			return ErrWorkflowTemplateNotFound
		}
		return b.Delete(key)
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

func cloneWorkspace(workspace *types.Workspace) *types.Workspace {
	if workspace == nil {
		return nil
	}
	copy := *workspace
	if len(workspace.GroupIDs) > 0 {
		copy.GroupIDs = append([]string(nil), workspace.GroupIDs...)
	}
	return &copy
}

func cloneWorktree(worktree *types.Worktree) *types.Worktree {
	if worktree == nil {
		return nil
	}
	copy := *worktree
	copy.NotificationOverrides = types.CloneNotificationSettingsPatch(worktree.NotificationOverrides)
	return &copy
}

func cloneWorkspaceGroup(group *types.WorkspaceGroup) *types.WorkspaceGroup {
	if group == nil {
		return nil
	}
	copy := *group
	return &copy
}

func cloneApproval(approval *types.Approval) *types.Approval {
	if approval == nil {
		return nil
	}
	copy := *approval
	if len(approval.Params) > 0 {
		copy.Params = append([]byte(nil), approval.Params...)
	}
	return &copy
}

func approvalSessionPrefix(sessionID string) []byte {
	return []byte(sessionID + "\x00")
}

func approvalKey(sessionID string, requestID int) []byte {
	return []byte(sessionID + "\x00" + strconv.Itoa(requestID))
}
