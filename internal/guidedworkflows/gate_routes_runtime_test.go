package guidedworkflows

import (
	"strings"
	"testing"
	"time"
)

func TestFindGateRouteByID(t *testing.T) {
	if _, ok := findGateRouteByID(nil, "route-1"); ok {
		t.Fatal("expected nil gate lookup to miss")
	}
	if _, ok := findGateRouteByID(&WorkflowGateRun{}, "   "); ok {
		t.Fatal("expected blank route id lookup to miss")
	}
	route, ok := findGateRouteByID(&WorkflowGateRun{
		Routes: []WorkflowGateRoute{
			{ID: "route-1", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetNextStep}},
		},
	}, " route-1 ")
	if !ok || route.ID != "route-1" {
		t.Fatalf("expected route lookup hit, got %#v ok=%v", route, ok)
	}
}

func TestFindWorkflowStepByID(t *testing.T) {
	if _, _, ok := findWorkflowStepByID(nil, "step-1"); ok {
		t.Fatal("expected nil run lookup to miss")
	}
	if _, _, ok := findWorkflowStepByID(&WorkflowRun{}, " "); ok {
		t.Fatal("expected blank step id lookup to miss")
	}
	run := &WorkflowRun{
		Phases: []PhaseRun{
			{
				ID: "phase-1",
				Steps: []StepRun{
					{ID: "step-1"},
				},
			},
		},
	}
	if _, _, ok := findWorkflowStepByID(run, "step-2"); ok {
		t.Fatal("expected missing step lookup to miss")
	}
	phaseIndex, stepIndex, ok := findWorkflowStepByID(run, " step-1 ")
	if !ok || phaseIndex != 0 || stepIndex != 0 {
		t.Fatalf("expected step lookup hit at 0,0 got (%d,%d) ok=%v", phaseIndex, stepIndex, ok)
	}
}

func TestCompareWorkflowStepLocations(t *testing.T) {
	if got := compareWorkflowStepLocations(0, 0, 1, 0); got >= 0 {
		t.Fatalf("expected earlier location to compare less, got %d", got)
	}
	if got := compareWorkflowStepLocations(1, 0, 0, 1); got <= 0 {
		t.Fatalf("expected later location to compare greater, got %d", got)
	}
	if got := compareWorkflowStepLocations(1, 2, 1, 2); got != 0 {
		t.Fatalf("expected identical locations to compare equal, got %d", got)
	}
}

func TestResolveSelectedGateRouteRejectsUndeclaredRouteWithGenericMessage(t *testing.T) {
	_, err := resolveSelectedGateRoute(&WorkflowGateRun{
		ID: "gate-1",
		Routes: []WorkflowGateRoute{
			{ID: "continue_default", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetNextStep}},
		},
	}, "skip_validation")
	if err == nil {
		t.Fatal("expected undeclared route error")
	}
	if !strings.Contains(err.Error(), `gate selected undeclared route "skip_validation"`) {
		t.Fatalf("expected generic undeclared-route error, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "llm_judge") {
		t.Fatalf("expected generic route error message, got %q", err.Error())
	}
}

func TestResolveSelectedGateRouteRejectsNilGate(t *testing.T) {
	_, err := resolveSelectedGateRoute(nil, "route-1")
	if err == nil || !strings.Contains(err.Error(), "gate context is unavailable") {
		t.Fatalf("expected missing gate context error, got %v", err)
	}
}

func TestPlanWorkflowGateRouteBuildsSkipPlanForStepTarget(t *testing.T) {
	run := &WorkflowRun{
		Phases: []PhaseRun{
			{
				ID: "phase-1",
				Steps: []StepRun{
					{ID: "step-1", Status: StepRunStatusCompleted},
				},
			},
			{
				ID: "phase-2",
				Steps: []StepRun{
					{ID: "step-2", Status: StepRunStatusPending},
					{ID: "step-3", Status: StepRunStatusPending},
				},
			},
		},
	}
	plan, err := planWorkflowGateRoute(run, WorkflowGateRoute{
		ID: "skip_validation",
		Target: WorkflowGateRouteTargetRef{
			Kind:   WorkflowGateRouteTargetStep,
			StepID: "step-3",
		},
	})
	if err != nil {
		t.Fatalf("planWorkflowGateRoute: %v", err)
	}
	if plan.recordSelectionOnly {
		t.Fatalf("expected step target to require state changes, got %#v", plan)
	}
	if len(plan.skipLocations) != 1 || plan.skipLocations[0] != (workflowStepLocation{phaseIndex: 1, stepIndex: 0}) {
		t.Fatalf("expected step-2 to be skipped, got %#v", plan.skipLocations)
	}
	if len(plan.finalizePhaseIndexes) != 1 || plan.finalizePhaseIndexes[0] != 1 {
		t.Fatalf("expected phase-2 finalization plan, got %#v", plan.finalizePhaseIndexes)
	}
	if plan.continuation != (workflowStepLocation{phaseIndex: 1, stepIndex: 1}) {
		t.Fatalf("expected continuation at step-3, got %#v", plan.continuation)
	}
}

func TestPlanWorkflowGateRouteRejectsInvalidSelections(t *testing.T) {
	tests := []struct {
		name    string
		run     *WorkflowRun
		route   WorkflowGateRoute
		wantErr string
	}{
		{
			name: "nil run",
			run:  nil,
			route: WorkflowGateRoute{
				ID: "route-1",
				Target: WorkflowGateRouteTargetRef{
					Kind: WorkflowGateRouteTargetNextStep,
				},
			},
			wantErr: "run context is unavailable",
		},
		{
			name: "no pending step remains",
			run: &WorkflowRun{
				Phases: []PhaseRun{
					{
						ID: "phase-1",
						Steps: []StepRun{
							{ID: "step-1", Status: StepRunStatusCompleted},
						},
					},
				},
			},
			route: WorkflowGateRoute{
				ID: "skip",
				Target: WorkflowGateRouteTargetRef{
					Kind:   WorkflowGateRouteTargetStep,
					StepID: "step-1",
				},
			},
			wantErr: "no pending step remains",
		},
		{
			name: "target step not found",
			run: &WorkflowRun{
				Phases: []PhaseRun{
					{
						ID: "phase-1",
						Steps: []StepRun{
							{ID: "step-1", Status: StepRunStatusPending},
						},
					},
				},
			},
			route: WorkflowGateRoute{
				ID: "skip",
				Target: WorkflowGateRouteTargetRef{
					Kind:   WorkflowGateRouteTargetStep,
					StepID: "missing",
				},
			},
			wantErr: `target step "missing" was not found`,
		},
		{
			name: "target behind continuation point",
			run: &WorkflowRun{
				Phases: []PhaseRun{
					{
						ID: "phase-1",
						Steps: []StepRun{
							{ID: "step-1", Status: StepRunStatusCompleted},
							{ID: "step-2", Status: StepRunStatusPending},
						},
					},
				},
			},
			route: WorkflowGateRoute{
				ID: "rewind",
				Target: WorkflowGateRouteTargetRef{
					Kind:   WorkflowGateRouteTargetStep,
					StepID: "step-1",
				},
			},
			wantErr: `target step "step-1" is behind the current continuation point`,
		},
		{
			name: "target step not pending",
			run: &WorkflowRun{
				Phases: []PhaseRun{
					{
						ID: "phase-1",
						Steps: []StepRun{
							{ID: "step-1", Status: StepRunStatusPending},
							{ID: "step-2", Status: StepRunStatusCompleted},
						},
					},
				},
			},
			route: WorkflowGateRoute{
				ID: "skip",
				Target: WorkflowGateRouteTargetRef{
					Kind:   WorkflowGateRouteTargetStep,
					StepID: "step-2",
				},
			},
			wantErr: `target step "step-2" is not pending`,
		},
		{
			name: "intervening step not pending",
			run: &WorkflowRun{
				Phases: []PhaseRun{
					{
						ID: "phase-1",
						Steps: []StepRun{
							{ID: "step-1", Status: StepRunStatusPending},
							{ID: "step-2", Status: StepRunStatusCompleted},
							{ID: "step-3", Status: StepRunStatusPending},
						},
					},
				},
			},
			route: WorkflowGateRoute{
				ID: "skip",
				Target: WorkflowGateRouteTargetRef{
					Kind:   WorkflowGateRouteTargetStep,
					StepID: "step-3",
				},
			},
			wantErr: `step "step-2" is not pending`,
		},
		{
			name: "unsupported target kind",
			run: &WorkflowRun{
				Phases: []PhaseRun{
					{
						ID: "phase-1",
						Steps: []StepRun{
							{ID: "step-1", Status: StepRunStatusPending},
						},
					},
				},
			},
			route: WorkflowGateRoute{
				ID: "mystery",
				Target: WorkflowGateRouteTargetRef{
					Kind: WorkflowGateRouteTargetKind("mystery"),
				},
			},
			wantErr: `target kind "mystery" is not supported`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := planWorkflowGateRoute(tt.run, tt.route)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestPlanWorkflowGateRouteRecordSelectionOnlyTargets(t *testing.T) {
	run := &WorkflowRun{
		Phases: []PhaseRun{
			{
				ID: "phase-1",
				Steps: []StepRun{
					{ID: "step-1", Status: StepRunStatusPending},
				},
			},
		},
	}
	for _, route := range []WorkflowGateRoute{
		{ID: "continue_default", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetNextStep}},
		{ID: "finish_phase", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetCompletePhase}},
	} {
		plan, err := planWorkflowGateRoute(run, route)
		if err != nil {
			t.Fatalf("planWorkflowGateRoute(%q): %v", route.ID, err)
		}
		if !plan.recordSelectionOnly {
			t.Fatalf("expected %q to be selection-only, got %#v", route.ID, plan)
		}
		if len(plan.skipLocations) != 0 || len(plan.finalizePhaseIndexes) != 0 {
			t.Fatalf("expected no skip/finalize plan for %q, got %#v", route.ID, plan)
		}
	}
}

func TestApplyWorkflowGateRoutePlanLockedSelectionOnlyDoesNotMutateContinuation(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	now := time.Unix(1700000000, 0)
	run := &WorkflowRun{
		ID:                "run-1",
		CurrentPhaseIndex: 1,
		CurrentStepIndex:  2,
		Phases: []PhaseRun{
			{ID: "phase-1"},
		},
	}
	phase := &run.Phases[0]
	gate := &WorkflowGateRun{
		ID:       "gate-1",
		Kind:     WorkflowGateKindLLMJudge,
		Boundary: WorkflowGateBoundaryRef{Boundary: WorkflowGateBoundaryPhaseEnd},
	}
	plan := workflowGateRoutePlan{
		route: WorkflowGateRoute{
			ID:     "continue_default",
			Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetNextStep},
		},
		recordSelectionOnly: true,
	}
	if err := service.applyWorkflowGateRoutePlanLocked(run, phase, gate, plan, now); err != nil {
		t.Fatalf("applyWorkflowGateRoutePlanLocked: %v", err)
	}
	if run.CurrentPhaseIndex != 1 || run.CurrentStepIndex != 2 {
		t.Fatalf("expected selection-only route to leave continuation unchanged, got phase=%d step=%d", run.CurrentPhaseIndex, run.CurrentStepIndex)
	}
	if len(service.timelines[run.ID]) != 1 || service.timelines[run.ID][0].Type != "gate_route_selected" {
		t.Fatalf("expected route selection timeline event, got %#v", service.timelines[run.ID])
	}
}

func TestFinalizePhaseAfterGateRouteLocked(t *testing.T) {
	service := NewRunService(Config{Enabled: true})
	now := time.Unix(1700000001, 0)
	run := &WorkflowRun{ID: "run-2"}
	gate := &WorkflowGateRun{ID: "gate-1"}
	route := WorkflowGateRoute{ID: "skip", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetStep, StepID: "step-2"}}

	notComplete := &PhaseRun{
		ID:     "phase-1",
		Status: PhaseRunStatusRunning,
		Steps: []StepRun{
			{ID: "step-1", Status: StepRunStatusCompleted},
			{ID: "step-2", Status: StepRunStatusPending},
		},
	}
	service.finalizePhaseAfterGateRouteLocked(run, notComplete, gate, route, now)
	if notComplete.Status == PhaseRunStatusCompleted {
		t.Fatalf("expected incomplete phase to remain incomplete, got %#v", notComplete)
	}

	complete := &PhaseRun{
		ID:     "phase-2",
		Status: PhaseRunStatusRunning,
		Steps: []StepRun{
			{ID: "step-1", Status: StepRunStatusCompleted},
			{ID: "step-2", Status: StepRunStatusCompleted},
		},
	}
	service.finalizePhaseAfterGateRouteLocked(run, complete, gate, route, now)
	if complete.Status != PhaseRunStatusCompleted || complete.CompletedAt == nil {
		t.Fatalf("expected complete phase to be finalized, got %#v", complete)
	}
	if len(service.timelines[run.ID]) == 0 || service.timelines[run.ID][0].Type != "phase_completed_by_gate_route" {
		t.Fatalf("expected phase completion timeline event, got %#v", service.timelines[run.ID])
	}
	if len(run.AuditTrail) == 0 || run.AuditTrail[0].Action != "phase_completed_by_gate_route" {
		t.Fatalf("expected phase completion audit event, got %#v", run.AuditTrail)
	}

	beforeTimelineCount := len(service.timelines[run.ID])
	beforeAuditCount := len(run.AuditTrail)
	service.finalizePhaseAfterGateRouteLocked(run, complete, gate, route, now)
	if len(service.timelines[run.ID]) != beforeTimelineCount || len(run.AuditTrail) != beforeAuditCount {
		t.Fatalf("expected already-complete phase to avoid duplicate events")
	}
}
