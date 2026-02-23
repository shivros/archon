package app

import (
	"strings"

	"control/internal/guidedworkflows"
)

const (
	workflowPromptUnavailable = "(prompt unavailable)"
	workflowPromptMaxRunes    = 160
)

type workflowPromptPresenter interface {
	Present(run *guidedworkflows.WorkflowRun) string
}

type workflowPromptSource interface {
	Resolve(run *guidedworkflows.WorkflowRun) string
}

type workflowPromptFormatter interface {
	Format(prompt string) string
}

type defaultWorkflowPromptPresenter struct {
	sources   []workflowPromptSource
	formatter workflowPromptFormatter
}

func newWorkflowPromptPresenter() workflowPromptPresenter {
	return &defaultWorkflowPromptPresenter{
		sources: []workflowPromptSource{
			workflowDisplayPromptSource{},
			workflowUserPromptSource{},
		},
		formatter: workflowPromptSummaryFormatter{
			maxRunes: workflowPromptMaxRunes,
		},
	}
}

func (p *defaultWorkflowPromptPresenter) Present(run *guidedworkflows.WorkflowRun) string {
	if p == nil {
		return workflowPromptUnavailable
	}
	formatter := p.formatter
	if formatter == nil {
		formatter = workflowPromptSummaryFormatter{maxRunes: workflowPromptMaxRunes}
	}
	for _, source := range p.sources {
		if source == nil {
			continue
		}
		prompt := formatter.Format(source.Resolve(run))
		if prompt != "" {
			return prompt
		}
	}
	return workflowPromptUnavailable
}

type workflowDisplayPromptSource struct{}

func (workflowDisplayPromptSource) Resolve(run *guidedworkflows.WorkflowRun) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(run.DisplayUserPrompt)
}

type workflowUserPromptSource struct{}

func (workflowUserPromptSource) Resolve(run *guidedworkflows.WorkflowRun) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(run.UserPrompt)
}

type workflowPromptSummaryFormatter struct {
	maxRunes int
}

func (f workflowPromptSummaryFormatter) Format(prompt string) string {
	prompt = strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
	if prompt == "" {
		return ""
	}
	return truncateRunes(prompt, f.maxRunes)
}
