package app

import (
	"sort"
	"strings"

	"control/internal/guidedworkflows"
)

type guidedWorkflowTemplatePicker struct {
	picker  *SelectPicker
	options []guidedWorkflowTemplateOption
	loading bool
	err     string
}

func newGuidedWorkflowTemplatePicker() guidedWorkflowTemplatePicker {
	return guidedWorkflowTemplatePicker{
		picker:  NewSelectPicker(minViewportWidth, 8),
		loading: true,
	}
}

func (p *guidedWorkflowTemplatePicker) Reset() {
	if p == nil {
		return
	}
	p.options = nil
	p.loading = true
	p.err = ""
	picker := p.ensurePicker()
	if picker == nil {
		return
	}
	picker.SetQuery("")
	picker.SetOptions(nil)
}

func (p *guidedWorkflowTemplatePicker) BeginLoad() {
	if p == nil {
		return
	}
	p.loading = true
	p.err = ""
}

func (p *guidedWorkflowTemplatePicker) SetSize(width, height int) {
	if p == nil {
		return
	}
	picker := p.ensurePicker()
	if picker == nil {
		return
	}
	if width <= 0 {
		width = minViewportWidth
	}
	if height <= 0 {
		height = 8
	}
	picker.SetSize(width, height)
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
	p.loading = false
	p.err = ""
	picker := p.ensurePicker()
	if picker == nil {
		return
	}
	items := make([]selectOption, 0, len(p.options))
	for _, option := range p.options {
		label := guidedWorkflowTemplateLabel(option)
		items = append(items, selectOption{
			id:     option.id,
			label:  label,
			search: strings.Join([]string{label, option.id, option.name, option.description}, " "),
		})
	}
	picker.SetOptions(items)
	if len(p.options) == 0 {
		return
	}
	previousID = strings.TrimSpace(previousID)
	if previousID != "" && picker.SelectID(previousID) {
		return
	}
	picker.SelectID(p.options[0].id)
}

func (p *guidedWorkflowTemplatePicker) Move(delta int) bool {
	if p == nil || p.loading || len(p.options) == 0 || delta == 0 || p.picker == nil {
		return false
	}
	return p.picker.Move(delta)
}

func (p *guidedWorkflowTemplatePicker) Selected() (guidedWorkflowTemplateOption, bool) {
	if p == nil || len(p.options) == 0 {
		return guidedWorkflowTemplateOption{}, false
	}
	if p.picker == nil {
		return guidedWorkflowTemplateOption{}, false
	}
	id := strings.TrimSpace(p.picker.SelectedID())
	if id == "" {
		return guidedWorkflowTemplateOption{}, false
	}
	for _, option := range p.options {
		if strings.EqualFold(strings.TrimSpace(option.id), id) {
			return option, true
		}
	}
	return guidedWorkflowTemplateOption{}, false
}

func (p *guidedWorkflowTemplatePicker) SelectedIndex() int {
	if p == nil || len(p.options) == 0 {
		return -1
	}
	selected, ok := p.Selected()
	if !ok {
		return -1
	}
	for idx, option := range p.options {
		if strings.EqualFold(strings.TrimSpace(option.id), strings.TrimSpace(selected.id)) {
			return idx
		}
	}
	return -1
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

func (p *guidedWorkflowTemplatePicker) Query() string {
	if p == nil || p.picker == nil {
		return ""
	}
	return p.picker.Query()
}

func (p *guidedWorkflowTemplatePicker) AppendQuery(text string) bool {
	if p == nil || p.loading || p.picker == nil {
		return false
	}
	return p.picker.AppendQuery(text)
}

func (p *guidedWorkflowTemplatePicker) BackspaceQuery() bool {
	if p == nil || p.loading || p.picker == nil {
		return false
	}
	return p.picker.BackspaceQuery()
}

func (p *guidedWorkflowTemplatePicker) ClearQuery() bool {
	if p == nil || p.loading || p.picker == nil {
		return false
	}
	return p.picker.ClearQuery()
}

func (p *guidedWorkflowTemplatePicker) HandleClick(row int) bool {
	if p == nil || p.loading || p.picker == nil {
		return false
	}
	return p.picker.HandleClick(row)
}

func (p *guidedWorkflowTemplatePicker) View() string {
	if p == nil || p.picker == nil {
		return ""
	}
	return p.picker.View()
}

func (p *guidedWorkflowTemplatePicker) ensurePicker() *SelectPicker {
	if p == nil {
		return nil
	}
	if p.picker == nil {
		p.picker = NewSelectPicker(minViewportWidth, 8)
	}
	return p.picker
}

func guidedWorkflowTemplateLabel(option guidedWorkflowTemplateOption) string {
	name := strings.TrimSpace(option.name)
	id := strings.TrimSpace(option.id)
	switch {
	case name == "" && id == "":
		return ""
	case name == "":
		return id
	case id == "" || strings.EqualFold(name, id):
		return name
	default:
		return name + " (" + id + ")"
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
