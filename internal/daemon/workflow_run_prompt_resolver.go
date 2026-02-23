package daemon

import (
	"context"
	"strings"

	"control/internal/guidedworkflows"
)

type workflowRunPromptResolver interface {
	ResolveDisplayPrompt(ctx context.Context, run *guidedworkflows.WorkflowRun) string
}

type workflowRunPromptSource interface {
	ResolvePrompt(ctx context.Context, run *guidedworkflows.WorkflowRun) string
}

type workflowRunPromptFormatter interface {
	Format(prompt string) string
}

type defaultWorkflowRunPromptResolver struct {
	sources   []workflowRunPromptSource
	formatter workflowRunPromptFormatter
}

func newWorkflowRunPromptResolver(stores *Stores) workflowRunPromptResolver {
	resolver := &defaultWorkflowRunPromptResolver{
		sources: []workflowRunPromptSource{
			workflowRunPromptFieldSource{},
		},
		formatter: workflowRunPromptTrimFormatter{},
	}
	if stores != nil && stores.SessionMeta != nil {
		resolver.sources = append(resolver.sources, workflowRunSessionMetaPromptSource{
			sessionMeta: stores.SessionMeta,
		})
	}
	return resolver
}

func (r *defaultWorkflowRunPromptResolver) ResolveDisplayPrompt(ctx context.Context, run *guidedworkflows.WorkflowRun) string {
	if r == nil || run == nil {
		return ""
	}
	formatter := r.formatter
	if formatter == nil {
		formatter = workflowRunPromptTrimFormatter{}
	}
	for _, source := range r.sources {
		if source == nil {
			continue
		}
		prompt := formatter.Format(source.ResolvePrompt(ctx, run))
		if prompt != "" {
			return prompt
		}
	}
	return ""
}

type workflowRunPromptFieldSource struct{}

func (workflowRunPromptFieldSource) ResolvePrompt(_ context.Context, run *guidedworkflows.WorkflowRun) string {
	if run == nil {
		return ""
	}
	return strings.TrimSpace(run.UserPrompt)
}

type workflowRunSessionMetaPromptSource struct {
	sessionMeta SessionMetaStore
}

func (s workflowRunSessionMetaPromptSource) ResolvePrompt(ctx context.Context, run *guidedworkflows.WorkflowRun) string {
	if s.sessionMeta == nil || run == nil {
		return ""
	}
	if sessionID := strings.TrimSpace(run.SessionID); sessionID != "" {
		meta, ok, err := s.sessionMeta.Get(ctx, sessionID)
		if err == nil && ok && meta != nil {
			if prompt := strings.TrimSpace(meta.InitialInput); prompt != "" {
				return prompt
			}
		}
	}
	runID := strings.TrimSpace(run.ID)
	if runID == "" {
		return ""
	}
	entries, err := s.sessionMeta.List(ctx)
	if err != nil {
		return ""
	}
	for _, meta := range entries {
		if meta == nil || strings.TrimSpace(meta.WorkflowRunID) != runID {
			continue
		}
		if prompt := strings.TrimSpace(meta.InitialInput); prompt != "" {
			return prompt
		}
	}
	return ""
}

type workflowRunPromptTrimFormatter struct{}

func (workflowRunPromptTrimFormatter) Format(prompt string) string {
	return strings.TrimSpace(prompt)
}
