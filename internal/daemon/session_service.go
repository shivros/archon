package daemon

import (
	"context"
	"errors"
	"strings"
	"time"

	"control/internal/store"
	"control/internal/types"
)

type SessionService struct {
	manager *SessionManager
	stores  *Stores
}

func NewSessionService(manager *SessionManager, stores *Stores) *SessionService {
	return &SessionService{manager: manager, stores: stores}
}

func (s *SessionService) List(ctx context.Context) ([]*types.Session, error) {
	sessions, _, err := s.ListWithMeta(ctx)
	return sessions, err
}

func (s *SessionService) ListWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	var sessions []*types.Session
	sessionMap := make(map[string]*types.Session)
	if s.stores != nil && s.stores.Sessions != nil {
		records, err := s.stores.Sessions.ListRecords(ctx)
		if err != nil {
			return nil, nil, unavailableError(err.Error(), err)
		}
		for _, record := range records {
			if record == nil || record.Session == nil {
				continue
			}
			sessionMap[record.Session.ID] = record.Session
		}
	}
	if s.manager != nil {
		live := s.manager.ListSessions()
		for _, session := range live {
			if session == nil {
				continue
			}
			sessionMap[session.ID] = session
			if s.stores != nil && s.stores.Sessions != nil {
				_, _ = s.stores.Sessions.UpsertRecord(ctx, &types.SessionRecord{
					Session: session,
					Source:  sessionSourceInternal,
				})
			}
		}
	}
	for _, session := range sessionMap {
		if session == nil {
			continue
		}
		if !isListableStatus(session.Status) {
			continue
		}
		sessions = append(sessions, session)
	}
	sortSessionsByCreatedAt(sessions)

	if s.stores == nil || s.stores.SessionMeta == nil {
		return sessions, nil, nil
	}
	meta, err := s.stores.SessionMeta.List(ctx)
	if err != nil {
		return sessions, nil, unavailableError(err.Error(), err)
	}
	return sessions, meta, nil
}

func (s *SessionService) Start(ctx context.Context, req StartSessionRequest) (*types.Session, error) {
	if s.manager == nil {
		return nil, unavailableError("session manager not available", nil)
	}
	if strings.TrimSpace(req.Provider) == "" {
		return nil, invalidError("provider is required", nil)
	}

	cwd := strings.TrimSpace(req.Cwd)
	workspacePath := ""
	if cwd == "" && s.stores != nil {
		resolved, root, err := s.resolveWorktreePath(ctx, req.WorkspaceID, req.WorktreeID)
		if err != nil {
			return nil, err
		}
		if resolved != "" {
			cwd = resolved
		}
		workspacePath = root
	} else if s.stores != nil && s.stores.Workspaces != nil && strings.TrimSpace(req.WorkspaceID) != "" {
		if ws, ok, err := s.stores.Workspaces.Get(ctx, req.WorkspaceID); err == nil && ok && ws != nil {
			workspacePath = ws.RepoPath
		}
	}

	rawInput := strings.Join(req.Args, " ")
	initialInput := sanitizeTitle(rawInput)
	title := sanitizeTitle(req.Title)
	if title == "" && strings.TrimSpace(req.Title) != "" {
		return nil, invalidError("title must contain displayable characters", nil)
	}
	if title == "" && initialInput != "" {
		title = trimTitle(initialInput)
	}
	codexHome := ""
	if req.Provider == "codex" && cwd != "" {
		codexHome = resolveCodexHome(cwd, workspacePath)
	}

	session, err := s.manager.StartSession(StartSessionConfig{
		Provider:     req.Provider,
		Cmd:          req.Cmd,
		Cwd:          cwd,
		Args:         req.Args,
		Env:          req.Env,
		CodexHome:    codexHome,
		Title:        title,
		Tags:         req.Tags,
		WorkspaceID:  req.WorkspaceID,
		WorktreeID:   req.WorktreeID,
		InitialInput: initialInput,
	})
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
	return session, nil
}

func (s *SessionService) Get(ctx context.Context, id string) (*types.Session, error) {
	if strings.TrimSpace(id) == "" {
		return nil, invalidError("session id is required", nil)
	}
	if s.manager != nil {
		if session, ok := s.manager.GetSession(id); ok {
			return session, nil
		}
	}
	if s.stores != nil && s.stores.Sessions != nil {
		record, ok, err := s.stores.Sessions.GetRecord(ctx, id)
		if err != nil {
			return nil, unavailableError(err.Error(), err)
		}
		if ok && record.Session != nil {
			return record.Session, nil
		}
	}
	return nil, notFoundError("session not found", ErrSessionNotFound)
}

func (s *SessionService) UpdateTitle(ctx context.Context, id, title string) error {
	if s.manager == nil {
		return unavailableError("session manager not available", nil)
	}
	if strings.TrimSpace(id) == "" {
		return invalidError("session id is required", nil)
	}
	title = sanitizeTitle(title)
	if title == "" {
		return invalidError("title must contain displayable characters", nil)
	}
	if err := s.manager.UpdateSessionTitle(id, title); err != nil {
		return invalidError(err.Error(), err)
	}
	return nil
}

func (s *SessionService) MarkExited(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidError("session id is required", nil)
	}
	if s.manager != nil {
		if err := s.manager.MarkExited(id); err == nil {
			return nil
		} else if errors.Is(err, ErrSessionNotFound) {
			// fall through to store lookup
		} else {
			return invalidError(err.Error(), err)
		}
	}
	if s.stores == nil || s.stores.Sessions == nil {
		return notFoundError("session not found", ErrSessionNotFound)
	}
	record, ok, err := s.stores.Sessions.GetRecord(ctx, id)
	if err != nil {
		return unavailableError(err.Error(), err)
	}
	if !ok || record == nil || record.Session == nil {
		return notFoundError("session not found", ErrSessionNotFound)
	}
	if isActiveStatus(record.Session.Status) {
		return invalidError("session is active; kill it first", nil)
	}
	copy := *record.Session
	copy.Status = types.SessionStatusExited
	now := time.Now().UTC()
	copy.ExitedAt = &now
	record.Session = &copy
	if _, err := s.stores.Sessions.UpsertRecord(ctx, record); err != nil {
		return unavailableError(err.Error(), err)
	}
	return nil
}

func (s *SessionService) Kill(ctx context.Context, id string) error {
	if s.manager == nil {
		return unavailableError("session manager not available", nil)
	}
	if strings.TrimSpace(id) == "" {
		return invalidError("session id is required", nil)
	}
	if err := s.manager.KillSession(id); err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return notFoundError("session not found", err)
		}
		return invalidError(err.Error(), err)
	}
	return nil
}

func (s *SessionService) TailItems(ctx context.Context, id string, lines int) ([]map[string]any, error) {
	if strings.TrimSpace(id) == "" {
		return nil, invalidError("session id is required", nil)
	}
	if s.manager != nil {
		if session, ok := s.manager.GetSession(id); ok && session != nil {
			out, _, _, err := s.manager.TailSession(id, "combined", lines)
			if err != nil {
				if errors.Is(err, ErrSessionNotFound) {
					return nil, notFoundError("session not found", err)
				}
				return nil, invalidError(err.Error(), err)
			}
			return logLinesToItems(out), nil
		}
	}
	if s.stores != nil && s.stores.Sessions != nil {
		record, ok, err := s.stores.Sessions.GetRecord(ctx, id)
		if err != nil {
			return nil, unavailableError(err.Error(), err)
		}
		if ok && record != nil && record.Source == sessionSourceCodex {
			return s.tailCodexThread(ctx, record.Session, lines)
		}
		if ok && record != nil && record.Session != nil {
			out, _, _, err := s.readSessionLogs(record.Session.ID, lines)
			if err != nil {
				return nil, invalidError(err.Error(), err)
			}
			return logLinesToItems(out), nil
		}
	}
	return nil, notFoundError("session not found", ErrSessionNotFound)
}

func (s *SessionService) Subscribe(ctx context.Context, id, stream string) (<-chan types.LogEvent, func(), error) {
	if s.manager == nil {
		return nil, nil, unavailableError("session manager not available", nil)
	}
	if strings.TrimSpace(id) == "" {
		return nil, nil, invalidError("session id is required", nil)
	}
	ch, cancel, err := s.manager.Subscribe(id, stream)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil, nil, notFoundError("session not found", err)
		}
		return nil, nil, invalidError(err.Error(), err)
	}
	return ch, cancel, nil
}

func (s *SessionService) resolveWorktreePath(ctx context.Context, workspaceID, worktreeID string) (string, string, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return "", "", nil
	}
	if s.stores == nil || s.stores.Workspaces == nil || s.stores.Worktrees == nil {
		return "", "", unavailableError("workspace store not available", nil)
	}
	ws, ok, err := s.stores.Workspaces.Get(ctx, workspaceID)
	if err != nil {
		return "", "", unavailableError(err.Error(), err)
	}
	if !ok {
		return "", "", notFoundError("workspace not found", store.ErrWorkspaceNotFound)
	}
	if strings.TrimSpace(worktreeID) == "" {
		return ws.RepoPath, ws.RepoPath, nil
	}
	entries, err := s.stores.Worktrees.ListWorktrees(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, store.ErrWorkspaceNotFound) {
			return "", "", notFoundError("workspace not found", err)
		}
		return "", "", invalidError(err.Error(), err)
	}
	for _, wt := range entries {
		if wt.ID == worktreeID {
			return wt.Path, ws.RepoPath, nil
		}
	}
	return "", ws.RepoPath, notFoundError("worktree not found", store.ErrWorktreeNotFound)
}

func trimTitle(input string) string {
	input = sanitizeTitle(input)
	if input == "" {
		return ""
	}
	const maxLen = 80
	if len(input) <= maxLen {
		return input
	}
	return strings.TrimSpace(input[:maxLen]) + "…"
}

func isListableStatus(status types.SessionStatus) bool {
	switch status {
	case types.SessionStatusCreated, types.SessionStatusStarting, types.SessionStatusRunning, types.SessionStatusInactive:
		return true
	default:
		return false
	}
}
