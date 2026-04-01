package guidedworkflows

import (
	"context"
	"strings"
	"testing"
)

type starterOnlyStub struct {
	kind   WorkflowGateKind
	result GateStartResult
}

func (s starterOnlyStub) Kind() WorkflowGateKind {
	return s.kind
}

func (s starterOnlyStub) Start(context.Context, GateStartInput) GateStartResult {
	return s.result
}

type asyncHandlerStub struct {
	starterOnlyStub
	signalResult GateSignalResult
}

func (s asyncHandlerStub) HandleSignal(context.Context, GateSignalInput) GateSignalResult {
	return s.signalResult
}

func pendingGateRun(kind WorkflowGateKind) *WorkflowRun {
	return &WorkflowRun{
		ID: "run-1",
		Phases: []PhaseRun{
			{
				ID:     "phase-1",
				Status: PhaseRunStatusCompleted,
				Gates: []WorkflowGateRun{
					{
						ID:     "gate-1",
						Kind:   kind,
						Status: WorkflowGateStatusPending,
						Boundary: WorkflowGateBoundaryRef{
							Boundary: WorkflowGateBoundaryPhaseEnd,
							PhaseID:  "phase-1",
						},
					},
				},
			},
		},
	}
}

func activeGateRun(kind WorkflowGateKind) *WorkflowRun {
	run := pendingGateRun(kind)
	run.Phases[0].Gates[0].Status = WorkflowGateStatusAwaitingSignal
	run.Phases[0].Gates[0].SignalID = "signal-1"
	return run
}

func TestGateCoordinatorResolvePendingGateRejectsUnknownOutcome(t *testing.T) {
	coordinator := NewGateCoordinator(starterOnlyStub{
		kind: WorkflowGateKindManualReview,
		result: GateStartResult{
			Outcome: GateOutcome("mystery"),
		},
	})
	_, err := coordinator.ResolvePendingGate(context.Background(), pendingGateRun(WorkflowGateKindManualReview))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid gate outcome") {
		t.Fatalf("expected invalid gate outcome error, got %v", err)
	}
}

func TestGateCoordinatorResolvePendingGateRejectsUnknownStatus(t *testing.T) {
	coordinator := NewGateCoordinator(starterOnlyStub{
		kind: WorkflowGateKindManualReview,
		result: GateStartResult{
			Outcome: GateOutcomePause,
			Status:  WorkflowGateStatus("nonsense"),
		},
	})
	_, err := coordinator.ResolvePendingGate(context.Background(), pendingGateRun(WorkflowGateKindManualReview))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid gate status") {
		t.Fatalf("expected invalid gate status error, got %v", err)
	}
}

func TestGateCoordinatorResolveSignalRejectsUnsupportedSignalHandler(t *testing.T) {
	coordinator := NewGateCoordinator(manualReviewGateHandler{})
	_, err := coordinator.ResolveSignal(context.Background(), activeGateRun(WorkflowGateKindManualReview), GateSignal{
		SignalID: "signal-1",
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "does not support signal handling") {
		t.Fatalf("expected unsupported signal handler error, got %v", err)
	}
}

func TestGateCoordinatorResolveSignalRejectsUnknownSignalOutcome(t *testing.T) {
	handler := asyncHandlerStub{
		starterOnlyStub: starterOnlyStub{
			kind: WorkflowGateKindLLMJudge,
			result: GateStartResult{
				Outcome: GateOutcomeAwaiting,
			},
		},
		signalResult: GateSignalResult{
			Consumed: true,
			Outcome:  GateOutcome("mystery"),
		},
	}
	coordinator := NewGateCoordinator(handler)
	_, err := coordinator.ResolveSignal(context.Background(), activeGateRun(WorkflowGateKindLLMJudge), GateSignal{
		SignalID: "signal-1",
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid gate signal outcome") {
		t.Fatalf("expected invalid gate signal outcome error, got %v", err)
	}
}

func TestGateCoordinatorHasActiveAndDeferredGate(t *testing.T) {
	coordinator := NewGateCoordinator()
	if coordinator.HasActiveGate(nil) {
		t.Fatalf("expected nil run to report no active gate")
	}
	if coordinator.HasDeferredGate(nil) {
		t.Fatalf("expected nil run to report no deferred gate")
	}

	run := pendingGateRun(WorkflowGateKindManualReview)
	if coordinator.HasActiveGate(run) {
		t.Fatalf("expected pending gate to report no active gate")
	}
	if coordinator.HasDeferredGate(run) {
		t.Fatalf("expected pending gate to report no deferred gate")
	}

	run.Phases[0].Gates[0].Status = WorkflowGateStatusAwaitingSignal
	if !coordinator.HasActiveGate(run) {
		t.Fatalf("expected awaiting gate to report active")
	}

	run.Phases[0].Gates[0].Status = WorkflowGateStatusWaitingDispatch
	if !coordinator.HasDeferredGate(run) {
		t.Fatalf("expected waiting dispatch gate to report deferred")
	}
}

func TestNewGateCoordinatorSkipsNilAndBlankKindStarters(t *testing.T) {
	coordinator := NewGateCoordinator(
		nil,
		starterOnlyStub{kind: "", result: GateStartResult{Outcome: GateOutcomePause}},
		manualReviewGateHandler{},
	)
	resolution, err := coordinator.ResolvePendingGate(context.Background(), pendingGateRun(WorkflowGateKindManualReview))
	if err != nil {
		t.Fatalf("ResolvePendingGate: %v", err)
	}
	if !resolution.Consumed || resolution.GateKind != WorkflowGateKindManualReview {
		t.Fatalf("expected manual review starter to be used, got %#v", resolution)
	}
}

func TestGateCoordinatorResolvePendingGateRejectsMissingHandler(t *testing.T) {
	coordinator := NewGateCoordinator(manualReviewGateHandler{})
	run := pendingGateRun(WorkflowGateKind("script"))
	resolution, err := coordinator.ResolvePendingGate(context.Background(), run)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "no gate handler") {
		t.Fatalf("expected missing handler error, got %v", err)
	}
	if resolution.GateID != "gate-1" || resolution.GateKind != WorkflowGateKind("script") {
		t.Fatalf("expected metadata to be preserved in error resolution, got %#v", resolution)
	}
}

func TestGateCoordinatorResolveSignalRejectsUnknownSignalStatus(t *testing.T) {
	handler := asyncHandlerStub{
		starterOnlyStub: starterOnlyStub{
			kind: WorkflowGateKindLLMJudge,
			result: GateStartResult{
				Outcome: GateOutcomeAwaiting,
			},
		},
		signalResult: GateSignalResult{
			Consumed: true,
			Outcome:  GateOutcomePause,
			Status:   WorkflowGateStatus("unexpected"),
		},
	}
	coordinator := NewGateCoordinator(handler)
	_, err := coordinator.ResolveSignal(context.Background(), activeGateRun(WorkflowGateKindLLMJudge), GateSignal{
		SignalID: "signal-1",
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "invalid gate status") {
		t.Fatalf("expected invalid gate status error, got %v", err)
	}
}

func TestManualReviewGateHandlerDefaultsReasonSummary(t *testing.T) {
	result := manualReviewGateHandler{}.Start(context.Background(), GateStartInput{
		Gate: WorkflowGateRun{},
	})
	if result.Summary != "manual review required before continuing" {
		t.Fatalf("expected default summary, got %q", result.Summary)
	}
	if result.Outcome != GateOutcomePause || result.Status != WorkflowGateStatusPaused {
		t.Fatalf("expected pause+paused outcome, got %#v", result)
	}
}

func TestLLMJudgeGateHandlerSignalRuntimeFailure(t *testing.T) {
	result := llmJudgeGateHandler{}.HandleSignal(context.Background(), GateSignalInput{
		Gate: WorkflowGateRun{SignalID: "signal-1"},
		Signal: GateSignal{
			SignalID: "signal-1",
			Status:   "failed",
			Error:    "transport unavailable",
			Terminal: true,
		},
	})
	if !result.Consumed || result.Outcome != GateOutcomePause || result.Status != WorkflowGateStatusFailed {
		t.Fatalf("expected consumed failed pause result, got %#v", result)
	}
	if result.ReasonCode != reasonGateLLMJudgeRuntimeFailure {
		t.Fatalf("expected runtime failure reason code, got %q", result.ReasonCode)
	}
}

func TestLLMJudgeGateHandlerSignalEmptyOutputFailsClosed(t *testing.T) {
	result := llmJudgeGateHandler{}.HandleSignal(context.Background(), GateSignalInput{
		Gate: WorkflowGateRun{SignalID: "signal-1"},
		Signal: GateSignal{
			SignalID: "signal-1",
			Status:   "completed",
			Terminal: true,
			Output:   "  ",
		},
	})
	if result.ReasonCode != reasonGateLLMJudgeInvalidOutput || result.Outcome != GateOutcomePause {
		t.Fatalf("expected invalid output pause, got %#v", result)
	}
}

func TestLLMJudgeGateHandlerSignalIgnoresSignalIDMismatch(t *testing.T) {
	result := llmJudgeGateHandler{}.HandleSignal(context.Background(), GateSignalInput{
		Gate: WorkflowGateRun{SignalID: "signal-1"},
		Signal: GateSignal{
			SignalID: "signal-2",
			Status:   "completed",
			Terminal: true,
			Output:   `{"passed": true, "reason": "ok"}`,
		},
	})
	if result.Consumed {
		t.Fatalf("expected mismatched signal to be ignored, got %#v", result)
	}
	if !strings.Contains(result.IgnoreReason, "signal_id mismatch") {
		t.Fatalf("expected signal mismatch ignore reason, got %#v", result)
	}
}

func TestLLMJudgeGateHandlerSignalIgnoresMissingSignalIDWhenExpected(t *testing.T) {
	result := llmJudgeGateHandler{}.HandleSignal(context.Background(), GateSignalInput{
		Gate: WorkflowGateRun{SignalID: "signal-1"},
		Signal: GateSignal{
			Status:   "completed",
			Terminal: true,
			Output:   `{"passed": true, "reason": "ok"}`,
		},
	})
	if result.Consumed {
		t.Fatalf("expected missing signal id to be ignored, got %#v", result)
	}
	if !strings.Contains(result.IgnoreReason, "missing signal_id") {
		t.Fatalf("expected missing signal id ignore reason, got %#v", result)
	}
}

func TestLLMJudgeGateHandlerSignalDefaultsSummaryWhenReasonMissing(t *testing.T) {
	pass := llmJudgeGateHandler{}.HandleSignal(context.Background(), GateSignalInput{
		Gate: WorkflowGateRun{SignalID: "signal-1"},
		Signal: GateSignal{
			SignalID: "signal-1",
			Status:   "completed",
			Terminal: true,
			Output:   `{"passed": true, "reason": ""}`,
		},
	})
	if pass.Summary != "llm_judge passed" || pass.Outcome != GateOutcomeContinue {
		t.Fatalf("expected pass summary fallback, got %#v", pass)
	}

	fail := llmJudgeGateHandler{}.HandleSignal(context.Background(), GateSignalInput{
		Gate: WorkflowGateRun{SignalID: "signal-1"},
		Signal: GateSignal{
			SignalID: "signal-1",
			Status:   "completed",
			Terminal: true,
			Output:   `{"passed": false, "reason": ""}`,
		},
	})
	if fail.Summary != "llm_judge rejected the phase" || fail.Outcome != GateOutcomePause {
		t.Fatalf("expected fail summary fallback, got %#v", fail)
	}
}

func TestLLMJudgeGateHandlerSignalPassesSelectedRouteID(t *testing.T) {
	result := llmJudgeGateHandler{}.HandleSignal(context.Background(), GateSignalInput{
		Gate: WorkflowGateRun{SignalID: "signal-1"},
		Signal: GateSignal{
			SignalID: "signal-1",
			Status:   "completed",
			Terminal: true,
			Output:   `{"passed": true, "reason": "phase looks good", "route": "skip_validation"}`,
		},
	})
	if !result.Consumed || result.Outcome != GateOutcomeContinue || result.Status != WorkflowGateStatusPassed {
		t.Fatalf("expected successful gate result, got %#v", result)
	}
	if result.SelectedRouteID != "skip_validation" {
		t.Fatalf("expected selected route id to propagate, got %#v", result)
	}
}

func TestLLMJudgeGateHandlerSignalTreatsBlankRouteAsNoSelection(t *testing.T) {
	result := llmJudgeGateHandler{}.HandleSignal(context.Background(), GateSignalInput{
		Gate: WorkflowGateRun{SignalID: "signal-1"},
		Signal: GateSignal{
			SignalID: "signal-1",
			Status:   "completed",
			Terminal: true,
			Output:   `{"passed": true, "reason": "phase looks good", "route": "   "}`,
		},
	})
	if !result.Consumed || result.Outcome != GateOutcomeContinue || result.Status != WorkflowGateStatusPassed {
		t.Fatalf("expected successful gate result, got %#v", result)
	}
	if result.SelectedRouteID != "" {
		t.Fatalf("expected blank route to normalize to no selection, got %#v", result)
	}
}

func TestLLMJudgeGateHandlerRejectsRouteOnFailedDecision(t *testing.T) {
	result := llmJudgeGateHandler{}.HandleSignal(context.Background(), GateSignalInput{
		Gate: WorkflowGateRun{SignalID: "signal-1"},
		Signal: GateSignal{
			SignalID: "signal-1",
			Status:   "completed",
			Terminal: true,
			Output:   `{"passed": false, "reason": "needs work", "route": "skip_validation"}`,
		},
	})
	if !result.Consumed || result.Outcome != GateOutcomePause || result.Status != WorkflowGateStatusFailed {
		t.Fatalf("expected invalid-output pause result, got %#v", result)
	}
	if result.ReasonCode != reasonGateLLMJudgeInvalidOutput {
		t.Fatalf("expected invalid output reason code, got %#v", result)
	}
}

func TestComposeLLMJudgeDispatchPromptIncludesStepError(t *testing.T) {
	prompt := composeLLMJudgeDispatchPrompt(
		WorkflowRun{TemplateName: "Template"},
		PhaseRun{
			Name: "Phase",
			Steps: []StepRun{
				{
					ID:      "step-1",
					Name:    "Step 1",
					Status:  StepRunStatusFailed,
					Outcome: "failed",
					Error:   "compile failed",
				},
			},
		},
		WorkflowGateRun{
			LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge this"},
		},
	)
	if !strings.Contains(prompt, "Error: compile failed") {
		t.Fatalf("expected step error evidence in prompt, got %q", prompt)
	}
}

func TestComposeLLMJudgeDispatchPromptIncludesRouteSchemaAndAllowedRoutes(t *testing.T) {
	prompt := composeLLMJudgeDispatchPrompt(
		WorkflowRun{
			TemplateName: "Template",
			Phases: []PhaseRun{
				{
					ID:   "phase-2",
					Name: "Phase 2",
					Steps: []StepRun{
						{ID: "step-2", Name: "Validation"},
					},
				},
			},
		},
		PhaseRun{Name: "Phase"},
		WorkflowGateRun{
			Routes: []WorkflowGateRoute{
				{ID: "skip_validation", Target: WorkflowGateRouteTargetRef{Kind: WorkflowGateRouteTargetStep, StepID: "step-2"}},
			},
			LLMJudgeConfig: &LLMJudgeConfig{Prompt: "judge this"},
		},
	)
	if !strings.Contains(prompt, `"route": "optional_route_id"`) {
		t.Fatalf("expected prompt schema to include optional route field, got %q", prompt)
	}
	if !strings.Contains(prompt, "Allowed routes:") || !strings.Contains(prompt, "skip_validation") {
		t.Fatalf("expected allowed routes in prompt, got %q", prompt)
	}
}

func TestParseLLMJudgeResponseHandlesEmptyInvalidAndCodeFence(t *testing.T) {
	if _, ok := parseLLMJudgeResponse(""); ok {
		t.Fatalf("expected empty payload parsing to fail")
	}
	if _, ok := parseLLMJudgeResponse("not-json"); ok {
		t.Fatalf("expected non-json parsing to fail")
	}
	parsed, ok := parseLLMJudgeResponse("```json\n{\"passed\": true, \"reason\": \"ok\"}\n```")
	if !ok || parsed.Passed == nil || !*parsed.Passed || parsed.Reason != "ok" {
		t.Fatalf("expected fenced payload parse success, got %#v ok=%v", parsed, ok)
	}
	parsed, ok = parseLLMJudgeResponse(`{"passed": true, "reason": "ok", "route": "skip_validation"}`)
	if !ok || parsed.Route != "skip_validation" {
		t.Fatalf("expected route field to parse, got %#v ok=%v", parsed, ok)
	}
}

func TestNormalizeWorkflowGateStatusInfersFromOutcomeAndRejectsUnknown(t *testing.T) {
	if status, err := normalizeWorkflowGateStatus("", GateOutcomePause); err != nil || status != WorkflowGateStatusPaused {
		t.Fatalf("expected paused inference, got status=%q err=%v", status, err)
	}
	if status, err := normalizeWorkflowGateStatus("", GateOutcomeAwaiting); err != nil || status != WorkflowGateStatusAwaitingSignal {
		t.Fatalf("expected awaiting inference, got status=%q err=%v", status, err)
	}
	if status, err := normalizeWorkflowGateStatus("", GateOutcomeContinue); err != nil || status != WorkflowGateStatusPassed {
		t.Fatalf("expected passed inference, got status=%q err=%v", status, err)
	}
	if _, err := normalizeWorkflowGateStatus("", GateOutcome("unknown")); err == nil {
		t.Fatalf("expected unknown outcome inference to fail")
	}
}
