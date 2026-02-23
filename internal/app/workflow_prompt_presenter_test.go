package app

import (
	"strings"
	"testing"

	"control/internal/guidedworkflows"
)

func TestWorkflowPromptPresenterPrefersDisplayPrompt(t *testing.T) {
	presenter := newWorkflowPromptPresenter()
	run := &guidedworkflows.WorkflowRun{
		UserPrompt:        "new run prompt",
		DisplayUserPrompt: "legacy session prompt",
	}
	if got := presenter.Present(run); got != "legacy session prompt" {
		t.Fatalf("expected display prompt to win, got %q", got)
	}
}

func TestWorkflowPromptPresenterFallsBackToUserPrompt(t *testing.T) {
	presenter := newWorkflowPromptPresenter()
	run := &guidedworkflows.WorkflowRun{
		UserPrompt: "fix parser and add tests",
	}
	if got := presenter.Present(run); got != "fix parser and add tests" {
		t.Fatalf("expected user prompt fallback, got %q", got)
	}
}

func TestWorkflowPromptPresenterUnavailableFallback(t *testing.T) {
	presenter := newWorkflowPromptPresenter()
	if got := presenter.Present(&guidedworkflows.WorkflowRun{}); got != workflowPromptUnavailable {
		t.Fatalf("expected unavailable fallback, got %q", got)
	}
}

func TestWorkflowPromptPresenterFormatsWhitespaceAndTruncates(t *testing.T) {
	presenter := newWorkflowPromptPresenter()
	longPrompt := "  first line\nsecond line\t" + strings.Repeat("x", workflowPromptMaxRunes+32) + "  "
	got := presenter.Present(&guidedworkflows.WorkflowRun{UserPrompt: longPrompt})
	if strings.Contains(got, "\n") || strings.Contains(got, "\t") {
		t.Fatalf("expected flattened whitespace, got %q", got)
	}
	if len([]rune(got)) > workflowPromptMaxRunes+3 {
		t.Fatalf("expected truncated prompt, got length %d", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis suffix for truncated prompt, got %q", got)
	}
}

func TestWorkflowPromptPresenterNilReceiverFallback(t *testing.T) {
	var presenter *defaultWorkflowPromptPresenter
	if got := presenter.Present(&guidedworkflows.WorkflowRun{UserPrompt: "x"}); got != workflowPromptUnavailable {
		t.Fatalf("expected unavailable fallback from nil presenter, got %q", got)
	}
}

func TestWorkflowPromptPresenterNilFormatterFallback(t *testing.T) {
	presenter := &defaultWorkflowPromptPresenter{
		sources: []workflowPromptSource{
			workflowUserPromptSource{},
		},
	}
	if got := presenter.Present(&guidedworkflows.WorkflowRun{UserPrompt: "  trim me  "}); got != "trim me" {
		t.Fatalf("expected default formatter fallback, got %q", got)
	}
}

func TestWorkflowPromptPresenterSkipsNilSource(t *testing.T) {
	presenter := &defaultWorkflowPromptPresenter{
		sources: []workflowPromptSource{
			nil,
			workflowUserPromptSource{},
		},
		formatter: workflowPromptSummaryFormatter{maxRunes: workflowPromptMaxRunes},
	}
	if got := presenter.Present(&guidedworkflows.WorkflowRun{UserPrompt: "prompt"}); got != "prompt" {
		t.Fatalf("expected presenter to skip nil source, got %q", got)
	}
}

func TestWorkflowPromptSourcesHandleNilRun(t *testing.T) {
	if got := (workflowDisplayPromptSource{}).Resolve(nil); got != "" {
		t.Fatalf("expected empty display prompt for nil run, got %q", got)
	}
	if got := (workflowUserPromptSource{}).Resolve(nil); got != "" {
		t.Fatalf("expected empty user prompt for nil run, got %q", got)
	}
}
