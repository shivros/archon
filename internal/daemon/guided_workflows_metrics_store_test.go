package daemon

import (
	"context"
	"strings"
	"sync"
	"testing"

	"control/internal/config"
	"control/internal/guidedworkflows"
	"control/internal/types"
)

type memoryAppStateStore struct {
	mu    sync.Mutex
	state *types.AppState
}

type memoryWorkflowRunStore struct {
	mu        sync.Mutex
	snapshots map[string]guidedworkflows.RunStatusSnapshot
}

func (s *memoryAppStateStore) Load(context.Context) (*types.AppState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return &types.AppState{}, nil
	}
	copy := *s.state
	return &copy, nil
}

func (s *memoryAppStateStore) Save(_ context.Context, state *types.AppState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state == nil {
		s.state = nil
		return nil
	}
	copy := *state
	s.state = &copy
	return nil
}

func (s *memoryWorkflowRunStore) ListWorkflowRuns(context.Context) ([]guidedworkflows.RunStatusSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]guidedworkflows.RunStatusSnapshot, 0, len(s.snapshots))
	for _, snapshot := range s.snapshots {
		out = append(out, snapshot)
	}
	return out, nil
}

func (s *memoryWorkflowRunStore) UpsertWorkflowRun(_ context.Context, snapshot guidedworkflows.RunStatusSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshots == nil {
		s.snapshots = map[string]guidedworkflows.RunStatusSnapshot{}
	}
	if snapshot.Run == nil || strings.TrimSpace(snapshot.Run.ID) == "" {
		return nil
	}
	s.snapshots[strings.TrimSpace(snapshot.Run.ID)] = snapshot
	return nil
}

func TestGuidedWorkflowMetricsStoreRoundTrip(t *testing.T) {
	appStateStore := &memoryAppStateStore{
		state: &types.AppState{
			ActiveWorkspaceID: "ws-keep",
		},
	}
	store := &guidedWorkflowMetricsStore{appState: appStateStore}
	snapshot := guidedworkflows.RunMetricsSnapshot{
		Enabled:       true,
		RunsStarted:   3,
		RunsCompleted: 2,
		PauseCount:    1,
		InterventionCauses: map[string]int{
			"user_pause_run": 1,
		},
	}
	if err := store.SaveRunMetrics(context.Background(), snapshot); err != nil {
		t.Fatalf("SaveRunMetrics: %v", err)
	}
	state, err := appStateStore.Load(context.Background())
	if err != nil {
		t.Fatalf("Load app state: %v", err)
	}
	if state.ActiveWorkspaceID != "ws-keep" {
		t.Fatalf("expected app state fields to be preserved, got %q", state.ActiveWorkspaceID)
	}
	loaded, err := store.LoadRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("LoadRunMetrics: %v", err)
	}
	if loaded.RunsStarted != 3 || loaded.RunsCompleted != 2 || loaded.PauseCount != 1 {
		t.Fatalf("unexpected loaded metrics: %#v", loaded)
	}
	if loaded.InterventionCauses["user_pause_run"] != 1 {
		t.Fatalf("expected intervention causes to round-trip, got %#v", loaded.InterventionCauses)
	}
}

func TestNewGuidedWorkflowRunServiceRestoresPersistedMetrics(t *testing.T) {
	setStableWorkflowTemplatesHome(t)

	appStateStore := &memoryAppStateStore{
		state: &types.AppState{
			GuidedWorkflowTelemetry: &types.GuidedWorkflowTelemetryState{
				RunsStarted: 5,
				PauseCount:  2,
			},
		},
	}
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Enabled = boolPtr(true)
	service := newGuidedWorkflowRunService(cfg, &Stores{AppState: appStateStore}, nil, nil, nil)

	metricsProvider, ok := any(service).(GuidedWorkflowRunMetricsService)
	if !ok {
		t.Fatalf("expected run service metrics provider")
	}
	metrics, err := metricsProvider.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.RunsStarted != 5 || metrics.PauseCount != 2 {
		t.Fatalf("expected persisted telemetry to restore, got %#v", metrics)
	}

	run, err := service.CreateRun(context.Background(), guidedworkflows.CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	reloadedState, err := appStateStore.Load(context.Background())
	if err != nil {
		t.Fatalf("Load app state: %v", err)
	}
	if reloadedState.GuidedWorkflowTelemetry == nil || reloadedState.GuidedWorkflowTelemetry.RunsStarted < 6 {
		t.Fatalf("expected persisted runs_started to increase, got %#v", reloadedState.GuidedWorkflowTelemetry)
	}
}

func TestNewGuidedWorkflowRunServiceRestoresPersistedRuns(t *testing.T) {
	setStableWorkflowTemplatesHome(t)

	runStore := &memoryWorkflowRunStore{
		snapshots: map[string]guidedworkflows.RunStatusSnapshot{
			"gwf-restored": {
				Run: &guidedworkflows.WorkflowRun{
					ID:         "gwf-restored",
					TemplateID: guidedworkflows.TemplateIDSolidPhaseDelivery,
					Status:     guidedworkflows.WorkflowRunStatusPaused,
				},
				Timeline: []guidedworkflows.RunTimelineEvent{
					{Type: "run_created", RunID: "gwf-restored"},
				},
			},
		},
	}
	cfg := config.DefaultCoreConfig()
	cfg.GuidedWorkflows.Enabled = boolPtr(true)
	service := newGuidedWorkflowRunService(cfg, &Stores{WorkflowRuns: runStore}, nil, nil, nil)

	run, err := service.GetRun(context.Background(), "gwf-restored")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run == nil || run.ID != "gwf-restored" || run.Status != guidedworkflows.WorkflowRunStatusPaused {
		t.Fatalf("expected persisted run to restore, got %#v", run)
	}
	timeline, err := service.GetRunTimeline(context.Background(), "gwf-restored")
	if err != nil {
		t.Fatalf("GetRunTimeline: %v", err)
	}
	if len(timeline) != 1 || timeline[0].Type != "run_created" {
		t.Fatalf("expected persisted timeline to restore, got %#v", timeline)
	}
}
