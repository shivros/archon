package app

import (
	"testing"

	"control/internal/types"
)

func TestSessionBootstrapPolicySelectionPlans(t *testing.T) {
	p := defaultSessionBootstrapPolicy{}

	itemPlan := p.SelectionLoadPlan("kilocode", types.SessionStatusInactive)
	if itemPlan.FetchHistory || !itemPlan.FetchApprovals || !itemPlan.OpenItems || itemPlan.OpenTail || itemPlan.OpenEvents {
		t.Fatalf("unexpected item selection plan: %#v", itemPlan)
	}

	codexPlan := p.SelectionLoadPlan("codex", types.SessionStatusRunning)
	if !codexPlan.FetchHistory || !codexPlan.FetchApprovals || !codexPlan.OpenTail || !codexPlan.OpenEvents || codexPlan.OpenItems {
		t.Fatalf("unexpected codex selection plan: %#v", codexPlan)
	}

	customPlan := p.SelectionLoadPlan("custom", types.SessionStatusInactive)
	if !customPlan.FetchHistory || !customPlan.FetchApprovals || customPlan.OpenTail || customPlan.OpenEvents || customPlan.OpenItems {
		t.Fatalf("unexpected custom selection plan: %#v", customPlan)
	}
}

func TestSessionBootstrapPolicySessionStartPlans(t *testing.T) {
	p := defaultSessionBootstrapPolicy{}

	itemPlan := p.SessionStartPlan("claude", types.SessionStatusRunning)
	if itemPlan.FetchHistory || !itemPlan.FetchApprovals || !itemPlan.OpenItems || itemPlan.OpenTail || itemPlan.OpenEvents {
		t.Fatalf("unexpected item start plan: %#v", itemPlan)
	}

	codexPlan := p.SessionStartPlan("codex", types.SessionStatusRunning)
	if !codexPlan.FetchHistory || !codexPlan.FetchApprovals || !codexPlan.OpenEvents || codexPlan.OpenTail || codexPlan.OpenItems {
		t.Fatalf("unexpected codex start plan: %#v", codexPlan)
	}

	customActive := p.SessionStartPlan("custom", types.SessionStatusRunning)
	if !customActive.FetchHistory || !customActive.FetchApprovals || !customActive.OpenTail || customActive.OpenEvents || customActive.OpenItems {
		t.Fatalf("unexpected custom active start plan: %#v", customActive)
	}
}

func TestWithSessionBootstrapPolicyOption(t *testing.T) {
	model := NewModel(nil, WithSessionBootstrapPolicy(defaultSessionBootstrapPolicy{}))
	if model.sessionBootstrapPolicy == nil {
		t.Fatalf("expected bootstrap policy to be set")
	}

	model2 := NewModel(nil, WithSessionBootstrapPolicy(nil))
	if model2.sessionBootstrapPolicy == nil {
		t.Fatalf("expected default bootstrap policy when nil policy passed")
	}
	if _, ok := model2.sessionBootstrapPolicyOrDefault().(defaultSessionBootstrapPolicy); !ok {
		t.Fatalf("expected default bootstrap policy fallback")
	}
}
