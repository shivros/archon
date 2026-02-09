package daemon

import (
	"context"
	"errors"
	"strings"

	"control/internal/store"
	"control/internal/types"
)

type NoteService struct {
	notes      NoteStore
	workspaces WorkspaceStore
	worktrees  WorktreeStore
	sessions   SessionIndexStore
	meta       SessionMetaStore
}

type PinSessionRequest struct {
	Scope         types.NoteScope  `json:"scope,omitempty"`
	WorkspaceID   string           `json:"workspace_id,omitempty"`
	WorktreeID    string           `json:"worktree_id,omitempty"`
	Title         string           `json:"title,omitempty"`
	Body          string           `json:"body,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
	Status        types.NoteStatus `json:"status,omitempty"`
	SourceBlockID string           `json:"source_block_id,omitempty"`
	SourceRole    string           `json:"source_role,omitempty"`
	SourceSnippet string           `json:"source_snippet,omitempty"`
}

func NewNoteService(stores *Stores) *NoteService {
	if stores == nil {
		return &NoteService{}
	}
	return &NoteService{
		notes:      stores.Notes,
		workspaces: stores.Workspaces,
		worktrees:  stores.Worktrees,
		sessions:   stores.Sessions,
		meta:       stores.SessionMeta,
	}
}

func (s *NoteService) List(ctx context.Context, filter store.NoteFilter) ([]*types.Note, error) {
	if s.notes == nil {
		return nil, unavailableError("note store not available", nil)
	}
	if filter.Scope != "" && !isValidNoteScope(filter.Scope) {
		return nil, invalidError("invalid scope", nil)
	}
	return s.notes.List(ctx, filter)
}

func (s *NoteService) Create(ctx context.Context, note *types.Note) (*types.Note, error) {
	if s.notes == nil {
		return nil, unavailableError("note store not available", nil)
	}
	if note == nil {
		return nil, invalidError("note payload is required", nil)
	}
	normalized, err := s.normalizeAndValidate(ctx, note)
	if err != nil {
		return nil, err
	}
	created, err := s.notes.Upsert(ctx, normalized)
	if err != nil {
		return nil, unavailableError(err.Error(), err)
	}
	return created, nil
}

func (s *NoteService) Update(ctx context.Context, id string, patch *types.Note) (*types.Note, error) {
	if s.notes == nil {
		return nil, unavailableError("note store not available", nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, invalidError("note id is required", nil)
	}
	if patch == nil {
		return nil, invalidError("note payload is required", nil)
	}

	existing, ok, err := s.notes.Get(ctx, id)
	if err != nil {
		return nil, unavailableError(err.Error(), err)
	}
	if !ok || existing == nil {
		return nil, notFoundError("note not found", store.ErrNoteNotFound)
	}

	merged := *existing
	if patch.Kind != "" {
		merged.Kind = patch.Kind
	}
	if patch.Scope != "" {
		if patch.Scope != merged.Scope {
			switch patch.Scope {
			case types.NoteScopeWorkspace:
				merged.WorktreeID = ""
				merged.SessionID = ""
			case types.NoteScopeWorktree:
				merged.SessionID = ""
			}
		}
		merged.Scope = patch.Scope
	}
	if strings.TrimSpace(patch.WorkspaceID) != "" {
		merged.WorkspaceID = strings.TrimSpace(patch.WorkspaceID)
	}
	if strings.TrimSpace(patch.WorktreeID) != "" {
		merged.WorktreeID = strings.TrimSpace(patch.WorktreeID)
	}
	if strings.TrimSpace(patch.SessionID) != "" {
		merged.SessionID = strings.TrimSpace(patch.SessionID)
	}
	if strings.TrimSpace(patch.Title) != "" {
		merged.Title = strings.TrimSpace(patch.Title)
	}
	if strings.TrimSpace(patch.Body) != "" {
		merged.Body = strings.TrimSpace(patch.Body)
	}
	if patch.Tags != nil {
		merged.Tags = append([]string(nil), patch.Tags...)
	}
	if patch.Status != "" {
		merged.Status = patch.Status
	}
	if patch.Source != nil {
		source := *patch.Source
		merged.Source = &source
	}

	normalized, validateErr := s.normalizeAndValidate(ctx, &merged)
	if validateErr != nil {
		return nil, validateErr
	}
	normalized.ID = id
	updated, upsertErr := s.notes.Upsert(ctx, normalized)
	if upsertErr != nil {
		return nil, unavailableError(upsertErr.Error(), upsertErr)
	}
	return updated, nil
}

func (s *NoteService) Delete(ctx context.Context, id string) error {
	if s.notes == nil {
		return unavailableError("note store not available", nil)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return invalidError("note id is required", nil)
	}
	if err := s.notes.Delete(ctx, id); err != nil {
		if errors.Is(err, store.ErrNoteNotFound) {
			return notFoundError("note not found", err)
		}
		return unavailableError(err.Error(), err)
	}
	return nil
}

func (s *NoteService) PinSession(ctx context.Context, sessionID string, req *PinSessionRequest) (*types.Note, error) {
	if req == nil {
		req = &PinSessionRequest{}
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, invalidError("session id is required", nil)
	}
	if err := s.ensureSessionExists(ctx, sessionID); err != nil {
		return nil, err
	}
	meta, _, _ := s.lookupSessionMeta(ctx, sessionID)

	scope := req.Scope
	if scope == "" {
		scope = types.NoteScopeSession
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	worktreeID := strings.TrimSpace(req.WorktreeID)
	if meta != nil {
		if workspaceID == "" {
			workspaceID = meta.WorkspaceID
		}
		if worktreeID == "" {
			worktreeID = meta.WorktreeID
		}
	}
	noteSessionID := sessionID
	if scope != types.NoteScopeSession {
		noteSessionID = ""
	}
	note := &types.Note{
		Kind:        types.NoteKindPin,
		Scope:       scope,
		WorkspaceID: workspaceID,
		WorktreeID:  worktreeID,
		SessionID:   noteSessionID,
		Title:       strings.TrimSpace(req.Title),
		Body:        strings.TrimSpace(req.Body),
		Tags:        append([]string(nil), req.Tags...),
		Status:      req.Status,
		Source: &types.NoteSource{
			SessionID: sessionID,
			BlockID:   strings.TrimSpace(req.SourceBlockID),
			Role:      strings.TrimSpace(req.SourceRole),
			Snippet:   strings.TrimSpace(req.SourceSnippet),
		},
	}

	normalized, err := s.normalizeAndValidate(ctx, note)
	if err != nil {
		return nil, err
	}
	created, err := s.notes.Upsert(ctx, normalized)
	if err != nil {
		return nil, unavailableError(err.Error(), err)
	}
	return created, nil
}

func (s *NoteService) normalizeAndValidate(ctx context.Context, note *types.Note) (*types.Note, error) {
	normalized := *note
	normalized.Kind = types.NoteKind(strings.ToLower(strings.TrimSpace(string(normalized.Kind))))
	normalized.Scope = types.NoteScope(strings.ToLower(strings.TrimSpace(string(normalized.Scope))))
	normalized.Status = types.NoteStatus(strings.ToLower(strings.TrimSpace(string(normalized.Status))))
	normalized.WorkspaceID = strings.TrimSpace(normalized.WorkspaceID)
	normalized.WorktreeID = strings.TrimSpace(normalized.WorktreeID)
	normalized.SessionID = strings.TrimSpace(normalized.SessionID)
	normalized.Title = strings.TrimSpace(normalized.Title)
	normalized.Body = strings.TrimSpace(normalized.Body)
	normalized.Tags = normalizeTags(normalized.Tags)
	if normalized.Source != nil {
		source := *normalized.Source
		source.SessionID = strings.TrimSpace(source.SessionID)
		source.BlockID = strings.TrimSpace(source.BlockID)
		source.Role = strings.TrimSpace(source.Role)
		source.Snippet = strings.TrimSpace(source.Snippet)
		normalized.Source = &source
	}

	if normalized.Kind == "" {
		normalized.Kind = types.NoteKindNote
	}
	if !isValidNoteKind(normalized.Kind) {
		return nil, invalidError("invalid note kind", nil)
	}
	if normalized.Scope == "" {
		return nil, invalidError("scope is required", nil)
	}
	if !isValidNoteScope(normalized.Scope) {
		return nil, invalidError("invalid scope", nil)
	}
	if normalized.Status != "" && !isValidNoteStatus(normalized.Status) {
		return nil, invalidError("invalid status", nil)
	}
	if normalized.Kind == types.NoteKindPin {
		if normalized.Source == nil || normalized.Source.SessionID == "" || normalized.Source.Snippet == "" {
			return nil, invalidError("pin notes require source session and snippet", nil)
		}
	}
	if err := validateScopeIdentifiers(&normalized); err != nil {
		return nil, invalidError(err.Error(), err)
	}
	if err := s.validateReferenceIDs(ctx, &normalized); err != nil {
		return nil, err
	}
	return &normalized, nil
}

func validateScopeIdentifiers(note *types.Note) error {
	switch note.Scope {
	case types.NoteScopeWorkspace:
		if note.WorkspaceID == "" {
			return errors.New("workspace_id is required for workspace notes")
		}
		if note.WorktreeID != "" || note.SessionID != "" {
			return errors.New("workspace notes cannot include worktree_id or session_id")
		}
	case types.NoteScopeWorktree:
		if note.WorkspaceID == "" || note.WorktreeID == "" {
			return errors.New("workspace_id and worktree_id are required for worktree notes")
		}
		if note.SessionID != "" {
			return errors.New("worktree notes cannot include session_id")
		}
	case types.NoteScopeSession:
		if note.SessionID == "" {
			return errors.New("session_id is required for session notes")
		}
	default:
		return errors.New("invalid scope")
	}
	return nil
}

func (s *NoteService) validateReferenceIDs(ctx context.Context, note *types.Note) error {
	if note.WorkspaceID != "" {
		if s.workspaces == nil {
			return unavailableError("workspace store not available", nil)
		}
		if _, ok, err := s.workspaces.Get(ctx, note.WorkspaceID); err != nil {
			return unavailableError(err.Error(), err)
		} else if !ok {
			return notFoundError("workspace not found", store.ErrWorkspaceNotFound)
		}
	}

	if note.WorktreeID != "" {
		if s.worktrees == nil {
			return unavailableError("worktree store not available", nil)
		}
		worktrees, err := s.worktrees.ListWorktrees(ctx, note.WorkspaceID)
		if err != nil {
			if errors.Is(err, store.ErrWorkspaceNotFound) {
				return notFoundError("workspace not found", err)
			}
			return unavailableError(err.Error(), err)
		}
		found := false
		for _, wt := range worktrees {
			if wt != nil && wt.ID == note.WorktreeID {
				found = true
				break
			}
		}
		if !found {
			return notFoundError("worktree not found", store.ErrWorktreeNotFound)
		}
	}

	if note.SessionID != "" {
		if err := s.ensureSessionExists(ctx, note.SessionID); err != nil {
			return err
		}
	}

	if note.Source != nil && note.Source.SessionID != "" {
		if err := s.ensureSessionExists(ctx, note.Source.SessionID); err != nil {
			return err
		}
	}
	return nil
}

func (s *NoteService) ensureSessionExists(ctx context.Context, sessionID string) error {
	if s.sessions == nil {
		return unavailableError("session store not available", nil)
	}
	if _, ok, err := s.sessions.GetRecord(ctx, sessionID); err != nil {
		return unavailableError(err.Error(), err)
	} else if !ok {
		return notFoundError("session not found", errors.New("session not found"))
	}
	return nil
}

func (s *NoteService) lookupSessionMeta(ctx context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	if s.meta == nil {
		return nil, false, nil
	}
	meta, ok, err := s.meta.Get(ctx, sessionID)
	if err != nil {
		return nil, false, unavailableError(err.Error(), err)
	}
	return meta, ok, nil
}

func normalizeTags(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func isValidNoteKind(kind types.NoteKind) bool {
	switch kind {
	case types.NoteKindNote, types.NoteKindPin:
		return true
	default:
		return false
	}
}

func isValidNoteScope(scope types.NoteScope) bool {
	switch scope {
	case types.NoteScopeWorkspace, types.NoteScopeWorktree, types.NoteScopeSession:
		return true
	default:
		return false
	}
}

func isValidNoteStatus(status types.NoteStatus) bool {
	switch status {
	case types.NoteStatusIdea, types.NoteStatusTodo, types.NoteStatusDecision:
		return true
	default:
		return false
	}
}
