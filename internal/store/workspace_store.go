package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"control/internal/types"
	"control/internal/workspacepaths"
)

var ErrWorkspaceNotFound = errors.New("workspace not found")
var ErrWorktreeNotFound = errors.New("worktree not found")
var ErrWorkspaceGroupNotFound = errors.New("workspace group not found")

const workspaceSchemaVersion = 1

type WorkspaceStore interface {
	List(ctx context.Context) ([]*types.Workspace, error)
	Get(ctx context.Context, id string) (*types.Workspace, bool, error)
	Add(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error)
	Update(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error)
	Delete(ctx context.Context, id string) error
}

type WorktreeStore interface {
	ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error)
	AddWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error)
	UpdateWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error)
	DeleteWorktree(ctx context.Context, workspaceID, worktreeID string) error
}

type WorkspaceGroupStore interface {
	ListGroups(ctx context.Context) ([]*types.WorkspaceGroup, error)
	GetGroup(ctx context.Context, id string) (*types.WorkspaceGroup, bool, error)
	AddGroup(ctx context.Context, group *types.WorkspaceGroup) (*types.WorkspaceGroup, error)
	UpdateGroup(ctx context.Context, group *types.WorkspaceGroup) (*types.WorkspaceGroup, error)
	DeleteGroup(ctx context.Context, id string) error
}

type FileWorkspaceStore struct {
	path string
	mu   sync.Mutex
}

type workspaceFile struct {
	Version    int                     `json:"version"`
	Workspaces []*types.Workspace      `json:"workspaces"`
	Worktrees  []*types.Worktree       `json:"worktrees"`
	Groups     []*types.WorkspaceGroup `json:"groups"`
}

func NewFileWorkspaceStore(path string) *FileWorkspaceStore {
	return &FileWorkspaceStore{path: path}
}

func (s *FileWorkspaceStore) List(ctx context.Context) ([]*types.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrWorkspaceNotFound) {
			return []*types.Workspace{}, nil
		}
		return nil, err
	}
	out := make([]*types.Workspace, 0, len(file.Workspaces))
	for _, ws := range file.Workspaces {
		out = append(out, cloneWorkspaceRecord(ws))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *FileWorkspaceStore) Get(ctx context.Context, id string) (*types.Workspace, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrWorkspaceNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	for _, ws := range file.Workspaces {
		if ws.ID == id {
			return cloneWorkspaceRecord(ws), true, nil
		}
	}
	return nil, false, nil
}

func (s *FileWorkspaceStore) Add(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil && !errors.Is(err, ErrWorkspaceNotFound) {
		return nil, err
	}
	if file == nil {
		file = newWorkspaceFile()
	}

	ws, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}

	for _, existing := range file.Workspaces {
		if existing.ID == ws.ID {
			return nil, errors.New("workspace already exists")
		}
	}

	file.Workspaces = append(file.Workspaces, ws)
	if err := s.save(file); err != nil {
		return nil, err
	}
	return cloneWorkspaceRecord(ws), nil
}

func (s *FileWorkspaceStore) Update(ctx context.Context, workspace *types.Workspace) (*types.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return nil, err
	}
	ws, err := normalizeWorkspace(workspace)
	if err != nil {
		return nil, err
	}

	updated := false
	for i, existing := range file.Workspaces {
		if existing.ID == ws.ID {
			ws.CreatedAt = existing.CreatedAt
			ws.UpdatedAt = time.Now().UTC()
			file.Workspaces[i] = ws
			updated = true
			break
		}
	}
	if !updated {
		return nil, ErrWorkspaceNotFound
	}
	if err := s.save(file); err != nil {
		return nil, err
	}
	return cloneWorkspaceRecord(ws), nil
}

func (s *FileWorkspaceStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return err
	}
	filtered := file.Workspaces[:0]
	found := false
	for _, ws := range file.Workspaces {
		if ws.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, ws)
	}
	file.Workspaces = filtered

	worktrees := file.Worktrees[:0]
	for _, wt := range file.Worktrees {
		if wt.WorkspaceID == id {
			continue
		}
		worktrees = append(worktrees, wt)
	}
	file.Worktrees = worktrees

	if !found {
		return ErrWorkspaceNotFound
	}
	return s.save(file)
}

func (s *FileWorkspaceStore) ListWorktrees(ctx context.Context, workspaceID string) ([]*types.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrWorkspaceNotFound) {
			return []*types.Worktree{}, nil
		}
		return nil, err
	}
	out := make([]*types.Worktree, 0)
	for _, wt := range file.Worktrees {
		if wt.WorkspaceID == workspaceID {
			copy := *wt
			out = append(out, &copy)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *FileWorkspaceStore) ListGroups(ctx context.Context) ([]*types.WorkspaceGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrWorkspaceNotFound) {
			return []*types.WorkspaceGroup{}, nil
		}
		return nil, err
	}
	out := make([]*types.WorkspaceGroup, 0, len(file.Groups))
	for _, group := range file.Groups {
		copy := *group
		out = append(out, &copy)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *FileWorkspaceStore) GetGroup(ctx context.Context, id string) (*types.WorkspaceGroup, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrWorkspaceNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	for _, group := range file.Groups {
		if group.ID == id {
			copy := *group
			return &copy, true, nil
		}
	}
	return nil, false, nil
}

func (s *FileWorkspaceStore) AddGroup(ctx context.Context, group *types.WorkspaceGroup) (*types.WorkspaceGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil && !errors.Is(err, ErrWorkspaceNotFound) {
		return nil, err
	}
	if file == nil {
		file = newWorkspaceFile()
	}
	normalized, err := normalizeWorkspaceGroup(group)
	if err != nil {
		return nil, err
	}
	for _, existing := range file.Groups {
		if existing.ID == normalized.ID {
			return nil, errors.New("workspace group already exists")
		}
		if strings.EqualFold(existing.Name, normalized.Name) {
			return nil, errors.New("workspace group name already exists")
		}
	}
	file.Groups = append(file.Groups, normalized)
	if err := s.save(file); err != nil {
		return nil, err
	}
	copy := *normalized
	return &copy, nil
}

func (s *FileWorkspaceStore) UpdateGroup(ctx context.Context, group *types.WorkspaceGroup) (*types.WorkspaceGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeWorkspaceGroup(group)
	if err != nil {
		return nil, err
	}
	updated := false
	for i, existing := range file.Groups {
		if existing.ID == normalized.ID {
			normalized.CreatedAt = existing.CreatedAt
			normalized.UpdatedAt = time.Now().UTC()
			file.Groups[i] = normalized
			updated = true
			break
		}
	}
	if !updated {
		return nil, ErrWorkspaceGroupNotFound
	}
	if err := s.save(file); err != nil {
		return nil, err
	}
	copy := *normalized
	return &copy, nil
}

func (s *FileWorkspaceStore) DeleteGroup(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return err
	}
	filtered := file.Groups[:0]
	found := false
	for _, group := range file.Groups {
		if group.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, group)
	}
	file.Groups = filtered
	if !found {
		return ErrWorkspaceGroupNotFound
	}
	return s.save(file)
}

func (s *FileWorkspaceStore) AddWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrWorkspaceNotFound) {
			return nil, ErrWorkspaceNotFound
		}
		return nil, err
	}
	if !workspaceExists(file.Workspaces, workspaceID) {
		return nil, ErrWorkspaceNotFound
	}

	wt, err := normalizeWorktree(workspaceID, worktree)
	if err != nil {
		return nil, err
	}
	for _, existing := range file.Worktrees {
		if existing.ID == wt.ID {
			return nil, errors.New("worktree already exists")
		}
		if existing.WorkspaceID == workspaceID && strings.EqualFold(existing.Path, wt.Path) {
			return nil, errors.New("worktree path already added")
		}
	}
	file.Worktrees = append(file.Worktrees, wt)
	if err := s.save(file); err != nil {
		return nil, err
	}
	copy := *wt
	return &copy, nil
}

func (s *FileWorkspaceStore) UpdateWorktree(ctx context.Context, workspaceID string, worktree *types.Worktree) (*types.Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return nil, err
	}
	if !workspaceExists(file.Workspaces, workspaceID) {
		return nil, ErrWorkspaceNotFound
	}

	wt, err := normalizeWorktree(workspaceID, worktree)
	if err != nil {
		return nil, err
	}
	for _, existing := range file.Worktrees {
		if existing.ID == wt.ID {
			continue
		}
		if existing.WorkspaceID == workspaceID && strings.EqualFold(existing.Path, wt.Path) {
			return nil, errors.New("worktree path already added")
		}
	}
	updated := false
	for i, existing := range file.Worktrees {
		if existing.ID == wt.ID && existing.WorkspaceID == workspaceID {
			wt.CreatedAt = existing.CreatedAt
			wt.UpdatedAt = time.Now().UTC()
			file.Worktrees[i] = wt
			updated = true
			break
		}
	}
	if !updated {
		return nil, ErrWorktreeNotFound
	}
	if err := s.save(file); err != nil {
		return nil, err
	}
	copy := *wt
	return &copy, nil
}

func (s *FileWorkspaceStore) DeleteWorktree(ctx context.Context, workspaceID, worktreeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return err
	}
	filtered := file.Worktrees[:0]
	found := false
	for _, wt := range file.Worktrees {
		if wt.ID == worktreeID && wt.WorkspaceID == workspaceID {
			found = true
			continue
		}
		filtered = append(filtered, wt)
	}
	file.Worktrees = filtered
	if !found {
		return ErrWorktreeNotFound
	}
	return s.save(file)
}

func (s *FileWorkspaceStore) load() (*workspaceFile, error) {
	file := newWorkspaceFile()
	err := readJSON(s.path, file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrWorkspaceNotFound
		}
		return nil, err
	}
	if file.Version == 0 {
		file.Version = workspaceSchemaVersion
	}
	return file, nil
}

func (s *FileWorkspaceStore) save(file *workspaceFile) error {
	file.Version = workspaceSchemaVersion
	return writeJSONAtomic(s.path, file)
}

func newWorkspaceFile() *workspaceFile {
	return &workspaceFile{
		Version:    workspaceSchemaVersion,
		Workspaces: []*types.Workspace{},
		Worktrees:  []*types.Worktree{},
		Groups:     []*types.WorkspaceGroup{},
	}
}

func normalizeWorkspace(workspace *types.Workspace) (*types.Workspace, error) {
	if workspace == nil {
		return nil, errors.New("workspace is required")
	}
	if strings.TrimSpace(workspace.RepoPath) == "" {
		return nil, errors.New("workspace path is required")
	}
	path, err := normalizePath(workspace.RepoPath)
	if err != nil {
		return nil, err
	}
	sessionSubpath, err := workspacepaths.NormalizeSubpath(workspace.SessionSubpath)
	if err != nil {
		return nil, err
	}
	additionalDirectories, err := workspacepaths.NormalizeAdditionalDirectories(workspace.AdditionalDirectories)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(workspace.Name)
	if name == "" {
		name = defaultName(path)
	}
	ws := &types.Workspace{
		ID:                    workspace.ID,
		Name:                  name,
		RepoPath:              path,
		SessionSubpath:        sessionSubpath,
		AdditionalDirectories: additionalDirectories,
		GroupIDs:              normalizeGroupIDs(workspace.GroupIDs),
		CreatedAt:             workspace.CreatedAt,
		UpdatedAt:             workspace.UpdatedAt,
	}
	if ws.ID == "" {
		id, err := newID()
		if err != nil {
			return nil, err
		}
		ws.ID = id
	}
	if ws.CreatedAt.IsZero() {
		ws.CreatedAt = time.Now().UTC()
	}
	if ws.UpdatedAt.IsZero() {
		ws.UpdatedAt = ws.CreatedAt
	}
	return ws, nil
}

func normalizeGroupIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" || trimmed == "ungrouped" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneWorkspaceRecord(workspace *types.Workspace) *types.Workspace {
	if workspace == nil {
		return nil
	}
	copy := *workspace
	if len(workspace.AdditionalDirectories) > 0 {
		copy.AdditionalDirectories = append([]string(nil), workspace.AdditionalDirectories...)
	}
	if len(workspace.GroupIDs) > 0 {
		copy.GroupIDs = append([]string(nil), workspace.GroupIDs...)
	}
	return &copy
}

func normalizeWorkspaceGroup(group *types.WorkspaceGroup) (*types.WorkspaceGroup, error) {
	if group == nil {
		return nil, errors.New("workspace group is required")
	}
	name := strings.TrimSpace(group.Name)
	if name == "" {
		return nil, errors.New("workspace group name is required")
	}
	out := &types.WorkspaceGroup{
		ID:        group.ID,
		Name:      name,
		CreatedAt: group.CreatedAt,
		UpdatedAt: group.UpdatedAt,
	}
	if out.ID == "" {
		id, err := newID()
		if err != nil {
			return nil, err
		}
		out.ID = id
	}
	if out.CreatedAt.IsZero() {
		out.CreatedAt = time.Now().UTC()
	}
	if out.UpdatedAt.IsZero() {
		out.UpdatedAt = out.CreatedAt
	}
	return out, nil
}

func normalizeWorktree(workspaceID string, worktree *types.Worktree) (*types.Worktree, error) {
	if worktree == nil {
		return nil, errors.New("worktree is required")
	}
	if strings.TrimSpace(worktree.Path) == "" {
		return nil, errors.New("worktree path is required")
	}
	path, err := normalizePath(worktree.Path)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(worktree.Name)
	if name == "" {
		name = defaultName(path)
	}
	wt := &types.Worktree{
		ID:                    worktree.ID,
		WorkspaceID:           workspaceID,
		Name:                  name,
		Path:                  path,
		NotificationOverrides: types.CloneNotificationSettingsPatch(worktree.NotificationOverrides),
		CreatedAt:             worktree.CreatedAt,
		UpdatedAt:             worktree.UpdatedAt,
	}
	if wt.ID == "" {
		id, err := newID()
		if err != nil {
			return nil, err
		}
		wt.ID = id
	}
	if wt.CreatedAt.IsZero() {
		wt.CreatedAt = time.Now().UTC()
	}
	if wt.UpdatedAt.IsZero() {
		wt.UpdatedAt = wt.CreatedAt
	}
	return wt, nil
}

func workspaceExists(workspaces []*types.Workspace, id string) bool {
	for _, ws := range workspaces {
		if ws.ID == id {
			return true
		}
	}
	return false
}

func normalizePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func defaultName(path string) string {
	base := filepath.Base(filepath.Clean(path))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return path
	}
	return base
}

func newID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
