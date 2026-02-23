package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"control/internal/config"
	"control/internal/guidedworkflows"
	"control/internal/logging"
	"control/internal/types"
)

type stubWorkflowTemplateStore struct {
	templates []guidedworkflows.WorkflowTemplate
}

type stubGuidedWorkflowSessionGateway struct {
	sessions  []*types.Session
	meta      []*types.SessionMeta
	sendErr   error
	sendErrs  []error
	turnID    string
	startErr  error
	started   []*types.Session
	startReqs []StartSessionRequest
	sendCalls []struct {
		sessionID string
		input     []map[string]any
	}
	sendWithOptionsCalls []struct {
		sessionID string
		input     []map[string]any
		options   SendMessageOptions
	}
}

type stubGuidedWorkflowSessionMetaStore struct {
	entries   map[string]*types.SessionMeta
	listErr   error
	listCalls int
}

type stubGuidedWorkflowSessionMetaLister struct {
	entries   []*types.SessionMeta
	listErr   error
	listCalls int
}

type stubGuidedWorkflowRunStore struct {
	snapshots   map[string]guidedworkflows.RunStatusSnapshot
	listErr     error
	upsertErr   error
	listCalls   int
	upsertCalls int
}

type stubMissingRunSnapshotPolicy struct {
	calls    int
	snapshot guidedworkflows.RunStatusSnapshot
	ok       bool
}

type guidedWorkflowMissingRunSnapshotPolicyFunc func(
	meta *types.SessionMeta,
	runID string,
	now time.Time,
) (guidedworkflows.RunStatusSnapshot, bool)

func setStableWorkflowTemplatesHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("HOME", home)
	dataDir := filepath.Join(home, ".archon")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return home
}

func (s *stubWorkflowTemplateStore) ListWorkflowTemplates(context.Context) ([]guidedworkflows.WorkflowTemplate, error) {
	out := make([]guidedworkflows.WorkflowTemplate, len(s.templates))
	copy(out, s.templates)
	return out, nil
}

func (s *stubGuidedWorkflowSessionGateway) ListWithMeta(context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return s.sessions, s.meta, nil
}

func (s *stubGuidedWorkflowSessionGateway) ListWithMetaIncludingWorkflowOwned(context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return s.sessions, s.meta, nil
}

func (s *stubGuidedWorkflowSessionGateway) SendMessage(_ context.Context, id string, input []map[string]any) (string, error) {
	s.sendCalls = append(s.sendCalls, struct {
		sessionID string
		input     []map[string]any
	}{
		sessionID: id,
		input:     input,
	})
	return s.sendMessage()
}

func (s *stubGuidedWorkflowSessionGateway) SendMessageWithOptions(
	_ context.Context,
	id string,
	input []map[string]any,
	options SendMessageOptions,
) (string, error) {
	s.sendWithOptionsCalls = append(s.sendWithOptionsCalls, struct {
		sessionID string
		input     []map[string]any
		options   SendMessageOptions
	}{
		sessionID: id,
		input:     input,
		options: SendMessageOptions{
			RuntimeOptions:       types.CloneRuntimeOptions(options.RuntimeOptions),
			PersistRuntimeOption: options.PersistRuntimeOption,
		},
	})
	return s.sendMessage()
}

func (s *stubGuidedWorkflowSessionGateway) sendMessage() (string, error) {
	if len(s.sendErrs) > 0 {
		err := s.sendErrs[0]
		if len(s.sendErrs) == 1 {
			s.sendErrs = s.sendErrs[:0]
		} else {
			s.sendErrs = s.sendErrs[1:]
		}
		if err != nil {
			return "", err
		}
	}
	if s.sendErr != nil {
		return "", s.sendErr
	}
	return s.turnID, nil
}

func (s *stubGuidedWorkflowSessionGateway) Start(_ context.Context, req StartSessionRequest) (*types.Session, error) {
	s.startReqs = append(s.startReqs, req)
	if s.startErr != nil {
		return nil, s.startErr
	}
	if len(s.started) == 0 {
		return nil, nil
	}
	session := s.started[0]
	if len(s.started) == 1 {
		s.started = s.started[:0]
	} else {
		s.started = s.started[1:]
	}
	s.sessions = append(s.sessions, session)
	return session, nil
}

func (s *stubGuidedWorkflowSessionMetaStore) List(context.Context) ([]*types.SessionMeta, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]*types.SessionMeta, 0, len(s.entries))
	for _, entry := range s.entries {
		copy := *entry
		out = append(out, &copy)
	}
	return out, nil
}

func (s *stubGuidedWorkflowSessionMetaStore) Get(_ context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	if s == nil || s.entries == nil {
		return nil, false, nil
	}
	entry, ok := s.entries[sessionID]
	if !ok || entry == nil {
		return nil, false, nil
	}
	copy := *entry
	return &copy, true, nil
}

func (s *stubGuidedWorkflowSessionMetaStore) Upsert(_ context.Context, meta *types.SessionMeta) (*types.SessionMeta, error) {
	if s.entries == nil {
		s.entries = map[string]*types.SessionMeta{}
	}
	copy := *meta
	s.entries[meta.SessionID] = &copy
	return &copy, nil
}

func (s *stubGuidedWorkflowSessionMetaStore) Delete(_ context.Context, sessionID string) error {
	delete(s.entries, sessionID)
	return nil
}

func (s *stubGuidedWorkflowSessionMetaLister) List(context.Context) ([]*types.SessionMeta, error) {
	if s == nil {
		return nil, nil
	}
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	return append([]*types.SessionMeta(nil), s.entries...), nil
}

func (s *stubGuidedWorkflowRunStore) ListWorkflowRuns(context.Context) ([]guidedworkflows.RunStatusSnapshot, error) {
	s.listCalls++
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]guidedworkflows.RunStatusSnapshot, 0, len(s.snapshots))
	for _, snapshot := range s.snapshots {
		out = append(out, snapshot)
	}
	return out, nil
}

func (s *stubGuidedWorkflowRunStore) UpsertWorkflowRun(_ context.Context, snapshot guidedworkflows.RunStatusSnapshot) error {
	s.upsertCalls++
	if s.upsertErr != nil {
		return s.upsertErr
	}
	if s.snapshots == nil {
		s.snapshots = map[string]guidedworkflows.RunStatusSnapshot{}
	}
	if snapshot.Run == nil || strings.TrimSpace(snapshot.Run.ID) == "" {
		return nil
	}
	s.snapshots[strings.TrimSpace(snapshot.Run.ID)] = snapshot
	return nil
}

func (s *stubMissingRunSnapshotPolicy) BuildMissingRunSnapshot(
	_ *types.SessionMeta,
	_ string,
	_ time.Time,
) (guidedworkflows.RunStatusSnapshot, bool) {
	if s == nil {
		return guidedworkflows.RunStatusSnapshot{}, false
	}
	s.calls++
	return s.snapshot, s.ok
}

func (f guidedWorkflowMissingRunSnapshotPolicyFunc) BuildMissingRunSnapshot(
	meta *types.SessionMeta,
	runID string,
	now time.Time,
) (guidedworkflows.RunStatusSnapshot, bool) {
	if f == nil {
		return guidedworkflows.RunStatusSnapshot{}, false
	}
	return f(meta, runID, now)
}

func TestGuidedWorkflowsConfigFromCoreConfigDefaults(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	out := guidedWorkflowsConfigFromCoreConfig(cfg)
	if out.Enabled {
		t.Fatalf("expected guided workflows disabled by default")
	}
	if out.AutoStart {
		t.Fatalf("expected guided workflows auto_start disabled by default")
	}
	if out.CheckpointStyle != "confidence_weighted" {
		t.Fatalf("unexpected checkpoint style: %q", out.CheckpointStyle)
	}
	if out.Mode != "guarded_autopilot" {
		t.Fatalf("unexpected mode: %q", out.Mode)
	}
	if out.Policy.ConfidenceThreshold != 0.70 {
		t.Fatalf("unexpected policy confidence threshold: %v", out.Policy.ConfidenceThreshold)
	}
	if out.Policy.PauseThreshold != 0.60 {
		t.Fatalf("unexpected policy pause threshold: %v", out.Policy.PauseThreshold)
	}
	if out.Policy.HighBlastRadiusFileCount != 20 {
		t.Fatalf("unexpected policy high blast radius file count: %d", out.Policy.HighBlastRadiusFileCount)
	}
	if !out.Policy.HardGates.AmbiguityBlocker || !out.Policy.ConditionalGates.HighBlastRadius {
		t.Fatalf("unexpected policy default gates: %#v", out.Policy)
	}
	controls := guidedWorkflowsExecutionControlsFromCoreConfig(cfg)
	if controls.Enabled {
		t.Fatalf("expected execution controls disabled by default rollout policy")
	}
}

func TestGuidedWorkflowsExecutionControlsFromCoreConfig(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Rollout.AutomationEnabled = boolPtr(true)
	cfg.GuidedWorkflows.Rollout.AllowQualityChecks = boolPtr(true)
	cfg.GuidedWorkflows.Rollout.AllowCommit = boolPtr(true)
	cfg.GuidedWorkflows.Rollout.RequireCommitApproval = boolPtr(false)
	cfg.GuidedWorkflows.Rollout.MaxRetryAttempts = 4

	controls := guidedWorkflowsExecutionControlsFromCoreConfig(cfg)
	if !controls.Enabled {
		t.Fatalf("expected execution controls to be enabled")
	}
	if !controls.Capabilities.QualityChecks || !controls.Quality.Enabled {
		t.Fatalf("expected quality automation enabled, got %#v", controls)
	}
	if !controls.Capabilities.Commit || !controls.Commit.Enabled {
		t.Fatalf("expected commit automation enabled, got %#v", controls)
	}
	if controls.Commit.RequireApproval {
		t.Fatalf("expected commit approval requirement disabled via rollout override")
	}
	if controls.RetryPolicy.MaxAttempts != 4 {
		t.Fatalf("unexpected retry max attempts: %d", controls.RetryPolicy.MaxAttempts)
	}
}

func TestNewGuidedWorkflowNotificationPublisherSkipsWrapperWhenDisabled(t *testing.T) {
	downstream := &recordNotificationPublisher{}
	orchestrator := &recordGuidedWorkflowOrchestrator{}

	got := NewGuidedWorkflowNotificationPublisher(downstream, orchestrator)
	if got != downstream {
		t.Fatalf("expected downstream publisher to be returned unchanged when orchestrator disabled")
	}
}

func TestNewGuidedWorkflowRunServiceBuildsService(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	service := newGuidedWorkflowRunService(cfg, nil, nil, nil, nil)
	if service == nil {
		t.Fatalf("expected run service")
	}
	if _, err := service.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}); err == nil {
		t.Fatalf("expected disabled guided workflows to reject run creation")
	}
}

func TestNewGuidedWorkflowRunServiceAppliesRolloutGuardrails(t *testing.T) {
	setStableWorkflowTemplatesHome(t)

	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Enabled = boolPtr(true)
	cfg.GuidedWorkflows.Rollout.TelemetryEnabled = boolPtr(false)
	cfg.GuidedWorkflows.Rollout.MaxActiveRuns = 1

	service := newGuidedWorkflowRunService(cfg, nil, nil, nil, nil)
	if service == nil {
		t.Fatalf("expected run service")
	}
	if _, err := service.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}); err != nil {
		t.Fatalf("unexpected create run error: %v", err)
	}
	if _, err := service.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-2",
	}); err == nil {
		t.Fatalf("expected create run to fail when max_active_runs guardrail is reached")
	}
	metricsProvider, ok := any(service).(GuidedWorkflowRunMetricsService)
	if !ok {
		t.Fatalf("expected run service to expose metrics provider")
	}
	metrics, err := metricsProvider.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.Enabled {
		t.Fatalf("expected rollout telemetry_enabled=false to disable telemetry")
	}
}

func TestNewGuidedWorkflowRunServiceDoesNotPerformReconciliationSideEffects(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Enabled = boolPtr(true)

	metaStore := &stubGuidedWorkflowSessionMetaStore{
		entries: map[string]*types.SessionMeta{
			"sess-1": {SessionID: "sess-1", WorkflowRunID: "gwf-missing"},
		},
	}
	runStore := &stubGuidedWorkflowRunStore{snapshots: map[string]guidedworkflows.RunStatusSnapshot{}}
	_ = newGuidedWorkflowRunService(cfg, &Stores{
		SessionMeta:  metaStore,
		WorkflowRuns: runStore,
	}, nil, nil, nil)
	if runStore.upsertCalls != 0 {
		t.Fatalf("expected no run-store writes during service construction, got %d", runStore.upsertCalls)
	}
}

func TestNewGuidedWorkflowRunServiceWiresMissingRunContextResolverFromStores(t *testing.T) {
	setStableWorkflowTemplatesHome(t)

	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Enabled = boolPtr(true)

	metaStore := &stubGuidedWorkflowSessionMetaStore{
		entries: map[string]*types.SessionMeta{
			"sess-1": {
				SessionID:     "sess-1",
				WorkspaceID:   "ws-1",
				WorktreeID:    "wt-1",
				WorkflowRunID: "gwf-missing",
			},
		},
	}
	runStore := &stubGuidedWorkflowRunStore{
		snapshots: map[string]guidedworkflows.RunStatusSnapshot{},
	}
	service := newGuidedWorkflowRunService(cfg, &Stores{
		SessionMeta:  metaStore,
		WorkflowRuns: runStore,
	}, nil, nil, nil)

	dismissed, err := service.DismissRun(context.Background(), "gwf-missing")
	if err != nil {
		t.Fatalf("DismissRun missing: %v", err)
	}
	if dismissed.DismissedAt == nil {
		t.Fatalf("expected dismissed_at to be set")
	}
	if dismissed.WorkspaceID != "ws-1" || dismissed.WorktreeID != "wt-1" || dismissed.SessionID != "sess-1" {
		t.Fatalf("expected context restored from session metadata, got %#v", dismissed)
	}
	if metaStore.listCalls == 0 {
		t.Fatalf("expected session metadata lookup for missing run context")
	}
	if runStore.upsertCalls == 0 {
		t.Fatalf("expected dismissed missing run tombstone to be persisted")
	}
	snapshot := runStore.snapshots["gwf-missing"]
	if snapshot.Run == nil || snapshot.Run.DismissedAt == nil {
		t.Fatalf("expected persisted snapshot to include dismissed missing run tombstone, got %#v", snapshot.Run)
	}
}

func TestReconcileGuidedWorkflowRunSnapshotsBackfillsMissingRunFromSessionMeta(t *testing.T) {
	metaStore := &stubGuidedWorkflowSessionMetaStore{
		entries: map[string]*types.SessionMeta{
			"sess-1": {
				SessionID:     "sess-1",
				WorkspaceID:   "ws-1",
				WorktreeID:    "wt-1",
				WorkflowRunID: "gwf-missing",
			},
		},
	}
	runStore := &stubGuidedWorkflowRunStore{
		snapshots: map[string]guidedworkflows.RunStatusSnapshot{},
	}
	result, err := reconcileGuidedWorkflowRunSnapshots(context.Background(), guidedWorkflowRunSnapshotReconciliationInput{
		SessionMeta: metaStore,
		RunStore:    runStore,
		Clock: func() time.Time {
			return time.Date(2026, 2, 20, 10, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("reconcile guided workflow runs: %v", err)
	}
	if result.CreatedSnapshots != 1 {
		t.Fatalf("expected one created snapshot, got %#v", result)
	}

	snapshot, ok := runStore.snapshots["gwf-missing"]
	if !ok {
		t.Fatalf("expected missing workflow run snapshot to be backfilled")
	}
	if snapshot.Run == nil {
		t.Fatalf("expected backfilled run payload")
	}
	if snapshot.Run.WorkspaceID != "ws-1" || snapshot.Run.WorktreeID != "wt-1" || snapshot.Run.SessionID != "sess-1" {
		t.Fatalf("expected backfilled context fields, got %#v", snapshot.Run)
	}
	if snapshot.Run.Status != guidedworkflows.WorkflowRunStatusCompleted {
		t.Fatalf("expected backfilled run to be completed, got %q", snapshot.Run.Status)
	}
}

func TestReconcileGuidedWorkflowRunSnapshotsUsesCustomPolicy(t *testing.T) {
	metaStore := &stubGuidedWorkflowSessionMetaStore{
		entries: map[string]*types.SessionMeta{
			"sess-1": {SessionID: "sess-1", WorkflowRunID: "gwf-custom"},
		},
	}
	runStore := &stubGuidedWorkflowRunStore{snapshots: map[string]guidedworkflows.RunStatusSnapshot{}}
	policy := &stubMissingRunSnapshotPolicy{
		ok: true,
		snapshot: guidedworkflows.RunStatusSnapshot{
			Run: &guidedworkflows.WorkflowRun{
				ID:     "gwf-custom",
				Status: guidedworkflows.WorkflowRunStatusFailed,
			},
		},
	}

	result, err := reconcileGuidedWorkflowRunSnapshots(context.Background(), guidedWorkflowRunSnapshotReconciliationInput{
		SessionMeta: metaStore,
		RunStore:    runStore,
		Policy:      policy,
	})
	if err != nil {
		t.Fatalf("reconcile guided workflow runs: %v", err)
	}
	if result.CreatedSnapshots != 1 || policy.calls != 1 {
		t.Fatalf("expected custom policy to create one run; result=%#v calls=%d", result, policy.calls)
	}
	run, ok := runStore.snapshots["gwf-custom"]
	if !ok || run.Run == nil || run.Run.Status != guidedworkflows.WorkflowRunStatusFailed {
		t.Fatalf("expected custom policy snapshot to be persisted, got %#v", run)
	}
}

func TestGuidedWorkflowRunSnapshotReconciliationInputFromStores(t *testing.T) {
	if got := guidedWorkflowRunSnapshotReconciliationInputFromStores(nil); got.RunStore != nil || got.SessionMeta != nil {
		t.Fatalf("expected nil stores to produce empty reconciliation input, got %#v", got)
	}
	metaStore := &stubGuidedWorkflowSessionMetaStore{}
	runStore := &stubGuidedWorkflowRunStore{}
	got := guidedWorkflowRunSnapshotReconciliationInputFromStores(&Stores{
		SessionMeta:  metaStore,
		WorkflowRuns: runStore,
	})
	if got.RunStore == nil || got.SessionMeta == nil {
		t.Fatalf("expected stores to be forwarded, got %#v", got)
	}
}

func TestReconcileGuidedWorkflowRunSnapshotsNoDepsNoop(t *testing.T) {
	result, err := reconcileGuidedWorkflowRunSnapshots(context.Background(), guidedWorkflowRunSnapshotReconciliationInput{})
	if err != nil {
		t.Fatalf("expected no error for noop reconciliation, got %v", err)
	}
	if result != (guidedWorkflowRunSnapshotReconciliationResult{}) {
		t.Fatalf("expected zero-value noop result, got %#v", result)
	}
}

func TestReconcileGuidedWorkflowRunSnapshotsReturnsRunStoreListError(t *testing.T) {
	boom := errors.New("run store unavailable")
	runStore := &stubGuidedWorkflowRunStore{listErr: boom}
	metaStore := &stubGuidedWorkflowSessionMetaStore{}
	_, err := reconcileGuidedWorkflowRunSnapshots(context.Background(), guidedWorkflowRunSnapshotReconciliationInput{
		RunStore:    runStore,
		SessionMeta: metaStore,
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected run-store list error, got %v", err)
	}
}

func TestReconcileGuidedWorkflowRunSnapshotsReturnsSessionMetaListError(t *testing.T) {
	boom := errors.New("session meta unavailable")
	runStore := &stubGuidedWorkflowRunStore{snapshots: map[string]guidedworkflows.RunStatusSnapshot{}}
	metaStore := &stubGuidedWorkflowSessionMetaStore{listErr: boom}
	result, err := reconcileGuidedWorkflowRunSnapshots(context.Background(), guidedWorkflowRunSnapshotReconciliationInput{
		RunStore:    runStore,
		SessionMeta: metaStore,
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected session-meta list error, got %v", err)
	}
	if result.ExistingSnapshots != 0 {
		t.Fatalf("expected no existing snapshots on error, got %#v", result)
	}
}

func TestReconcileGuidedWorkflowRunSnapshotsTracksSkipAndFailureCounters(t *testing.T) {
	existingRun := guidedworkflows.RunStatusSnapshot{
		Run: &guidedworkflows.WorkflowRun{ID: "gwf-existing"},
	}
	runStore := &stubGuidedWorkflowRunStore{
		snapshots: map[string]guidedworkflows.RunStatusSnapshot{
			"gwf-existing": existingRun,
		},
		upsertErr: errors.New("write failed"),
	}
	metaLister := &stubGuidedWorkflowSessionMetaLister{
		entries: []*types.SessionMeta{
			nil, // counts as empty run id through normalization path
			{SessionID: "s-existing", WorkflowRunID: "gwf-existing"},
			{SessionID: "s-policy-skip", WorkflowRunID: "gwf-policy-skip"},
			{SessionID: "s-write-fail", WorkflowRunID: "gwf-write-fail"},
		},
	}
	policyCalls := 0
	policy := guidedWorkflowMissingRunSnapshotPolicyFunc(func(meta *types.SessionMeta, runID string, now time.Time) (guidedworkflows.RunStatusSnapshot, bool) {
		policyCalls++
		if strings.TrimSpace(runID) == "gwf-policy-skip" {
			return guidedworkflows.RunStatusSnapshot{}, false
		}
		return guidedworkflows.RunStatusSnapshot{
			Run: &guidedworkflows.WorkflowRun{
				ID:        runID,
				SessionID: strings.TrimSpace(meta.SessionID),
				CreatedAt: now,
				Status:    guidedworkflows.WorkflowRunStatusCompleted,
			},
		}, true
	})
	result, err := reconcileGuidedWorkflowRunSnapshots(context.Background(), guidedWorkflowRunSnapshotReconciliationInput{
		RunStore:    runStore,
		SessionMeta: metaLister,
		Policy:      policy,
		Clock: func() time.Time {
			return time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("unexpected reconcile error: %v", err)
	}
	if result.ExistingSnapshots != 1 || result.SkippedEmptyRunID != 1 || result.SkippedExisting != 1 || result.SkippedByPolicy != 1 || result.FailedWrites != 1 || result.CreatedSnapshots != 0 {
		t.Fatalf("unexpected reconciliation counters: %#v", result)
	}
	if policyCalls != 2 {
		t.Fatalf("expected policy to be called for two missing IDs, got %d", policyCalls)
	}
}

func TestDefaultGuidedWorkflowMissingRunSnapshotPolicy(t *testing.T) {
	policy := defaultGuidedWorkflowMissingRunSnapshotPolicy{}
	if _, ok := policy.BuildMissingRunSnapshot(nil, "   ", time.Now().UTC()); ok {
		t.Fatalf("expected empty run id to be rejected")
	}
	lastActive := time.Date(2026, 2, 19, 17, 30, 0, 0, time.UTC)
	snapshot, ok := policy.BuildMissingRunSnapshot(&types.SessionMeta{
		SessionID:     "s1",
		WorkspaceID:   "ws-1",
		WorktreeID:    "wt-1",
		WorkflowRunID: "gwf-a",
		LastActiveAt:  &lastActive,
	}, "gwf-a", time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC))
	if !ok || snapshot.Run == nil {
		t.Fatalf("expected valid snapshot from default policy, got %#v ok=%v", snapshot, ok)
	}
	if snapshot.Run.CompletedAt == nil || !snapshot.Run.CompletedAt.Equal(lastActive) {
		t.Fatalf("expected default policy to prefer last_active timestamp, got %#v", snapshot.Run.CompletedAt)
	}
}

func TestGuidedWorkflowMissingRunContextResolverSelectsBestSessionMetaMatch(t *testing.T) {
	lister := &stubGuidedWorkflowSessionMetaLister{
		entries: []*types.SessionMeta{
			{SessionID: "sess-min", WorkflowRunID: "gwf-ctx"},
			{
				SessionID:     "sess-best",
				WorkspaceID:   "ws-1",
				WorktreeID:    "wt-1",
				WorkflowRunID: "gwf-ctx",
			},
		},
	}
	resolver := newGuidedWorkflowMissingRunContextResolver(lister)
	if resolver == nil {
		t.Fatalf("expected non-nil resolver")
	}

	ctx, ok, err := resolver.ResolveMissingRunContext(context.Background(), "gwf-ctx")
	if err != nil {
		t.Fatalf("ResolveMissingRunContext: %v", err)
	}
	if !ok {
		t.Fatalf("expected resolver to find context")
	}
	if ctx.WorkspaceID != "ws-1" || ctx.WorktreeID != "wt-1" || ctx.SessionID != "sess-best" {
		t.Fatalf("unexpected resolved context: %#v", ctx)
	}
}

func TestNewGuidedWorkflowMissingRunContextResolverNilSessionMeta(t *testing.T) {
	resolver := newGuidedWorkflowMissingRunContextResolver(nil)
	if resolver != nil {
		t.Fatalf("expected nil resolver when session metadata lister is nil")
	}
}

func TestGuidedWorkflowMissingRunContextResolverNilReceiver(t *testing.T) {
	var resolver *guidedWorkflowMissingRunContextResolver

	ctx, ok, err := resolver.ResolveMissingRunContext(context.Background(), "gwf-any")
	if err != nil {
		t.Fatalf("ResolveMissingRunContext: %v", err)
	}
	if ok {
		t.Fatalf("expected nil receiver to resolve no context, got %#v", ctx)
	}
}

func TestGuidedWorkflowMissingRunContextResolverSkipsBlankRunID(t *testing.T) {
	lister := &stubGuidedWorkflowSessionMetaLister{
		entries: []*types.SessionMeta{
			{SessionID: "sess-1", WorkflowRunID: "gwf-1"},
		},
	}
	resolver := newGuidedWorkflowMissingRunContextResolver(lister)

	ctx, ok, err := resolver.ResolveMissingRunContext(context.Background(), "   ")
	if err != nil {
		t.Fatalf("ResolveMissingRunContext: %v", err)
	}
	if ok {
		t.Fatalf("expected blank run id to resolve no context, got %#v", ctx)
	}
	if lister.listCalls != 0 {
		t.Fatalf("expected blank run id to short-circuit before store lookup, got %d list calls", lister.listCalls)
	}
}

func TestGuidedWorkflowMissingRunContextResolverAllowsNilContext(t *testing.T) {
	lister := &stubGuidedWorkflowSessionMetaLister{
		entries: []*types.SessionMeta{
			{
				SessionID:     "sess-ctx",
				WorkspaceID:   "ws-ctx",
				WorktreeID:    "wt-ctx",
				WorkflowRunID: "gwf-ctx",
			},
		},
	}
	resolver := newGuidedWorkflowMissingRunContextResolver(lister)

	ctx, ok, err := resolver.ResolveMissingRunContext(nil, "gwf-ctx")
	if err != nil {
		t.Fatalf("ResolveMissingRunContext: %v", err)
	}
	if !ok {
		t.Fatalf("expected resolver to find context with nil ctx")
	}
	if ctx.WorkspaceID != "ws-ctx" || ctx.WorktreeID != "wt-ctx" || ctx.SessionID != "sess-ctx" {
		t.Fatalf("unexpected resolved context: %#v", ctx)
	}
	if lister.listCalls != 1 {
		t.Fatalf("expected one session metadata lookup, got %d", lister.listCalls)
	}
}

func TestGuidedWorkflowMissingRunContextResolverPropagatesListError(t *testing.T) {
	boom := errors.New("session metadata unavailable")
	lister := &stubGuidedWorkflowSessionMetaLister{
		listErr: boom,
	}
	resolver := newGuidedWorkflowMissingRunContextResolver(lister)

	ctx, ok, err := resolver.ResolveMissingRunContext(context.Background(), "gwf-ctx")
	if !errors.Is(err, boom) {
		t.Fatalf("expected list error %v, got %v", boom, err)
	}
	if ok {
		t.Fatalf("expected resolver failure to return ok=false, got context %#v", ctx)
	}
}

func TestGuidedWorkflowMissingRunContextResolverReturnsFalseWhenMissing(t *testing.T) {
	lister := &stubGuidedWorkflowSessionMetaLister{
		entries: []*types.SessionMeta{
			{SessionID: "sess-a", WorkflowRunID: "gwf-a"},
		},
	}
	resolver := newGuidedWorkflowMissingRunContextResolver(lister)

	ctx, ok, err := resolver.ResolveMissingRunContext(context.Background(), "gwf-missing")
	if err != nil {
		t.Fatalf("ResolveMissingRunContext: %v", err)
	}
	if ok {
		t.Fatalf("expected no context, got %#v", ctx)
	}
}

func TestReconcileGuidedWorkflowRunSnapshotsKeepsExistingDismissedSnapshot(t *testing.T) {
	dismissedAt := time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC)
	runStore := &stubGuidedWorkflowRunStore{
		snapshots: map[string]guidedworkflows.RunStatusSnapshot{
			"gwf-dismissed": {
				Run: &guidedworkflows.WorkflowRun{
					ID:          "gwf-dismissed",
					Status:      guidedworkflows.WorkflowRunStatusCompleted,
					DismissedAt: &dismissedAt,
				},
			},
		},
	}
	metaStore := &stubGuidedWorkflowSessionMetaLister{
		entries: []*types.SessionMeta{
			{
				SessionID:     "sess-1",
				WorkspaceID:   "ws-1",
				WorktreeID:    "wt-1",
				WorkflowRunID: "gwf-dismissed",
			},
		},
	}

	result, err := reconcileGuidedWorkflowRunSnapshots(context.Background(), guidedWorkflowRunSnapshotReconciliationInput{
		RunStore:    runStore,
		SessionMeta: metaStore,
	})
	if err != nil {
		t.Fatalf("reconcile guided workflow runs: %v", err)
	}
	if result.CreatedSnapshots != 0 || result.SkippedExisting != 1 {
		t.Fatalf("expected existing dismissed snapshot to be preserved, got %#v", result)
	}
	snapshot := runStore.snapshots["gwf-dismissed"]
	if snapshot.Run == nil || snapshot.Run.DismissedAt == nil {
		t.Fatalf("expected dismissed snapshot to remain dismissed, got %#v", snapshot.Run)
	}
}

func TestNewGuidedWorkflowRunServiceLoadsTemplatesFromFileConfig(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Enabled = boolPtr(true)
	home := setStableWorkflowTemplatesHome(t)
	dataDir := filepath.Join(home, ".archon")
	if err := os.WriteFile(filepath.Join(dataDir, "workflow_templates.json"), []byte(`{
  "version": 1,
  "templates": [
    {
      "id": "custom_flow",
      "name": "Custom Flow",
      "phases": [
        {
          "id": "phase_1",
          "name": "Phase 1",
          "steps": [
            {
              "id": "step_1",
              "name": "Step 1",
              "prompt": "custom prompt"
            }
          ]
        }
      ]
    }
  ]
}`), 0o600); err != nil {
		t.Fatalf("WriteFile workflow_templates.json: %v", err)
	}
	service := newGuidedWorkflowRunService(cfg, nil, nil, nil, nil)
	run, err := service.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		TemplateID:  "custom_flow",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun with custom template: %v", err)
	}
	if run.TemplateID != "custom_flow" || run.TemplateName != "Custom Flow" {
		t.Fatalf("expected custom template to be used, got id=%q name=%q", run.TemplateID, run.TemplateName)
	}
	if len(run.Phases) != 1 || len(run.Phases[0].Steps) != 1 || run.Phases[0].Steps[0].Prompt != "custom prompt" {
		t.Fatalf("expected custom prompt to be snapshotted, got %#v", run.Phases)
	}
	if _, err := service.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		TemplateID:  guidedworkflows.TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}); !errors.Is(err, guidedworkflows.ErrTemplateNotFound) {
		t.Fatalf("expected built-in template to be unavailable with explicit file config, got %v", err)
	}
}

func TestGuidedWorkflowDispatchDefaultsFromCoreConfig(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Defaults.Provider = "opencode"
	cfg.GuidedWorkflows.Defaults.Model = "gpt-5.3-codex"
	cfg.GuidedWorkflows.Defaults.Access = "on_request"
	cfg.GuidedWorkflows.Defaults.Reasoning = "high"

	defaults := guidedWorkflowDispatchDefaultsFromCoreConfig(cfg)
	if defaults.Provider != "opencode" {
		t.Fatalf("expected normalized provider opencode, got %q", defaults.Provider)
	}
	if defaults.Model != "gpt-5.3-codex" {
		t.Fatalf("expected configured model, got %q", defaults.Model)
	}
	if defaults.Access != types.AccessOnRequest {
		t.Fatalf("expected configured access, got %q", defaults.Access)
	}
	if defaults.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected configured reasoning, got %q", defaults.Reasoning)
	}
}

func TestGuidedWorkflowDispatchDefaultsFromCoreConfigClearsUnsupportedProvider(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Defaults.Provider = "claude"
	cfg.GuidedWorkflows.Defaults.Model = "gpt-5.3-codex"
	cfg.GuidedWorkflows.Defaults.Access = "read_only"
	cfg.GuidedWorkflows.Defaults.Reasoning = "low"

	defaults := guidedWorkflowDispatchDefaultsFromCoreConfig(cfg)
	if defaults.Provider != "" {
		t.Fatalf("expected unsupported provider to be cleared, got %q", defaults.Provider)
	}
	if defaults.Model != "gpt-5.3-codex" {
		t.Fatalf("expected model to remain configured when provider unsupported, got %q", defaults.Model)
	}
	if defaults.Access != types.AccessReadOnly {
		t.Fatalf("expected configured access to remain set, got %q", defaults.Access)
	}
	if defaults.Reasoning != types.ReasoningLow {
		t.Fatalf("expected configured reasoning to remain set, got %q", defaults.Reasoning)
	}
}

func TestGuidedWorkflowPromptDispatcherUsesExplicitSession(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-1", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{
				SessionID:   "sess-1",
				WorkspaceID: "ws-1",
				WorktreeID:  "wt-1",
				RuntimeOptions: &types.SessionRuntimeOptions{
					Model: "gpt-5",
				},
			},
		},
		turnID: "turn-1",
	}
	metaStore := &stubGuidedWorkflowSessionMetaStore{}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway, sessionMeta: metaStore}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-1" || result.TurnID != "turn-1" {
		t.Fatalf("unexpected dispatch result: %#v", result)
	}
	if result.Provider != "codex" || result.Model != "gpt-5" {
		t.Fatalf("expected provider/model in dispatch result, got %#v", result)
	}
	if len(gateway.sendCalls) != 1 || gateway.sendCalls[0].sessionID != "sess-1" {
		t.Fatalf("expected prompt to be sent to explicit session, got %#v", gateway.sendCalls)
	}
	linked, ok, err := metaStore.Get(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("meta get: %v", err)
	}
	if !ok || linked.WorkflowRunID != "gwf-1" {
		t.Fatalf("expected workflow ownership link for sess-1, got %#v", linked)
	}
}

func TestGuidedWorkflowPromptDispatcherUsesPerStepRuntimeOverrides(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-1", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{
				SessionID:   "sess-1",
				WorkspaceID: "ws-1",
				WorktreeID:  "wt-1",
				RuntimeOptions: &types.SessionRuntimeOptions{
					Model: "gpt-5.1-codex",
				},
			},
		},
		turnID: "turn-runtime",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		StepID:      "implementation",
		Prompt:      "implement now",
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model:     "gpt-5.3-codex",
			Reasoning: types.ReasoningHigh,
		},
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-1" || result.TurnID != "turn-runtime" {
		t.Fatalf("unexpected dispatch result: %#v", result)
	}
	if result.Model != "gpt-5.3-codex" {
		t.Fatalf("expected dispatch result model from step runtime override, got %q", result.Model)
	}
	if len(gateway.sendWithOptionsCalls) != 1 {
		t.Fatalf("expected one sendWithOptions call, got %d", len(gateway.sendWithOptionsCalls))
	}
	if len(gateway.sendCalls) != 0 {
		t.Fatalf("expected runtime-aware send path, got legacy send calls: %#v", gateway.sendCalls)
	}
	send := gateway.sendWithOptionsCalls[0]
	if send.sessionID != "sess-1" {
		t.Fatalf("expected runtime-aware send to target sess-1, got %q", send.sessionID)
	}
	if !send.options.PersistRuntimeOption {
		t.Fatalf("expected runtime override persistence to be enabled")
	}
	if send.options.RuntimeOptions == nil {
		t.Fatalf("expected runtime options in sendWithOptions request")
	}
	if send.options.RuntimeOptions.Model != "gpt-5.3-codex" {
		t.Fatalf("expected step model override in sendWithOptions request, got %q", send.options.RuntimeOptions.Model)
	}
	if send.options.RuntimeOptions.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected step reasoning override in sendWithOptions request, got %q", send.options.RuntimeOptions.Reasoning)
	}
}

func TestGuidedWorkflowPromptDispatcherRecoversToOwnedSessionWhenExplicitMissing(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-owned", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-owned", WorkflowRunID: "gwf-1", WorkspaceID: "ws-1", WorktreeID: "wt-1"},
		},
		turnID: "turn-owned",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-missing",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "continue",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-owned" {
		t.Fatalf("expected dispatch to recovered owned session, got %#v", result)
	}
	if len(gateway.startReqs) != 0 {
		t.Fatalf("expected no replacement session start request, got %d", len(gateway.startReqs))
	}
	if len(gateway.sendCalls) != 1 || gateway.sendCalls[0].sessionID != "sess-owned" {
		t.Fatalf("expected one send call to recovered session, got %#v", gateway.sendCalls)
	}
}

func TestGuidedWorkflowPromptDispatcherDoesNotStartReplacementWhenExplicitMissing(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		started: []*types.Session{
			{ID: "sess-replacement", Provider: "codex", Status: types.SessionStatusRunning},
		},
		turnID: "turn-replacement",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-missing",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "continue",
	})
	if err == nil {
		t.Fatalf("expected dispatch error for missing explicit session")
	}
	if !errors.Is(err, guidedworkflows.ErrStepDispatch) {
		t.Fatalf("expected ErrStepDispatch, got %v", err)
	}
	if result.Dispatched {
		t.Fatalf("expected no dispatched result, got %#v", result)
	}
	if len(gateway.startReqs) != 0 {
		t.Fatalf("expected no replacement start attempts, got %d", len(gateway.startReqs))
	}
	if len(gateway.sendCalls) != 0 {
		t.Fatalf("expected no send calls when explicit session is missing, got %#v", gateway.sendCalls)
	}
}

func TestGuidedWorkflowPromptDispatcherStartsWorkflowOwnedSessionWhenUnspecified(t *testing.T) {
	older := time.Now().UTC().Add(-2 * time.Hour)
	newer := time.Now().UTC()
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-old", Provider: "codex", Status: types.SessionStatusRunning},
			{ID: "sess-new", Provider: "codex", Status: types.SessionStatusRunning},
		},
		started: []*types.Session{
			{ID: "sess-workflow", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{
				SessionID:      "sess-old",
				WorkspaceID:    "ws-1",
				WorktreeID:     "wt-1",
				LastActiveAt:   &older,
				RuntimeOptions: &types.SessionRuntimeOptions{Model: "gpt-4.1"},
			},
			{
				SessionID:      "sess-new",
				WorkspaceID:    "ws-1",
				WorktreeID:     "wt-1",
				LastActiveAt:   &newer,
				RuntimeOptions: &types.SessionRuntimeOptions{Model: "gpt-5"},
			},
		},
		turnID: "turn-2",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-workflow" {
		t.Fatalf("expected workflow-owned session dispatch, got %#v", result)
	}
	if result.Provider != "codex" {
		t.Fatalf("expected codex provider to be carried through, got %#v", result)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one workflow session start request, got %d", len(gateway.startReqs))
	}
	if len(gateway.sendCalls) != 1 || gateway.sendCalls[0].sessionID != "sess-workflow" {
		t.Fatalf("expected send to sess-workflow, got %#v", gateway.sendCalls)
	}
}

func TestGuidedWorkflowPromptDispatcherReusesOwnedWorkflowSession(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-owned", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-owned", WorkspaceID: "ws-1", WorktreeID: "wt-1", WorkflowRunID: "gwf-1"},
		},
		turnID: "turn-owned",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-owned",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "continue",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-owned" {
		t.Fatalf("expected dispatch to owned workflow session, got %#v", result)
	}
	if len(gateway.startReqs) != 0 {
		t.Fatalf("expected no fallback start request, got %d", len(gateway.startReqs))
	}
}

func TestGuidedWorkflowPromptDispatcherReusesOwnedWorkflowSessionWithDefaultsConfigured(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-owned", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-owned", WorkspaceID: "ws-1", WorktreeID: "wt-1", WorkflowRunID: "gwf-1"},
		},
		turnID: "turn-owned",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions: gateway,
		defaults: guidedWorkflowDispatchDefaults{
			Provider:  "opencode",
			Model:     "gpt-5.3-codex",
			Access:    types.AccessOnRequest,
			Reasoning: types.ReasoningHigh,
		},
	}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-owned",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "continue",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-owned" {
		t.Fatalf("expected dispatch to owned workflow session, got %#v", result)
	}
	if len(gateway.startReqs) != 0 {
		t.Fatalf("expected defaults to not force new session when owned session is reusable, got %d start requests", len(gateway.startReqs))
	}
}

func TestGuidedWorkflowPromptDispatcherDefersWhenOwnedSessionBusy(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-owned", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-owned", WorkspaceID: "ws-1", WorktreeID: "wt-1", WorkflowRunID: "gwf-1"},
		},
		sendErrs: []error{
			errors.New("turn already in progress"),
			errors.New("turn already in progress"),
			errors.New("turn already in progress"),
		},
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-owned",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "continue",
	})
	if err == nil {
		t.Fatalf("expected dispatch to defer while existing session turn is active")
	}
	if !errors.Is(err, guidedworkflows.ErrStepDispatchDeferred) {
		t.Fatalf("expected ErrStepDispatchDeferred, got %v", err)
	}
	if result.Dispatched {
		t.Fatalf("expected no dispatch result while session is busy, got %#v", result)
	}
	if len(gateway.startReqs) != 0 {
		t.Fatalf("expected no replacement session start request, got %d", len(gateway.startReqs))
	}
	if len(gateway.sendCalls) != 3 {
		t.Fatalf("expected in-session retries only, got %d calls", len(gateway.sendCalls))
	}
}

func TestGuidedWorkflowPromptDispatcherDefersWhenOwnedSessionBusyWithRuntimeOptions(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-owned", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-owned", WorkspaceID: "ws-1", WorktreeID: "wt-1", WorkflowRunID: "gwf-1"},
		},
		sendErrs: []error{
			errors.New("turn already in progress"),
			errors.New("turn already in progress"),
			errors.New("turn already in progress"),
		},
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-owned",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "continue",
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model:     "gpt-5.3-codex",
			Reasoning: types.ReasoningHigh,
		},
	})
	if err == nil {
		t.Fatalf("expected dispatch to defer while existing session turn is active")
	}
	if !errors.Is(err, guidedworkflows.ErrStepDispatchDeferred) {
		t.Fatalf("expected ErrStepDispatchDeferred, got %v", err)
	}
	if result.Dispatched {
		t.Fatalf("expected no dispatch result while session is busy, got %#v", result)
	}
	if len(gateway.sendCalls) != 0 {
		t.Fatalf("expected runtime dispatch to use send with options, got %d plain calls", len(gateway.sendCalls))
	}
	if len(gateway.sendWithOptionsCalls) != 3 {
		t.Fatalf("expected three send-with-options retries, got %d", len(gateway.sendWithOptionsCalls))
	}
	for i, call := range gateway.sendWithOptionsCalls {
		if !call.options.PersistRuntimeOption {
			t.Fatalf("expected persist runtime option=true for retry %d", i+1)
		}
		if call.options.RuntimeOptions == nil {
			t.Fatalf("expected runtime options for retry %d", i+1)
		}
		if call.options.RuntimeOptions.Model != "gpt-5.3-codex" || call.options.RuntimeOptions.Reasoning != types.ReasoningHigh {
			t.Fatalf("unexpected runtime options for retry %d: %#v", i+1, call.options.RuntimeOptions)
		}
	}
}

func TestGuidedWorkflowPromptDispatcherReturnsFatalErrorAfterBusyRetriesWhenNonBusyErrorArrives(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-owned", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-owned", WorkspaceID: "ws-1", WorktreeID: "wt-1", WorkflowRunID: "gwf-1"},
		},
		sendErrs: []error{
			errors.New("turn already in progress"),
			errors.New("turn already in progress"),
			errors.New("thread not found"),
		},
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-owned",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "continue",
	})
	if err == nil {
		t.Fatalf("expected dispatch to fail when a non-busy error arrives after busy retries")
	}
	if errors.Is(err, guidedworkflows.ErrStepDispatchDeferred) {
		t.Fatalf("expected fatal dispatch error classification, got %v", err)
	}
	if !errors.Is(err, guidedworkflows.ErrStepDispatch) {
		t.Fatalf("expected wrapped ErrStepDispatch, got %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "thread not found") {
		t.Fatalf("expected terminal error detail in dispatch error, got %v", err)
	}
	if result.Dispatched {
		t.Fatalf("expected no dispatch result when terminal send error occurs, got %#v", result)
	}
	if len(gateway.sendCalls) != 3 {
		t.Fatalf("expected three in-session send attempts, got %d", len(gateway.sendCalls))
	}
}

func TestGuidedWorkflowPromptDispatcherFailsWhenOwnedSessionThreadMissing(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-owned", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-owned", WorkspaceID: "ws-1", WorktreeID: "wt-1", WorkflowRunID: "gwf-1"},
		},
		sendErr: errors.New("thread not found"),
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "continue",
	})
	if err == nil {
		t.Fatalf("expected dispatch to fail when owned session thread is missing")
	}
	if !errors.Is(err, guidedworkflows.ErrStepDispatch) {
		t.Fatalf("expected ErrStepDispatch, got %v", err)
	}
	if result.Dispatched {
		t.Fatalf("expected no dispatch result when thread is missing, got %#v", result)
	}
	if len(gateway.startReqs) != 0 {
		t.Fatalf("expected no replacement session start request, got %d", len(gateway.startReqs))
	}
	if len(gateway.sendCalls) != 1 {
		t.Fatalf("expected single dispatch attempt for missing thread, got %d", len(gateway.sendCalls))
	}
}

func TestGuidedWorkflowPromptDispatcherLinksSessionBeforeDispatchFailure(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-owned", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-owned", WorkspaceID: "ws-1", WorktreeID: "wt-1"},
		},
		sendErr: errors.New("thread not found"),
	}
	metaStore := &stubGuidedWorkflowSessionMetaStore{}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions:    gateway,
		sessionMeta: metaStore,
	}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		SessionID:   "sess-owned",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "continue",
	})
	if err == nil {
		t.Fatalf("expected dispatch to fail when thread is missing")
	}
	if !errors.Is(err, guidedworkflows.ErrStepDispatch) {
		t.Fatalf("expected ErrStepDispatch, got %v", err)
	}
	if result.Dispatched {
		t.Fatalf("expected no dispatch result when thread is missing, got %#v", result)
	}
	meta, ok := metaStore.entries["sess-owned"]
	if !ok || meta == nil {
		t.Fatalf("expected workflow link persisted before send failure")
	}
	if meta.WorkflowRunID != "gwf-1" {
		t.Fatalf("expected workflow run id to be linked, got %q", meta.WorkflowRunID)
	}
}

func TestGuidedWorkflowPromptDispatcherSkipsUnsupportedProvider(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-1", Provider: "claude", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-1", WorkspaceID: "ws-1"},
		},
		turnID: "turn-3",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		WorkspaceID: "ws-1",
		Prompt:      "hello",
	})
	if err == nil {
		t.Fatalf("expected unsupported provider flow to fail dispatch")
	}
	if !errors.Is(err, guidedworkflows.ErrStepDispatch) {
		t.Fatalf("expected ErrStepDispatch, got %v", err)
	}
	if result.Dispatched {
		t.Fatalf("expected unsupported provider to skip dispatch, got %#v", result)
	}
	if len(gateway.sendCalls) != 0 {
		t.Fatalf("expected no send calls for unsupported provider, got %#v", gateway.sendCalls)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one fallback start attempt, got %d", len(gateway.startReqs))
	}
	if gateway.startReqs[0].Provider != "codex" {
		t.Fatalf("expected codex fallback provider, got %q", gateway.startReqs[0].Provider)
	}
	if gateway.startReqs[0].WorkspaceID != "ws-1" {
		t.Fatalf("expected workspace context on fallback start, got %+v", gateway.startReqs[0])
	}
}

func TestGuidedWorkflowPromptDispatcherFallsBackToSupportedSession(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-claude", Provider: "claude", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-claude", WorkspaceID: "ws-1", WorktreeID: "wt-1"},
		},
		started: []*types.Session{
			{ID: "sess-codex", Provider: "codex", Status: types.SessionStatusRunning},
		},
		turnID: "turn-4",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-codex" || result.Provider != "codex" {
		t.Fatalf("expected fallback dispatch through supported session, got %#v", result)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one fallback start request, got %d", len(gateway.startReqs))
	}
	if gateway.startReqs[0].Provider != "codex" {
		t.Fatalf("expected codex provider for fallback start, got %q", gateway.startReqs[0].Provider)
	}
	if len(gateway.sendCalls) != 1 || gateway.sendCalls[0].sessionID != "sess-codex" {
		t.Fatalf("expected send call against fallback session, got %#v", gateway.sendCalls)
	}
}

func TestGuidedWorkflowPromptDispatcherReturnsErrorWhenFallbackStartReturnsNilSession(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-claude", Provider: "claude", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{SessionID: "sess-claude", WorkspaceID: "ws-1", WorktreeID: "wt-1"},
		},
		turnID: "turn-4",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err == nil {
		t.Fatalf("expected dispatch error when fallback session could not be created")
	}
	if !errors.Is(err, guidedworkflows.ErrStepDispatch) {
		t.Fatalf("expected ErrStepDispatch, got %v", err)
	}
	if result.Dispatched {
		t.Fatalf("expected no dispatched result, got %#v", result)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one fallback start request, got %d", len(gateway.startReqs))
	}
}

func TestGuidedWorkflowPromptDispatcherDoesNotStartSessionWithoutContext(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{turnID: "turn-no-context"}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:  "gwf-1",
		Prompt: "hello",
	})
	if err == nil {
		t.Fatalf("expected dispatch error without workspace/worktree/session context")
	}
	if !errors.Is(err, guidedworkflows.ErrStepDispatch) {
		t.Fatalf("expected ErrStepDispatch, got %v", err)
	}
	if result.Dispatched {
		t.Fatalf("expected dispatch result to remain false, got %#v", result)
	}
	if len(gateway.startReqs) != 0 {
		t.Fatalf("expected no fallback start without context, got %d", len(gateway.startReqs))
	}
}

func TestGuidedWorkflowPromptDispatcherUsesConfiguredDefaultsForAutoCreatedSession(t *testing.T) {
	var logOut bytes.Buffer
	gateway := &stubGuidedWorkflowSessionGateway{
		started: []*types.Session{
			{ID: "sess-opencode", Provider: "opencode", Status: types.SessionStatusRunning},
		},
		turnID: "turn-defaults",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions: gateway,
		defaults: guidedWorkflowDispatchDefaults{
			Provider:  "opencode",
			Model:     "gpt-5.3-codex",
			Access:    types.AccessOnRequest,
			Reasoning: types.ReasoningHigh,
		},
		logger: logging.New(&logOut, logging.Info),
	}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-opencode" {
		t.Fatalf("expected configured-default dispatch session, got %#v", result)
	}
	if result.Model != "gpt-5.3-codex" {
		t.Fatalf("expected configured model in dispatch result, got %q", result.Model)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one start request, got %d", len(gateway.startReqs))
	}
	if gateway.startReqs[0].Provider != "opencode" {
		t.Fatalf("expected configured provider on start request, got %q", gateway.startReqs[0].Provider)
	}
	runtime := gateway.startReqs[0].RuntimeOptions
	if runtime == nil {
		t.Fatalf("expected runtime options from configured defaults")
	}
	if runtime.Model != "gpt-5.3-codex" {
		t.Fatalf("expected configured model in runtime options, got %q", runtime.Model)
	}
	if runtime.Access != types.AccessOnRequest {
		t.Fatalf("expected configured access in runtime options, got %q", runtime.Access)
	}
	if runtime.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected configured reasoning in runtime options, got %q", runtime.Reasoning)
	}
	logs := logOut.String()
	if !strings.Contains(logs, "msg=guided_workflow_session_start_requested") {
		t.Fatalf("expected guided_workflow_session_start_requested log, got %q", logs)
	}
	if !strings.Contains(logs, "msg=guided_workflow_session_started") {
		t.Fatalf("expected guided_workflow_session_started log, got %q", logs)
	}
	if !strings.Contains(logs, "effective_provider=opencode") {
		t.Fatalf("expected effective provider in session creation logs, got %q", logs)
	}
	if !strings.Contains(logs, "effective_model=gpt-5.3-codex") {
		t.Fatalf("expected effective model in session creation logs, got %q", logs)
	}
	if !strings.Contains(logs, "effective_access=on_request") {
		t.Fatalf("expected effective access in session creation logs, got %q", logs)
	}
	if !strings.Contains(logs, "effective_reasoning=high") {
		t.Fatalf("expected effective reasoning in session creation logs, got %q", logs)
	}
}

func TestGuidedWorkflowPromptDispatcherLogsSessionLinkTelemetry(t *testing.T) {
	now := time.Now().UTC()
	var logOut bytes.Buffer
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{
			{ID: "sess-1", Provider: "codex", Status: types.SessionStatusRunning},
		},
		meta: []*types.SessionMeta{
			{
				SessionID:   "sess-1",
				WorkspaceID: "ws-1",
				WorktreeID:  "wt-1",
			},
		},
		turnID: "turn-link-telemetry",
	}
	metaStore := &stubGuidedWorkflowSessionMetaStore{
		entries: map[string]*types.SessionMeta{
			"sess-1": {
				SessionID:     "sess-1",
				WorkflowRunID: "gwf-old",
				DismissedAt:   &now,
			},
		},
	}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions:    gateway,
		sessionMeta: metaStore,
		logger:      logging.New(&logOut, logging.Info),
	}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-new",
		SessionID:   "sess-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-1" {
		t.Fatalf("expected dispatched result on sess-1, got %#v", result)
	}
	logs := logOut.String()
	if !strings.Contains(logs, "msg=guided_workflow_session_link_requested") {
		t.Fatalf("expected link requested telemetry, got %q", logs)
	}
	if !strings.Contains(logs, "msg=guided_workflow_session_linked") {
		t.Fatalf("expected link success telemetry, got %q", logs)
	}
	if !strings.Contains(logs, "existing_workflow_run_id=gwf-old") {
		t.Fatalf("expected previous workflow run id in telemetry, got %q", logs)
	}
	if !strings.Contains(logs, "existing_dismissed=true") {
		t.Fatalf("expected previous dismissed marker in telemetry, got %q", logs)
	}
	if !strings.Contains(logs, "run_id=gwf-new") {
		t.Fatalf("expected target run id in telemetry, got %q", logs)
	}
}

func TestGuidedWorkflowPromptDispatcherPersistsWorkspaceContextOnLink(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		sessions: []*types.Session{{ID: "sess-ctx", Provider: "codex", Status: types.SessionStatusRunning}},
		turnID:   "turn-link-context",
	}
	metaStore := &stubGuidedWorkflowSessionMetaStore{}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions:    gateway,
		sessionMeta: metaStore,
	}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "run-ctx",
		SessionID:   "sess-ctx",
		WorkspaceID: "ws-ctx",
		WorktreeID:  "wt-ctx",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-ctx" {
		t.Fatalf("expected dispatched result on sess-ctx, got %#v", result)
	}
	meta, ok := metaStore.entries["sess-ctx"]
	if !ok || meta == nil {
		t.Fatalf("expected session meta entry, got %#v", meta)
	}
	if meta.WorkflowRunID != "run-ctx" {
		t.Fatalf("expected workflow run id stored, got %q", meta.WorkflowRunID)
	}
	if meta.WorkspaceID != "ws-ctx" {
		t.Fatalf("expected workspace context stored, got %q", meta.WorkspaceID)
	}
	if meta.WorktreeID != "wt-ctx" {
		t.Fatalf("expected worktree context stored, got %q", meta.WorktreeID)
	}
}

func TestGuidedWorkflowPromptDispatcherAppliesPartialDefaultsModelOnly(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		started: []*types.Session{
			{ID: "sess-codex", Provider: "codex", Status: types.SessionStatusRunning},
		},
		turnID: "turn-model-only",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions: gateway,
		defaults: guidedWorkflowDispatchDefaults{
			Model: "gpt-5.3-codex",
		},
	}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-model-only",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-codex" {
		t.Fatalf("expected dispatch on auto-created session, got %#v", result)
	}
	if result.Model != "gpt-5.3-codex" {
		t.Fatalf("expected result model from defaults, got %q", result.Model)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one start request, got %d", len(gateway.startReqs))
	}
	if gateway.startReqs[0].Provider != "codex" {
		t.Fatalf("expected provider fallback to codex when default provider unset, got %q", gateway.startReqs[0].Provider)
	}
	runtime := gateway.startReqs[0].RuntimeOptions
	if runtime == nil {
		t.Fatalf("expected runtime options for model-only defaults")
	}
	if runtime.Model != "gpt-5.3-codex" {
		t.Fatalf("expected model-only runtime options, got %q", runtime.Model)
	}
	if runtime.Access != "" || runtime.Reasoning != "" {
		t.Fatalf("expected only model in runtime options, got %+v", runtime)
	}
}

func TestGuidedWorkflowPromptDispatcherAppliesPartialDefaultsAccessOnly(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		started: []*types.Session{
			{ID: "sess-codex", Provider: "codex", Status: types.SessionStatusRunning},
		},
		turnID: "turn-access-only",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions: gateway,
		defaults: guidedWorkflowDispatchDefaults{
			Access: types.AccessFull,
		},
	}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-access-only",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-codex" {
		t.Fatalf("expected dispatch on auto-created session, got %#v", result)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one start request, got %d", len(gateway.startReqs))
	}
	runtime := gateway.startReqs[0].RuntimeOptions
	if runtime == nil {
		t.Fatalf("expected runtime options for access-only defaults")
	}
	if runtime.Access != types.AccessFull {
		t.Fatalf("expected access-only runtime options, got %q", runtime.Access)
	}
	if runtime.Model != "" || runtime.Reasoning != "" {
		t.Fatalf("expected only access in runtime options, got %+v", runtime)
	}
}

func TestGuidedWorkflowPromptDispatcherAppliesPartialDefaultsReasoningOnly(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		started: []*types.Session{
			{ID: "sess-codex", Provider: "codex", Status: types.SessionStatusRunning},
		},
		turnID: "turn-reasoning-only",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions: gateway,
		defaults: guidedWorkflowDispatchDefaults{
			Reasoning: types.ReasoningExtraHigh,
		},
	}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-reasoning-only",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-codex" {
		t.Fatalf("expected dispatch on auto-created session, got %#v", result)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one start request, got %d", len(gateway.startReqs))
	}
	runtime := gateway.startReqs[0].RuntimeOptions
	if runtime == nil {
		t.Fatalf("expected runtime options for reasoning-only defaults")
	}
	if runtime.Reasoning != types.ReasoningExtraHigh {
		t.Fatalf("expected reasoning-only runtime options, got %q", runtime.Reasoning)
	}
	if runtime.Model != "" || runtime.Access != "" {
		t.Fatalf("expected only reasoning in runtime options, got %+v", runtime)
	}
}

func TestGuidedWorkflowEffectiveDispatchSettingsUsesCodexFallback(t *testing.T) {
	settings := guidedWorkflowEffectiveDispatchSettings("", guidedWorkflowDispatchDefaults{
		Provider: "claude",
	})
	if settings.Provider != "codex" {
		t.Fatalf("expected provider fallback to codex, got %q", settings.Provider)
	}
	if settings.Model != "" || settings.Access != "" || settings.Reasoning != "" {
		t.Fatalf("expected empty runtime details without defaults, got %+v", settings)
	}
	if settings.RuntimeOptions != nil {
		t.Fatalf("expected nil runtime options without defaults, got %+v", settings.RuntimeOptions)
	}
}

func TestGuidedWorkflowDispatchDefaultsFromCoreConfigInvalidValuesFallbackGracefully(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Defaults.Provider = "claude"
	cfg.GuidedWorkflows.Defaults.Model = "   "
	cfg.GuidedWorkflows.Defaults.Access = "invalid_access"
	cfg.GuidedWorkflows.Defaults.Reasoning = "invalid_reasoning"
	defaults := guidedWorkflowDispatchDefaultsFromCoreConfig(cfg)
	if defaults.Provider != "" {
		t.Fatalf("expected unsupported provider to normalize to empty, got %q", defaults.Provider)
	}
	if defaults.Model != "" {
		t.Fatalf("expected blank model to normalize to empty, got %q", defaults.Model)
	}
	if defaults.Access != "" {
		t.Fatalf("expected invalid access to normalize to empty, got %q", defaults.Access)
	}
	if defaults.Reasoning != "" {
		t.Fatalf("expected invalid reasoning to normalize to empty, got %q", defaults.Reasoning)
	}
}

func TestGuidedWorkflowPromptDispatcherFallsBackGracefullyWhenDefaultsInvalid(t *testing.T) {
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Defaults.Provider = "claude"
	cfg.GuidedWorkflows.Defaults.Model = "   "
	cfg.GuidedWorkflows.Defaults.Access = "invalid_access"
	cfg.GuidedWorkflows.Defaults.Reasoning = "invalid_reasoning"
	defaults := guidedWorkflowDispatchDefaultsFromCoreConfig(cfg)

	gateway := &stubGuidedWorkflowSessionGateway{
		started: []*types.Session{
			{ID: "sess-codex", Provider: "codex", Status: types.SessionStatusRunning},
		},
		turnID: "turn-invalid-defaults",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions: gateway,
		defaults: defaults,
	}
	result, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-invalid-defaults",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if !result.Dispatched || result.SessionID != "sess-codex" {
		t.Fatalf("expected dispatch to fallback codex session, got %#v", result)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one start request, got %d", len(gateway.startReqs))
	}
	if gateway.startReqs[0].Provider != "codex" {
		t.Fatalf("expected invalid provider default to fallback to codex, got %q", gateway.startReqs[0].Provider)
	}
	if gateway.startReqs[0].RuntimeOptions != nil {
		t.Fatalf("expected invalid defaults to produce nil runtime options, got %+v", gateway.startReqs[0].RuntimeOptions)
	}
}

func TestNormalizeGuidedWorkflowDispatchProvider(t *testing.T) {
	if got := normalizeGuidedWorkflowDispatchProvider(""); got != "codex" {
		t.Fatalf("expected empty provider to default to codex, got %q", got)
	}
	if got := normalizeGuidedWorkflowDispatchProvider("opencode"); got != "opencode" {
		t.Fatalf("expected supported provider to be preserved, got %q", got)
	}
	if got := normalizeGuidedWorkflowDispatchProvider("claude"); got != "codex" {
		t.Fatalf("expected unsupported provider to fallback to codex, got %q", got)
	}
}

func TestGuidedWorkflowPromptDispatcherTemplateAccessOverridesConfiguredDefaultAccess(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		started: []*types.Session{
			{ID: "sess-codex", Provider: "codex", Status: types.SessionStatusRunning},
		},
		turnID: "turn-access",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions: gateway,
		defaults: guidedWorkflowDispatchDefaults{
			Provider: "codex",
			Model:    "gpt-5.2-codex",
			Access:   types.AccessFull,
		},
	}
	_, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:              "gwf-1",
		WorkspaceID:        "ws-1",
		WorktreeID:         "wt-1",
		DefaultAccessLevel: types.AccessReadOnly,
		Prompt:             "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one start request, got %d", len(gateway.startReqs))
	}
	runtime := gateway.startReqs[0].RuntimeOptions
	if runtime == nil {
		t.Fatalf("expected runtime options to be present")
	}
	if runtime.Access != types.AccessReadOnly {
		t.Fatalf("expected template access to override configured default access, got %q", runtime.Access)
	}
	if runtime.Model != "gpt-5.2-codex" {
		t.Fatalf("expected configured default model to remain set, got %q", runtime.Model)
	}
}

func TestGuidedWorkflowPromptDispatcherFallsBackToCodexWhenConfiguredProviderUnsupported(t *testing.T) {
	gateway := &stubGuidedWorkflowSessionGateway{
		started: []*types.Session{
			{ID: "sess-codex", Provider: "codex", Status: types.SessionStatusRunning},
		},
		turnID: "turn-provider-fallback",
	}
	dispatcher := &guidedWorkflowPromptDispatcher{
		sessions: gateway,
		defaults: guidedWorkflowDispatchDefaults{
			Provider: "claude",
		},
	}
	_, err := dispatcher.DispatchStepPrompt(context.Background(), guidedworkflows.StepPromptDispatchRequest{
		RunID:       "gwf-1",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		Prompt:      "hello",
	})
	if err != nil {
		t.Fatalf("DispatchStepPrompt: %v", err)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one start request, got %d", len(gateway.startReqs))
	}
	if gateway.startReqs[0].Provider != "codex" {
		t.Fatalf("expected codex fallback provider for unsupported configured provider, got %q", gateway.startReqs[0].Provider)
	}
}

func TestGuidedWorkflowRunServiceDispatchCreatesSessionAndReusesItAcrossSteps(t *testing.T) {
	template := guidedworkflows.WorkflowTemplate{
		ID:                 "gwf_integration_simple",
		Name:               "Simple",
		DefaultAccessLevel: types.AccessReadOnly,
		Phases: []guidedworkflows.WorkflowTemplatePhase{
			{
				ID:   "phase_1",
				Name: "Phase 1",
				Steps: []guidedworkflows.WorkflowTemplateStep{
					{ID: "step_1", Name: "Step 1", Prompt: "overall plan prompt"},
					{ID: "step_2", Name: "Step 2", Prompt: "phase plan prompt"},
				},
			},
		},
	}
	now := time.Now().UTC()
	gateway := &stubGuidedWorkflowSessionGateway{
		turnID: "turn-dispatch",
		started: []*types.Session{
			{
				ID:        "sess-created",
				Provider:  "codex",
				Status:    types.SessionStatusRunning,
				CreatedAt: now,
			},
		},
	}
	metaStore := &stubGuidedWorkflowSessionMetaStore{}
	dispatcher := &guidedWorkflowPromptDispatcher{sessions: gateway, sessionMeta: metaStore}
	runService := guidedworkflows.NewRunService(
		guidedworkflows.Config{Enabled: true},
		guidedworkflows.WithTemplate(template),
		guidedworkflows.WithStepPromptDispatcher(dispatcher),
	)

	run, err := runService.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		TemplateID:  "gwf_integration_simple",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		UserPrompt:  "Fix setup workflow dispatch",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = runService.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if len(gateway.startReqs) != 1 {
		t.Fatalf("expected one auto-created session request, got %d", len(gateway.startReqs))
	}
	if gateway.startReqs[0].Provider != "codex" {
		t.Fatalf("expected codex provider for auto-created workflow session, got %q", gateway.startReqs[0].Provider)
	}
	if gateway.startReqs[0].WorkspaceID != "ws-1" || gateway.startReqs[0].WorktreeID != "wt-1" {
		t.Fatalf("expected workspace/worktree context to be propagated, got %+v", gateway.startReqs[0])
	}
	if gateway.startReqs[0].RuntimeOptions == nil || gateway.startReqs[0].RuntimeOptions.Access != types.AccessReadOnly {
		t.Fatalf("expected template default access on auto-created session, got %+v", gateway.startReqs[0].RuntimeOptions)
	}
	if run.SessionID != "sess-created" {
		t.Fatalf("expected run to bind created session, got %q", run.SessionID)
	}
	if len(gateway.sendCalls) != 1 {
		t.Fatalf("expected first prompt dispatch call, got %d", len(gateway.sendCalls))
	}
	if gateway.sendCalls[0].sessionID != "sess-created" {
		t.Fatalf("expected first dispatch to created session, got %q", gateway.sendCalls[0].sessionID)
	}
	firstInput := gateway.sendCalls[0].input
	if len(firstInput) != 1 {
		t.Fatalf("expected single input item on first dispatch, got %#v", firstInput)
	}
	firstText, _ := firstInput[0]["text"].(string)
	if firstText != "Fix setup workflow dispatch\n\noverall plan prompt" {
		t.Fatalf("unexpected first step prompt payload: %q", firstText)
	}
	linked, ok, err := metaStore.Get(context.Background(), "sess-created")
	if err != nil {
		t.Fatalf("meta get: %v", err)
	}
	if !ok || linked.WorkflowRunID != run.ID {
		t.Fatalf("expected created session to be linked to workflow run, got %#v", linked)
	}

	updated, err := runService.OnTurnCompleted(context.Background(), guidedworkflows.TurnSignal{
		SessionID: "sess-created",
		TurnID:    "turn-dispatch",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one updated run after turn completion, got %d", len(updated))
	}
	run = updated[0]
	if len(gateway.sendCalls) != 2 {
		t.Fatalf("expected second prompt dispatch call, got %d", len(gateway.sendCalls))
	}
	if gateway.sendCalls[1].sessionID != "sess-created" {
		t.Fatalf("expected second dispatch to same session, got %q", gateway.sendCalls[1].sessionID)
	}
	secondInput := gateway.sendCalls[1].input
	if len(secondInput) != 1 {
		t.Fatalf("expected single input item on second dispatch, got %#v", secondInput)
	}
	secondText, _ := secondInput[0]["text"].(string)
	if secondText != "phase plan prompt" {
		t.Fatalf("unexpected second step prompt payload: %q", secondText)
	}
	secondStep := run.Phases[0].Steps[1]
	if secondStep.Execution == nil || secondStep.Execution.SessionID != "sess-created" {
		t.Fatalf("expected second step execution to link same session, got %#v", secondStep.Execution)
	}
}

func TestGuidedWorkflowNotificationPublisherForwardsAndObservesTurnCompleted(t *testing.T) {
	downstream := &recordNotificationPublisher{}
	orchestrator := &recordGuidedWorkflowOrchestrator{enabled: true}

	publisher := NewGuidedWorkflowNotificationPublisher(downstream, orchestrator)
	if publisher == nil {
		t.Fatalf("expected wrapped publisher")
	}

	publisher.Publish(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
	})
	publisher.Publish(types.NotificationEvent{
		Trigger:   types.NotificationTriggerSessionFailed,
		SessionID: "sess-1",
	})

	if len(downstream.events) != 2 {
		t.Fatalf("expected downstream notifications to be preserved, got %d", len(downstream.events))
	}
	if len(orchestrator.turnEvents) != 1 {
		t.Fatalf("expected one guided workflow turn event, got %d", len(orchestrator.turnEvents))
	}
	if orchestrator.turnEvents[0].Trigger != types.NotificationTriggerTurnCompleted {
		t.Fatalf("unexpected guided workflow event trigger: %q", orchestrator.turnEvents[0].Trigger)
	}
}

func TestGuidedWorkflowNotificationPublisherEmitsDecisionNeededPayload(t *testing.T) {
	downstream := &recordNotificationPublisher{}
	orchestrator := &recordGuidedWorkflowOrchestrator{enabled: true}
	runService := guidedworkflows.NewRunService(guidedworkflows.Config{Enabled: true})
	preCommitHardGate := true
	run, err := runService.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
		PolicyOverrides: &guidedworkflows.CheckpointPolicyOverride{
			HardGates: &guidedworkflows.CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = runService.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != guidedworkflows.WorkflowRunStatusRunning {
		t.Fatalf("expected running before turn progression, got %q", run.Status)
	}

	publisher := NewGuidedWorkflowNotificationPublisher(downstream, orchestrator, runService)
	decisionCount := 0
	lastTurnID := ""
	for i := 0; i < 20; i++ {
		lastTurnID = fmt.Sprintf("turn-%d", i+1)
		publisher.Publish(types.NotificationEvent{
			Trigger:     types.NotificationTriggerTurnCompleted,
			SessionID:   "sess-1",
			WorkspaceID: "ws-1",
			WorktreeID:  "wt-1",
			Provider:    "codex",
			TurnID:      lastTurnID,
		})
		decisionCount = countDecisionNeededEvents(downstream.events)
		if decisionCount > 0 {
			break
		}
	}
	if decisionCount != 1 {
		t.Fatalf("expected exactly one decision-needed event, got %d", decisionCount)
	}
	decisionEvent := lastDecisionNeededEvent(downstream.events)
	if decisionEvent == nil {
		t.Fatalf("expected decision-needed event")
	}
	if decisionEvent.Status != "decision_needed" {
		t.Fatalf("expected decision_needed status, got %q", decisionEvent.Status)
	}
	if reason := asString(decisionEvent.Payload["reason"]); reason == "" {
		t.Fatalf("expected reason in payload")
	}
	if riskSummary := asString(decisionEvent.Payload["risk_summary"]); riskSummary == "" {
		t.Fatalf("expected risk_summary in payload")
	}
	if recommended := asString(decisionEvent.Payload["recommended_action"]); recommended == "" {
		t.Fatalf("expected recommended_action in payload")
	}

	// Replaying the same turn event should not emit duplicate decision-needed notifications.
	publisher.Publish(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-1",
		TurnID:    lastTurnID,
	})
	if got := countDecisionNeededEvents(downstream.events); got != 1 {
		t.Fatalf("expected duplicate replay to be deduped, got %d decision-needed events", got)
	}
}

type recordNotificationPublisher struct {
	events []types.NotificationEvent
}

func (p *recordNotificationPublisher) Publish(event types.NotificationEvent) {
	p.events = append(p.events, event)
}

func countDecisionNeededEvents(events []types.NotificationEvent) int {
	count := 0
	for _, event := range events {
		if event.Status == "decision_needed" {
			count++
		}
	}
	return count
}

func lastDecisionNeededEvent(events []types.NotificationEvent) *types.NotificationEvent {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Status == "decision_needed" {
			event := events[i]
			return &event
		}
	}
	return nil
}

type recordGuidedWorkflowOrchestrator struct {
	enabled    bool
	turnEvents []types.NotificationEvent
}

func (o *recordGuidedWorkflowOrchestrator) Enabled() bool {
	return o.enabled
}

func (o *recordGuidedWorkflowOrchestrator) Config() guidedworkflows.Config {
	return guidedworkflows.Config{
		Enabled: o.enabled,
	}
}

func (o *recordGuidedWorkflowOrchestrator) StartRun(context.Context, guidedworkflows.StartRunRequest) (*guidedworkflows.Run, error) {
	return nil, nil
}

func (o *recordGuidedWorkflowOrchestrator) OnTurnEvent(_ context.Context, event types.NotificationEvent) {
	o.turnEvents = append(o.turnEvents, event)
}

func boolPtr(v bool) *bool {
	return &v
}
