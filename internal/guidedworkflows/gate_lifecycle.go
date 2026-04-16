package guidedworkflows

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *InMemoryRunService) completeAwaitingGateLocked(ctx context.Context, run *WorkflowRun, gateSignal GateSignal) (bool, error) {
	if s == nil || run == nil {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if phaseIndex, gateIndex, _, ok := findActiveGate(run); ok &&
		phaseIndex >= 0 && phaseIndex < len(run.Phases) &&
		gateIndex >= 0 && gateIndex < len(run.Phases[phaseIndex].Gates) {
		activeGate := &run.Phases[phaseIndex].Gates[gateIndex]
		if !s.gateSignalMatcherOrDefault().Matches(activeGate, gateSignal) {
			s.recordGateSignalIgnored(run, &run.Phases[phaseIndex], activeGate, "signal mismatch while awaiting gate")
			return false, nil
		}
	}
	resolution, err := s.gateCoordinatorOrDefault().ResolveSignal(ctx, run, gateSignal)
	if err != nil {
		return false, err
	}
	if !resolution.Consumed {
		if resolution.IgnoreReason != "" &&
			resolution.PhaseIndex >= 0 &&
			resolution.PhaseIndex < len(run.Phases) &&
			resolution.GateIndex >= 0 &&
			resolution.GateIndex < len(run.Phases[resolution.PhaseIndex].Gates) {
			phase := &run.Phases[resolution.PhaseIndex]
			gate := &phase.Gates[resolution.GateIndex]
			s.recordGateSignalIgnored(run, phase, gate, resolution.IgnoreReason)
		}
		return false, nil
	}
	if resolution.PhaseIndex < 0 || resolution.PhaseIndex >= len(run.Phases) {
		return false, fmt.Errorf("%w: awaiting gate index out of range", ErrInvalidTransition)
	}
	phase := &run.Phases[resolution.PhaseIndex]
	if resolution.GateIndex < 0 || resolution.GateIndex >= len(phase.Gates) {
		return false, fmt.Errorf("%w: awaiting gate index out of range", ErrInvalidTransition)
	}
	gate := &phase.Gates[resolution.GateIndex]
	now := s.engine.now()
	gate.LastSignal = buildGateSignalContext(gateSignal, now)
	if strings.TrimSpace(gateSignal.Output) != "" {
		gate.Output = strings.TrimSpace(gateSignal.Output)
	}
	resolution = s.applyGateContinuationRouteLocked(run, phase, gate, now, resolution)
	switch resolution.Outcome {
	case GateOutcomePause:
		s.applyGatePause(run, phase, gate, gateSignal, now, resolution)
	default:
		s.applyGatePass(run, phase, gate, gateSignal, now, resolution)
	}
	return true, nil
}

func (s *InMemoryRunService) recordGateSignalIgnored(run *WorkflowRun, phase *PhaseRun, gate *WorkflowGateRun, reason string) {
	if s == nil || run == nil || phase == nil || gate == nil {
		return
	}
	now := s.engine.now()
	appendRunAudit(run, RunAuditEntry{
		At:       now,
		Scope:    "gate",
		Action:   "gate_signal_ignored",
		PhaseID:  phase.ID,
		GateID:   gate.ID,
		GateKind: gate.Kind,
		Boundary: gate.Boundary.Boundary,
		Outcome:  "awaiting_signal",
		Detail:   reason,
	})
	s.appendTimelineEventLocked(run.ID, RunTimelineEvent{
		At:       now,
		Type:     "gate_signal_ignored",
		RunID:    run.ID,
		PhaseID:  phase.ID,
		GateID:   gate.ID,
		GateKind: gate.Kind,
		Boundary: gate.Boundary.Boundary,
		Message:  reason,
	})
}

func (s *InMemoryRunService) applyGatePass(
	run *WorkflowRun,
	phase *PhaseRun,
	gate *WorkflowGateRun,
	signal GateSignal,
	now time.Time,
	resolution GateResolution,
) {
	if run == nil || phase == nil || gate == nil {
		return
	}
	gate.Status = WorkflowGateStatusPassed
	gate.CompletedAt = &now
	gate.Error = ""
	gate.Outcome = "passed"
	gate.Summary = firstNonEmpty(resolution.Summary, "gate passed")
	gate.SelectedRouteID = strings.TrimSpace(resolution.SelectedRouteID)
	if signalID := strings.TrimSpace(signal.SignalID); signalID != "" {
		gate.SignalID = signalID
	}
	recordGateExecutionCompletion(run, phase, gate, signal, now)
	appendRunAudit(run, RunAuditEntry{
		At:       now,
		Scope:    "gate",
		Action:   "gate_passed",
		PhaseID:  phase.ID,
		GateID:   gate.ID,
		GateKind: gate.Kind,
		Boundary: gate.Boundary.Boundary,
		Outcome:  gate.Outcome,
		Detail:   gate.Summary,
	})
	s.appendTimelineEventLocked(run.ID, RunTimelineEvent{
		At:       now,
		Type:     "gate_passed",
		RunID:    run.ID,
		PhaseID:  phase.ID,
		GateID:   gate.ID,
		GateKind: gate.Kind,
		Boundary: gate.Boundary.Boundary,
		Message:  gate.Summary,
	})
}

func (s *InMemoryRunService) applyGatePause(
	run *WorkflowRun,
	phase *PhaseRun,
	gate *WorkflowGateRun,
	signal GateSignal,
	now time.Time,
	resolution GateResolution,
) {
	if run == nil || phase == nil || gate == nil {
		return
	}
	summary := firstNonEmpty(resolution.Summary, "gate rejected the phase")
	outcome := "failed"
	reasonCode := strings.TrimSpace(resolution.ReasonCode)
	if reasonCode == "" {
		reasonCode = reasonGateLLMJudgeFailed
	}
	gate.Status = WorkflowGateStatusPaused
	if resolution.Status == WorkflowGateStatusFailed {
		gate.Status = WorkflowGateStatusFailed
	}
	gate.CompletedAt = &now
	gate.Error = summary
	gate.Outcome = outcome
	gate.Summary = summary
	if signalID := strings.TrimSpace(signal.SignalID); signalID != "" {
		gate.SignalID = signalID
	}
	recordGateExecutionCompletion(run, phase, gate, signal, now)
	appendRunAudit(run, RunAuditEntry{
		At:       now,
		Scope:    "gate",
		Action:   "gate_failed",
		PhaseID:  phase.ID,
		GateID:   gate.ID,
		GateKind: gate.Kind,
		Boundary: gate.Boundary.Boundary,
		Outcome:  outcome,
		Detail:   summary,
	})
	s.appendTimelineEventLocked(run.ID, RunTimelineEvent{
		At:       now,
		Type:     "gate_failed",
		RunID:    run.ID,
		PhaseID:  phase.ID,
		GateID:   gate.ID,
		GateKind: gate.Kind,
		Boundary: gate.Boundary.Boundary,
		Message:  summary,
	})
	metadata := CheckpointDecisionMetadata{
		Action:              CheckpointActionPause,
		Reasons:             []CheckpointReason{{Code: reasonCode, Message: summary, HardGate: true}},
		Severity:            DecisionSeverityCritical,
		Tier:                DecisionTier3,
		Style:               run.CheckpointStyle,
		Confidence:          0,
		ConfidenceThreshold: run.Policy.ConfidenceThreshold,
		Score:               1,
		PauseThreshold:      run.Policy.PauseThreshold,
		HardGateTriggered:   true,
		EvaluatedAt:         now.UTC(),
	}
	decision := CheckpointDecision{
		ID:          fmt.Sprintf("cd-%d", len(run.CheckpointDecisions)+1),
		RunID:       run.ID,
		PhaseID:     phase.ID,
		GateID:      gate.ID,
		GateKind:    gate.Kind,
		Boundary:    gate.Boundary.Boundary,
		Decision:    string(CheckpointActionPause),
		Reason:      summary,
		Source:      "gate",
		RequestedAt: now,
		DecidedAt:   &now,
		Metadata:    metadata,
	}
	run.CheckpointDecisions = append(run.CheckpointDecisions, decision)
	copy := decision
	run.LatestDecision = &copy
	run.Status = WorkflowRunStatusPaused
	run.PausedAt = &now
	s.recordPauseLocked()
	s.recordInterventionCauseLocked(reasonCode)
	appendRunAudit(run, RunAuditEntry{
		At:       now,
		Scope:    "decision",
		Action:   "gate_pause",
		PhaseID:  phase.ID,
		GateID:   gate.ID,
		GateKind: gate.Kind,
		Boundary: gate.Boundary.Boundary,
		Outcome:  string(metadata.Severity),
		Detail:   summary,
	})
	s.appendTimelineEventLocked(run.ID, RunTimelineEvent{
		At:       now,
		Type:     "checkpoint_requested",
		RunID:    run.ID,
		PhaseID:  phase.ID,
		GateID:   gate.ID,
		GateKind: gate.Kind,
		Boundary: gate.Boundary.Boundary,
		Message:  summary,
	})
}

func (s *InMemoryRunService) prepareGateDispatchContext(ctx context.Context, run *WorkflowRun) (gateDispatchContext, bool, error) {
	if s == nil || run == nil {
		return gateDispatchContext{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		action, err := s.resolvePendingGateActionLocked(ctx, run)
		dispatchCtx, hasDispatch, continueResolving := s.applyPendingGateActionLocked(run, action)
		if err != nil {
			return gateDispatchContext{}, false, err
		}
		if hasDispatch {
			return dispatchCtx, true, nil
		}
		if !continueResolving {
			return gateDispatchContext{}, false, nil
		}
	}
}

func (s *InMemoryRunService) resolvePendingGateActionLocked(ctx context.Context, run *WorkflowRun) (pendingGateAction, error) {
	if s == nil || run == nil {
		return pendingGateAction{kind: pendingGateActionNone}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return pendingGateAction{kind: pendingGateActionNone}, err
	}
	resolution, err := s.gateCoordinatorOrDefault().ResolvePendingGate(ctx, run)
	if err != nil {
		action := pendingGateAction{kind: pendingGateActionNone}
		if _, _, ok := s.phaseAndGateForResolutionLocked(run, resolution); ok {
			action = pendingGateAction{
				kind:       pendingGateActionFail,
				resolution: resolution,
				failCause:  err,
			}
		}
		return action, err
	}
	if !resolution.Consumed {
		return pendingGateAction{kind: pendingGateActionNone}, nil
	}
	phase, gate, ok := s.phaseAndGateForResolutionLocked(run, resolution)
	if !ok {
		return pendingGateAction{kind: pendingGateActionNone}, nil
	}
	resolution = s.applyGateContinuationRouteLocked(run, phase, gate, s.engine.now(), resolution)
	switch resolution.Outcome {
	case GateOutcomePause:
		return pendingGateAction{kind: pendingGateActionPause, resolution: resolution}, nil
	case GateOutcomeContinue:
		return pendingGateAction{kind: pendingGateActionContinue, resolution: resolution}, nil
	case GateOutcomeAwaiting:
		dispatchCtx, err := s.buildAwaitingGateDispatchContextLocked(run, resolution)
		if err != nil {
			return pendingGateAction{
				kind:       pendingGateActionFail,
				resolution: resolution,
				failCause:  err,
			}, err
		}
		return pendingGateAction{
			kind:       pendingGateActionAwaitingDispatch,
			resolution: resolution,
			dispatch:   dispatchCtx,
		}, nil
	default:
		return pendingGateAction{kind: pendingGateActionNone}, fmt.Errorf("%w: unsupported gate outcome %q", ErrInvalidTransition, resolution.Outcome)
	}
}

func (s *InMemoryRunService) applyPendingGateActionLocked(run *WorkflowRun, action pendingGateAction) (gateDispatchContext, bool, bool) {
	if s == nil || run == nil {
		return gateDispatchContext{}, false, false
	}
	switch action.kind {
	case pendingGateActionNone:
		return gateDispatchContext{}, false, false
	case pendingGateActionFail:
		if _, gate, ok := s.phaseAndGateForResolutionLocked(run, action.resolution); ok {
			s.ensureGateStartedForActionLocked(gate)
			s.failRunForGateDispatchLocked(run, action.resolution.PhaseIndex, action.resolution.GateIndex, action.failCause)
		}
		return gateDispatchContext{}, false, false
	case pendingGateActionPause:
		if phase, gate, ok := s.phaseAndGateForResolutionLocked(run, action.resolution); ok {
			now := s.engine.now()
			s.ensureGateStartedForActionLocked(gate)
			s.applyGatePause(run, phase, gate, GateSignal{}, now, action.resolution)
		}
		return gateDispatchContext{}, false, false
	case pendingGateActionContinue:
		if phase, gate, ok := s.phaseAndGateForResolutionLocked(run, action.resolution); ok {
			now := s.engine.now()
			s.ensureGateStartedForActionLocked(gate)
			s.applyGatePass(run, phase, gate, GateSignal{}, now, action.resolution)
		}
		return gateDispatchContext{}, false, true
	case pendingGateActionAwaitingDispatch:
		if _, gate, ok := s.phaseAndGateForResolutionLocked(run, action.resolution); ok {
			s.ensureGateStartedForActionLocked(gate)
		}
		return action.dispatch, true, false
	default:
		return gateDispatchContext{}, false, false
	}
}

func (s *InMemoryRunService) buildAwaitingGateDispatchContextLocked(run *WorkflowRun, resolution GateResolution) (gateDispatchContext, error) {
	dispatchPrompt := strings.TrimSpace(resolution.DispatchPrompt)
	if dispatchPrompt == "" {
		return gateDispatchContext{}, fmt.Errorf("%w: gate dispatch prompt is empty", ErrGateDispatch)
	}
	if s == nil || s.gateDispatcher == nil {
		return gateDispatchContext{}, fmt.Errorf("%w: gate dispatcher unavailable", ErrGateDispatch)
	}
	return gateDispatchContext{
		runID:          strings.TrimSpace(run.ID),
		phaseIndex:     resolution.PhaseIndex,
		gateIndex:      resolution.GateIndex,
		dispatchPrompt: dispatchPrompt,
		req:            s.buildGateDispatchRequest(run, resolution.PhaseIndex, resolution.GateIndex, dispatchPrompt),
		beforeStatus:   run.Status,
	}, nil
}

func (s *InMemoryRunService) phaseAndGateForResolutionLocked(run *WorkflowRun, resolution GateResolution) (*PhaseRun, *WorkflowGateRun, bool) {
	if run == nil || resolution.PhaseIndex < 0 || resolution.PhaseIndex >= len(run.Phases) {
		return nil, nil, false
	}
	phase := &run.Phases[resolution.PhaseIndex]
	if resolution.GateIndex < 0 || resolution.GateIndex >= len(phase.Gates) {
		return nil, nil, false
	}
	return phase, &phase.Gates[resolution.GateIndex], true
}

func (s *InMemoryRunService) ensureGateStartedForActionLocked(gate *WorkflowGateRun) {
	if s == nil || gate == nil || gate.StartedAt != nil {
		return
	}
	now := s.engine.now()
	gate.StartedAt = &now
}

func (s *InMemoryRunService) canApplyGateDispatchResultLocked(dispatchCtx gateDispatchContext) bool {
	if s == nil {
		return false
	}
	run, ok := s.getRunByIDLocked(dispatchCtx.runID)
	if !ok || run == nil || run.Status != WorkflowRunStatusRunning {
		return false
	}
	if dispatchCtx.phaseIndex < 0 || dispatchCtx.phaseIndex >= len(run.Phases) {
		return false
	}
	phase := &run.Phases[dispatchCtx.phaseIndex]
	if phase.Status != PhaseRunStatusCompleted {
		return false
	}
	if dispatchCtx.gateIndex < 0 || dispatchCtx.gateIndex >= len(phase.Gates) {
		return false
	}
	return phase.Gates[dispatchCtx.gateIndex].Status == WorkflowGateStatusPending
}
