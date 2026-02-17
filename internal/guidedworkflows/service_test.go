package guidedworkflows

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type stubRunMetricsStore struct {
	loadSnapshot RunMetricsSnapshot
	loadErr      error
	saved        []RunMetricsSnapshot
}

func (s *stubRunMetricsStore) LoadRunMetrics(context.Context) (RunMetricsSnapshot, error) {
	if s == nil {
		return RunMetricsSnapshot{}, nil
	}
	if s.loadErr != nil {
		return RunMetricsSnapshot{}, s.loadErr
	}
	return s.loadSnapshot, nil
}

func (s *stubRunMetricsStore) SaveRunMetrics(_ context.Context, snapshot RunMetricsSnapshot) error {
	if s == nil {
		return nil
	}
	s.saved = append(s.saved, snapshot)
	return nil
}

func TestRunLifecycleNoopEndToEnd(t *testing.T) {
	service := NewRunService(Config{
		Enabled:         true,
		CheckpointStyle: "confidence_weighted",
		Mode:            "guarded_autopilot",
	})

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  TemplateIDSolidPhaseDelivery,
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.Status != WorkflowRunStatusCreated {
		t.Fatalf("expected created status, got %q", run.Status)
	}

	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}

	// StartRun advances one no-op step. Continue advancing to completion.
	for i := 0; i < 16; i++ {
		if run.Status != WorkflowRunStatusRunning {
			break
		}
		run, err = service.AdvanceRun(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("AdvanceRun iteration %d: %v", i, err)
		}
	}
	if run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed status, got %q", run.Status)
	}
	if run.CompletedAt == nil {
		t.Fatalf("expected completed timestamp")
	}

	timeline, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline: %v", err)
	}
	if len(timeline) == 0 {
		t.Fatalf("expected non-empty timeline")
	}
	if timeline[0].Type != "run_created" {
		t.Fatalf("unexpected first event: %q", timeline[0].Type)
	}
	last := timeline[len(timeline)-1]
	if last.Type != "run_completed" {
		t.Fatalf("unexpected final event: %q", last.Type)
	}
}

func TestRunLifecyclePauseResumeTransitions(t *testing.T) {
	service := NewRunService(Config{Enabled: true})

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running, got %q", run.Status)
	}

	run, err = service.PauseRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("PauseRun: %v", err)
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused, got %q", run.Status)
	}

	run, err = service.ResumeRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("ResumeRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning && run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected running or completed after resume, got %q", run.Status)
	}
}

func TestRunLifecycleInvalidTransitions(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	if _, err := service.ResumeRun(context.Background(), run.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for resume before pause, got %v", err)
	}
	if _, err := service.PauseRun(context.Background(), run.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for pause before start, got %v", err)
	}

	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for duplicate start, got %v", err)
	}
}

func TestRunLifecycleCreateRunValidation(t *testing.T) {
	disabled := NewRunService(Config{})
	if _, err := disabled.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}

	enabled := NewRunService(Config{Enabled: true})
	if _, err := enabled.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  TemplateIDSolidPhaseDelivery,
		WorkspaceID: "",
		WorktreeID:  "",
	}); !errors.Is(err, ErrMissingContext) {
		t.Fatalf("expected ErrMissingContext, got %v", err)
	}
	if _, err := enabled.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "unknown",
		WorkspaceID: "ws-1",
	}); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("expected ErrTemplateNotFound, got %v", err)
	}
}

func TestRunLifecycleStepHandlerFailureTransitionsToFailed(t *testing.T) {
	engine := NewEngine(WithStepHandler("implementation", func(context.Context, *WorkflowRun, *PhaseRun, *StepRun) error {
		return errors.New("boom")
	}))
	service := NewRunService(Config{Enabled: true}, WithEngine(engine))

	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}

	_, err = service.AdvanceRun(context.Background(), run.ID)
	if err == nil {
		t.Fatalf("expected advance to fail on implementation step")
	}
	run, err = service.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != WorkflowRunStatusFailed {
		t.Fatalf("expected failed status, got %q", run.Status)
	}
	if run.LastError == "" {
		t.Fatalf("expected last error to be captured")
	}
}

func TestRunLifecyclePerRunPolicyOverridesApplied(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	confidenceThreshold := 0.82
	blastRadiusCount := 12
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		PolicyOverrides: &CheckpointPolicyOverride{
			ConfidenceThreshold:      &confidenceThreshold,
			HighBlastRadiusFileCount: &blastRadiusCount,
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.Policy.ConfidenceThreshold != 0.82 {
		t.Fatalf("unexpected per-run confidence threshold: %v", run.Policy.ConfidenceThreshold)
	}
	if run.Policy.HighBlastRadiusFileCount != 12 {
		t.Fatalf("unexpected per-run blast radius threshold: %d", run.Policy.HighBlastRadiusFileCount)
	}
}

func TestRunLifecyclePolicyPauseDecisionMetadata(t *testing.T) {
	service := NewRunService(Config{Enabled: true}, WithTemplate(WorkflowTemplate{
		ID:   "single_commit",
		Name: "Single Commit",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "one",
				Name: "one",
				Steps: []WorkflowTemplateStep{
					{ID: "commit", Name: "commit"},
				},
			},
		},
	}))
	preCommitHardGate := true
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "single_commit",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		PolicyOverrides: &CheckpointPolicyOverride{
			HardGates: &CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected run paused by pre-commit hard gate, got %q", run.Status)
	}
	if run.LatestDecision == nil {
		t.Fatalf("expected latest decision metadata")
	}
	if run.LatestDecision.Metadata.Action != CheckpointActionPause {
		t.Fatalf("expected pause decision action, got %q", run.LatestDecision.Metadata.Action)
	}
	if run.LatestDecision.Metadata.Severity != DecisionSeverityCritical {
		t.Fatalf("expected critical severity, got %q", run.LatestDecision.Metadata.Severity)
	}
	if len(run.LatestDecision.Metadata.Reasons) == 0 {
		t.Fatalf("expected decision reasons")
	}
}

func TestRunLifecycleHandleDecisionActions(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}

	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionActionPauseRun,
		Note:   "pause for review",
	})
	if err != nil {
		t.Fatalf("HandleDecision pause_run: %v", err)
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused after pause_run decision, got %q", run.Status)
	}

	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionActionRequestRevision,
		Note:   "needs revision",
	})
	if err != nil {
		t.Fatalf("HandleDecision request_revision: %v", err)
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused after request_revision decision, got %q", run.Status)
	}

	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionActionApproveContinue,
		Note:   "continue anyway",
	})
	if err != nil {
		t.Fatalf("HandleDecision approve_continue: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning && run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected running/completed after approve_continue, got %q", run.Status)
	}
}

func TestRunLifecycleHandleDecisionIdempotent(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionActionPauseRun,
	})
	if err != nil {
		t.Fatalf("HandleDecision pause_run: %v", err)
	}
	timelineBefore, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline: %v", err)
	}
	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action:     DecisionActionRequestRevision,
		DecisionID: run.LatestDecision.ID,
	})
	if err != nil {
		t.Fatalf("HandleDecision request_revision first: %v", err)
	}
	timelineAfterFirst, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline first: %v", err)
	}
	if len(timelineAfterFirst) <= len(timelineBefore) {
		t.Fatalf("expected timeline growth after first request_revision")
	}
	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action:     DecisionActionRequestRevision,
		DecisionID: run.LatestDecision.ID,
	})
	if err != nil {
		t.Fatalf("HandleDecision request_revision second: %v", err)
	}
	timelineAfterSecond, err := service.GetRunTimeline(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("GetRunTimeline second: %v", err)
	}
	if len(timelineAfterSecond) != len(timelineAfterFirst) {
		t.Fatalf("expected idempotent repeated decision action; first=%d second=%d", len(timelineAfterFirst), len(timelineAfterSecond))
	}
}

func TestRunLifecycleOnTurnCompletedIdempotent(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-turn",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}
	updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-turn",
		TurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted first: %v", err)
	}
	if len(updated) != 1 {
		t.Fatalf("expected one run update on first turn, got %d", len(updated))
	}
	updated, err = service.OnTurnCompleted(context.Background(), TurnSignal{
		SessionID: "sess-turn",
		TurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("OnTurnCompleted duplicate: %v", err)
	}
	if len(updated) != 0 {
		t.Fatalf("expected duplicate turn event to be deduped, got %d updates", len(updated))
	}
}

func TestRunLifecycleOnTurnCompletedCanReachPolicyPause(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	preCommitHardGate := true
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		SessionID:   "sess-policy",
		PolicyOverrides: &CheckpointPolicyOverride{
			HardGates: &CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusRunning {
		t.Fatalf("expected running after start, got %q", run.Status)
	}
	for i := 0; i < 20 && run.Status == WorkflowRunStatusRunning; i++ {
		updated, err := service.OnTurnCompleted(context.Background(), TurnSignal{
			SessionID: "sess-policy",
			TurnID:    fmt.Sprintf("turn-%d", i+1),
		})
		if err != nil {
			t.Fatalf("OnTurnCompleted iteration %d: %v", i, err)
		}
		if len(updated) > 0 {
			run = updated[len(updated)-1]
		}
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected run paused by policy on turn progression, got %q", run.Status)
	}
	if run.LatestDecision == nil || run.LatestDecision.Metadata.Action != CheckpointActionPause {
		t.Fatalf("expected latest pause decision metadata after turn progression")
	}
}

func TestRunLifecycleHandleDecisionRejectsUnknownAction(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action: DecisionAction("unknown"),
	}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition for unknown decision action, got %v", err)
	}
}

func TestRunLifecycleMetricsSnapshot(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	for i := 0; i < 32 && run.Status == WorkflowRunStatusRunning; i++ {
		run, err = service.AdvanceRun(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("AdvanceRun %d: %v", i, err)
		}
	}
	if run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed run status, got %q", run.Status)
	}
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if !metrics.Enabled {
		t.Fatalf("expected metrics enabled")
	}
	if metrics.RunsStarted != 1 {
		t.Fatalf("expected runs_started=1, got %d", metrics.RunsStarted)
	}
	if metrics.RunsCompleted != 1 {
		t.Fatalf("expected runs_completed=1, got %d", metrics.RunsCompleted)
	}
	if metrics.PauseRate != 0 {
		t.Fatalf("expected pause_rate=0 for uninterrupted run, got %f", metrics.PauseRate)
	}
}

func TestRunLifecycleMetricsApprovalLatencyAndInterventionCause(t *testing.T) {
	service := NewRunService(Config{Enabled: true}, WithTemplate(WorkflowTemplate{
		ID:   "single_commit",
		Name: "Single Commit",
		Phases: []WorkflowTemplatePhase{
			{
				ID:   "phase",
				Name: "phase",
				Steps: []WorkflowTemplateStep{
					{ID: "commit", Name: "commit"},
				},
			},
		},
	}))
	preCommitHardGate := true
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		TemplateID:  "single_commit",
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
		PolicyOverrides: &CheckpointPolicyOverride{
			HardGates: &CheckpointPolicyGatesOverride{
				PreCommitApproval: &preCommitHardGate,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if run.Status != WorkflowRunStatusPaused {
		t.Fatalf("expected paused status from policy checkpoint, got %q", run.Status)
	}
	run, err = service.HandleDecision(context.Background(), run.ID, DecisionActionRequest{
		Action:     DecisionActionApproveContinue,
		DecisionID: run.LatestDecision.ID,
	})
	if err != nil {
		t.Fatalf("HandleDecision approve_continue: %v", err)
	}
	if run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed after approval, got %q", run.Status)
	}
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.PauseCount != 1 {
		t.Fatalf("expected pause_count=1, got %d", metrics.PauseCount)
	}
	if metrics.PauseRate != 1 {
		t.Fatalf("expected pause_rate=1, got %f", metrics.PauseRate)
	}
	if metrics.ApprovalCount != 1 {
		t.Fatalf("expected approval_count=1, got %d", metrics.ApprovalCount)
	}
	if metrics.ApprovalLatencyAvgMS < 0 || metrics.ApprovalLatencyMaxMS < metrics.ApprovalLatencyAvgMS {
		t.Fatalf("unexpected approval latency metrics: avg=%d max=%d", metrics.ApprovalLatencyAvgMS, metrics.ApprovalLatencyMaxMS)
	}
	if metrics.InterventionCauses["pre_commit_approval_required"] != 1 {
		t.Fatalf("expected pre_commit_approval intervention cause, got %#v", metrics.InterventionCauses)
	}
}

func TestRunLifecycleTelemetryCanBeDisabled(t *testing.T) {
	service := NewRunService(Config{Enabled: true}, WithTelemetryEnabled(false))
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.Enabled {
		t.Fatalf("expected telemetry disabled")
	}
	if metrics.RunsStarted != 0 || metrics.PauseCount != 0 || metrics.ApprovalCount != 0 {
		t.Fatalf("expected zeroed metrics when telemetry disabled: %#v", metrics)
	}
}

func TestRunLifecycleMaxActiveRunsGuardrail(t *testing.T) {
	service := NewRunService(Config{Enabled: true}, WithMaxActiveRuns(1))
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun first: %v", err)
	}
	if _, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-2",
	}); !errors.Is(err, ErrRunLimitExceeded) {
		t.Fatalf("expected ErrRunLimitExceeded, got %v", err)
	}
	run, err = service.StartRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	for i := 0; i < 32 && run.Status == WorkflowRunStatusRunning; i++ {
		run, err = service.AdvanceRun(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("AdvanceRun %d: %v", i, err)
		}
	}
	if run.Status != WorkflowRunStatusCompleted {
		t.Fatalf("expected completed run, got %q", run.Status)
	}
	if _, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-2",
	}); err != nil {
		t.Fatalf("expected create to succeed after active run completed, got %v", err)
	}
}

func TestRunLifecycleTelemetryPersistenceRoundTrip(t *testing.T) {
	store := &stubRunMetricsStore{
		loadSnapshot: RunMetricsSnapshot{
			Enabled:       true,
			RunsStarted:   4,
			RunsCompleted: 3,
			PauseCount:    2,
			InterventionCauses: map[string]int{
				"pre_commit_approval_required": 2,
			},
		},
	}
	service := NewRunService(Config{Enabled: true}, WithRunMetricsStore(store))
	initial, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics initial: %v", err)
	}
	if initial.RunsStarted != 4 || initial.RunsCompleted != 3 || initial.PauseCount != 2 {
		t.Fatalf("expected persisted metrics to be restored, got %#v", initial)
	}
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if len(store.saved) == 0 {
		t.Fatalf("expected metrics persistence on run start")
	}
	last := store.saved[len(store.saved)-1]
	if last.RunsStarted != 5 {
		t.Fatalf("expected runs_started to persist as 5, got %d", last.RunsStarted)
	}
	if last.InterventionCauses["pre_commit_approval_required"] != 2 {
		t.Fatalf("expected persisted intervention causes to be preserved, got %#v", last.InterventionCauses)
	}
}

func TestRunLifecycleTelemetryPersistenceSkippedWhenDisabled(t *testing.T) {
	store := &stubRunMetricsStore{
		loadSnapshot: RunMetricsSnapshot{
			Enabled:     true,
			RunsStarted: 7,
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTelemetryEnabled(false),
		WithRunMetricsStore(store),
	)
	metrics, err := service.GetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("GetRunMetrics: %v", err)
	}
	if metrics.RunsStarted != 0 || metrics.Enabled {
		t.Fatalf("expected telemetry-disabled service to ignore persisted counters, got %#v", metrics)
	}
	run, err := service.CreateRun(context.Background(), CreateRunRequest{
		WorkspaceID: "ws-1",
		WorktreeID:  "wt-1",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := service.StartRun(context.Background(), run.ID); err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if len(store.saved) != 0 {
		t.Fatalf("expected no persistence writes when telemetry disabled, got %d", len(store.saved))
	}
}

func TestRunLifecycleResetMetrics(t *testing.T) {
	store := &stubRunMetricsStore{
		loadSnapshot: RunMetricsSnapshot{
			Enabled:       true,
			RunsStarted:   4,
			RunsCompleted: 3,
			PauseCount:    1,
		},
	}
	service := NewRunService(Config{Enabled: true}, WithRunMetricsStore(store))
	snapshot, err := service.ResetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("ResetRunMetrics: %v", err)
	}
	if !snapshot.Enabled {
		t.Fatalf("expected reset snapshot enabled=true")
	}
	if snapshot.RunsStarted != 0 || snapshot.RunsCompleted != 0 || snapshot.PauseCount != 0 || snapshot.ApprovalCount != 0 {
		t.Fatalf("expected zeroed reset snapshot, got %#v", snapshot)
	}
	if len(snapshot.InterventionCauses) != 0 {
		t.Fatalf("expected no intervention causes after reset, got %#v", snapshot.InterventionCauses)
	}
	if len(store.saved) == 0 {
		t.Fatalf("expected reset to persist metrics snapshot")
	}
	last := store.saved[len(store.saved)-1]
	if last.RunsStarted != 0 || last.PauseCount != 0 {
		t.Fatalf("expected persisted reset snapshot to be zeroed, got %#v", last)
	}
}

func TestRunLifecycleResetMetricsPersistsWhenTelemetryDisabled(t *testing.T) {
	store := &stubRunMetricsStore{
		loadSnapshot: RunMetricsSnapshot{
			Enabled:       true,
			RunsStarted:   8,
			RunsCompleted: 7,
		},
	}
	service := NewRunService(
		Config{Enabled: true},
		WithTelemetryEnabled(false),
		WithRunMetricsStore(store),
	)
	snapshot, err := service.ResetRunMetrics(context.Background())
	if err != nil {
		t.Fatalf("ResetRunMetrics: %v", err)
	}
	if snapshot.Enabled {
		t.Fatalf("expected reset snapshot enabled=false when telemetry is disabled")
	}
	if len(store.saved) != 1 {
		t.Fatalf("expected reset to force one persistence write, got %d", len(store.saved))
	}
	if store.saved[0].RunsStarted != 0 || store.saved[0].RunsCompleted != 0 {
		t.Fatalf("expected forced reset persistence to zero counters, got %#v", store.saved[0])
	}
}
