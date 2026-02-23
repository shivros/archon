package daemon

import (
	"context"
	"errors"
	"testing"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

func TestWorkflowRunPromptResolverPrefersRunUserPrompt(t *testing.T) {
	resolver := newWorkflowRunPromptResolver(&Stores{
		SessionMeta: &stubWorkflowRunPromptSessionMetaStore{
			entries: map[string]*types.SessionMeta{
				"s1": {SessionID: "s1", InitialInput: "from session meta"},
			},
		},
	})
	run := &guidedworkflows.WorkflowRun{
		ID:         "gwf-1",
		SessionID:  "s1",
		UserPrompt: "from run",
	}
	if got := resolver.ResolveDisplayPrompt(context.Background(), run); got != "from run" {
		t.Fatalf("expected run prompt to win, got %q", got)
	}
}

func TestWorkflowRunPromptResolverFallsBackToSessionMetaBySessionID(t *testing.T) {
	resolver := newWorkflowRunPromptResolver(&Stores{
		SessionMeta: &stubWorkflowRunPromptSessionMetaStore{
			entries: map[string]*types.SessionMeta{
				"s1": {SessionID: "s1", InitialInput: "from session meta"},
			},
		},
	})
	run := &guidedworkflows.WorkflowRun{
		ID:        "gwf-1",
		SessionID: "s1",
	}
	if got := resolver.ResolveDisplayPrompt(context.Background(), run); got != "from session meta" {
		t.Fatalf("expected session-meta fallback, got %q", got)
	}
}

func TestWorkflowRunPromptResolverFallsBackToSessionMetaByRunID(t *testing.T) {
	resolver := newWorkflowRunPromptResolver(&Stores{
		SessionMeta: &stubWorkflowRunPromptSessionMetaStore{
			listEntries: []*types.SessionMeta{
				{SessionID: "s1", WorkflowRunID: "gwf-1", InitialInput: "from linked run"},
			},
		},
	})
	run := &guidedworkflows.WorkflowRun{ID: "gwf-1"}
	if got := resolver.ResolveDisplayPrompt(context.Background(), run); got != "from linked run" {
		t.Fatalf("expected run-id fallback, got %q", got)
	}
}

func TestWorkflowRunPromptResolverReturnsEmptyWhenUnavailable(t *testing.T) {
	resolver := newWorkflowRunPromptResolver(&Stores{
		SessionMeta: &stubWorkflowRunPromptSessionMetaStore{
			getErr:  errors.New("get failed"),
			listErr: errors.New("list failed"),
		},
	})
	run := &guidedworkflows.WorkflowRun{ID: "gwf-1", SessionID: "s1"}
	if got := resolver.ResolveDisplayPrompt(context.Background(), run); got != "" {
		t.Fatalf("expected empty prompt on errors, got %q", got)
	}
}

func TestWorkflowRunPromptResolverNilReceiverAndFormatterFallback(t *testing.T) {
	var nilResolver *defaultWorkflowRunPromptResolver
	if got := nilResolver.ResolveDisplayPrompt(context.Background(), &guidedworkflows.WorkflowRun{UserPrompt: "x"}); got != "" {
		t.Fatalf("expected empty prompt from nil resolver, got %q", got)
	}

	resolver := &defaultWorkflowRunPromptResolver{
		sources: []workflowRunPromptSource{
			workflowRunPromptFieldSource{},
		},
	}
	if got := resolver.ResolveDisplayPrompt(context.Background(), &guidedworkflows.WorkflowRun{UserPrompt: "  from run  "}); got != "from run" {
		t.Fatalf("expected default formatter fallback, got %q", got)
	}
}

func TestWorkflowRunPromptResolverSkipsNilSource(t *testing.T) {
	resolver := &defaultWorkflowRunPromptResolver{
		sources: []workflowRunPromptSource{
			nil,
			workflowRunPromptFieldSource{},
		},
		formatter: workflowRunPromptTrimFormatter{},
	}
	if got := resolver.ResolveDisplayPrompt(context.Background(), &guidedworkflows.WorkflowRun{UserPrompt: "prompt"}); got != "prompt" {
		t.Fatalf("expected resolver to skip nil source, got %q", got)
	}
}

func TestWorkflowRunPromptSourcesHandleNilInputs(t *testing.T) {
	if got := (workflowRunPromptFieldSource{}).ResolvePrompt(context.Background(), nil); got != "" {
		t.Fatalf("expected empty prompt for nil run, got %q", got)
	}
	if got := (workflowRunSessionMetaPromptSource{}).ResolvePrompt(context.Background(), &guidedworkflows.WorkflowRun{ID: "gwf-1"}); got != "" {
		t.Fatalf("expected empty prompt for nil session-meta store, got %q", got)
	}
	if got := (workflowRunSessionMetaPromptSource{sessionMeta: &stubWorkflowRunPromptSessionMetaStore{}}).ResolvePrompt(context.Background(), nil); got != "" {
		t.Fatalf("expected empty prompt for nil run in session-meta source, got %q", got)
	}
}

func TestWorkflowRunSessionMetaPromptSourceReturnsEmptyWhenRunIDMissing(t *testing.T) {
	source := workflowRunSessionMetaPromptSource{
		sessionMeta: &stubWorkflowRunPromptSessionMetaStore{
			listEntries: []*types.SessionMeta{
				{SessionID: "s1", WorkflowRunID: "gwf-1", InitialInput: "from linked run"},
			},
		},
	}
	run := &guidedworkflows.WorkflowRun{
		ID:        "   ",
		SessionID: "",
	}
	if got := source.ResolvePrompt(context.Background(), run); got != "" {
		t.Fatalf("expected empty prompt when run id is missing, got %q", got)
	}
}

type stubWorkflowRunPromptSessionMetaStore struct {
	entries     map[string]*types.SessionMeta
	listEntries []*types.SessionMeta
	getErr      error
	listErr     error
}

func (s *stubWorkflowRunPromptSessionMetaStore) List(context.Context) ([]*types.SessionMeta, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if len(s.listEntries) > 0 {
		out := make([]*types.SessionMeta, 0, len(s.listEntries))
		for _, meta := range s.listEntries {
			if meta == nil {
				continue
			}
			copyMeta := *meta
			out = append(out, &copyMeta)
		}
		return out, nil
	}
	out := make([]*types.SessionMeta, 0, len(s.entries))
	for _, meta := range s.entries {
		if meta == nil {
			continue
		}
		copyMeta := *meta
		out = append(out, &copyMeta)
	}
	return out, nil
}

func (s *stubWorkflowRunPromptSessionMetaStore) Get(_ context.Context, sessionID string) (*types.SessionMeta, bool, error) {
	if s.getErr != nil {
		return nil, false, s.getErr
	}
	if s.entries == nil {
		return nil, false, nil
	}
	meta, ok := s.entries[sessionID]
	if !ok || meta == nil {
		return nil, false, nil
	}
	copyMeta := *meta
	return &copyMeta, true, nil
}

func (s *stubWorkflowRunPromptSessionMetaStore) Upsert(_ context.Context, meta *types.SessionMeta) (*types.SessionMeta, error) {
	if s.entries == nil {
		s.entries = map[string]*types.SessionMeta{}
	}
	if meta == nil {
		return nil, nil
	}
	copyMeta := *meta
	s.entries[meta.SessionID] = &copyMeta
	return &copyMeta, nil
}

func (s *stubWorkflowRunPromptSessionMetaStore) Delete(_ context.Context, sessionID string) error {
	if s.entries != nil {
		delete(s.entries, sessionID)
	}
	return nil
}
