package app

import (
	"testing"

	"control/internal/types"
)

func TestSessionBootstrapPolicySelectionPlans(t *testing.T) {
	p := defaultSessionBootstrapPolicy{}

	itemPlan := p.SelectionLoadPlan("kilocode", types.SessionStatusInactive)
	if !itemPlan.FetchTranscript || !itemPlan.FetchApprovals || itemPlan.OpenTranscript {
		t.Fatalf("unexpected item selection plan: %#v", itemPlan)
	}

	codexPlan := p.SelectionLoadPlan("codex", types.SessionStatusRunning)
	if !codexPlan.FetchTranscript || !codexPlan.FetchApprovals || codexPlan.OpenTranscript {
		t.Fatalf("unexpected codex selection plan: %#v", codexPlan)
	}

	customPlan := p.SelectionLoadPlan("custom", types.SessionStatusInactive)
	if !customPlan.FetchTranscript || !customPlan.FetchApprovals || customPlan.OpenTranscript {
		t.Fatalf("unexpected custom selection plan: %#v", customPlan)
	}
}

func TestSessionBootstrapPolicySessionStartPlans(t *testing.T) {
	p := defaultSessionBootstrapPolicy{}

	itemPlan := p.SessionStartPlan("claude", types.SessionStatusRunning)
	if !itemPlan.FetchTranscript || !itemPlan.FetchApprovals || itemPlan.OpenTranscript {
		t.Fatalf("unexpected item start plan: %#v", itemPlan)
	}

	codexPlan := p.SessionStartPlan("codex", types.SessionStatusRunning)
	if !codexPlan.FetchTranscript || !codexPlan.FetchApprovals || codexPlan.OpenTranscript {
		t.Fatalf("unexpected codex start plan: %#v", codexPlan)
	}

	customActive := p.SessionStartPlan("custom", types.SessionStatusRunning)
	if !customActive.FetchTranscript || !customActive.FetchApprovals || customActive.OpenTranscript {
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

	var nilModel *Model
	WithSessionBootstrapPolicy(defaultSessionBootstrapPolicy{})(nilModel)
}

func TestSessionBootstrapPolicyPrefersSharedTranscriptFollowForSameSession(t *testing.T) {
	if !prefersSharedTranscriptFollow("s1", "s1") {
		t.Fatalf("expected shared follow preference for same active session")
	}
	if prefersSharedTranscriptFollow("s1", "s2") {
		t.Fatalf("expected no shared follow preference for different sessions")
	}
	if prefersSharedTranscriptFollow("", "s1") {
		t.Fatalf("expected no shared follow preference when active session is empty")
	}
}

func TestSessionBootstrapPolicyOrDefaultHandlesNilModel(t *testing.T) {
	var nilModel *Model
	if _, ok := nilModel.sessionBootstrapPolicyOrDefault().(defaultSessionBootstrapPolicy); !ok {
		t.Fatalf("expected nil model bootstrap policy fallback")
	}
}
