package app

import (
	"sort"
	"strings"

	"control/internal/guidedworkflows"
)

type guidedWorkflowTemplatePicker struct {
	options []guidedWorkflowTemplateOption
	index   int
	loading bool
	err     string
}

func newGuidedWorkflowTemplatePicker() guidedWorkflowTemplatePicker {
	return guidedWorkflowTemplatePicker{loading: true}
}

func (p *guidedWorkflowTemplatePicker) Reset() {
	if p == nil {
		return
	}
	p.options = nil
	p.index = 0
	p.loading = true
	p.err = ""
}

func (p *guidedWorkflowTemplatePicker) BeginLoad() {
	if p == nil {
		return
	}
	p.loading = true
	p.err = ""
}

func (p *guidedWorkflowTemplatePicker) SetError(err error) {
	if p == nil {
		return
	}
	p.loading = false
	p.err = errorText(err)
}

func (p *guidedWorkflowTemplatePicker) SetTemplates(raw []guidedworkflows.WorkflowTemplate, previousID string) {
	if p == nil {
		return
	}
	p.options = normalizeGuidedWorkflowTemplateOptions(raw)
	p.index = 0
	p.loading = false
	p.err = ""
	if len(p.options) == 0 {
		return
	}
	previousID = strings.TrimSpace(previousID)
	for idx, option := range p.options {
		if strings.EqualFold(strings.TrimSpace(option.id), previousID) {
			p.index = idx
			break
		}
	}
	p.clampIndex()
}

func (p *guidedWorkflowTemplatePicker) Move(delta int) bool {
	if p == nil || p.loading || len(p.options) == 0 || delta == 0 {
		return false
	}
	next := (p.index + delta + len(p.options)) % len(p.options)
	if next == p.index {
		return false
	}
	p.index = next
	p.clampIndex()
	return true
}

func (p *guidedWorkflowTemplatePicker) Selected() (guidedWorkflowTemplateOption, bool) {
	if p == nil || len(p.options) == 0 {
		return guidedWorkflowTemplateOption{}, false
	}
	p.clampIndex()
	if p.index < 0 || p.index >= len(p.options) {
		return guidedWorkflowTemplateOption{}, false
	}
	return p.options[p.index], true
}

func (p *guidedWorkflowTemplatePicker) SelectedIndex() int {
	if p == nil || len(p.options) == 0 {
		return -1
	}
	p.clampIndex()
	return p.index
}

func (p *guidedWorkflowTemplatePicker) Options() []guidedWorkflowTemplateOption {
	if p == nil {
		return nil
	}
	return p.options
}

func (p *guidedWorkflowTemplatePicker) Loading() bool {
	if p == nil {
		return false
	}
	return p.loading
}

func (p *guidedWorkflowTemplatePicker) Error() string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(p.err)
}

func (p *guidedWorkflowTemplatePicker) HasSelection() bool {
	_, ok := p.Selected()
	return ok
}

func (p *guidedWorkflowTemplatePicker) clampIndex() {
	if p == nil {
		return
	}
	if len(p.options) == 0 {
		p.index = 0
		return
	}
	if p.index < 0 {
		p.index = 0
	}
	if p.index >= len(p.options) {
		p.index = len(p.options) - 1
	}
}

func normalizeGuidedWorkflowTemplateOptions(raw []guidedworkflows.WorkflowTemplate) []guidedWorkflowTemplateOption {
	if len(raw) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	options := make([]guidedWorkflowTemplateOption, 0, len(raw))
	for _, template := range raw {
		id := strings.TrimSpace(template.ID)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		options = append(options, guidedWorkflowTemplateOption{
			id:          id,
			name:        strings.TrimSpace(template.Name),
			description: strings.TrimSpace(template.Description),
		})
	}
	sort.Slice(options, func(i, j int) bool {
		leftName := strings.ToLower(strings.TrimSpace(options[i].name))
		rightName := strings.ToLower(strings.TrimSpace(options[j].name))
		if leftName != rightName {
			return leftName < rightName
		}
		leftID := strings.ToLower(strings.TrimSpace(options[i].id))
		rightID := strings.ToLower(strings.TrimSpace(options[j].id))
		return leftID < rightID
	})
	return options
}
