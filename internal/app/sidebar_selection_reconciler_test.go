package app

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/list"

	"control/internal/types"
)

type staticSidebarSelectionStrategy struct {
	reason sidebarSelectionResolutionReason
	key    string
}

func (s staticSidebarSelectionStrategy) Reason() sidebarSelectionResolutionReason {
	return s.reason
}

func (s staticSidebarSelectionStrategy) Candidate(sidebarSelectionDecisionInput) string {
	return s.key
}

type stubSidebarSelectionLookup struct {
	sessions map[string]struct{}
}

func (s stubSidebarSelectionLookup) HasSession(sessionID string) bool {
	if len(s.sessions) == 0 {
		return false
	}
	_, ok := s.sessions[sessionID]
	return ok
}

func TestSidebarSelectionDecisionServicePreservesExactVisibleKey(t *testing.T) {
	decider := NewDefaultSidebarSelectionDecisionService()
	items := []list.Item{
		&sidebarItem{kind: sidebarWorkspace, workspace: &types.Workspace{ID: "ws1", Name: "Workspace"}},
		&sidebarItem{kind: sidebarSession, session: &types.Session{ID: "s1", CreatedAt: time.Now().UTC()}},
	}
	decision := decider.Decide(sidebarSelectionDecisionInput{
		Items:       items,
		SelectedKey: "sess:s1",
	})
	if len(decision.Candidates) == 0 || decision.Candidates[0] != "sess:s1" {
		t.Fatalf("expected exact key candidate first, got %+v", decision.Candidates)
	}
	if decision.Reason != sidebarSelectionReasonPreserveKey {
		t.Fatalf("expected preserve-key reason, got %q", decision.Reason)
	}
}

func TestSidebarSelectionDecisionServicePreservesSessionIdentityWhenHidden(t *testing.T) {
	decider := NewDefaultSidebarSelectionDecisionService()
	items := []list.Item{
		&sidebarItem{kind: sidebarWorkspace, workspace: &types.Workspace{ID: "ws1", Name: "Workspace"}},
		&sidebarItem{kind: sidebarWorkflow, workflowID: "gwf-1"},
	}
	decision := decider.Decide(sidebarSelectionDecisionInput{
		Items:       items,
		SelectedKey: "sess:s1",
		Lookup: stubSidebarSelectionLookup{
			sessions: map[string]struct{}{"s1": {}},
		},
	})
	if len(decision.Candidates) == 0 || decision.Candidates[0] != "sess:s1" {
		t.Fatalf("expected hidden selected session candidate first, got %+v", decision.Candidates)
	}
	if decision.Reason != sidebarSelectionReasonPreserveSession {
		t.Fatalf("expected preserve-session reason, got %q", decision.Reason)
	}
}

func TestSidebarSelectionDecisionServiceBackgroundPrefersVisibleSession(t *testing.T) {
	decider := NewDefaultSidebarSelectionDecisionService()
	items := []list.Item{
		&sidebarItem{kind: sidebarWorktree, worktree: &types.Worktree{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree"}},
		&sidebarItem{kind: sidebarWorkflow, workflowID: "gwf-1"},
		&sidebarItem{
			kind:    sidebarSession,
			session: &types.Session{ID: "s2", CreatedAt: time.Now().UTC()},
			meta:    &types.SessionMeta{SessionID: "s2", WorkspaceID: "ws1", WorktreeID: "wt2"},
		},
	}
	decision := decider.Decide(sidebarSelectionDecisionInput{
		Items:            items,
		SelectedKey:      "sess:s1",
		ActiveWorktreeID: "wt1",
		Reason:           sidebarApplyReasonBackground,
		Lookup:           stubSidebarSelectionLookup{},
	})
	if len(decision.Candidates) == 0 || decision.Candidates[0] != "sess:s2" {
		t.Fatalf("expected background fallback to visible session, got %+v", decision.Candidates)
	}
	if decision.Reason != sidebarSelectionReasonActiveContext {
		t.Fatalf("expected active-context reason, got %q", decision.Reason)
	}
}

type stubListItem struct{ value string }

func (s stubListItem) FilterValue() string { return s.value }

func TestSidebarSelectionDecisionServiceFiltersNilAndDuplicateStrategies(t *testing.T) {
	decider := defaultSidebarSelectionDecisionService{
		strategies: []sidebarSelectionStrategy{
			nil,
			staticSidebarSelectionStrategy{reason: sidebarSelectionReasonVisibleFallback, key: "sess:s1"},
			staticSidebarSelectionStrategy{reason: sidebarSelectionReasonVisibleSession, key: "sess:s1"},
			staticSidebarSelectionStrategy{reason: sidebarSelectionReasonActiveContext, key: "ws:ws1"},
		},
	}
	decision := decider.Decide(sidebarSelectionDecisionInput{Items: []list.Item{&sidebarItem{kind: sidebarWorkspace, workspace: &types.Workspace{ID: "ws1"}}}})
	if len(decision.Candidates) != 2 {
		t.Fatalf("expected 2 unique candidates, got %+v", decision.Candidates)
	}
	if decision.Candidates[0] != "sess:s1" || decision.Candidates[1] != "ws:ws1" {
		t.Fatalf("unexpected candidate order: %+v", decision.Candidates)
	}
	if decision.Reason != sidebarSelectionReasonVisibleFallback {
		t.Fatalf("expected first strategy reason to win, got %q", decision.Reason)
	}
}

func TestSidebarSelectionDecisionServiceBackgroundNoVisibleSessionKeepsActiveContextCandidate(t *testing.T) {
	decider := NewDefaultSidebarSelectionDecisionService()
	items := []list.Item{
		&sidebarItem{kind: sidebarWorkspace, workspace: &types.Workspace{ID: "ws1", Name: "Workspace"}},
		&sidebarItem{kind: sidebarWorktree, worktree: &types.Worktree{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree"}},
	}
	decision := decider.Decide(sidebarSelectionDecisionInput{
		Items:            items,
		SelectedKey:      "sess:s1",
		ActiveWorktreeID: "wt1",
		Reason:           sidebarApplyReasonBackground,
		Lookup:           stubSidebarSelectionLookup{},
	})
	if len(decision.Candidates) == 0 || decision.Candidates[0] != "wt:wt1" {
		t.Fatalf("expected active-context worktree candidate, got %+v", decision.Candidates)
	}
}

func TestSidebarSelectionDecisionStrategiesExposeReasons(t *testing.T) {
	if got := (firstVisibleSessionFallbackStrategy{}).Reason(); got != sidebarSelectionReasonVisibleSession {
		t.Fatalf("unexpected session fallback reason: %q", got)
	}
	if got := (firstVisibleItemFallbackStrategy{}).Reason(); got != sidebarSelectionReasonVisibleFallback {
		t.Fatalf("unexpected item fallback reason: %q", got)
	}
}

func TestFirstVisibleItemKeyReturnsEmptyWhenNoSidebarItems(t *testing.T) {
	items := []list.Item{stubListItem{value: "external"}}
	if got := firstVisibleItemKey(items); got != "" {
		t.Fatalf("expected empty key, got %q", got)
	}
}

func TestActiveContextFallbackStrategyReturnsEmptyWhenSelectedItemIsNotSidebarItem(t *testing.T) {
	strategy := activeContextFallbackStrategy{}
	key := strategy.Candidate(sidebarSelectionDecisionInput{
		Items: []list.Item{
			stubListItem{value: "external"},
		},
	})
	if key != "" {
		t.Fatalf("expected empty candidate, got %q", key)
	}
}

func TestActiveContextFallbackStrategyReturnsEmptyWhenSidebarKeyIsEmpty(t *testing.T) {
	strategy := activeContextFallbackStrategy{}
	key := strategy.Candidate(sidebarSelectionDecisionInput{
		Items: []list.Item{
			&sidebarItem{kind: sidebarItemKind(999)},
		},
	})
	if key != "" {
		t.Fatalf("expected empty candidate for keyless sidebar item, got %q", key)
	}
}
