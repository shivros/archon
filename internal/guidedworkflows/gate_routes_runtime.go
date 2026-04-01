package guidedworkflows

import (
	"fmt"
	"strings"
	"time"
)

type workflowStepLocation struct {
	phaseIndex int
	stepIndex  int
}

type workflowGateRoutePlan struct {
	route                WorkflowGateRoute
	recordSelectionOnly  bool
	skipLocations        []workflowStepLocation
	finalizePhaseIndexes []int
	continuation         workflowStepLocation
}

func findGateRouteByID(gate *WorkflowGateRun, routeID string) (WorkflowGateRoute, bool) {
	if gate == nil {
		return WorkflowGateRoute{}, false
	}
	normalizedID := strings.TrimSpace(routeID)
	if normalizedID == "" {
		return WorkflowGateRoute{}, false
	}
	for _, route := range gate.Routes {
		if strings.TrimSpace(route.ID) == normalizedID {
			return route, true
		}
	}
	return WorkflowGateRoute{}, false
}

func describeGateRouteTarget(run *WorkflowRun, route WorkflowGateRoute) string {
	switch route.Target.Kind {
	case WorkflowGateRouteTargetNextStep:
		return "continue with the next pending step"
	case WorkflowGateRouteTargetCompletePhase:
		return "continue after completing the current phase"
	case WorkflowGateRouteTargetStep:
		label := strings.TrimSpace(route.Target.StepID)
		if phaseIndex, stepIndex, ok := findWorkflowStepByID(run, route.Target.StepID); ok &&
			phaseIndex >= 0 &&
			phaseIndex < len(run.Phases) &&
			stepIndex >= 0 &&
			stepIndex < len(run.Phases[phaseIndex].Steps) {
			step := run.Phases[phaseIndex].Steps[stepIndex]
			label = firstNonEmpty(strings.TrimSpace(step.Name), strings.TrimSpace(step.ID))
		}
		return "jump to step " + label
	default:
		return "continue with the selected route"
	}
}

func findWorkflowStepByID(run *WorkflowRun, stepID string) (phaseIndex int, stepIndex int, ok bool) {
	if run == nil {
		return 0, 0, false
	}
	normalizedID := strings.TrimSpace(stepID)
	if normalizedID == "" {
		return 0, 0, false
	}
	for pIndex, phase := range run.Phases {
		for sIndex, step := range phase.Steps {
			if strings.TrimSpace(step.ID) == normalizedID {
				return pIndex, sIndex, true
			}
		}
	}
	return 0, 0, false
}

func compareWorkflowStepLocations(leftPhase, leftStep, rightPhase, rightStep int) int {
	switch {
	case leftPhase < rightPhase:
		return -1
	case leftPhase > rightPhase:
		return 1
	case leftStep < rightStep:
		return -1
	case leftStep > rightStep:
		return 1
	default:
		return 0
	}
}

func (s *InMemoryRunService) applyGateContinuationRouteLocked(
	run *WorkflowRun,
	phase *PhaseRun,
	gate *WorkflowGateRun,
	now time.Time,
	resolution GateResolution,
) GateResolution {
	if s == nil || run == nil || phase == nil || gate == nil || resolution.Outcome != GateOutcomeContinue {
		return resolution
	}
	selectedRouteID := strings.TrimSpace(resolution.SelectedRouteID)
	if selectedRouteID == "" {
		return resolution
	}
	route, err := resolveSelectedGateRoute(gate, selectedRouteID)
	if err != nil {
		return gateRouteFailureResolution(resolution, err.Error())
	}
	plan, err := planWorkflowGateRoute(run, route)
	if err != nil {
		return gateRouteFailureResolution(resolution, err.Error())
	}
	if err := s.applyWorkflowGateRoutePlanLocked(run, phase, gate, plan, now); err != nil {
		return gateRouteFailureResolution(resolution, err.Error())
	}
	resolution.SelectedRouteID = route.ID
	return resolution
}

func resolveSelectedGateRoute(gate *WorkflowGateRun, selectedRouteID string) (WorkflowGateRoute, error) {
	if gate == nil {
		return WorkflowGateRoute{}, fmt.Errorf("selected route %q could not be resolved: gate context is unavailable", strings.TrimSpace(selectedRouteID))
	}
	route, ok := findGateRouteByID(gate, selectedRouteID)
	if !ok {
		return WorkflowGateRoute{}, fmt.Errorf("gate selected undeclared route %q", strings.TrimSpace(selectedRouteID))
	}
	return route, nil
}

func gateRouteFailureResolution(resolution GateResolution, summary string) GateResolution {
	resolution.Outcome = GateOutcomePause
	resolution.Status = WorkflowGateStatusFailed
	resolution.Summary = strings.TrimSpace(summary)
	resolution.ReasonCode = reasonGateLLMJudgeInvalidRoute
	return resolution
}

func planWorkflowGateRoute(
	run *WorkflowRun,
	route WorkflowGateRoute,
) (workflowGateRoutePlan, error) {
	plan := workflowGateRoutePlan{route: route}
	if run == nil {
		return workflowGateRoutePlan{}, fmt.Errorf("selected route %q could not be applied: run context is unavailable", strings.TrimSpace(route.ID))
	}
	switch route.Target.Kind {
	case WorkflowGateRouteTargetNextStep, WorkflowGateRouteTargetCompletePhase:
		plan.recordSelectionOnly = true
		return plan, nil
	case WorkflowGateRouteTargetStep:
		nextPhaseIndex, nextStepIndex, hasPending := findNextPending(run)
		if !hasPending {
			return workflowGateRoutePlan{}, fmt.Errorf("selected route %q could not be applied: no pending step remains", strings.TrimSpace(route.ID))
		}
		targetPhaseIndex, targetStepIndex, ok := findWorkflowStepByID(run, route.Target.StepID)
		if !ok {
			return workflowGateRoutePlan{}, fmt.Errorf("selected route %q could not be applied: target step %q was not found", strings.TrimSpace(route.ID), strings.TrimSpace(route.Target.StepID))
		}
		if compareWorkflowStepLocations(targetPhaseIndex, targetStepIndex, nextPhaseIndex, nextStepIndex) < 0 {
			return workflowGateRoutePlan{}, fmt.Errorf("selected route %q could not be applied: target step %q is behind the current continuation point", strings.TrimSpace(route.ID), strings.TrimSpace(route.Target.StepID))
		}
		targetStep := run.Phases[targetPhaseIndex].Steps[targetStepIndex]
		if targetStep.Status != StepRunStatusPending {
			return workflowGateRoutePlan{}, fmt.Errorf("selected route %q could not be applied: target step %q is not pending", strings.TrimSpace(route.ID), strings.TrimSpace(route.Target.StepID))
		}
		plan.skipLocations = make([]workflowStepLocation, 0)
		for pIndex := nextPhaseIndex; pIndex <= targetPhaseIndex; pIndex++ {
			startStepIndex := 0
			if pIndex == nextPhaseIndex {
				startStepIndex = nextStepIndex
			}
			endStepIndex := len(run.Phases[pIndex].Steps)
			if pIndex == targetPhaseIndex {
				endStepIndex = targetStepIndex
			}
			for sIndex := startStepIndex; sIndex < endStepIndex; sIndex++ {
				step := run.Phases[pIndex].Steps[sIndex]
				if step.Status != StepRunStatusPending {
					return workflowGateRoutePlan{}, fmt.Errorf("selected route %q could not be applied: step %q is not pending", strings.TrimSpace(route.ID), strings.TrimSpace(step.ID))
				}
				plan.skipLocations = append(plan.skipLocations, workflowStepLocation{phaseIndex: pIndex, stepIndex: sIndex})
			}
		}
		plan.finalizePhaseIndexes = make([]int, 0, targetPhaseIndex-nextPhaseIndex+1)
		for pIndex := nextPhaseIndex; pIndex <= targetPhaseIndex; pIndex++ {
			plan.finalizePhaseIndexes = append(plan.finalizePhaseIndexes, pIndex)
		}
		plan.continuation = workflowStepLocation{phaseIndex: targetPhaseIndex, stepIndex: targetStepIndex}
		return plan, nil
	default:
		return workflowGateRoutePlan{}, fmt.Errorf("selected route %q could not be applied: target kind %q is not supported", strings.TrimSpace(route.ID), strings.TrimSpace(string(route.Target.Kind)))
	}
}

func (s *InMemoryRunService) applyWorkflowGateRoutePlanLocked(
	run *WorkflowRun,
	phase *PhaseRun,
	gate *WorkflowGateRun,
	plan workflowGateRoutePlan,
	now time.Time,
) error {
	if s == nil || run == nil || phase == nil || gate == nil {
		return fmt.Errorf("selected route %q could not be applied: gate context is unavailable", strings.TrimSpace(plan.route.ID))
	}
	s.recordGateRouteSelectionLocked(run, phase, gate, plan.route, now)
	if plan.recordSelectionOnly {
		return nil
	}
	for _, location := range plan.skipLocations {
		s.markStepSkippedByGateRouteLocked(run, &run.Phases[location.phaseIndex], &run.Phases[location.phaseIndex].Steps[location.stepIndex], gate, plan.route, now)
	}
	for _, phaseIndex := range plan.finalizePhaseIndexes {
		s.finalizePhaseAfterGateRouteLocked(run, &run.Phases[phaseIndex], gate, plan.route, now)
	}
	run.CurrentPhaseIndex = plan.continuation.phaseIndex
	run.CurrentStepIndex = plan.continuation.stepIndex
	return nil
}

func (s *InMemoryRunService) recordGateRouteSelectionLocked(
	run *WorkflowRun,
	phase *PhaseRun,
	gate *WorkflowGateRun,
	route WorkflowGateRoute,
	now time.Time,
) {
	if s == nil || run == nil || phase == nil || gate == nil {
		return
	}
	message := fmt.Sprintf("selected route %q: %s", strings.TrimSpace(route.ID), describeGateRouteTarget(run, route))
	appendRunAudit(run, RunAuditEntry{
		At:       now,
		Scope:    "gate",
		Action:   "gate_route_selected",
		PhaseID:  phase.ID,
		GateID:   gate.ID,
		GateKind: gate.Kind,
		Boundary: gate.Boundary.Boundary,
		Outcome:  strings.TrimSpace(route.ID),
		Detail:   message,
	})
	s.appendTimelineEventLocked(run.ID, RunTimelineEvent{
		At:       now,
		Type:     "gate_route_selected",
		RunID:    run.ID,
		PhaseID:  phase.ID,
		GateID:   gate.ID,
		GateKind: gate.Kind,
		Boundary: gate.Boundary.Boundary,
		Message:  message,
	})
}

func (s *InMemoryRunService) markStepSkippedByGateRouteLocked(
	run *WorkflowRun,
	phase *PhaseRun,
	step *StepRun,
	gate *WorkflowGateRun,
	route WorkflowGateRoute,
	now time.Time,
) {
	if s == nil || run == nil || phase == nil || step == nil || gate == nil {
		return
	}
	step.Status = StepRunStatusCompleted
	step.AwaitingTurn = false
	step.CompletedAt = &now
	step.Error = ""
	step.Outcome = "skipped"
	step.ExecutionMessage = "skipped by gate route"
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "step",
		Action:  "step_skipped_by_gate_route",
		PhaseID: phase.ID,
		StepID:  step.ID,
		Outcome: "skipped",
		Detail:  fmt.Sprintf("skipped by gate %q via route %q", strings.TrimSpace(gate.ID), strings.TrimSpace(route.ID)),
	})
	s.appendTimelineEventLocked(run.ID, RunTimelineEvent{
		At:      now,
		Type:    "step_skipped_by_gate_route",
		RunID:   run.ID,
		PhaseID: phase.ID,
		StepID:  step.ID,
		Message: fmt.Sprintf("skipped by route %q", strings.TrimSpace(route.ID)),
	})
}

func (s *InMemoryRunService) finalizePhaseAfterGateRouteLocked(
	run *WorkflowRun,
	phase *PhaseRun,
	gate *WorkflowGateRun,
	route WorkflowGateRoute,
	now time.Time,
) {
	if s == nil || run == nil || phase == nil || gate == nil {
		return
	}
	if !phaseComplete(phase) || phase.Status == PhaseRunStatusCompleted {
		return
	}
	phase.Status = PhaseRunStatusCompleted
	phase.CompletedAt = &now
	appendRunAudit(run, RunAuditEntry{
		At:      now,
		Scope:   "phase",
		Action:  "phase_completed_by_gate_route",
		PhaseID: phase.ID,
		Outcome: "success",
		Detail:  fmt.Sprintf("completed by gate %q via route %q", strings.TrimSpace(gate.ID), strings.TrimSpace(route.ID)),
	})
	s.appendTimelineEventLocked(run.ID, RunTimelineEvent{
		At:      now,
		Type:    "phase_completed_by_gate_route",
		RunID:   run.ID,
		PhaseID: phase.ID,
		Message: fmt.Sprintf("completed by route %q", strings.TrimSpace(route.ID)),
	})
}
