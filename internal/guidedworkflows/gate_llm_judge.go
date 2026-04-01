package guidedworkflows

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type llmJudgeGateHandler struct{}

func (llmJudgeGateHandler) Kind() WorkflowGateKind {
	return WorkflowGateKindLLMJudge
}

func (llmJudgeGateHandler) Start(_ context.Context, input GateStartInput) GateStartResult {
	return GateStartResult{
		Outcome:        GateOutcomeAwaiting,
		Status:         WorkflowGateStatusAwaitingSignal,
		DispatchPrompt: composeLLMJudgeDispatchPrompt(input.Run, input.Phase, input.Gate),
	}
}

func (llmJudgeGateHandler) HandleSignal(_ context.Context, input GateSignalInput) GateSignalResult {
	expectedSignalID := strings.TrimSpace(input.Gate.SignalID)
	if expectedSignalID == "" && input.Gate.Execution != nil {
		expectedSignalID = strings.TrimSpace(input.Gate.Execution.SignalID)
	}
	signalID := strings.TrimSpace(input.Signal.SignalID)
	if expectedSignalID != "" && signalID != expectedSignalID {
		reason := "signal_id mismatch while awaiting gate"
		if signalID == "" {
			reason = "missing signal_id while gate awaits " + expectedSignalID
		}
		return GateSignalResult{
			Consumed:     false,
			IgnoreReason: reason,
		}
	}
	if failure, failed := GateSignalFailureDetail(input.Signal); failed {
		return GateSignalResult{
			Consumed:   true,
			Outcome:    GateOutcomePause,
			Status:     WorkflowGateStatusFailed,
			Summary:    strings.TrimSpace(failure),
			ReasonCode: reasonGateLLMJudgeRuntimeFailure,
		}
	}
	output := strings.TrimSpace(input.Signal.Output)
	if output == "" {
		return GateSignalResult{
			Consumed:   true,
			Outcome:    GateOutcomePause,
			Status:     WorkflowGateStatusFailed,
			Summary:    "llm_judge returned no output",
			ReasonCode: reasonGateLLMJudgeInvalidOutput,
		}
	}
	parsed, ok := parseLLMJudgeResponse(output)
	if !ok || parsed.Passed == nil {
		return GateSignalResult{
			Consumed:   true,
			Outcome:    GateOutcomePause,
			Status:     WorkflowGateStatusFailed,
			Summary:    "llm_judge returned invalid output; expected JSON with passed, reason, and optional route",
			ReasonCode: reasonGateLLMJudgeInvalidOutput,
		}
	}
	selectedRouteID := strings.TrimSpace(parsed.Route)
	if selectedRouteID != "" && !*parsed.Passed {
		return GateSignalResult{
			Consumed:   true,
			Outcome:    GateOutcomePause,
			Status:     WorkflowGateStatusFailed,
			Summary:    "llm_judge returned invalid output; route may only be selected when passed is true",
			ReasonCode: reasonGateLLMJudgeInvalidOutput,
		}
	}
	summary := strings.TrimSpace(parsed.Reason)
	if summary == "" {
		if *parsed.Passed {
			summary = "llm_judge passed"
		} else {
			summary = "llm_judge rejected the phase"
		}
	}
	if *parsed.Passed {
		return GateSignalResult{
			Consumed:        true,
			Outcome:         GateOutcomeContinue,
			Status:          WorkflowGateStatusPassed,
			Summary:         summary,
			SelectedRouteID: selectedRouteID,
		}
	}
	return GateSignalResult{
		Consumed:   true,
		Outcome:    GateOutcomePause,
		Status:     WorkflowGateStatusFailed,
		Summary:    summary,
		ReasonCode: reasonGateLLMJudgeFailed,
	}
}

type llmJudgeResponse struct {
	Passed *bool  `json:"passed"`
	Reason string `json:"reason"`
	Route  string `json:"route,omitempty"`
}

func composeLLMJudgeDispatchPrompt(run WorkflowRun, phase PhaseRun, gate WorkflowGateRun) string {
	lines := []string{
		"You are evaluating whether the just-completed workflow phase succeeded.",
		"Return ONLY valid JSON with this exact schema:",
		`{"passed": true, "reason": "short explanation", "route": "optional_route_id"}`,
		`Omit "route" when no declared continuation route should be selected.`,
		"",
		"Workflow run: " + firstNonEmpty(strings.TrimSpace(run.TemplateName), strings.TrimSpace(run.TemplateID), strings.TrimSpace(run.ID)),
		"Phase: " + firstNonEmpty(strings.TrimSpace(phase.Name), strings.TrimSpace(phase.ID)),
	}
	judgePrompt := ""
	if gate.LLMJudgeConfig != nil {
		judgePrompt = strings.TrimSpace(gate.LLMJudgeConfig.Prompt)
	}
	if judgePrompt != "" {
		lines = append(lines, "", "Judge instructions:", judgePrompt)
	}
	if len(gate.Routes) > 0 {
		lines = append(lines, "", "Allowed routes:")
		for _, route := range gate.Routes {
			lines = append(lines, "- "+strings.TrimSpace(route.ID)+": "+describeGateRouteTarget(&run, route))
		}
	}
	lines = append(lines, "", "Phase evidence:")
	for idx, step := range phase.Steps {
		label := firstNonEmpty(strings.TrimSpace(step.Name), strings.TrimSpace(step.ID))
		lines = append(lines, fmt.Sprintf("%d. %s | outcome=%s", idx+1, label, firstNonEmpty(step.Outcome, string(step.Status))))
		if prompt := strings.TrimSpace(step.Prompt); prompt != "" {
			lines = append(lines, "Prompt: "+clampOutput(prompt))
		}
		if output := strings.TrimSpace(step.Output); output != "" {
			lines = append(lines, "Output: "+clampOutput(output))
		}
		if errText := strings.TrimSpace(step.Error); errText != "" {
			lines = append(lines, "Error: "+clampOutput(errText))
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func parseLLMJudgeResponse(raw string) (llmJudgeResponse, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return llmJudgeResponse{}, false
	}
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(raw, "```"), "```json"))
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return llmJudgeResponse{}, false
	}
	payload := strings.TrimSpace(raw[start : end+1])
	var parsed llmJudgeResponse
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return llmJudgeResponse{}, false
	}
	return parsed, true
}
