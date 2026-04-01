package guidedworkflows

import (
	"context"
	"fmt"
	"strings"
)

const (
	reasonGateManualReviewRequired   = "gate_manual_review_required"
	reasonGateLLMJudgeFailed         = "gate_llm_judge_failed"
	reasonGateLLMJudgeInvalidOutput  = "gate_llm_judge_invalid_output"
	reasonGateLLMJudgeInvalidRoute   = "gate_llm_judge_invalid_route"
	reasonGateLLMJudgeRuntimeFailure = "gate_llm_judge_runtime_failure"
)

type GateCoordinator interface {
	HasActiveGate(run *WorkflowRun) bool
	HasDeferredGate(run *WorkflowRun) bool
	ResolvePendingGate(ctx context.Context, run *WorkflowRun) (GateResolution, error)
	ResolveSignal(ctx context.Context, run *WorkflowRun, signal GateSignal) (GateResolution, error)
}

type GateStarter interface {
	Kind() WorkflowGateKind
	Start(ctx context.Context, input GateStartInput) GateStartResult
}

type GateSignalHandler interface {
	Kind() WorkflowGateKind
	HandleSignal(ctx context.Context, input GateSignalInput) GateSignalResult
}

type GateStartInput struct {
	RunID   string
	PhaseID string
	Run     WorkflowRun
	Phase   PhaseRun
	Gate    WorkflowGateRun
}

type GateSignalInput struct {
	RunID   string
	PhaseID string
	Run     WorkflowRun
	Phase   PhaseRun
	Gate    WorkflowGateRun
	Signal  GateSignal
}

type GateStartResult struct {
	Outcome         GateOutcome
	Status          WorkflowGateStatus
	Summary         string
	ReasonCode      string
	DispatchPrompt  string
	SelectedRouteID string
}

type GateSignalResult struct {
	Consumed        bool
	Outcome         GateOutcome
	Status          WorkflowGateStatus
	Summary         string
	ReasonCode      string
	SelectedRouteID string
	IgnoreReason    string
}

type GateResolution struct {
	Consumed        bool
	PhaseIndex      int
	GateIndex       int
	GateID          string
	GateKind        WorkflowGateKind
	Boundary        WorkflowGateBoundary
	Outcome         GateOutcome
	Status          WorkflowGateStatus
	Summary         string
	ReasonCode      string
	DispatchPrompt  string
	SelectedRouteID string
	IgnoreReason    string
}

type defaultGateCoordinator struct {
	starters       map[WorkflowGateKind]GateStarter
	signalHandlers map[WorkflowGateKind]GateSignalHandler
}

func NewGateCoordinator(starters ...GateStarter) GateCoordinator {
	byKind := map[WorkflowGateKind]GateStarter{}
	signalByKind := map[WorkflowGateKind]GateSignalHandler{}
	all := starters
	if len(all) == 0 {
		all = []GateStarter{
			manualReviewGateHandler{},
			llmJudgeGateHandler{},
		}
	}
	for _, starter := range all {
		if starter == nil {
			continue
		}
		kind := starter.Kind()
		if kind == "" {
			continue
		}
		byKind[kind] = starter
		if signalHandler, ok := starter.(GateSignalHandler); ok {
			signalByKind[kind] = signalHandler
		}
	}
	return &defaultGateCoordinator{
		starters:       byKind,
		signalHandlers: signalByKind,
	}
}

func (c *defaultGateCoordinator) HasActiveGate(run *WorkflowRun) bool {
	_, _, _, ok := findActiveGate(run)
	return ok
}

func (c *defaultGateCoordinator) HasDeferredGate(run *WorkflowRun) bool {
	if run == nil {
		return false
	}
	for _, phase := range run.Phases {
		for _, gate := range phase.Gates {
			if gate.Status == WorkflowGateStatusWaitingDispatch {
				return true
			}
		}
	}
	return false
}

func (c *defaultGateCoordinator) ResolvePendingGate(ctx context.Context, run *WorkflowRun) (GateResolution, error) {
	phaseIndex, gateIndex, gate, ok := findPendingGate(run)
	if !ok {
		return GateResolution{}, nil
	}
	starter := c.starterFor(gate.Kind)
	if starter == nil {
		return GateResolution{
			Consumed:   true,
			PhaseIndex: phaseIndex,
			GateIndex:  gateIndex,
			GateID:     gate.ID,
			GateKind:   gate.Kind,
			Boundary:   gate.Boundary.Boundary,
		}, fmt.Errorf("%w: no gate handler for kind %q", ErrInvalidTransition, gate.Kind)
	}
	phase := run.Phases[phaseIndex]
	start := starter.Start(ctx, GateStartInput{
		RunID:   strings.TrimSpace(run.ID),
		PhaseID: strings.TrimSpace(phase.ID),
		Run:     *cloneWorkflowRun(run),
		Phase:   phase,
		Gate:    gate,
	})
	outcome, ok := normalizeGateOutcome(start.Outcome)
	if !ok {
		return GateResolution{
			Consumed:   true,
			PhaseIndex: phaseIndex,
			GateIndex:  gateIndex,
			GateID:     gate.ID,
			GateKind:   gate.Kind,
			Boundary:   gate.Boundary.Boundary,
		}, fmt.Errorf("%w: invalid gate outcome %q for kind %q", ErrInvalidTransition, start.Outcome, gate.Kind)
	}
	status, err := normalizeWorkflowGateStatus(start.Status, outcome)
	if err != nil {
		return GateResolution{
			Consumed:   true,
			PhaseIndex: phaseIndex,
			GateIndex:  gateIndex,
			GateID:     gate.ID,
			GateKind:   gate.Kind,
			Boundary:   gate.Boundary.Boundary,
		}, err
	}
	return GateResolution{
		Consumed:        true,
		PhaseIndex:      phaseIndex,
		GateIndex:       gateIndex,
		GateID:          gate.ID,
		GateKind:        gate.Kind,
		Boundary:        gate.Boundary.Boundary,
		Outcome:         outcome,
		Status:          status,
		Summary:         strings.TrimSpace(start.Summary),
		ReasonCode:      strings.TrimSpace(start.ReasonCode),
		DispatchPrompt:  strings.TrimSpace(start.DispatchPrompt),
		SelectedRouteID: strings.TrimSpace(start.SelectedRouteID),
	}, nil
}

func (c *defaultGateCoordinator) ResolveSignal(ctx context.Context, run *WorkflowRun, signal GateSignal) (GateResolution, error) {
	phaseIndex, gateIndex, gate, ok := findActiveGate(run)
	if !ok {
		return GateResolution{}, nil
	}
	signalHandler := c.signalHandlerFor(gate.Kind)
	if signalHandler == nil {
		return GateResolution{
			Consumed:   true,
			PhaseIndex: phaseIndex,
			GateIndex:  gateIndex,
			GateID:     gate.ID,
			GateKind:   gate.Kind,
			Boundary:   gate.Boundary.Boundary,
		}, fmt.Errorf("%w: gate kind %q does not support signal handling", ErrInvalidTransition, gate.Kind)
	}
	phase := run.Phases[phaseIndex]
	result := signalHandler.HandleSignal(ctx, GateSignalInput{
		RunID:   strings.TrimSpace(run.ID),
		PhaseID: strings.TrimSpace(phase.ID),
		Run:     *cloneWorkflowRun(run),
		Phase:   phase,
		Gate:    gate,
		Signal:  signal,
	})
	if !result.Consumed {
		return GateResolution{
			Consumed:     false,
			PhaseIndex:   phaseIndex,
			GateIndex:    gateIndex,
			GateID:       gate.ID,
			GateKind:     gate.Kind,
			Boundary:     gate.Boundary.Boundary,
			IgnoreReason: strings.TrimSpace(result.IgnoreReason),
		}, nil
	}
	outcome, ok := normalizeGateOutcome(result.Outcome)
	if !ok {
		return GateResolution{
			Consumed:   true,
			PhaseIndex: phaseIndex,
			GateIndex:  gateIndex,
			GateID:     gate.ID,
			GateKind:   gate.Kind,
			Boundary:   gate.Boundary.Boundary,
		}, fmt.Errorf("%w: invalid gate signal outcome %q for kind %q", ErrInvalidTransition, result.Outcome, gate.Kind)
	}
	status, err := normalizeWorkflowGateStatus(result.Status, outcome)
	if err != nil {
		return GateResolution{
			Consumed:   true,
			PhaseIndex: phaseIndex,
			GateIndex:  gateIndex,
			GateID:     gate.ID,
			GateKind:   gate.Kind,
			Boundary:   gate.Boundary.Boundary,
		}, err
	}
	return GateResolution{
		Consumed:        true,
		PhaseIndex:      phaseIndex,
		GateIndex:       gateIndex,
		GateID:          gate.ID,
		GateKind:        gate.Kind,
		Boundary:        gate.Boundary.Boundary,
		Outcome:         outcome,
		Status:          status,
		Summary:         strings.TrimSpace(result.Summary),
		ReasonCode:      strings.TrimSpace(result.ReasonCode),
		SelectedRouteID: strings.TrimSpace(result.SelectedRouteID),
		IgnoreReason:    strings.TrimSpace(result.IgnoreReason),
	}, nil
}

func (c *defaultGateCoordinator) starterFor(kind WorkflowGateKind) GateStarter {
	if c == nil || len(c.starters) == 0 {
		return nil
	}
	return c.starters[kind]
}

func (c *defaultGateCoordinator) signalHandlerFor(kind WorkflowGateKind) GateSignalHandler {
	if c == nil || len(c.signalHandlers) == 0 {
		return nil
	}
	return c.signalHandlers[kind]
}

type manualReviewGateHandler struct{}

func (manualReviewGateHandler) Kind() WorkflowGateKind {
	return WorkflowGateKindManualReview
}

func (manualReviewGateHandler) Start(_ context.Context, input GateStartInput) GateStartResult {
	summary := ""
	if input.Gate.ManualReviewConfig != nil {
		summary = strings.TrimSpace(input.Gate.ManualReviewConfig.Reason)
	}
	if summary == "" {
		summary = "manual review required before continuing"
	}
	return GateStartResult{
		Outcome:    GateOutcomePause,
		Status:     WorkflowGateStatusPaused,
		Summary:    summary,
		ReasonCode: reasonGateManualReviewRequired,
	}
}

func findPendingGate(run *WorkflowRun) (phaseIndex int, gateIndex int, gate WorkflowGateRun, ok bool) {
	if run == nil {
		return 0, 0, WorkflowGateRun{}, false
	}
	for pIndex := range run.Phases {
		phase := &run.Phases[pIndex]
		if phase.Status != PhaseRunStatusCompleted {
			continue
		}
		for gIndex := range phase.Gates {
			candidate := phase.Gates[gIndex]
			if candidate.Boundary.Boundary != WorkflowGateBoundaryPhaseEnd {
				continue
			}
			if candidate.Status != WorkflowGateStatusPending {
				continue
			}
			return pIndex, gIndex, candidate, true
		}
	}
	return 0, 0, WorkflowGateRun{}, false
}

func findActiveGate(run *WorkflowRun) (phaseIndex int, gateIndex int, gate WorkflowGateRun, ok bool) {
	if run == nil {
		return 0, 0, WorkflowGateRun{}, false
	}
	for pIndex := range run.Phases {
		phase := &run.Phases[pIndex]
		for gIndex := range phase.Gates {
			candidate := phase.Gates[gIndex]
			if candidate.Status != WorkflowGateStatusAwaitingSignal {
				continue
			}
			return pIndex, gIndex, candidate, true
		}
	}
	return 0, 0, WorkflowGateRun{}, false
}

func normalizeGateOutcome(raw GateOutcome) (GateOutcome, bool) {
	switch strings.ToLower(strings.TrimSpace(string(raw))) {
	case string(GateOutcomeContinue):
		return GateOutcomeContinue, true
	case string(GateOutcomePause):
		return GateOutcomePause, true
	case string(GateOutcomeAwaiting):
		return GateOutcomeAwaiting, true
	default:
		return "", false
	}
}

func normalizeWorkflowGateStatus(raw WorkflowGateStatus, outcome GateOutcome) (WorkflowGateStatus, error) {
	trimmedRaw := strings.TrimSpace(string(raw))
	if trimmedRaw == "" {
		switch outcome {
		case GateOutcomePause:
			return WorkflowGateStatusPaused, nil
		case GateOutcomeAwaiting:
			return WorkflowGateStatusAwaitingSignal, nil
		case GateOutcomeContinue:
			return WorkflowGateStatusPassed, nil
		default:
			return "", fmt.Errorf("%w: cannot infer gate status for outcome %q", ErrInvalidTransition, outcome)
		}
	}
	switch WorkflowGateStatus(trimmedRaw) {
	case WorkflowGateStatusPending,
		WorkflowGateStatusAwaitingSignal,
		WorkflowGateStatusPassed,
		WorkflowGateStatusPaused,
		WorkflowGateStatusFailed,
		WorkflowGateStatusWaitingDispatch,
		WorkflowGateStatusStopped:
		return WorkflowGateStatus(trimmedRaw), nil
	default:
		return "", fmt.Errorf("%w: invalid gate status %q", ErrInvalidTransition, trimmedRaw)
	}
}
