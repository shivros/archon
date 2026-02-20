package guidedworkflows

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

type workflowTemplateDefaultsFile struct {
	Version   int                `json:"version"`
	Templates []WorkflowTemplate `json:"templates"`
}

var (
	//go:embed default_workflow_templates.json
	defaultWorkflowTemplatesJSON []byte

	defaultWorkflowTemplatesOnce sync.Once
	defaultWorkflowTemplates     []WorkflowTemplate
)

func DefaultWorkflowTemplates() []WorkflowTemplate {
	defaultWorkflowTemplatesOnce.Do(func() {
		defaultWorkflowTemplates = mustParseDefaultWorkflowTemplates(defaultWorkflowTemplatesJSON)
	})
	return cloneWorkflowTemplateSlice(defaultWorkflowTemplates)
}

func defaultWorkflowTemplateByID(id string) (WorkflowTemplate, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return WorkflowTemplate{}, false
	}
	templates := DefaultWorkflowTemplates()
	for _, tpl := range templates {
		if strings.TrimSpace(tpl.ID) != id {
			continue
		}
		return cloneTemplate(tpl), true
	}
	return WorkflowTemplate{}, false
}

func mustParseDefaultWorkflowTemplates(raw []byte) []WorkflowTemplate {
	var file workflowTemplateDefaultsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		panic("guidedworkflows: failed to parse default workflow templates JSON: " + err.Error())
	}
	if len(file.Templates) == 0 {
		panic("guidedworkflows: default workflow templates JSON contains no templates")
	}
	out := make([]WorkflowTemplate, 0, len(file.Templates))
	for _, tpl := range file.Templates {
		id := strings.TrimSpace(tpl.ID)
		if id == "" {
			continue
		}
		if !templateHasSteps(tpl) {
			continue
		}
		normalizedAccess, ok := NormalizeTemplateAccessLevel(tpl.DefaultAccessLevel)
		if strings.TrimSpace(string(tpl.DefaultAccessLevel)) != "" && !ok {
			continue
		}
		tpl.ID = id
		if ok {
			tpl.DefaultAccessLevel = normalizedAccess
		}
		out = append(out, cloneTemplate(tpl))
	}
	if len(out) == 0 {
		panic("guidedworkflows: default workflow templates JSON has no valid templates")
	}
	return out
}

func cloneWorkflowTemplateSlice(in []WorkflowTemplate) []WorkflowTemplate {
	out := make([]WorkflowTemplate, 0, len(in))
	for _, tpl := range in {
		out = append(out, cloneTemplate(tpl))
	}
	return out
}
