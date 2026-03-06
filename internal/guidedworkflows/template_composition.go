package guidedworkflows

import (
	"encoding/json"
	"fmt"
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
}

type rawWorkflowStep struct {
	ID             string                       `json:"id,omitempty"`
	Name           string                       `json:"name,omitempty"`
	Prompt         string                       `json:"prompt,omitempty"`
	PromptRef      string                       `json:"prompt_ref,omitempty"`
	StepRef        string                       `json:"step_ref,omitempty"`
	RuntimeOptions *types.SessionRuntimeOptions `json:"runtime_options,omitempty"`
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
	if len(rawPhase.Steps) > 0 || len(rawPhase.StepRefs) > 0 {
		return WorkflowTemplatePhase{}, fmt.Errorf("%w: %s cannot combine phase_template_ref with steps/step_refs", ErrTemplateConfigInvalid, ctx)
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
	return phase, nil
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
	return out
}

func cloneWorkflowTemplateStep(in WorkflowTemplateStep) WorkflowTemplateStep {
	out := in
	out.RuntimeOptions = types.CloneRuntimeOptions(in.RuntimeOptions)
	return out
}
