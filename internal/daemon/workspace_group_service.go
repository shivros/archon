package daemon

import (
	"context"
	"strings"

	"control/internal/store"
	"control/internal/types"
)

type WorkspaceGroupService struct {
	groups WorkspaceGroupStore
}

func NewWorkspaceGroupService(stores *Stores) *WorkspaceGroupService {
	if stores == nil {
		return &WorkspaceGroupService{}
	}
	return &WorkspaceGroupService{groups: stores.Groups}
}

func (s *WorkspaceGroupService) List(ctx context.Context) ([]*types.WorkspaceGroup, error) {
	if s.groups == nil {
		return nil, unavailableError("workspace group store not available", nil)
	}
	return s.groups.ListGroups(ctx)
}

func (s *WorkspaceGroupService) Create(ctx context.Context, req *types.WorkspaceGroup) (*types.WorkspaceGroup, error) {
	if s.groups == nil {
		return nil, unavailableError("workspace group store not available", nil)
	}
	if req == nil {
		return nil, invalidError("workspace group payload is required", nil)
	}
	group, err := s.groups.AddGroup(ctx, req)
	if err != nil {
		return nil, invalidError("failed to create workspace group", err)
	}
	return group, nil
}

func (s *WorkspaceGroupService) Update(ctx context.Context, id string, req *types.WorkspaceGroup) (*types.WorkspaceGroup, error) {
	if s.groups == nil {
		return nil, unavailableError("workspace group store not available", nil)
	}
	if strings.TrimSpace(id) == "" {
		return nil, invalidError("workspace group id is required", nil)
	}
	if req == nil {
		return nil, invalidError("workspace group payload is required", nil)
	}
	req.ID = id
	group, err := s.groups.UpdateGroup(ctx, req)
	if err != nil {
		if err == store.ErrWorkspaceGroupNotFound {
			return nil, notFoundError("workspace group not found", err)
		}
		return nil, invalidError("failed to update workspace group", err)
	}
	return group, nil
}

func (s *WorkspaceGroupService) Delete(ctx context.Context, id string) error {
	if s.groups == nil {
		return unavailableError("workspace group store not available", nil)
	}
	if strings.TrimSpace(id) == "" {
		return invalidError("workspace group id is required", nil)
	}
	if err := s.groups.DeleteGroup(ctx, id); err != nil {
		if err == store.ErrWorkspaceGroupNotFound {
			return notFoundError("workspace group not found", err)
		}
		return invalidError("failed to delete workspace group", err)
	}
	return nil
}
