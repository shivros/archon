package daemon

import (
	"context"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type guidedWorkflowMetricsStore struct {
	appState AppStateStore
}

func newGuidedWorkflowMetricsStore(stores *Stores) guidedworkflows.RunMetricsStore {
	if stores == nil || stores.AppState == nil {
		return nil
	}
	return &guidedWorkflowMetricsStore{appState: stores.AppState}
}

func (s *guidedWorkflowMetricsStore) LoadRunMetrics(ctx context.Context) (guidedworkflows.RunMetricsSnapshot, error) {
	if s == nil || s.appState == nil {
		return guidedworkflows.RunMetricsSnapshot{}, nil
	}
	state, err := s.appState.Load(ctx)
	if err != nil {
		return guidedworkflows.RunMetricsSnapshot{}, err
	}
	if state == nil || state.GuidedWorkflowTelemetry == nil {
		return guidedworkflows.RunMetricsSnapshot{}, nil
	}
	return runMetricsSnapshotFromState(state.GuidedWorkflowTelemetry), nil
}

func (s *guidedWorkflowMetricsStore) SaveRunMetrics(ctx context.Context, snapshot guidedworkflows.RunMetricsSnapshot) error {
	if s == nil || s.appState == nil {
		return nil
	}
	state, err := s.appState.Load(ctx)
	if err != nil {
		return err
	}
	if state == nil {
		state = &types.AppState{}
	}
	state.GuidedWorkflowTelemetry = guidedWorkflowTelemetryStateFromSnapshot(snapshot)
	return s.appState.Save(ctx, state)
}

func runMetricsSnapshotFromState(state *types.GuidedWorkflowTelemetryState) guidedworkflows.RunMetricsSnapshot {
	if state == nil {
		return guidedworkflows.RunMetricsSnapshot{}
	}
	out := guidedworkflows.RunMetricsSnapshot{
		Enabled:              true,
		CapturedAt:           state.CapturedAt,
		RunsStarted:          state.RunsStarted,
		RunsCompleted:        state.RunsCompleted,
		RunsFailed:           state.RunsFailed,
		PauseCount:           state.PauseCount,
		PauseRate:            state.PauseRate,
		ApprovalCount:        state.ApprovalCount,
		ApprovalLatencyAvgMS: state.ApprovalLatencyAvgMS,
		ApprovalLatencyMaxMS: state.ApprovalLatencyMaxMS,
		InterventionCauses:   map[string]int{},
	}
	for cause, count := range state.InterventionCauses {
		out.InterventionCauses[cause] = count
	}
	return out
}

func guidedWorkflowTelemetryStateFromSnapshot(snapshot guidedworkflows.RunMetricsSnapshot) *types.GuidedWorkflowTelemetryState {
	out := &types.GuidedWorkflowTelemetryState{
		CapturedAt:           snapshot.CapturedAt,
		RunsStarted:          snapshot.RunsStarted,
		RunsCompleted:        snapshot.RunsCompleted,
		RunsFailed:           snapshot.RunsFailed,
		PauseCount:           snapshot.PauseCount,
		PauseRate:            snapshot.PauseRate,
		ApprovalCount:        snapshot.ApprovalCount,
		ApprovalLatencyAvgMS: snapshot.ApprovalLatencyAvgMS,
		ApprovalLatencyMaxMS: snapshot.ApprovalLatencyMaxMS,
	}
	if len(snapshot.InterventionCauses) > 0 {
		out.InterventionCauses = make(map[string]int, len(snapshot.InterventionCauses))
		for cause, count := range snapshot.InterventionCauses {
			out.InterventionCauses[cause] = count
		}
	}
	return out
}
