package app

import (
	"context"
	"errors"
	"testing"

	"control/internal/types"
)

func TestFetchSessionsWithMetaCmdUsesSessionListQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts fetchSessionsOptions
		want SessionListQuery
	}{
		{
			name: "default",
			opts: fetchSessionsOptions{},
			want: SessionListQuery{},
		},
		{
			name: "include dismissed and workflow owned",
			opts: fetchSessionsOptions{
				includeDismissed:     true,
				includeWorkflowOwned: true,
			},
			want: SessionListQuery{
				IncludeDismissed:     true,
				IncludeWorkflowOwned: true,
			},
		},
		{
			name: "refresh workspace query",
			opts: fetchSessionsOptions{
				refresh:              true,
				workspaceID:          "ws-1",
				includeDismissed:     true,
				includeWorkflowOwned: true,
			},
			want: SessionListQuery{
				Refresh:              true,
				WorkspaceID:          "ws-1",
				IncludeDismissed:     true,
				IncludeWorkflowOwned: true,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			api := &sessionListWithMetaQueryMock{
				sessionsToReturn: []*types.Session{{ID: "s-query"}},
			}
			cmd := fetchSessionsWithMetaCmd(api, tt.opts)
			msg, ok := cmd().(sessionsWithMetaMsg)
			if !ok {
				t.Fatalf("expected sessionsWithMetaMsg")
			}
			if msg.err != nil {
				t.Fatalf("unexpected error: %v", msg.err)
			}
			if len(msg.sessions) != 1 || msg.sessions[0].ID != "s-query" {
				t.Fatalf("unexpected sessions: %#v", msg.sessions)
			}
			if api.queryCalls != 1 {
				t.Fatalf("expected query api to be called once, got %d", api.queryCalls)
			}
			if api.lastQuery != tt.want {
				t.Fatalf("unexpected query: got %#v want %#v", api.lastQuery, tt.want)
			}
		})
	}
}

func TestModelFetchSessionsCmdShowDismissedUsesQueryContract(t *testing.T) {
	t.Parallel()

	m := NewModel(nil)
	api := &sessionListWithMetaQueryMock{
		sessionsToReturn: []*types.Session{{ID: "s-model"}},
	}
	m.sessionSelectionAPI = api
	m.setShowDismissed(true)

	cmd := m.fetchSessionsCmd(false)
	if cmd == nil {
		t.Fatalf("expected fetch sessions command")
	}
	msg, ok := cmd().(sessionsWithMetaMsg)
	if !ok {
		t.Fatalf("expected sessionsWithMetaMsg")
	}
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if api.queryCalls != 1 {
		t.Fatalf("expected query api call, got %d", api.queryCalls)
	}
	if !api.lastQuery.IncludeDismissed || !api.lastQuery.IncludeWorkflowOwned {
		t.Fatalf("expected include_dismissed/include_workflow_owned true, got %#v", api.lastQuery)
	}
}

func TestFetchSessionsWithMetaCmdPropagatesQueryError(t *testing.T) {
	t.Parallel()

	api := &sessionListWithMetaQueryMock{
		errToReturn: errors.New("query failed"),
	}
	cmd := fetchSessionsWithMetaCmd(api, fetchSessionsOptions{
		includeDismissed: true,
	})
	msg, ok := cmd().(sessionsWithMetaMsg)
	if !ok {
		t.Fatalf("expected sessionsWithMetaMsg")
	}
	if msg.err == nil {
		t.Fatalf("expected error to propagate")
	}
	if msg.err.Error() != "query failed" {
		t.Fatalf("unexpected error: %v", msg.err)
	}
}

func TestClientAPIImplementsSessionSelectionAPI(t *testing.T) {
	t.Parallel()
	var _ SessionSelectionAPI = (*ClientAPI)(nil)
}

type sessionListWithMetaQueryMock struct {
	sessionsToReturn []*types.Session
	metaToReturn     []*types.SessionMeta
	errToReturn      error

	queryCalls int
	lastQuery  SessionListQuery
}

func (m *sessionListWithMetaQueryMock) ListSessionsWithMetaQuery(_ context.Context, query SessionListQuery) ([]*types.Session, []*types.SessionMeta, error) {
	m.queryCalls++
	m.lastQuery = query
	return m.sessionsToReturn, m.metaToReturn, m.errToReturn
}
