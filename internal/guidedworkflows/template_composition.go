package guidedworkflows

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"control/internal/types"
)

type ParsedWorkflowTemplateCatalog struct {
	Version   int
	Templates []WorkflowTemplate
}

type rawWorkflowTemplateCatalog struct {
	Version     int                    `json:"version"`
	Definitions rawWorkflowDefinitions `json:"definitions,omitempty"`
	Templates   []rawWorkflowTemplate  `json:"templates"`
}

type rawWorkflowDefinitions struct {
	Prompts        map[string]string           `json:"prompts,omitempty"`
	Steps          map[string]rawWorkflowStep  `json:"steps,omitempty"`
	PhaseTemplates map[string]rawWorkflowPhase `json:"phase_templates,omitempty"`
}

type rawWorkflowTemplate struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Description        string             `json:"description,omitempty"`
	DefaultAccessLevel types.AccessLevel  `json:"default_access_level,omitempty"`
	Phases             []rawWorkflowPhase `json:"phases,omitempty"`
}

type rawWorkflowPhase struct {
	ID               string            `json:"id,omitempty"`
	Name             string            `json:"name,omitempty"`
	PhaseTemplateRef string            `json:"phase_template_ref,omitempty"`
	Steps            []rawWorkflowStep `json:"steps,omitempty"`
	StepRefs         []string          `json:"step_refs,omitempty"`
	Gates            []rawWorkflowGate `json:"gates,omitempty"`
}

type rawWorkflowStep struct {
	ID             string                       `json:"id,omitempty"`
	Name           string                       `json:"name,omitempty"`
	Prompt         string                       `json:"prompt,omitempty"`
	PromptRef      string                       `json:"prompt_ref,omitempty"`
	StepRef        string                       `json:"step_ref,omitempty"`
	RuntimeOptions *types.SessionRuntimeOptions `json:"runtime_options,omitempty"`
}

type rawWorkflowGate struct {
	ID        string                 `json:"id,omitempty"`
	Kind      string                 `json:"kind,omitempty"`
	Boundary  string                 `json:"boundary,omitempty"`
	Prompt    string                 `json:"prompt,omitempty"`
	PromptRef string                 `json:"prompt_ref,omitempty"`
	Reason    string                 `json:"reason,omitempty"`
	Routes    []rawWorkflowGateRoute `json:"routes,omitempty"`
	fields    map[string]json.RawMessage
}

type rawWorkflowGateRoute struct {
	ID     string                     `json:"id,omitempty"`
	Target rawWorkflowGateRouteTarget `json:"target,omitempty"`
	fields map[string]json.RawMessage
}

type rawWorkflowGateRouteTarget struct {
	Kind   string `json:"kind,omitempty"`
	StepID string `json:"step_id,omitempty"`
	fields map[string]json.RawMessage
}

func (g *rawWorkflowGate) UnmarshalJSON(data []byte) error {
	type gateAlias rawWorkflowGate
	var alias gateAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*g = rawWorkflowGate(alias)
	g.fields = fields
	return nil
}

func (r *rawWorkflowGateRoute) UnmarshalJSON(data []byte) error {
	type routeAlias rawWorkflowGateRoute
	var alias routeAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*r = rawWorkflowGateRoute(alias)
	r.fields = fields
	return nil
}

func (t *rawWorkflowGateRouteTarget) UnmarshalJSON(data []byte) error {
	type targetAlias rawWorkflowGateRouteTarget
	var alias targetAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*t = rawWorkflowGateRouteTarget(alias)
	t.fields = fields
	return nil
}

func (g rawWorkflowGate) unknownFields(allowed ...string) []string {
	if len(g.fields) == 0 {
		return nil
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	unknown := make([]string, 0)
	for field := range g.fields {
		if _, ok := allowedSet[field]; ok {
			continue
		}
		unknown = append(unknown, field)
	}
	sort.Strings(unknown)
	return unknown
}

func (r rawWorkflowGateRoute) unknownFields(allowed ...string) []string {
	if len(r.fields) == 0 {
		return nil
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	unknown := make([]string, 0)
	for field := range r.fields {
		if _, ok := allowedSet[field]; ok {
			continue
		}
		unknown = append(unknown, field)
	}
	sort.Strings(unknown)
	return unknown
}

func (t rawWorkflowGateRouteTarget) unknownFields(allowed ...string) []string {
	if len(t.fields) == 0 {
		return nil
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	unknown := make([]string, 0)
	for field := range t.fields {
		if _, ok := allowedSet[field]; ok {
			continue
		}
		unknown = append(unknown, field)
	}
	sort.Strings(unknown)
	return unknown
}

type templateCompositionDefinitions struct {
	prompts        map[string]string
	steps          map[string]WorkflowTemplateStep
	phaseTemplates map[string]WorkflowTemplatePhase
}

func ParseWorkflowTemplateCatalogJSON(raw []byte) (ParsedWorkflowTemplateCatalog, error) {
	var file rawWorkflowTemplateCatalog
	if err := json.Unmarshal(raw, &file); err != nil {
		return ParsedWorkflowTemplateCatalog{}, err
	}
	defs, err := compileTemplateCompositionDefinitions(file.Definitions)
	if err != nil {
		return ParsedWorkflowTemplateCatalog{}, err
	}
	expanded, err := expandRawTemplates(file.Templates, defs)
	if err != nil {
		return ParsedWorkflowTemplateCatalog{}, err
	}
	return ParsedWorkflowTemplateCatalog{
		Version:   file.Version,
		Templates: expanded,
	}, nil
}

func compileTemplateCompositionDefinitions(raw rawWorkflowDefinitions) (templateCompositionDefinitions, error) {
	defs := templateCompositionDefinitions{
		prompts:        map[string]string{},
		steps:          map[string]WorkflowTemplateStep{},
		phaseTemplates: map[string]WorkflowTemplatePhase{},
	}
	for rawID, value := range raw.Prompts {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return defs, fmt.Errorf("%w: definitions.prompts id is required", ErrTemplateConfigInvalid)
		}
		prompt := strings.TrimSpace(value)
		if prompt == "" {
			return defs, fmt.Errorf("%w: definitions.prompts[%q] value is required", ErrTemplateConfigInvalid, id)
		}
		defs.prompts[id] = prompt
	}
	for rawRef, rawStep := range raw.Steps {
		ref := strings.TrimSpace(rawRef)
		if ref == "" {
			return defs, fmt.Errorf("%w: definitions.steps id is required", ErrTemplateConfigInvalid)
		}
		// Keep step identity explicit in definitions to avoid hidden key-based IDs.
		if strings.TrimSpace(rawStep.StepRef) != "" {
			return defs, fmt.Errorf("%w: definitions.steps[%q] cannot include step_ref", ErrTemplateConfigInvalid, ref)
		}
		step, err := expandConcreteStep(rawStep, defs.prompts, "definitions.steps["+ref+"]")
		if err != nil {
			return defs, err
		}
		if strings.TrimSpace(step.Name) == "" {
			return defs, fmt.Errorf("%w: definitions.steps[%q].name is required", ErrTemplateConfigInvalid, ref)
		}
		defs.steps[ref] = step
	}
	for rawRef, rawPhase := range raw.PhaseTemplates {
		ref := strings.TrimSpace(rawRef)
		if ref == "" {
			return defs, fmt.Errorf("%w: definitions.phase_templates id is required", ErrTemplateConfigInvalid)
		}
		if strings.TrimSpace(rawPhase.PhaseTemplateRef) != "" {
			return defs, fmt.Errorf("%w: definitions.phase_templates[%q] cannot include phase_template_ref", ErrTemplateConfigInvalid, ref)
		}
		phase, err := expandConcretePhase(rawPhase, defs, "definitions.phase_templates["+ref+"]")
		if err != nil {
			return defs, err
		}
		if strings.TrimSpace(phase.ID) == "" {
			phase.ID = ref
		}
		if strings.TrimSpace(phase.Name) == "" {
			return defs, fmt.Errorf("%w: definitions.phase_templates[%q].name is required", ErrTemplateConfigInvalid, ref)
		}
		defs.phaseTemplates[ref] = phase
	}
	return defs, nil
}

func expandRawTemplates(rawTemplates []rawWorkflowTemplate, defs templateCompositionDefinitions) ([]WorkflowTemplate, error) {
	out := make([]WorkflowTemplate, 0, len(rawTemplates))
	templateIDs := map[string]struct{}{}
	for idx, rawTemplate := range rawTemplates {
		ctx := fmt.Sprintf("templates[%d]", idx)
		tpl := WorkflowTemplate{
			ID:                 strings.TrimSpace(rawTemplate.ID),
			Name:               strings.TrimSpace(rawTemplate.Name),
			Description:        strings.TrimSpace(rawTemplate.Description),
			DefaultAccessLevel: rawTemplate.DefaultAccessLevel,
		}
		if tpl.ID == "" {
			return nil, fmt.Errorf("%w: %s.id is required", ErrTemplateConfigInvalid, ctx)
		}
		if tpl.Name == "" {
			return nil, fmt.Errorf("%w: %s.name is required", ErrTemplateConfigInvalid, ctx)
		}
		if _, exists := templateIDs[tpl.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate template id %q", ErrTemplateConfigInvalid, tpl.ID)
		}
		templateIDs[tpl.ID] = struct{}{}
		phases := make([]WorkflowTemplatePhase, 0, len(rawTemplate.Phases))
		for phaseIndex, rawPhase := range rawTemplate.Phases {
			phaseCtx := fmt.Sprintf("%s.phases[%d]", ctx, phaseIndex)
			if strings.TrimSpace(rawPhase.PhaseTemplateRef) != "" {
				phase, err := expandPhaseTemplateReference(rawPhase, defs, phaseCtx)
				if err != nil {
					return nil, err
				}
				phases = append(phases, phase)
				continue
			}
			phase, err := expandConcretePhase(rawPhase, defs, phaseCtx)
			if err != nil {
				return nil, err
			}
			phases = append(phases, phase)
		}
		if len(phases) == 0 {
			return nil, fmt.Errorf("%w: %s.phases are required", ErrTemplateConfigInvalid, ctx)
		}
		phaseIDs := map[string]struct{}{}
		for _, phase := range phases {
			if _, exists := phaseIDs[phase.ID]; exists {
				return nil, fmt.Errorf("%w: template %q has duplicate phase id %q", ErrTemplateConfigInvalid, tpl.ID, phase.ID)
			}
			phaseIDs[phase.ID] = struct{}{}
			stepIDs := map[string]struct{}{}
			for _, step := range phase.Steps {
				if strings.TrimSpace(step.Prompt) == "" {
					return nil, fmt.Errorf("%w: template %q phase %q step %q resolved prompt is required", ErrTemplateConfigInvalid, tpl.ID, phase.ID, step.ID)
				}
				if _, exists := stepIDs[step.ID]; exists {
					return nil, fmt.Errorf("%w: template %q phase %q has duplicate step id %q", ErrTemplateConfigInvalid, tpl.ID, phase.ID, step.ID)
				}
				stepIDs[step.ID] = struct{}{}
			}
		}
		tpl.Phases = phases
		normalized, err := NormalizeWorkflowTemplate(tpl)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrTemplateConfigInvalid, err.Error())
		}
		tpl = normalized
		out = append(out, tpl)
	}
	return out, nil
}

func expandPhaseTemplateReference(rawPhase rawWorkflowPhase, defs templateCompositionDefinitions, ctx string) (WorkflowTemplatePhase, error) {
	ref := strings.TrimSpace(rawPhase.PhaseTemplateRef)
	phase, ok := defs.phaseTemplates[ref]
	if !ok {
		return WorkflowTemplatePhase{}, fmt.Errorf("%w: unknown phase_template_ref %q at %s", ErrTemplateConfigInvalid, ref, ctx)
	}
	if len(rawPhase.Steps) > 0 || len(rawPhase.StepRefs) > 0 || len(rawPhase.Gates) > 0 {
		return WorkflowTemplatePhase{}, fmt.Errorf("%w: %s cannot combine phase_template_ref with steps/step_refs/gates", ErrTemplateConfigInvalid, ctx)
	}
	out := cloneWorkflowTemplatePhase(phase)
	if id := strings.TrimSpace(rawPhase.ID); id != "" {
		out.ID = id
	}
	if name := strings.TrimSpace(rawPhase.Name); name != "" {
		out.Name = name
	}
	return out, nil
}

func expandConcretePhase(rawPhase rawWorkflowPhase, defs templateCompositionDefinitions, ctx string) (WorkflowTemplatePhase, error) {
	phase := WorkflowTemplatePhase{
		ID:   strings.TrimSpace(rawPhase.ID),
		Name: strings.TrimSpace(rawPhase.Name),
	}
	if phase.ID == "" {
		return WorkflowTemplatePhase{}, fmt.Errorf("%w: %s.id is required", ErrTemplateConfigInvalid, ctx)
	}
	if phase.Name == "" {
		return WorkflowTemplatePhase{}, fmt.Errorf("%w: %s.name is required", ErrTemplateConfigInvalid, ctx)
	}
	steps, err := expandPhaseSteps(rawPhase, defs, ctx)
	if err != nil {
		return WorkflowTemplatePhase{}, err
	}
	if len(steps) == 0 {
		return WorkflowTemplatePhase{}, fmt.Errorf("%w: %s must define steps or step_refs", ErrTemplateConfigInvalid, ctx)
	}
	phase.Steps = steps
	gates, err := expandPhaseGates(rawPhase, defs.prompts, phase.ID, ctx)
	if err != nil {
		return WorkflowTemplatePhase{}, err
	}
	phase.Gates = gates
	return phase, nil
}

func expandPhaseGates(rawPhase rawWorkflowPhase, prompts map[string]string, phaseID string, ctx string) ([]WorkflowGateSpec, error) {
	if len(rawPhase.Gates) == 0 {
		return nil, nil
	}
	out := make([]WorkflowGateSpec, 0, len(rawPhase.Gates))
	gateIDs := map[string]struct{}{}
	for idx, rawGate := range rawPhase.Gates {
		gateCtx := fmt.Sprintf("%s.gates[%d]", ctx, idx)
		kindRaw := strings.TrimSpace(rawGate.Kind)
		if kindRaw == "" {
			return nil, fmt.Errorf("%w: %s.kind is required", ErrTemplateConfigInvalid, gateCtx)
		}
		kind, ok := normalizeWorkflowGateKind(WorkflowGateKind(kindRaw))
		if !ok {
			return nil, fmt.Errorf("%w: %s.kind %q is not supported", ErrTemplateConfigInvalid, gateCtx, kindRaw)
		}
		boundary, ok := normalizeWorkflowGateBoundary(WorkflowGateBoundary(strings.TrimSpace(rawGate.Boundary)))
		if !ok {
			return nil, fmt.Errorf("%w: %s.boundary %q is not supported", ErrTemplateConfigInvalid, gateCtx, boundary)
		}
		id := strings.TrimSpace(rawGate.ID)
		if id == "" {
			id = fmt.Sprintf("%s_gate_%d", strings.TrimSpace(phaseID), idx+1)
		}
		if _, exists := gateIDs[id]; exists {
			return nil, fmt.Errorf("%w: duplicate gate id %q at %s", ErrTemplateConfigInvalid, id, gateCtx)
		}
		gateIDs[id] = struct{}{}
		gate := WorkflowGateSpec{
			ID:   id,
			Kind: kind,
			Boundary: WorkflowGateBoundaryRef{
				Boundary: boundary,
				PhaseID:  strings.TrimSpace(phaseID),
			},
		}
		routes, err := expandGateRoutes(rawGate.Routes, gateCtx)
		if err != nil {
			return nil, err
		}
		gate.Routes = routes
		switch kind {
		case WorkflowGateKindManualReview:
			unknown := rawGate.unknownFields("id", "kind", "boundary", "reason", "routes")
			if len(unknown) > 0 {
				return nil, fmt.Errorf("%w: %s manual_review has unknown field(s): %s", ErrTemplateConfigInvalid, gateCtx, strings.Join(unknown, ", "))
			}
			if strings.TrimSpace(rawGate.Prompt) != "" || strings.TrimSpace(rawGate.PromptRef) != "" {
				return nil, fmt.Errorf("%w: %s manual_review does not accept prompt/prompt_ref", ErrTemplateConfigInvalid, gateCtx)
			}
			gate.ManualReviewConfig = &ManualReviewConfig{
				Reason: strings.TrimSpace(rawGate.Reason),
			}
		case WorkflowGateKindLLMJudge:
			unknown := rawGate.unknownFields("id", "kind", "boundary", "prompt", "prompt_ref", "routes")
			if len(unknown) > 0 {
				return nil, fmt.Errorf("%w: %s llm_judge has unknown field(s): %s", ErrTemplateConfigInvalid, gateCtx, strings.Join(unknown, ", "))
			}
			if strings.TrimSpace(rawGate.Reason) != "" {
				return nil, fmt.Errorf("%w: %s llm_judge does not accept reason", ErrTemplateConfigInvalid, gateCtx)
			}
			prompt, err := resolvePrompt(rawGate.Prompt, rawGate.PromptRef, prompts, gateCtx)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(prompt) == "" {
				return nil, fmt.Errorf("%w: %s llm_judge resolved prompt is required", ErrTemplateConfigInvalid, gateCtx)
			}
			gate.LLMJudgeConfig = &LLMJudgeConfig{
				Prompt: strings.TrimSpace(prompt),
			}
		}
		out = append(out, gate)
	}
	return out, nil
}

func expandGateRoutes(rawRoutes []rawWorkflowGateRoute, gateCtx string) ([]WorkflowGateRoute, error) {
	if len(rawRoutes) == 0 {
		return nil, nil
	}
	out := make([]WorkflowGateRoute, 0, len(rawRoutes))
	routeIDs := map[string]struct{}{}
	for idx, rawRoute := range rawRoutes {
		routeCtx := fmt.Sprintf("%s.routes[%d]", gateCtx, idx)
		if unknown := rawRoute.unknownFields("id", "target"); len(unknown) > 0 {
			return nil, fmt.Errorf("%w: %s has unknown field(s): %s", ErrTemplateConfigInvalid, routeCtx, strings.Join(unknown, ", "))
		}
		id := strings.TrimSpace(rawRoute.ID)
		if id == "" {
			return nil, fmt.Errorf("%w: %s.id is required", ErrTemplateConfigInvalid, routeCtx)
		}
		if _, exists := routeIDs[id]; exists {
			return nil, fmt.Errorf("%w: duplicate route id %q at %s", ErrTemplateConfigInvalid, id, routeCtx)
		}
		routeIDs[id] = struct{}{}
		if unknown := rawRoute.Target.unknownFields("kind", "step_id"); len(unknown) > 0 {
			return nil, fmt.Errorf("%w: %s.target has unknown field(s): %s", ErrTemplateConfigInvalid, routeCtx, strings.Join(unknown, ", "))
		}
		if strings.TrimSpace(rawRoute.Target.Kind) == "" {
			return nil, fmt.Errorf("%w: %s.target.kind is required", ErrTemplateConfigInvalid, routeCtx)
		}
		route := WorkflowGateRoute{
			ID: id,
			Target: WorkflowGateRouteTargetRef{
				Kind:   WorkflowGateRouteTargetKind(strings.TrimSpace(rawRoute.Target.Kind)),
				StepID: strings.TrimSpace(rawRoute.Target.StepID),
			},
		}
		out = append(out, route)
	}
	return out, nil
}

func expandPhaseSteps(rawPhase rawWorkflowPhase, defs templateCompositionDefinitions, ctx string) ([]WorkflowTemplateStep, error) {
	steps := make([]WorkflowTemplateStep, 0, len(rawPhase.StepRefs)+len(rawPhase.Steps))
	for idx, rawRef := range rawPhase.StepRefs {
		ref := strings.TrimSpace(rawRef)
		step, ok := defs.steps[ref]
		if !ok {
			return nil, fmt.Errorf("%w: unknown step_ref %q at %s.step_refs[%d]", ErrTemplateConfigInvalid, ref, ctx, idx)
		}
		steps = append(steps, cloneWorkflowTemplateStep(step))
	}
	for idx, rawStep := range rawPhase.Steps {
		stepCtx := fmt.Sprintf("%s.steps[%d]", ctx, idx)
		ref := strings.TrimSpace(rawStep.StepRef)
		if ref == "" {
			step, err := expandConcreteStep(rawStep, defs.prompts, stepCtx)
			if err != nil {
				return nil, err
			}
			steps = append(steps, step)
			continue
		}
		baseStep, ok := defs.steps[ref]
		if !ok {
			return nil, fmt.Errorf("%w: unknown step_ref %q at %s", ErrTemplateConfigInvalid, ref, stepCtx)
		}
		step := cloneWorkflowTemplateStep(baseStep)
		if overrideID := strings.TrimSpace(rawStep.ID); overrideID != "" && overrideID != step.ID {
			return nil, fmt.Errorf("%w: %s cannot override step id when using step_ref", ErrTemplateConfigInvalid, stepCtx)
		}
		if overrideName := strings.TrimSpace(rawStep.Name); overrideName != "" {
			step.Name = overrideName
		}
		if rawStep.RuntimeOptions != nil {
			step.RuntimeOptions = types.CloneRuntimeOptions(rawStep.RuntimeOptions)
		}
		promptSet := strings.TrimSpace(rawStep.Prompt) != "" || strings.TrimSpace(rawStep.PromptRef) != ""
		if promptSet {
			prompt, err := resolvePrompt(rawStep.Prompt, rawStep.PromptRef, defs.prompts, stepCtx)
			if err != nil {
				return nil, err
			}
			step.Prompt = prompt
		}
		if strings.TrimSpace(step.Name) == "" {
			return nil, fmt.Errorf("%w: %s resolved name is required", ErrTemplateConfigInvalid, stepCtx)
		}
		if strings.TrimSpace(step.Prompt) == "" {
			return nil, fmt.Errorf("%w: %s resolved prompt is required", ErrTemplateConfigInvalid, stepCtx)
		}
		steps = append(steps, step)
	}
	return steps, nil
}

func expandConcreteStep(rawStep rawWorkflowStep, prompts map[string]string, ctx string) (WorkflowTemplateStep, error) {
	id := strings.TrimSpace(rawStep.ID)
	name := strings.TrimSpace(rawStep.Name)
	if id == "" {
		return WorkflowTemplateStep{}, fmt.Errorf("%w: %s.id is required", ErrTemplateConfigInvalid, ctx)
	}
	if name == "" {
		return WorkflowTemplateStep{}, fmt.Errorf("%w: %s.name is required", ErrTemplateConfigInvalid, ctx)
	}
	prompt, err := resolvePrompt(rawStep.Prompt, rawStep.PromptRef, prompts, ctx)
	if err != nil {
		return WorkflowTemplateStep{}, err
	}
	if prompt == "" {
		return WorkflowTemplateStep{}, fmt.Errorf("%w: %s resolved prompt is required", ErrTemplateConfigInvalid, ctx)
	}
	return WorkflowTemplateStep{
		ID:             id,
		Name:           name,
		Prompt:         prompt,
		RuntimeOptions: types.CloneRuntimeOptions(rawStep.RuntimeOptions),
	}, nil
}

func resolvePrompt(promptRaw, promptRefRaw string, prompts map[string]string, ctx string) (string, error) {
	prompt := strings.TrimSpace(promptRaw)
	promptRef := strings.TrimSpace(promptRefRaw)
	if prompt != "" && promptRef != "" {
		return "", fmt.Errorf("%w: %s cannot define both prompt and prompt_ref", ErrTemplateConfigInvalid, ctx)
	}
	if prompt != "" {
		return prompt, nil
	}
	if promptRef != "" {
		resolved, ok := prompts[promptRef]
		if !ok {
			return "", fmt.Errorf("%w: unknown prompt_ref %q at %s", ErrTemplateConfigInvalid, promptRef, ctx)
		}
		resolved = strings.TrimSpace(resolved)
		if resolved == "" {
			return "", fmt.Errorf("%w: prompt_ref %q at %s resolved to empty prompt", ErrTemplateConfigInvalid, promptRef, ctx)
		}
		return resolved, nil
	}
	return "", nil
}

func cloneWorkflowTemplatePhase(in WorkflowTemplatePhase) WorkflowTemplatePhase {
	out := in
	out.Steps = make([]WorkflowTemplateStep, len(in.Steps))
	for idx, step := range in.Steps {
		out.Steps[idx] = cloneWorkflowTemplateStep(step)
	}
	if len(in.Gates) > 0 {
		out.Gates = make([]WorkflowGateSpec, 0, len(in.Gates))
		for _, gate := range in.Gates {
			out.Gates = append(out.Gates, cloneWorkflowGateSpec(gate))
		}
	}
	return out
}

func cloneWorkflowGateSpec(in WorkflowGateSpec) WorkflowGateSpec {
	out := in
	out.Routes = cloneWorkflowGateRoutes(in.Routes)
	if in.ManualReviewConfig != nil {
		cfg := *in.ManualReviewConfig
		out.ManualReviewConfig = &cfg
	}
	if in.LLMJudgeConfig != nil {
		cfg := *in.LLMJudgeConfig
		out.LLMJudgeConfig = &cfg
	}
	return out
}

func cloneWorkflowTemplateStep(in WorkflowTemplateStep) WorkflowTemplateStep {
	out := in
	out.RuntimeOptions = types.CloneRuntimeOptions(in.RuntimeOptions)
	return out
}
