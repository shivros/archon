package daemon

import (
	"context"
	"errors"
	"strings"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/providers"
	"control/internal/store"
	"control/internal/types"
	"control/internal/workspacepaths"
)

type SessionService struct {
	manager      *SessionManager
	stores       *Stores
	liveManager  LiveManager
	logger       logging.Logger
	paths        WorkspacePathResolver
	notifier     NotificationPublisher
	adapters     *conversationAdapterRegistry
	history      *conversationHistoryStrategyRegistry
	codexPool    CodexHistoryPool
	approvalSync *ApprovalResyncService
	guided       guidedworkflows.Orchestrator
}

type SendMessageOptions struct {
	RuntimeOptions       *types.SessionRuntimeOptions
	PersistRuntimeOption bool
}

var ErrRuntimeOptionsPersistFailed = errors.New("runtime options persistence failed")

type SessionServiceOption func(*SessionService)

func WithSessionHistoryStrategies(history *conversationHistoryStrategyRegistry) SessionServiceOption {
	return func(s *SessionService) {
		if s == nil || history == nil {
			return
		}
		s.history = history
	}
}

func WithCodexHistoryPool(pool CodexHistoryPool) SessionServiceOption {
	return func(s *SessionService) {
		if s == nil || pool == nil {
			return
		}
		s.codexPool = pool
	}
}

func WithNotificationPublisher(notifier NotificationPublisher) SessionServiceOption {
	return func(s *SessionService) {
		if s == nil || notifier == nil {
			return
		}
		s.notifier = notifier
	}
}

func WithGuidedWorkflowOrchestrator(orchestrator guidedworkflows.Orchestrator) SessionServiceOption {
	return func(s *SessionService) {
		if s == nil || orchestrator == nil {
			return
		}
		s.guided = orchestrator
	}
}

func WithWorkspacePathResolver(resolver WorkspacePathResolver) SessionServiceOption {
	return func(s *SessionService) {
		if s == nil || resolver == nil {
			return
		}
		s.paths = resolver
	}
}

func WithLiveManager(liveManager LiveManager) SessionServiceOption {
	return func(s *SessionService) {
		if s == nil || liveManager == nil {
			return
		}
		s.liveManager = liveManager
	}
}

func NewSessionService(manager *SessionManager, stores *Stores, logger logging.Logger, opts ...SessionServiceOption) *SessionService {
	if logger == nil {
		logger = logging.Nop()
	}
	if manager != nil {
		manager.SetLogger(logger)
	}
	svc := &SessionService{
		manager:      manager,
		stores:       stores,
		logger:       logger,
		paths:        NewWorkspacePathResolver(),
		adapters:     newConversationAdapterRegistry(),
		history:      newConversationHistoryStrategyRegistry(),
		codexPool:    NewCodexHistoryPool(logger),
		approvalSync: NewApprovalResyncService(stores, logger),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	if svc.history == nil {
		svc.history = newConversationHistoryStrategyRegistry()
	}
	if svc.codexPool == nil {
		svc.codexPool = NewCodexHistoryPool(logger)
	}
	return svc
}

func (s *SessionService) StartGuidedWorkflowRun(ctx context.Context, req guidedworkflows.StartRunRequest) (*guidedworkflows.Run, error) {
	if s == nil || s.guided == nil {
		return nil, guidedworkflows.ErrDisabled
	}
	return s.guided.StartRun(ctx, req)
}

func (s *SessionService) publishTurnCompleted(session *types.Session, meta *types.SessionMeta, turnID string, source string) {
	if s == nil || s.notifier == nil || session == nil {
		return
	}
	event := notificationEventFromSession(session, types.NotificationTriggerTurnCompleted, source)
	if meta != nil {
		event.WorkspaceID = strings.TrimSpace(meta.WorkspaceID)
		event.WorktreeID = strings.TrimSpace(meta.WorktreeID)
	}
	event.TurnID = strings.TrimSpace(turnID)
	s.notifier.Publish(event)
}

func (s *SessionService) List(ctx context.Context) ([]*types.Session, error) {
	sessions, _, err := s.ListWithMeta(ctx)
	return sessions, err
}

func (s *SessionService) ListWithMeta(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return s.listWithMeta(ctx, sessionListOptions{})
}

func (s *SessionService) ListWithMetaIncludingDismissed(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return s.listWithMeta(ctx, sessionListOptions{includeDismissed: true})
}

func (s *SessionService) ListWithMetaIncludingWorkflowOwned(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return s.listWithMeta(ctx, sessionListOptions{includeWorkflowOwned: true})
}

func (s *SessionService) ListWithMetaIncludingDismissedAndWorkflowOwned(ctx context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return s.listWithMeta(ctx, sessionListOptions{includeDismissed: true, includeWorkflowOwned: true})
}

type sessionListOptions struct {
	includeDismissed     bool
	includeWorkflowOwned bool
}

func (s *SessionService) listWithMeta(ctx context.Context, options sessionListOptions) ([]*types.Session, []*types.SessionMeta, error) {
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
	workflowCanonicalByRunID := s.workflowCanonicalSessionsByRunID(ctx, sessionMap, sourceByID, liveIDs, metaBySessionID)
	sessions := dedupeSessionsForList(
		sessionMap,
		sourceByID,
		liveIDs,
		metaBySessionID,
		workflowCanonicalByRunID,
		options.includeDismissed,
		options.includeWorkflowOwned,
	)
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

// migrateCodexDualEntries reconciles old-format codex sessions where an
// internal session (random ID) and a codex-synced session (thread ID) exist
// for the same conversation. It merges them under the thread ID and removes
// the old internal entry.
func (s *SessionService) migrateCodexDualEntries(ctx context.Context) {
	if s.stores == nil || s.stores.Sessions == nil || s.stores.SessionMeta == nil {
		return
	}
	metaEntries, err := s.stores.SessionMeta.List(ctx)
	if err != nil {
		return
	}
	metaByID := make(map[string]*types.SessionMeta, len(metaEntries))
	for _, m := range metaEntries {
		if m != nil {
			metaByID[m.SessionID] = m
		}
	}
	records, err := s.stores.Sessions.ListRecords(ctx)
	if err != nil {
		return
	}

	for _, record := range records {
		if record == nil || record.Session == nil || record.Source != sessionSourceInternal {
			continue
		}
		if providers.Normalize(record.Session.Provider) != "codex" {
			continue
		}
		meta := metaByID[record.Session.ID]
		if meta == nil {
			continue
		}
		threadID := strings.TrimSpace(meta.ThreadID)
		if threadID == "" || threadID == record.Session.ID {
			continue // Already re-keyed or no thread ID.
		}

		// This is an old-format internal codex session whose ID differs
		// from its thread ID. Merge it under the thread ID.
		merged := *record.Session
		merged.ID = threadID

		_, _ = s.stores.Sessions.UpsertRecord(ctx, &types.SessionRecord{
			Session: &merged,
			Source:  sessionSourceInternal,
		})

		// Merge meta, preserving the internal session's title/lock.
		mergedMeta := *meta
		mergedMeta.SessionID = threadID
		if mergedMeta.ThreadID == "" {
			mergedMeta.ThreadID = threadID
		}
		// Carry over workspace/worktree from the codex-synced entry if present.
		if codexMeta := metaByID[threadID]; codexMeta != nil {
			if mergedMeta.WorkspaceID == "" {
				mergedMeta.WorkspaceID = codexMeta.WorkspaceID
			}
			if mergedMeta.WorktreeID == "" {
				mergedMeta.WorktreeID = codexMeta.WorktreeID
			}
		}
		_, _ = s.stores.SessionMeta.Upsert(ctx, &mergedMeta)

		// Remove the old internal entry.
		_ = s.stores.Sessions.DeleteRecord(ctx, record.Session.ID)
		_ = s.stores.SessionMeta.Delete(ctx, record.Session.ID)
	}
}

// migrateLegacyDismissedSessions migrates older "dismissed as orphaned status"
// records to metadata-driven dismissal so runtime status can remain independent.
func (s *SessionService) migrateLegacyDismissedSessions(ctx context.Context) {
	if s.stores == nil || s.stores.Sessions == nil || s.stores.SessionMeta == nil {
		return
	}
	records, err := s.stores.Sessions.ListRecords(ctx)
	if err != nil {
		return
	}
	for _, record := range records {
		if record == nil || record.Session == nil {
			continue
		}
		if record.Session.Status != types.SessionStatusOrphaned {
			continue
		}
		sessionID := strings.TrimSpace(record.Session.ID)
		if sessionID == "" {
			continue
		}
		meta, ok, err := s.stores.SessionMeta.Get(ctx, sessionID)
		if err == nil && ok && meta != nil && meta.DismissedAt == nil {
			dismissedAt := time.Now().UTC()
			if record.Session.ExitedAt != nil && !record.Session.ExitedAt.IsZero() {
				dismissedAt = record.Session.ExitedAt.UTC()
			}
			_, _ = s.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
				SessionID:   sessionID,
				DismissedAt: &dismissedAt,
			})
		}

		next := *record.Session
		next.PID = 0
		next.ExitCode = nil
		if next.ExitedAt != nil && !next.ExitedAt.IsZero() {
			next.Status = types.SessionStatusExited
		} else {
			next.Status = types.SessionStatusInactive
			next.ExitedAt = nil
		}
		if _, err := s.stores.Sessions.UpsertRecord(ctx, &types.SessionRecord{
			Session: &next,
			Source:  record.Source,
		}); err != nil {
			continue
		}
	}
}

func dedupeSessionsForList(
	sessionMap map[string]*types.Session,
	sourceByID map[string]string,
	liveIDs map[string]struct{},
	metaBySessionID map[string]*types.SessionMeta,
	workflowCanonicalByRunID map[string]string,
	includeDismissed bool,
	includeWorkflowOwned bool,
) []*types.Session {
	chosenByKey := map[string]*types.Session{}
	chosenIDByKey := map[string]string{}
	for id, session := range sessionMap {
		if session == nil || !isListableStatus(session.Status) {
			continue
		}
		meta := metaBySessionID[id]
		runID := ""
		if meta != nil {
			runID = strings.TrimSpace(meta.WorkflowRunID)
		}
		if runID != "" {
			if canonicalID := strings.TrimSpace(workflowCanonicalByRunID[runID]); canonicalID != "" && canonicalID != strings.TrimSpace(id) {
				continue
			}
		}
		if !includeDismissed && isSessionDismissed(meta, session.Status) {
			continue
		}
		if !includeWorkflowOwned && runID != "" {
			continue
		}
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

func (s *SessionService) workflowCanonicalSessionsByRunID(
	ctx context.Context,
	sessionMap map[string]*types.Session,
	sourceByID map[string]string,
	liveIDs map[string]struct{},
	metaBySessionID map[string]*types.SessionMeta,
) map[string]string {
	out := map[string]string{}
	if len(metaBySessionID) == 0 {
		return out
	}

	// Prefer the run service's canonical session when it exists in the list.
	if s != nil && s.stores != nil && s.stores.WorkflowRuns != nil {
		snapshots, err := s.stores.WorkflowRuns.ListWorkflowRuns(ctx)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("session_list_workflow_snapshot_lookup_failed", logging.F("error", err))
			}
		} else {
			for _, snapshot := range snapshots {
				if snapshot.Run == nil {
					continue
				}
				runID := strings.TrimSpace(snapshot.Run.ID)
				sessionID := strings.TrimSpace(snapshot.Run.SessionID)
				if runID == "" || sessionID == "" {
					continue
				}
				if _, ok := sessionMap[sessionID]; !ok {
					continue
				}
				out[runID] = sessionID
			}
		}
	}

	// Fall back to the best available linked session when run snapshots are
	// stale or missing a usable session ID.
	for sessionID, meta := range metaBySessionID {
		if meta == nil {
			continue
		}
		runID := strings.TrimSpace(meta.WorkflowRunID)
		if runID == "" {
			continue
		}
		candidate := sessionMap[sessionID]
		if candidate == nil {
			continue
		}
		currentID := strings.TrimSpace(out[runID])
		if currentID == "" {
			out[runID] = strings.TrimSpace(sessionID)
			continue
		}
		current := sessionMap[currentID]
		if current == nil || preferListSession(sessionID, candidate, currentID, current, sourceByID, liveIDs, metaBySessionID) {
			out[runID] = strings.TrimSpace(sessionID)
		}
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
	var workspace *types.Workspace
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
			workspace = ws
		}
	}
	if workspace == nil && s.stores != nil && s.stores.Workspaces != nil && strings.TrimSpace(req.WorkspaceID) != "" {
		if ws, ok, err := s.stores.Workspaces.Get(ctx, req.WorkspaceID); err == nil && ok && ws != nil {
			workspace = ws
		}
	}
	additionalDirectories, err := s.resolveAdditionalDirectoriesForWorkspace(cwd, workspace)
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}

	rawInput := strings.Join(req.Args, " ")
	initialInput := sanitizeTitle(rawInput)
	initialText := strings.TrimSpace(rawInput)
	providerDef, hasProviderDef := providers.Lookup(req.Provider)
	runtimeOptions, err := resolveRuntimeOptions(req.Provider, nil, req.RuntimeOptions, false)
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
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
	needsAsyncInitialSend := false
	if hasProviderDef && (providerDef.Runtime == providers.RuntimeClaude || providerDef.Runtime == providers.RuntimeOpenCodeServer) {
		initialTextForStart = ""
		needsAsyncInitialSend = true
	}
	session, err := s.manager.StartSession(StartSessionConfig{
		Provider:              req.Provider,
		Cmd:                   req.Cmd,
		Cwd:                   cwd,
		AdditionalDirectories: additionalDirectories,
		Args:                  req.Args,
		Env:                   req.Env,
		CodexHome:             codexHome,
		Title:                 title,
		Tags:                  req.Tags,
		RuntimeOptions:        runtimeOptions,
		WorkspaceID:           req.WorkspaceID,
		WorktreeID:            req.WorktreeID,
		InitialInput:          initialInput,
		InitialText:           initialTextForStart,
		NotificationOverrides: types.CloneNotificationSettingsPatch(req.NotificationOverrides),
	})
	if err != nil {
		return nil, invalidError(err.Error(), err)
	}
	if needsAsyncInitialSend && initialText != "" && s.manager != nil {
		payload := buildInitialProviderPayload(providerDef, initialText, runtimeOptions)
		if len(payload) == 0 {
			return session, nil
		}
		if s.logger != nil && s.logger.Enabled(logging.Debug) {
			s.logger.Debug("provider_initial_send_enqueued",
				logging.F("session_id", session.ID),
				logging.F("provider", req.Provider),
				logging.F("input_len", len(initialText)),
			)
		}
		logFailure := func(name string, sessionID string, err error) {
			if s.logger != nil {
				s.logger.Warn(name,
					logging.F("session_id", sessionID),
					logging.F("provider", req.Provider),
					logging.F("error", err),
				)
			}
		}
		go func(sessionID string) {
			if err := s.manager.SendInput(sessionID, payload); err != nil {
				logFailure("provider_initial_send_failed", sessionID, err)
				return
			}
			if s.logger != nil && s.logger.Enabled(logging.Debug) {
				s.logger.Debug("provider_initial_send_dispatched",
					logging.F("session_id", sessionID),
					logging.F("provider", req.Provider),
				)
			}
		}(session.ID)
	}
	return session, nil
}

func buildInitialProviderPayload(providerDef providers.Definition, text string, runtimeOptions *types.SessionRuntimeOptions) []byte {
	switch providerDef.Runtime {
	case providers.RuntimeClaude:
		return buildClaudeUserPayloadWithRuntime(text, runtimeOptions)
	case providers.RuntimeOpenCodeServer:
		return buildOpenCodeUserPayloadWithRuntime(text, runtimeOptions)
	default:
		return nil
	}
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
	return s.historyStrategy(session.Provider).History(ctx, s, session, meta, source, lines)
}

func (s *SessionService) SendMessage(ctx context.Context, id string, input []map[string]any) (string, error) {
	return s.SendMessageWithOptions(ctx, id, input, SendMessageOptions{})
}

func (s *SessionService) SendMessageWithOptions(ctx context.Context, id string, input []map[string]any, options SendMessageOptions) (string, error) {
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
	effectiveMeta := meta
	var mergedRuntimeOptions *types.SessionRuntimeOptions
	if options.RuntimeOptions != nil {
		baseRuntimeOptions := (*types.SessionRuntimeOptions)(nil)
		if meta != nil {
			baseRuntimeOptions = meta.RuntimeOptions
		}
		var resolveErr error
		mergedRuntimeOptions, resolveErr = resolveRuntimeOptions(session.Provider, baseRuntimeOptions, options.RuntimeOptions, false)
		if resolveErr != nil {
			return "", invalidError(resolveErr.Error(), resolveErr)
		}
		if mergedRuntimeOptions != nil {
			if meta != nil {
				metaCopy := *meta
				effectiveMeta = &metaCopy
			} else {
				effectiveMeta = &types.SessionMeta{SessionID: session.ID}
			}
			effectiveMeta.RuntimeOptions = mergedRuntimeOptions
		}
	}
	s.ensureSessionCwd(ctx, session, effectiveMeta)
	turnID, sendErr := s.conversationAdapter(session.Provider).SendMessage(ctx, s, session, effectiveMeta, input)
	if sendErr != nil {
		return "", sendErr
	}
	if options.PersistRuntimeOption && mergedRuntimeOptions != nil {
		if persistErr := s.persistRuntimeOptionsAfterSend(ctx, session.ID, mergedRuntimeOptions); persistErr != nil {
			if s.logger != nil {
				s.logger.Error("send_runtime_options_persist_failed",
					logging.F("session_id", session.ID),
					logging.F("turn_id", turnID),
					logging.F("error", persistErr),
				)
			}
			return "", unavailableError("failed to persist runtime options after send", persistErr)
		}
	}
	return turnID, nil
}

func (s *SessionService) persistRuntimeOptionsAfterSend(
	ctx context.Context,
	sessionID string,
	runtimeOptions *types.SessionRuntimeOptions,
) error {
	if s == nil || s.stores == nil || s.stores.SessionMeta == nil {
		return ErrRuntimeOptionsPersistFailed
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ErrRuntimeOptionsPersistFailed
	}
	if runtimeOptions == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC()
	_, err := s.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
		SessionID:      sessionID,
		RuntimeOptions: types.CloneRuntimeOptions(runtimeOptions),
		LastActiveAt:   &now,
	})
	if err != nil {
		return errors.Join(ErrRuntimeOptionsPersistFailed, err)
	}
	return nil
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
	if s.approvalSync != nil {
		session, _, err := s.getSessionRecord(ctx, id)
		if err == nil && session != nil {
			meta := s.getSessionMeta(ctx, session.ID)
			if syncErr := s.approvalSync.SyncSession(ctx, session, meta); syncErr != nil && s.logger != nil {
				s.logger.Warn("approval_resync_session_failed",
					logging.F("session_id", id),
					logging.F("provider", session.Provider),
					logging.F("error", syncErr),
				)
			}
		}
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

func (s *SessionService) Update(ctx context.Context, id string, req UpdateSessionRequest) error {
	if strings.TrimSpace(id) == "" {
		return invalidError("session id is required", nil)
	}
	hasTitle := strings.TrimSpace(req.Title) != ""
	hasRuntimeOptions := req.RuntimeOptions != nil
	hasNotificationOverrides := req.NotificationOverrides != nil
	if !hasTitle && !hasRuntimeOptions && !hasNotificationOverrides {
		return invalidError("at least one update field is required", nil)
	}
	session, _, err := s.getSessionRecord(ctx, id)
	if session == nil {
		if errors.Is(err, ErrSessionNotFound) {
			return notFoundError("session not found", ErrSessionNotFound)
		}
		if err != nil {
			return invalidError(err.Error(), err)
		}
		return notFoundError("session not found", ErrSessionNotFound)
	}
	if hasTitle {
		if err := s.UpdateTitle(ctx, id, req.Title); err != nil {
			return err
		}
	}
	if hasRuntimeOptions {
		if s.stores == nil || s.stores.SessionMeta == nil {
			return unavailableError("session metadata store not available", nil)
		}
		existingMeta := s.getSessionMeta(ctx, id)
		var baseOptions *types.SessionRuntimeOptions
		if existingMeta != nil {
			baseOptions = existingMeta.RuntimeOptions
		}
		runtimeOptions, err := resolveRuntimeOptions(session.Provider, baseOptions, req.RuntimeOptions, false)
		if err != nil {
			return invalidError(err.Error(), err)
		}
		now := time.Now().UTC()
		_, err = s.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:      id,
			RuntimeOptions: runtimeOptions,
			LastActiveAt:   &now,
		})
		if err != nil {
			return unavailableError(err.Error(), err)
		}
	}
	if hasNotificationOverrides {
		if s.stores == nil || s.stores.SessionMeta == nil {
			return unavailableError("session metadata store not available", nil)
		}
		now := time.Now().UTC()
		_, err = s.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
			SessionID:             id,
			NotificationOverrides: types.CloneNotificationSettingsPatch(req.NotificationOverrides),
			LastActiveAt:          &now,
		})
		if err != nil {
			return unavailableError(err.Error(), err)
		}
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

func (s *SessionService) Dismiss(ctx context.Context, id string) error {
	return s.setSessionVisibility(ctx, id, true)
}

func (s *SessionService) Undismiss(ctx context.Context, id string) error {
	return s.setSessionVisibility(ctx, id, false)
}

func (s *SessionService) setSessionVisibility(ctx context.Context, id string, dismissed bool) error {
	if strings.TrimSpace(id) == "" {
		return invalidError("session id is required", nil)
	}
	action := sessionVisibilityAction(dismissed)
	opID := logging.NewRequestID()
	s.logSessionVisibilityRequested(opID, action, id)

	if s.manager != nil {
		var err error
		if dismissed {
			err = s.manager.DismissSession(id)
		} else {
			err = s.manager.UndismissSession(id)
		}
		if err == nil {
			s.logSessionVisibilityApplied(opID, action, id, "session_manager")
			return nil
		}
		if !errors.Is(err, ErrSessionNotFound) {
			s.logSessionVisibilityFailed(opID, action, id, "session_manager", err)
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
	if dismissed && isActiveStatus(record.Session.Status) {
		if !providers.CapabilitiesFor(record.Session.Provider).NoProcess {
			return invalidError("session is active; kill it first", nil)
		}
	}
	if s.stores == nil || s.stores.SessionMeta == nil {
		return unavailableError("session metadata store not available", nil)
	}
	clear := time.Time{}
	dismissedAt := &clear
	if dismissed {
		now := time.Now().UTC()
		dismissedAt = &now
	}
	if _, err := s.stores.SessionMeta.Upsert(ctx, &types.SessionMeta{
		SessionID:   id,
		DismissedAt: dismissedAt,
	}); err != nil {
		s.logSessionVisibilityFailed(opID, action, id, "session_meta_store", err)
		return unavailableError(err.Error(), err)
	}
	s.logSessionVisibilityApplied(opID, action, id, "session_meta_store")
	return nil
}

func sessionVisibilityAction(dismissed bool) string {
	if dismissed {
		return "dismiss"
	}
	return "undismiss"
}

func (s *SessionService) logSessionVisibilityRequested(opID, action, sessionID string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info("session_visibility_change_requested",
		logging.F("op_id", opID),
		logging.F("action", action),
		logging.F("session_id", sessionID),
	)
}

func (s *SessionService) logSessionVisibilityApplied(opID, action, sessionID, path string) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Info("session_visibility_change_applied",
		logging.F("op_id", opID),
		logging.F("action", action),
		logging.F("session_id", sessionID),
		logging.F("path", path),
	)
}

func (s *SessionService) logSessionVisibilityFailed(opID, action, sessionID, path string, err error) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger.Warn("session_visibility_change_failed",
		logging.F("op_id", opID),
		logging.F("action", action),
		logging.F("session_id", sessionID),
		logging.F("path", path),
		logging.F("error", err),
	)
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

func (s *SessionService) historyStrategy(provider string) conversationHistoryStrategy {
	if s.history == nil {
		s.history = newConversationHistoryStrategyRegistry()
	}
	return s.history.strategyFor(provider)
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

func (s *SessionService) ReadDebug(ctx context.Context, id string, lines int) ([]types.DebugEvent, bool, error) {
	if strings.TrimSpace(id) == "" {
		return nil, false, invalidError("session id is required", nil)
	}
	if lines <= 0 {
		lines = 200
	}
	events, truncated, err := s.readSessionDebug(id, lines)
	if err != nil {
		return nil, false, invalidError(err.Error(), err)
	}
	return events, truncated, nil
}

func (s *SessionService) SubscribeDebug(ctx context.Context, id string) (<-chan types.DebugEvent, func(), error) {
	if s.manager == nil {
		return nil, nil, unavailableError("session manager not available", nil)
	}
	if strings.TrimSpace(id) == "" {
		return nil, nil, invalidError("session id is required", nil)
	}
	ch, cancel, err := s.manager.SubscribeDebug(id)
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
	resolver := workspacePathResolverOrDefault(s.paths)
	workspaceSessionPath, err := resolver.ResolveWorkspaceSessionPath(ws)
	if err != nil {
		return "", "", invalidError(err.Error(), err)
	}
	if strings.TrimSpace(worktreeID) == "" {
		return workspaceSessionPath, ws.RepoPath, nil
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
			worktreeSessionPath, err := resolver.ResolveWorktreeSessionPath(ws, wt)
			if err != nil {
				return "", "", invalidError(err.Error(), err)
			}
			return worktreeSessionPath, ws.RepoPath, nil
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

func (s *SessionService) resolveAdditionalDirectoriesForWorkspace(cwd string, workspace *types.Workspace) ([]string, error) {
	if workspace == nil || len(workspace.AdditionalDirectories) == 0 {
		return nil, nil
	}
	return workspacepaths.ResolveAdditionalDirectories(cwd, workspace.AdditionalDirectories, nil)
}

func (s *SessionService) resolveAdditionalDirectoriesForSession(ctx context.Context, session *types.Session, meta *types.SessionMeta) ([]string, error) {
	if meta == nil || strings.TrimSpace(meta.WorkspaceID) == "" {
		return nil, nil
	}
	if s.stores == nil || s.stores.Workspaces == nil {
		return nil, nil
	}
	ws, ok, err := s.stores.Workspaces.Get(ctx, meta.WorkspaceID)
	if err != nil || !ok || ws == nil {
		return nil, err
	}
	cwd := ""
	if session != nil {
		cwd = strings.TrimSpace(session.Cwd)
	}
	if cwd == "" {
		if resolved, _, pathErr := s.resolveWorktreePath(ctx, meta.WorkspaceID, meta.WorktreeID); pathErr == nil {
			cwd = resolved
		}
	}
	return s.resolveAdditionalDirectoriesForWorkspace(cwd, ws)
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
	case types.SessionStatusCreated, types.SessionStatusStarting, types.SessionStatusRunning, types.SessionStatusInactive, types.SessionStatusExited:
		return true
	case types.SessionStatusOrphaned:
		// Legacy status; include only when a migrated record has not been
		// normalized yet. Current dismissal state should come from metadata.
		return true
	default:
		return false
	}
}

func isSessionDismissed(meta *types.SessionMeta, status types.SessionStatus) bool {
	if meta != nil && meta.DismissedAt != nil {
		return true
	}
	// Legacy fallback while orphaned records are being migrated.
	return status == types.SessionStatusOrphaned
}
