package guidedworkflows

import (
	"errors"
	"fmt"
	"strings"

	"control/internal/types"
)

func NormalizeWorkflowTemplate(template WorkflowTemplate) (WorkflowTemplate, error) {
	template.ID = strings.TrimSpace(template.ID)
	template.Name = strings.TrimSpace(template.Name)
	template.Description = strings.TrimSpace(template.Description)

	normalizedAccess, ok := NormalizeTemplateAccessLevel(template.DefaultAccessLevel)
	if !ok {
		return WorkflowTemplate{}, errors.New("invalid template default_access_level: " + strings.TrimSpace(string(template.DefaultAccessLevel)))
	}
	template.DefaultAccessLevel = normalizedAccess

	if template.ID == "" {
		return WorkflowTemplate{}, errors.New("template id is required")
	}
	if template.Name == "" {
		return WorkflowTemplate{}, errors.New("template name is required")
	}
	if len(template.Phases) == 0 {
		return WorkflowTemplate{}, errors.New("template phases are required")
	}

	phaseIDs := map[string]struct{}{}
	stepIDs := map[string]struct{}{}
	for pIdx := range template.Phases {
		phase := &template.Phases[pIdx]
		phase.ID = strings.TrimSpace(phase.ID)
		phase.Name = strings.TrimSpace(phase.Name)
		if phase.ID == "" {
			return WorkflowTemplate{}, errors.New("phase id is required")
		}
		if phase.Name == "" {
			return WorkflowTemplate{}, errors.New("phase name is required")
		}
		if _, exists := phaseIDs[phase.ID]; exists {
			return WorkflowTemplate{}, errors.New("duplicate phase id: " + phase.ID)
		}
		phaseIDs[phase.ID] = struct{}{}
		if len(phase.Steps) == 0 {
			return WorkflowTemplate{}, errors.New("phase steps are required")
		}

		phaseStepIDs := map[string]struct{}{}
		for sIdx := range phase.Steps {
			step := &phase.Steps[sIdx]
			step.ID = strings.TrimSpace(step.ID)
			step.Name = strings.TrimSpace(step.Name)
			step.Prompt = strings.TrimSpace(step.Prompt)
			if step.ID == "" {
				return WorkflowTemplate{}, errors.New("step id is required")
			}
			if step.Name == "" {
				return WorkflowTemplate{}, errors.New("step name is required")
			}
			if step.Prompt == "" {
				return WorkflowTemplate{}, errors.New("step prompt is required")
			}
			if _, exists := phaseStepIDs[step.ID]; exists {
				return WorkflowTemplate{}, errors.New("duplicate step id in phase: " + step.ID)
			}
			phaseStepIDs[step.ID] = struct{}{}
			if _, exists := stepIDs[step.ID]; exists {
				return WorkflowTemplate{}, errors.New("duplicate step id: " + step.ID)
			}
			stepIDs[step.ID] = struct{}{}

			runtimeOptions, err := normalizeWorkflowStepRuntimeOptions(step.RuntimeOptions)
			if err != nil {
				return WorkflowTemplate{}, err
			}
			step.RuntimeOptions = runtimeOptions
		}
	}

	for pIdx := range template.Phases {
		gates, err := normalizeWorkflowGateSpecs(template.Phases[pIdx].Gates, template.Phases[pIdx].ID, stepIDs)
		if err != nil {
			return WorkflowTemplate{}, err
		}
		template.Phases[pIdx].Gates = gates
	}

	return template, nil
}

func normalizeWorkflowStepRuntimeOptions(in *types.SessionRuntimeOptions) (*types.SessionRuntimeOptions, error) {
	if in == nil {
		return nil, nil
	}
	out := types.CloneRuntimeOptions(in)
	if out == nil {
		return nil, nil
	}
	out.Model = strings.TrimSpace(out.Model)
	if out.Reasoning != "" {
		normalizedReasoning, ok := types.NormalizeReasoningLevel(out.Reasoning)
		if !ok {
			return nil, errors.New("invalid step runtime_options.reasoning: " + strings.TrimSpace(string(out.Reasoning)))
		}
		out.Reasoning = normalizedReasoning
	}
	if out.Access != "" {
		normalizedAccess, ok := types.NormalizeAccessLevel(out.Access)
		if !ok {
			return nil, errors.New("invalid step runtime_options.access: " + strings.TrimSpace(string(out.Access)))
		}
		out.Access = normalizedAccess
	}
	if out.Provider != nil && len(out.Provider) == 0 {
		out.Provider = nil
	}
	if out.Version == 0 {
		out.Version = 1
	}
	if out.Model == "" && out.Reasoning == "" && out.Access == "" && len(out.Provider) == 0 {
		return nil, nil
	}
	return out, nil
}

func normalizeWorkflowGateKind(kind WorkflowGateKind) (WorkflowGateKind, bool) {
	switch strings.TrimSpace(string(kind)) {
	case string(WorkflowGateKindManualReview):
		return WorkflowGateKindManualReview, true
	case string(WorkflowGateKindLLMJudge):
		return WorkflowGateKindLLMJudge, true
	default:
		return "", false
	}
}

func normalizeWorkflowGateBoundary(boundary WorkflowGateBoundary) (WorkflowGateBoundary, bool) {
	switch strings.TrimSpace(string(boundary)) {
	case "", string(WorkflowGateBoundaryPhaseEnd):
		return WorkflowGateBoundaryPhaseEnd, true
	default:
		return "", false
	}
}

func normalizeWorkflowGateRouteTargetKind(kind WorkflowGateRouteTargetKind) (WorkflowGateRouteTargetKind, bool) {
	switch strings.TrimSpace(string(kind)) {
	case string(WorkflowGateRouteTargetNextStep):
		return WorkflowGateRouteTargetNextStep, true
	case string(WorkflowGateRouteTargetStep):
		return WorkflowGateRouteTargetStep, true
	case string(WorkflowGateRouteTargetCompletePhase):
		return WorkflowGateRouteTargetCompletePhase, true
	default:
		return "", false
	}
}

func normalizeWorkflowGateSpecs(gates []WorkflowGateSpec, phaseID string, validStepIDs map[string]struct{}) ([]WorkflowGateSpec, error) {
	if len(gates) == 0 {
		return nil, nil
	}
	normalizedPhaseID := strings.TrimSpace(phaseID)
	out := make([]WorkflowGateSpec, 0, len(gates))
	gateIDs := map[string]struct{}{}
	for idx, gate := range gates {
		gate = cloneWorkflowGateSpec(gate)
		gate.ID = strings.TrimSpace(gate.ID)
		if gate.ID == "" {
			gate.ID = fmt.Sprintf("%s_gate_%d", normalizedPhaseID, idx+1)
		}
		if _, exists := gateIDs[gate.ID]; exists {
			return nil, fmt.Errorf("duplicate gate id: %s", gate.ID)
		}
		gateIDs[gate.ID] = struct{}{}

		normalizedKind, ok := normalizeWorkflowGateKind(gate.Kind)
		if !ok {
			return nil, fmt.Errorf("gate %q kind %q is not supported", gate.ID, strings.TrimSpace(string(gate.Kind)))
		}
		gate.Kind = normalizedKind

		normalizedBoundary, ok := normalizeWorkflowGateBoundary(gate.Boundary.Boundary)
		if !ok {
			return nil, fmt.Errorf("gate %q boundary %q is not supported", gate.ID, strings.TrimSpace(string(gate.Boundary.Boundary)))
		}
		gate.Boundary.Boundary = normalizedBoundary
		if strings.TrimSpace(gate.Boundary.StepID) != "" {
			return nil, fmt.Errorf("gate %q boundary.step_id is not supported", gate.ID)
		}
		if gate.Boundary.PhaseID == "" {
			gate.Boundary.PhaseID = normalizedPhaseID
		}
		if strings.TrimSpace(gate.Boundary.PhaseID) != normalizedPhaseID {
			return nil, fmt.Errorf("gate %q boundary.phase_id must match containing phase %q", gate.ID, normalizedPhaseID)
		}

		routes, err := normalizeWorkflowGateRoutes(gate.Routes, gate.ID, validStepIDs)
		if err != nil {
			return nil, err
		}
		gate.Routes = routes

		switch gate.Kind {
		case WorkflowGateKindManualReview:
			if gate.LLMJudgeConfig != nil {
				return nil, fmt.Errorf("gate %q manual_review cannot define llm_judge_config", gate.ID)
			}
			gate.ManualReviewConfig = cloneManualReviewConfig(gate.ManualReviewConfig)
			if gate.ManualReviewConfig == nil {
				gate.ManualReviewConfig = &ManualReviewConfig{}
			}
		case WorkflowGateKindLLMJudge:
			if gate.ManualReviewConfig != nil {
				return nil, fmt.Errorf("gate %q llm_judge cannot define manual_review_config", gate.ID)
			}
			gate.LLMJudgeConfig = cloneLLMJudgeConfig(gate.LLMJudgeConfig)
			if gate.LLMJudgeConfig == nil || strings.TrimSpace(gate.LLMJudgeConfig.Prompt) == "" {
				return nil, fmt.Errorf("gate %q llm_judge prompt is required", gate.ID)
			}
			gate.LLMJudgeConfig.Prompt = strings.TrimSpace(gate.LLMJudgeConfig.Prompt)
		}

		out = append(out, gate)
	}
	return out, nil
}

func normalizeWorkflowGateRoutes(routes []WorkflowGateRoute, gateID string, validStepIDs map[string]struct{}) ([]WorkflowGateRoute, error) {
	if len(routes) == 0 {
		return nil, nil
	}
	out := make([]WorkflowGateRoute, 0, len(routes))
	routeIDs := map[string]struct{}{}
	for _, route := range routes {
		route.ID = strings.TrimSpace(route.ID)
		if route.ID == "" {
			return nil, fmt.Errorf("gate %q route id is required", gateID)
		}
		if _, exists := routeIDs[route.ID]; exists {
			return nil, fmt.Errorf("gate %q has duplicate route id %q", gateID, route.ID)
		}
		routeIDs[route.ID] = struct{}{}

		normalizedKind, ok := normalizeWorkflowGateRouteTargetKind(route.Target.Kind)
		if !ok {
			return nil, fmt.Errorf("gate %q route %q target kind %q is not supported", gateID, route.ID, strings.TrimSpace(string(route.Target.Kind)))
		}
		route.Target.Kind = normalizedKind
		route.Target.StepID = strings.TrimSpace(route.Target.StepID)
		switch route.Target.Kind {
		case WorkflowGateRouteTargetNextStep:
			if route.Target.StepID != "" {
				return nil, fmt.Errorf("gate %q route %q target %q does not accept step_id", gateID, route.ID, route.Target.Kind)
			}
		case WorkflowGateRouteTargetStep:
			if route.Target.StepID == "" {
				return nil, fmt.Errorf("gate %q route %q target.step_id is required", gateID, route.ID)
			}
			if _, ok := validStepIDs[route.Target.StepID]; !ok {
				return nil, fmt.Errorf("gate %q route %q target.step_id %q does not match any template step", gateID, route.ID, route.Target.StepID)
			}
		case WorkflowGateRouteTargetCompletePhase:
			if route.Target.StepID != "" {
				return nil, fmt.Errorf("gate %q route %q target %q does not accept step_id", gateID, route.ID, route.Target.Kind)
			}
		}

		out = append(out, route)
	}
	return out, nil
}

func cloneWorkflowGateRoutes(in []WorkflowGateRoute) []WorkflowGateRoute {
	if len(in) == 0 {
		return nil
	}
	return append([]WorkflowGateRoute(nil), in...)
}
