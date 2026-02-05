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

var ErrApprovalNotFound = errors.New("approval not found")

const approvalSchemaVersion = 1

type ApprovalStore interface {
	ListBySession(ctx context.Context, sessionID string) ([]*types.Approval, error)
	Get(ctx context.Context, sessionID string, requestID int) (*types.Approval, bool, error)
	Upsert(ctx context.Context, approval *types.Approval) (*types.Approval, error)
	Delete(ctx context.Context, sessionID string, requestID int) error
	DeleteSession(ctx context.Context, sessionID string) error
}

type FileApprovalStore struct {
	path string
	mu   sync.Mutex
}

type approvalsFile struct {
	Version   int               `json:"version"`
	Approvals []*types.Approval `json:"approvals"`
}

func NewFileApprovalStore(path string) *FileApprovalStore {
	return &FileApprovalStore{path: path}
}

func (s *FileApprovalStore) ListBySession(ctx context.Context, sessionID string) ([]*types.Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrApprovalNotFound) {
			return []*types.Approval{}, nil
		}
		return nil, err
	}
	out := make([]*types.Approval, 0)
	for _, approval := range file.Approvals {
		if approval == nil || approval.SessionID != sessionID {
			continue
		}
		copy := *approval
		out = append(out, &copy)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *FileApprovalStore) Get(ctx context.Context, sessionID string, requestID int) (*types.Approval, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		if errors.Is(err, ErrApprovalNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	for _, approval := range file.Approvals {
		if approval == nil {
			continue
		}
		if approval.SessionID == sessionID && approval.RequestID == requestID {
			copy := *approval
			return &copy, true, nil
		}
	}
	return nil, false, nil
}

func (s *FileApprovalStore) Upsert(ctx context.Context, approval *types.Approval) (*types.Approval, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if approval == nil || approval.SessionID == "" || approval.RequestID < 0 {
		return nil, errors.New("approval requires session_id and request_id")
	}

	file, err := s.load()
	if err != nil && !errors.Is(err, ErrApprovalNotFound) {
		return nil, err
	}
	if file == nil {
		file = newApprovalsFile()
	}

	normalized := normalizeApproval(approval, nil)
	updated := false
	for i, existing := range file.Approvals {
		if existing == nil {
			continue
		}
		if existing.SessionID == approval.SessionID && existing.RequestID == approval.RequestID {
			file.Approvals[i] = normalizeApproval(approval, existing)
			updated = true
			break
		}
	}
	if !updated {
		file.Approvals = append(file.Approvals, normalized)
	}

	if err := s.save(file); err != nil {
		return nil, err
	}
	copy := *normalized
	return &copy, nil
}

func (s *FileApprovalStore) Delete(ctx context.Context, sessionID string, requestID int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return err
	}
	filtered := file.Approvals[:0]
	found := false
	for _, approval := range file.Approvals {
		if approval == nil {
			continue
		}
		if approval.SessionID == sessionID && approval.RequestID == requestID {
			found = true
			continue
		}
		filtered = append(filtered, approval)
	}
	file.Approvals = filtered
	if !found {
		return ErrApprovalNotFound
	}
	return s.save(file)
}

func (s *FileApprovalStore) DeleteSession(ctx context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	file, err := s.load()
	if err != nil {
		return err
	}
	filtered := file.Approvals[:0]
	found := false
	for _, approval := range file.Approvals {
		if approval == nil {
			continue
		}
		if approval.SessionID == sessionID {
			found = true
			continue
		}
		filtered = append(filtered, approval)
	}
	file.Approvals = filtered
	if !found {
		return ErrApprovalNotFound
	}
	return s.save(file)
}

func (s *FileApprovalStore) load() (*approvalsFile, error) {
	file := newApprovalsFile()
	err := readJSON(s.path, file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrApprovalNotFound
		}
		return nil, err
	}
	if file.Version == 0 {
		file.Version = approvalSchemaVersion
	}
	return file, nil
}

func (s *FileApprovalStore) save(file *approvalsFile) error {
	file.Version = approvalSchemaVersion
	return writeJSONAtomic(s.path, file)
}

func newApprovalsFile() *approvalsFile {
	return &approvalsFile{
		Version:   approvalSchemaVersion,
		Approvals: []*types.Approval{},
	}
}

func normalizeApproval(approval *types.Approval, existing *types.Approval) *types.Approval {
	normalized := *approval
	if existing != nil && normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = existing.CreatedAt
	}
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = time.Now().UTC()
	}
	return &normalized
}
