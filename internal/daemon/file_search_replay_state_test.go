package daemon

import (
	"testing"

	"control/internal/types"
)

func TestFileSearchReplayStateApplyHandlesNilReceiver(t *testing.T) {
	var state *fileSearchReplayState
	state.Apply(&types.FileSearchSession{ID: "fs-1"}, types.FileSearchEvent{Kind: types.FileSearchEventUpdated})
}

func TestFileSearchReplayStateReplayEventHandlesNilReceiver(t *testing.T) {
	var state *fileSearchReplayState
	if replay := state.ReplayEvent(&types.FileSearchSession{ID: "fs-1"}); replay != nil {
		t.Fatalf("expected nil replay for nil receiver, got %#v", replay)
	}
}

func TestFileSearchReplayStatePrefersLatestMatchingResults(t *testing.T) {
	state := newFileSearchReplayState()
	session := &types.FileSearchSession{
		ID:       "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex", WorkspaceID: "ws-1"},
		Query:    "main",
		Limit:    5,
		Status:   types.FileSearchStatusActive,
	}

	state.Apply(session, types.FileSearchEvent{
		Kind:     types.FileSearchEventResults,
		SearchID: "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex", WorkspaceID: "ws-1"},
		Query:    "main",
		Limit:    5,
		Status:   types.FileSearchStatusActive,
		Candidates: []types.FileSearchCandidate{
			{Path: "main.go", DisplayPath: "./main.go"},
		},
	})
	state.Apply(session, types.FileSearchEvent{
		Kind:     types.FileSearchEventUpdated,
		SearchID: "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex", WorkspaceID: "ws-1"},
		Query:    "main",
		Limit:    5,
		Status:   types.FileSearchStatusActive,
	})

	replay := state.ReplayEvent(session)
	if replay == nil || replay.Kind != types.FileSearchEventResults || len(replay.Candidates) != 1 || replay.Candidates[0].Path != "main.go" {
		t.Fatalf("expected replayed results, got %#v", replay)
	}
}

func TestFileSearchReplayStateDropsStaleResultsWhenSessionChanges(t *testing.T) {
	state := newFileSearchReplayState()
	initial := &types.FileSearchSession{
		ID:       "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex"},
		Query:    "main",
		Limit:    5,
		Status:   types.FileSearchStatusActive,
	}

	state.Apply(initial, types.FileSearchEvent{
		Kind:     types.FileSearchEventResults,
		SearchID: "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex"},
		Query:    "main",
		Limit:    5,
		Status:   types.FileSearchStatusActive,
		Candidates: []types.FileSearchCandidate{
			{Path: "main.go", DisplayPath: "./main.go"},
		},
	})

	updated := cloneFileSearchSession(initial)
	updated.Query = "other"
	updated.Limit = 7
	state.Apply(updated, types.FileSearchEvent{
		Kind:     types.FileSearchEventUpdated,
		SearchID: "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex"},
		Query:    "other",
		Limit:    7,
		Status:   types.FileSearchStatusActive,
	})

	replay := state.ReplayEvent(updated)
	if replay == nil || replay.Kind != types.FileSearchEventUpdated || replay.Query != "other" || replay.Limit != 7 {
		t.Fatalf("expected replayed updated event, got %#v", replay)
	}
}

func TestFileSearchReplayStateSuppressesTerminalReplay(t *testing.T) {
	state := newFileSearchReplayState()
	session := &types.FileSearchSession{
		ID:       "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex"},
		Query:    "main",
		Limit:    5,
		Status:   types.FileSearchStatusFailed,
	}

	state.Apply(session, types.FileSearchEvent{
		Kind:     types.FileSearchEventFailed,
		SearchID: "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex"},
		Query:    "main",
		Limit:    5,
		Status:   types.FileSearchStatusFailed,
		Error:    "boom",
	})

	if replay := state.ReplayEvent(session); replay != nil {
		t.Fatalf("expected no replay for terminal event, got %#v", replay)
	}
}

func TestFileSearchReplayStateFallsBackToLatestNonTerminalEventWithoutResults(t *testing.T) {
	state := newFileSearchReplayState()
	session := &types.FileSearchSession{
		ID:       "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex"},
		Query:    "main",
		Limit:    5,
		Status:   types.FileSearchStatusActive,
	}

	state.Apply(session, types.FileSearchEvent{
		Kind:     types.FileSearchEventUpdated,
		SearchID: "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex"},
		Query:    "main",
		Limit:    5,
		Status:   types.FileSearchStatusActive,
	})

	replay := state.ReplayEvent(session)
	if replay == nil || replay.Kind != types.FileSearchEventUpdated || replay.Query != "main" {
		t.Fatalf("expected replayed updated event, got %#v", replay)
	}
}

func TestFileSearchResultsEventMatchesSession(t *testing.T) {
	session := &types.FileSearchSession{
		ID:       "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex", WorkspaceID: "ws-1"},
		Query:    "main",
		Limit:    5,
	}
	base := &types.FileSearchEvent{
		Kind:     types.FileSearchEventResults,
		SearchID: "fs-1",
		Provider: "codex",
		Scope:    types.FileSearchScope{Provider: "codex", WorkspaceID: "ws-1"},
		Query:    "main",
		Limit:    5,
	}

	tests := []struct {
		name    string
		event   *types.FileSearchEvent
		session *types.FileSearchSession
		want    bool
	}{
		{name: "match", event: base, session: session, want: true},
		{name: "nil event", event: nil, session: session, want: false},
		{name: "nil session", event: base, session: nil, want: false},
		{name: "wrong kind", event: &types.FileSearchEvent{Kind: types.FileSearchEventUpdated, SearchID: "fs-1", Provider: "codex", Scope: base.Scope, Query: "main", Limit: 5}, session: session, want: false},
		{name: "wrong search id", event: &types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: "fs-2", Provider: "codex", Scope: base.Scope, Query: "main", Limit: 5}, session: session, want: false},
		{name: "wrong provider", event: &types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: "fs-1", Provider: "stub", Scope: base.Scope, Query: "main", Limit: 5}, session: session, want: false},
		{name: "wrong scope", event: &types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: "fs-1", Provider: "codex", Scope: types.FileSearchScope{Provider: "codex", WorkspaceID: "ws-2"}, Query: "main", Limit: 5}, session: session, want: false},
		{name: "wrong query", event: &types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: "fs-1", Provider: "codex", Scope: base.Scope, Query: "other", Limit: 5}, session: session, want: false},
		{name: "wrong limit", event: &types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: "fs-1", Provider: "codex", Scope: base.Scope, Query: "main", Limit: 7}, session: session, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fileSearchResultsEventMatchesSession(tc.event, tc.session); got != tc.want {
				t.Fatalf("fileSearchResultsEventMatchesSession() = %v, want %v", got, tc.want)
			}
		})
	}
}
