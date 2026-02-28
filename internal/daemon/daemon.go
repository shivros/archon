package daemon

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/store"
	"control/internal/types"
)

type Daemon struct {
	addr    string
	token   string
	version string
	server  *http.Server
	manager *SessionManager
	stores  *Stores
	logger  logging.Logger
}

type Stores struct {
	Workspaces        WorkspaceStore
	Worktrees         WorktreeStore
	Groups            WorkspaceGroupStore
	WorkflowTemplates WorkflowTemplateStore
	WorkflowRuns      WorkflowRunStore
	AppState          AppStateStore
	SessionMeta       SessionMetaStore
	Sessions          SessionIndexStore
	Approvals         ApprovalStore
	Notes             NoteStore
}

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

type AppStateStore interface {
	Load(ctx context.Context) (*types.AppState, error)
	Save(ctx context.Context, state *types.AppState) error
}

type WorkflowTemplateStore interface {
	ListWorkflowTemplates(ctx context.Context) ([]guidedworkflows.WorkflowTemplate, error)
}

type WorkflowRunStore interface {
	ListWorkflowRuns(ctx context.Context) ([]guidedworkflows.RunStatusSnapshot, error)
	UpsertWorkflowRun(ctx context.Context, snapshot guidedworkflows.RunStatusSnapshot) error
}

type ProviderRegistry interface {
	List() []types.ProviderInfo
}

type SessionMetaStore interface {
	List(ctx context.Context) ([]*types.SessionMeta, error)
	Get(ctx context.Context, sessionID string) (*types.SessionMeta, bool, error)
	Upsert(ctx context.Context, meta *types.SessionMeta) (*types.SessionMeta, error)
	Delete(ctx context.Context, sessionID string) error
}

type SessionIndexStore interface {
	ListRecords(ctx context.Context) ([]*types.SessionRecord, error)
	GetRecord(ctx context.Context, sessionID string) (*types.SessionRecord, bool, error)
	UpsertRecord(ctx context.Context, record *types.SessionRecord) (*types.SessionRecord, error)
	DeleteRecord(ctx context.Context, sessionID string) error
}

type ApprovalStore interface {
	ListBySession(ctx context.Context, sessionID string) ([]*types.Approval, error)
	Get(ctx context.Context, sessionID string, requestID int) (*types.Approval, bool, error)
	Upsert(ctx context.Context, approval *types.Approval) (*types.Approval, error)
	Delete(ctx context.Context, sessionID string, requestID int) error
	DeleteSession(ctx context.Context, sessionID string) error
}

type NoteStore interface {
	List(ctx context.Context, filter store.NoteFilter) ([]*types.Note, error)
	Get(ctx context.Context, id string) (*types.Note, bool, error)
	Upsert(ctx context.Context, note *types.Note) (*types.Note, error)
	Delete(ctx context.Context, id string) error
}

type guidedWorkflowRunCloser interface {
	Close()
}

var newGuidedWorkflowRunServiceFn = newGuidedWorkflowRunService

func New(addr, token, version string, manager *SessionManager, stores *Stores) *Daemon {
	if manager != nil && stores != nil && stores.SessionMeta != nil {
		manager.SetMetaStore(stores.SessionMeta)
	}
	if manager != nil && stores != nil && stores.Sessions != nil {
		manager.SetSessionStore(stores.Sessions)
	}
	return &Daemon{
		addr:    addr,
		token:   token,
		version: version,
		manager: manager,
		stores:  stores,
	}
}

func (d *Daemon) Run(ctx context.Context) error {
	coreCfg := loadCoreConfigOrDefault()
	if d.logger == nil {
		d.logger = logging.New(log.Writer(), logging.ParseLevel(coreCfg.LogLevel()))
	}
	api := &API{
		Version: d.version,
		Manager: d.manager,
		Stores:  d.stores,
		Logger:  d.logger,
	}
	notifier := NewNotificationService(
		NewNotificationPolicyResolver(notificationDefaultsFromCoreConfig(coreCfg), d.stores, d.logger),
		NewNotificationDispatcher(defaultNotificationSinks(), d.logger),
		d.logger,
	)
	defer notifier.Close()
	liveCodex := NewCodexLiveManager(d.stores, d.logger)
	guided := newGuidedWorkflowOrchestrator(coreCfg)
	reconcileResult, reconcileErr := reconcileGuidedWorkflowRunSnapshots(
		context.Background(),
		guidedWorkflowRunSnapshotReconciliationInputFromStores(d.stores),
	)
	logGuidedWorkflowRunReconciliationOutcome(d.logger, reconcileResult, reconcileErr)
	turnNotifier := NewTurnCompletionNotifier(nil, d.stores)
	approvalStore := NewStoreApprovalStorage(d.stores)
	artifactRepository := newFileSessionItemsRepository(d.manager)
	compositeLive := NewCompositeLiveManager(
		d.stores,
		d.logger,
		newCodexLiveSessionFactory(liveCodex),
		newClaudeLiveSessionFactory(d.manager, d.stores, artifactRepository, turnNotifier, d.logger),
		newOpenCodeLiveSessionFactory("opencode", turnNotifier, approvalStore, artifactRepository, defaultTurnCompletionPayloadBuilder{}, NewTurnEvidenceFreshnessTracker(), d.logger),
		newOpenCodeLiveSessionFactory("kilocode", turnNotifier, approvalStore, artifactRepository, defaultTurnCompletionPayloadBuilder{}, NewTurnEvidenceFreshnessTracker(), d.logger),
	)
	workflowRuns := newGuidedWorkflowRunServiceFn(coreCfg, d.stores, d.manager, compositeLive, d.logger)
	if closer, ok := any(workflowRuns).(guidedWorkflowRunCloser); ok {
		defer closer.Close()
	}
	var turnProcessor guidedworkflows.TurnEventProcessor
	if processor, ok := any(workflowRuns).(guidedworkflows.TurnEventProcessor); ok {
		turnProcessor = processor
	}
	eventPublisher := NewGuidedWorkflowNotificationPublisher(notifier, guided, turnProcessor)
	if d.manager != nil {
		d.manager.SetNotificationPublisher(eventPublisher)
	}
	api.Notifier = eventPublisher
	api.GuidedWorkflows = guided
	api.WorkflowRuns = workflowRuns
	api.WorkflowSessionVisibility = newWorkflowRunSessionVisibilitySyncService(d.stores, d.logger)
	api.WorkflowSessionInterrupt = newWorkflowRunSessionInterruptService(d.manager, d.stores, liveCodex, d.logger)
	api.WorkflowRunStop = newWorkflowRunStopCoordinator(api.WorkflowRuns, api.WorkflowSessionInterrupt, d.logger)
	if metrics, ok := any(workflowRuns).(GuidedWorkflowRunMetricsService); ok {
		api.WorkflowRunMetrics = metrics
	}
	if reset, ok := any(workflowRuns).(GuidedWorkflowRunMetricsResetService); ok {
		api.WorkflowRunMetricsReset = reset
	}
	api.WorkflowTemplates = workflowRuns
	api.WorkflowPolicy = newGuidedWorkflowPolicyResolver(coreCfg)
	api.WorkflowDispatchDefaults = guidedWorkflowDispatchDefaultsFromCoreConfig(coreCfg)
	api.CodexHistoryPool = NewCodexHistoryPool(d.logger)
	defer api.CodexHistoryPool.Close()
	syncer := NewCodexSyncer(d.stores, d.logger)
	api.Syncer = syncer
	api.LiveCodex = liveCodex
	api.LiveCodex.SetNotificationPublisher(eventPublisher)
	compositeLive.SetNotificationPublisher(eventPublisher)
	turnNotifier.SetNotificationPublisher(eventPublisher)
	api.LiveManager = compositeLive
	approvalSync := NewApprovalResyncService(d.stores, d.logger)

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	handler := TokenAuthMiddleware(d.token, mux)
	handler = LoggingMiddleware(d.logger, handler)
	d.server = &http.Server{
		Addr:    d.addr,
		Handler: handler,
	}
	api.Shutdown = d.server.Shutdown

	go func() {
		_ = syncer.SyncAll(context.Background())
		_ = approvalSync.SyncAll(context.Background())
	}()

	errCh := make(chan error, 1)
	go func() {
		d.logger.Info("daemon_listening", logging.F("addr", d.addr))
		errCh <- d.server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func logGuidedWorkflowRunReconciliationOutcome(
	logger logging.Logger,
	result guidedWorkflowRunSnapshotReconciliationResult,
	err error,
) {
	if logger == nil {
		return
	}
	if err != nil {
		logger.Warn("guided_workflow_runs_reconcile_failed", logging.F("error", err))
		return
	}
	if result.CreatedSnapshots <= 0 && result.FailedWrites <= 0 && result.SessionMetaWithRunID <= 0 {
		return
	}
	logger.Info("guided_workflow_runs_reconciled_from_session_meta",
		logging.F("created_runs", result.CreatedSnapshots),
		logging.F("failed_writes", result.FailedWrites),
		logging.F("existing_snapshots", result.ExistingSnapshots),
		logging.F("session_meta_scanned", result.SessionMetaScanned),
		logging.F("session_meta_with_run_id", result.SessionMetaWithRunID),
		logging.F("session_meta_dismissed", result.SessionMetaDismissed),
		logging.F("created_from_dismissed_meta", result.CreatedFromDismissedMeta),
		logging.F("skipped_existing", result.SkippedExisting),
		logging.F("skipped_empty_run_id", result.SkippedEmptyRunID),
		logging.F("skipped_by_policy", result.SkippedByPolicy),
	)
}
