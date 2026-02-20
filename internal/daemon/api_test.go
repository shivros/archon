package daemon

import (
	"testing"

	"control/internal/types"
)

func TestAPIWorkflowDispatchDefaultsAccessor(t *testing.T) {
	var nilAPI *API
	if got := nilAPI.workflowDispatchDefaults(); got.Provider != "" || got.Model != "" || got.Access != "" || got.Reasoning != "" {
		t.Fatalf("expected zero-value defaults for nil API, got %+v", got)
	}

	api := &API{}
	if got := api.workflowDispatchDefaults(); got.Provider != "" || got.Model != "" || got.Access != "" || got.Reasoning != "" {
		t.Fatalf("expected zero-value defaults for empty API, got %+v", got)
	}

	api.WorkflowDispatchDefaults = guidedWorkflowDispatchDefaults{
		Provider:  "opencode",
		Model:     "gpt-5.3-codex",
		Access:    types.AccessReadOnly,
		Reasoning: types.ReasoningHigh,
	}
	got := api.workflowDispatchDefaults()
	if got.Provider != "opencode" {
		t.Fatalf("expected configured provider, got %q", got.Provider)
	}
	if got.Model != "gpt-5.3-codex" {
		t.Fatalf("expected configured model, got %q", got.Model)
	}
	if got.Access != types.AccessReadOnly {
		t.Fatalf("expected configured access, got %q", got.Access)
	}
	if got.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected configured reasoning, got %q", got.Reasoning)
	}
}
