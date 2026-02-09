package daemon

import (
	"context"
	"errors"
	"strings"
	"time"

	"control/internal/logging"
	"control/internal/providers"
	"control/internal/store"
	"control/internal/types"
)

type SessionService struct {
	manager  *SessionManager
	stores   *Stores
	live     *CodexLiveManager
	logger   logging.Logger
	adapters *conversationAdapterRegistry
}

func NewSessionService(manager *SessionManager, stores *Stores, live *CodexLiveManager, logger logging.Logger) *SessionService {
	if logger == nil {
		logger = logging.Nop()
	}
	return &SessionService{
		manager:  manager,
		stores:   stores,
		live:     live,
		logger:   logger,
		adapters: newConversationAdapterRegistry(),
	}
}

func (s *SessionService) List(ctx context.Context) ([]*types.Session, error) {
	sessions, _, err := s.ListWithMeta(ctx)
	return sessions, err
}

func (s *SessionService) ListWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	sessionMap := make(map[string]*types.Session)
	sourceByID := make(map[string]string)
	liveIDs := make(map[string]struct{})
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
			sourceByID[record.Session.ID] = strings.TrimSpace(record.Source)
		}
	}
	if s.manager != nil {
		live := s.manager.ListSessions()
		for _, session := range live {
			if session == nil {
				continue
			}
			sessionMap[session.ID] = session
			sourceByID[session.ID] = sessionSourceInternal
			liveIDs[session.ID] = struct{}{}
			if s.stores != nil && s.stores.Sessions != nil {
				_, _ = s.stores.Sessions.UpsertRecord(ctx, &types.SessionRecord{
					Session: session,
					Source:  sessionSourceInternal,
				})
			}
		}
	}
	var meta []*types.SessionMeta
	metaBySessionID := map[string]*types.SessionMeta{}
	if s.stores == nil || s.stores.SessionMeta == nil {
		meta = nil
	} else {
		var err error
		meta, err = s.stores.SessionMeta.List(ctx)
		if err != nil {
			return nil, nil, unavailableError(err.Error(), err)
		}
		for _, entry := range meta {
			if entry == nil || strings.TrimSpace(entry.SessionID) == "" {
				continue
			}
			metaBySessionID[entry.SessionID] = entry
		}
	}

	s.normalizeSessionStatuses(ctx, sessionMap, sourceByID, liveIDs)
	sessions := dedupeSessionsForList(sessionMap, sourceByID, liveIDs, metaBySessionID)
	sortSessionsByCreatedAt(sessions)
	return sessions, meta, nil
}

func (s *SessionService) normalizeSessionStatuses(ctx context.Context, sessionMap map[string]*types.Session, sourceByID map[string]string, liveIDs map[string]struct{}) {
	if len(sessionMap) == 0 {
		return
	}
	for id, session := range sessionMap {
		if session == nil {
			continue
		}
		if !isActiveStatus(session.Status) {
			continue
		}
		_, isLive := liveIDs[id]
		noProcess := providers.CapabilitiesFor(session.Provider).NoProcess
		if !noProcess && isLive {
			continue
		}
		copy := *session
		copy.Status = types.SessionStatusInactive
		copy.PID = 0
		copy.ExitedAt = nil
		sessionMap[id] = &copy
		if s.stores != nil && s.stores.Sessions != nil {
			source := strings.TrimSpace(sourceByID[id])
			if source == "" {
				source = sessionSourceInternal
			}
			_, _ = s.stores.Sessions.UpsertRecord(ctx, &types.SessionRecord{
				Session: &copy,
				Source:  source,
			})
		}
	}
}

func dedupeSessionsForList(sessionMap map[string]*types.Session, sourceByID map[string]string, liveIDs map[string]struct{}, metaBySessionID map[string]*types.SessionMeta) []*types.Session {
	chosenByKey := map[string]*types.Session{}
	chosenIDByKey := map[string]string{}
	for id, session := range sessionMap {
		if session == nil || !isListableStatus(session.Status) {
			continue
		}
		meta := metaBySessionID[id]
		key := listDedupKey(session, meta)
		current, ok := chosenByKey[key]
		if !ok {
			chosenByKey[key] = session
			chosenIDByKey[key] = id
			continue
		}
		currentID := chosenIDByKey[key]
		if preferListSession(id, session, currentID, current, sourceByID, liveIDs, metaBySessionID) {
			chosenByKey[key] = session
			chosenIDByKey[key] = id
		}
	}
	out := make([]*types.Session, 0, len(chosenByKey))
	for _, session := range chosenByKey {
		out = append(out, session)
	}
	return out
}

func listDedupKey(session *types.Session, meta *types.SessionMeta) string {
	if session == nil {
		return ""
	}
	if providers.Normalize(session.Provider) != "codex" {
		return "session:" + strings.TrimSpace(session.ID)
	}
	threadID := strings.TrimSpace(resolveThreadID(session, meta))
	if threadID == "" {
		threadID = strings.TrimSpace(session.ID)
	}
	return "codex:" + threadID
}

func preferListSession(candidateID string, candidate *types.Session, existingID string, existing *types.Session, sourceByID map[string]string, liveIDs map[string]struct{}, metaBySessionID map[string]*types.SessionMeta) bool {
	candidateLive := sessionIsLive(candidateID, liveIDs)
	existingLive := sessionIsLive(existingID, liveIDs)
	if candidateLive != existingLive {
		return candidateLive
	}

	candidateSource := listSourcePriority(sourceByID[candidateID])
	existingSource := listSourcePriority(sourceByID[existingID])
	if candidateSource != existingSource {
		return candidateSource > existingSource
	}

	candidateStatus := listStatusPriority(candidate)
	existingStatus := listStatusPriority(existing)
	if candidateStatus != existingStatus {
		return candidateStatus > existingStatus
	}

	candidateMeta := metaBySessionID[candidateID]
	existingMeta := metaBySessionID[existingID]
	candidateLast := listLastActive(candidate, candidateMeta)
	existingLast := listLastActive(existing, existingMeta)
	if candidateLast.After(existingLast) {
		return true
	}
	if existingLast.After(candidateLast) {
		return false
	}

	if candidate.CreatedAt.After(existing.CreatedAt) {
		return true
	}
	if existing.CreatedAt.After(candidate.CreatedAt) {
		return false
	}
	return strings.TrimSpace(candidateID) < strings.TrimSpace(existingID)
}

func sessionIsLive(sessionID string, liveIDs map[string]struct{}) bool {
	if len(liveIDs) == 0 {
		return false
	}
	_, ok := liveIDs[sessionID]
	return ok
}

func listSourcePriority(source string) int {
	switch strings.TrimSpace(source) {
	case sessionSourceInternal:
		return 3
	case "":
		return 2
	case sessionSourceCodex:
		return 1
	default:
		return 2
	}
}

func listStatusPriority(session *types.Session) int {
	if session == nil {
		return 0
	}
	if isActiveStatus(session.Status) {
		return 2
	}
	if session.Status == types.SessionStatusInactive {
		return 1
	}
	return 0
}

func listLastActive(session *types.Session, meta *types.SessionMeta) time.Time {
	if meta != nil && meta.LastActiveAt != nil && !meta.LastActiveAt.IsZero() {
		return meta.LastActiveAt.UTC()
	}
	if session != nil && session.StartedAt != nil && !session.StartedAt.IsZero() {
		return session.StartedAt.UTC()
	}
	if session != nil {
		return session.CreatedAt.UTC()
	}
	return time.Time{}
}

func (s *SessionService) Start(ctx context.Context, req StartSessionRequest) (*types.Session, error) {
	if s.manager == nil {
		return nil, unavailableError("session manager not available", nil)
	}
	if len(req.Args) == 0 && strings.TrimSpace(req.Text) != "" {
		req.Args = []string{strings.TrimSpace(req.Text)}
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
	initialText := strings.TrimSpace(rawInput)
	providerDef, hasProviderDef := providers.Lookup(req.Provider)
	title := sanitizeTitle(req.Title)
	if title == "" && strings.TrimSpace(req.Title) != "" {
		return nil, invalidError("title must contain displayable characters", nil)
	}
	if title == "" && initialInput != "" {
		title = trimTitle(initialInput)
	}
	codexHome := ""
	if hasProviderDef && providerDef.Runtime == providers.RuntimeCodex && cwd != "" {
		codexHome = resolveCodexHome(cwd, workspacePath)
	}

	initialTextForStart := initialText
	if hasProviderDef && providerDef.Runtime == providers.RuntimeClaude {
		initialTextForStart = ""
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
		InitialText:  initialTextForStart,
	})
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
	if hasProviderDef && providerDef.Runtime == providers.RuntimeClaude && initialText != "" && s.manager != nil {
		payload := buildClaudeUserPayload(initialText)
		go func(sessionID string) {
			if err := s.manager.SendInput(sessionID, payload); err != nil && s.logger != nil {
				s.logger.Warn("claude_initial_send_failed",
					logging.F("session_id", sessionID),
					logging.F("error", err),
				)
			}
		}(session.ID)
	}
	return session, nil
}

func (s *SessionService) History(ctx context.Context, id string, lines int) ([]map[string]any, error) {
	if strings.TrimSpace(id) == "" {
		return nil, invalidError("session id is required", nil)
	}
	session, source, err := s.getSessionRecord(ctx, id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, notFoundError("session not found", ErrSessionNotFound)
	}
	meta := s.getSessionMeta(ctx, id)
	return s.conversationAdapter(session.Provider).History(ctx, s, session, meta, source, lines)
}

func (s *SessionService) SendMessage(ctx context.Context, id string, input []map[string]any) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", invalidError("session id is required", nil)
	}
	if len(input) == 0 {
		return "", invalidError("input is required", nil)
	}
	s.logger.Info("send_lookup", logging.F("session_id", id))
	session, _, err := s.getSessionRecord(ctx, id)
	if session == nil {
		s.logger.Warn("send_not_found", logging.F("session_id", id), logging.F("error", err))
		return "", notFoundError("session not found", ErrSessionNotFound)
	}
	meta := s.getSessionMeta(ctx, session.ID)
	s.ensureSessionCwd(ctx, session, meta)
	return s.conversationAdapter(session.Provider).SendMessage(ctx, s, session, meta, input)
}

func (s *SessionService) Approve(ctx context.Context, id string, requestID int, decision string, responses []string, acceptSettings map[string]any) error {
	if strings.TrimSpace(id) == "" {
		return invalidError("session id is required", nil)
	}
	if requestID < 0 {
		return invalidError("request id is required", nil)
	}
	if strings.TrimSpace(decision) == "" {
		return invalidError("decision is required", nil)
	}
	session, _, err := s.getSessionRecord(ctx, id)
	if session == nil {
		if errors.Is(err, ErrSessionNotFound) {
			return notFoundError("session not found", ErrSessionNotFound)
		}
		return invalidError("session not found", ErrSessionNotFound)
	}
	meta := s.getSessionMeta(ctx, session.ID)
	s.ensureSessionCwd(ctx, session, meta)
	return s.conversationAdapter(session.Provider).Approve(ctx, s, session, meta, requestID, decision, responses, acceptSettings)
}

func (s *SessionService) ListApprovals(ctx context.Context, id string) ([]*types.Approval, error) {
	if strings.TrimSpace(id) == "" {
		return nil, invalidError("session id is required", nil)
	}
	if s.stores == nil || s.stores.Approvals == nil {
		return []*types.Approval{}, nil
	}
	approvals, err := s.stores.Approvals.ListBySession(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrApprovalNotFound) {
			return []*types.Approval{}, nil
		}
		return nil, unavailableError(err.Error(), err)
	}
	return approvals, nil
}

func (s *SessionService) InterruptTurn(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidError("session id is required", nil)
	}
	session, _, err := s.getSessionRecord(ctx, id)
	if session == nil {
		if errors.Is(err, ErrSessionNotFound) {
			return notFoundError("session not found", ErrSessionNotFound)
		}
		return invalidError("session not found", ErrSessionNotFound)
	}
	meta := s.getSessionMeta(ctx, session.ID)
	s.ensureSessionCwd(ctx, session, meta)
	return s.conversationAdapter(session.Provider).Interrupt(ctx, s, session, meta)
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
		if !providers.CapabilitiesFor(record.Session.Provider).NoProcess {
			return invalidError("session is active; kill it first", nil)
		}
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
			if providerUsesItems(session.Provider) {
				if items, _, err := s.readSessionItems(session.ID, lines); err == nil && items != nil {
					return items, nil
				}
			}
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
			meta := s.getSessionMeta(ctx, id)
			threadID := resolveThreadID(record.Session, meta)
			return s.tailCodexThread(ctx, record.Session, threadID, lines)
		}
		if ok && record != nil && record.Session != nil {
			if providerUsesItems(record.Session.Provider) {
				if items, _, err := s.readSessionItems(record.Session.ID, lines); err == nil && items != nil {
					return items, nil
				}
			}
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

func (s *SessionService) SubscribeEvents(ctx context.Context, id string) (<-chan types.CodexEvent, func(), error) {
	if strings.TrimSpace(id) == "" {
		return nil, nil, invalidError("session id is required", nil)
	}
	session, _, err := s.getSessionRecord(ctx, id)
	if session == nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil, nil, notFoundError("session not found", err)
		}
		if err != nil {
			return nil, nil, invalidError(err.Error(), err)
		}
		return nil, nil, notFoundError("session not found", ErrSessionNotFound)
	}
	meta := s.getSessionMeta(ctx, session.ID)
	s.ensureSessionCwd(ctx, session, meta)
	return s.conversationAdapter(session.Provider).SubscribeEvents(ctx, s, session, meta)
}

func (s *SessionService) conversationAdapter(provider string) conversationAdapter {
	if s.adapters == nil {
		s.adapters = newConversationAdapterRegistry()
	}
	return s.adapters.adapterFor(provider)
}

func (s *SessionService) ensureSessionCwd(ctx context.Context, session *types.Session, meta *types.SessionMeta) {
	if session == nil || meta == nil || strings.TrimSpace(session.Cwd) != "" {
		return
	}
	if cwd, _, err := s.resolveWorktreePath(ctx, meta.WorkspaceID, meta.WorktreeID); err == nil && strings.TrimSpace(cwd) != "" {
		session.Cwd = cwd
	}
}

func (s *SessionService) SubscribeItems(ctx context.Context, id string) (<-chan map[string]any, func(), error) {
	if s.manager == nil {
		return nil, nil, unavailableError("session manager not available", nil)
	}
	if strings.TrimSpace(id) == "" {
		return nil, nil, invalidError("session id is required", nil)
	}
	session, _, err := s.getSessionRecord(ctx, id)
	if session == nil {
		if errors.Is(err, ErrSessionNotFound) {
			return nil, nil, notFoundError("session not found", err)
		}
		if err != nil {
			return nil, nil, invalidError(err.Error(), err)
		}
		return nil, nil, notFoundError("session not found", ErrSessionNotFound)
	}
	if !providerUsesItems(session.Provider) {
		return nil, nil, invalidError("provider does not support item streaming", nil)
	}
	ch, cancel, err := s.manager.SubscribeItems(id)
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

func (s *SessionService) getSessionRecord(ctx context.Context, id string) (*types.Session, string, error) {
	if s.manager != nil {
		if session, ok := s.manager.GetSession(id); ok && session != nil {
			return session, sessionSourceInternal, nil
		}
	}
	if s.stores != nil && s.stores.Sessions != nil {
		record, ok, err := s.stores.Sessions.GetRecord(ctx, id)
		if err != nil {
			return nil, "", unavailableError(err.Error(), err)
		}
		if ok && record != nil {
			return record.Session, record.Source, nil
		}
	}
	return nil, "", notFoundError("session not found", ErrSessionNotFound)
}

func (s *SessionService) resolveWorkspacePath(ctx context.Context, meta *types.SessionMeta) string {
	if meta == nil || strings.TrimSpace(meta.WorkspaceID) == "" {
		return ""
	}
	if s.stores == nil || s.stores.Workspaces == nil {
		return ""
	}
	if ws, ok, err := s.stores.Workspaces.Get(ctx, meta.WorkspaceID); err == nil && ok && ws != nil {
		return ws.RepoPath
	}
	return ""
}

func (s *SessionService) getSessionMeta(ctx context.Context, sessionID string) *types.SessionMeta {
	if s.stores == nil || s.stores.SessionMeta == nil {
		return nil
	}
	meta, ok, err := s.stores.SessionMeta.Get(ctx, sessionID)
	if err != nil || !ok {
		return nil
	}
	return meta
}

func resolveThreadID(session *types.Session, meta *types.SessionMeta) string {
	if meta != nil && strings.TrimSpace(meta.ThreadID) != "" {
		return meta.ThreadID
	}
	if session != nil && session.Provider == "codex" && session.ID != "" {
		return session.ID
	}
	return ""
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

func extractTextInput(input []map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	parts := make([]string, 0, len(input))
	for _, item := range input {
		if item == nil {
			continue
		}
		typ, _ := item["type"].(string)
		if typ != "" && typ != "text" {
			continue
		}
		text, _ := item["text"].(string)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func isListableStatus(status types.SessionStatus) bool {
	switch status {
	case types.SessionStatusCreated, types.SessionStatusStarting, types.SessionStatusRunning, types.SessionStatusInactive:
		return true
	default:
		return false
	}
}
