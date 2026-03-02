package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

type SelectionLoadBootstrapInput struct {
	Provider     string
	Status       types.SessionStatus
	SessionID    string
	SessionKey   string
	InitialLines int
	LoadContext  context.Context
	HistoryAPI   SessionHistoryAPI
	SessionAPI   SessionAPI
}

type SessionStartBootstrapInput struct {
	Provider     string
	Status       types.SessionStatus
	SessionID    string
	SessionKey   string
	InitialLines int
	LoadContext  context.Context
	HistoryAPI   SessionHistoryAPI
	SessionAPI   SessionAPI
}

type SessionReconnectBootstrapInput struct {
	Provider             string
	SessionID            string
	SessionAPI           SessionAPI
	ItemStreamConnected  bool
	EventStreamConnected bool
}

type SessionBootstrapCoordinator interface {
	BuildSelectionLoadCommands(input SelectionLoadBootstrapInput) []tea.Cmd
	BuildSessionStartCommands(input SessionStartBootstrapInput) []tea.Cmd
	BuildReconnectCommands(input SessionReconnectBootstrapInput) []tea.Cmd
}

type defaultSessionBootstrapCoordinator struct {
	policy SessionBootstrapPolicy
}

func NewDefaultSessionBootstrapCoordinator(policy SessionBootstrapPolicy) SessionBootstrapCoordinator {
	return defaultSessionBootstrapCoordinator{policy: policy}
}

func WithSessionBootstrapCoordinator(coordinator SessionBootstrapCoordinator) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.sessionBootstrapCoordinator = coordinator
	}
}

func (c defaultSessionBootstrapCoordinator) BuildSelectionLoadCommands(input SelectionLoadBootstrapInput) []tea.Cmd {
	plan := c.policyOrDefault().SelectionLoadPlan(input.Provider, input.Status)
	return buildBootstrapCommands(plan, input.SessionID, input.SessionKey, input.InitialLines, input.LoadContext, input.HistoryAPI, input.SessionAPI)
}

func (c defaultSessionBootstrapCoordinator) BuildSessionStartCommands(input SessionStartBootstrapInput) []tea.Cmd {
	plan := c.policyOrDefault().SessionStartPlan(input.Provider, input.Status)
	return buildBootstrapCommands(plan, input.SessionID, input.SessionKey, input.InitialLines, input.LoadContext, input.HistoryAPI, input.SessionAPI)
}

func (c defaultSessionBootstrapCoordinator) BuildReconnectCommands(input SessionReconnectBootstrapInput) []tea.Cmd {
	plan := c.policyOrDefault().SendReconnectPlan(input.Provider)
	cmds := make([]tea.Cmd, 0, 2)
	if plan.OpenItems && !input.ItemStreamConnected {
		cmds = append(cmds, openItemsCmd(input.SessionAPI, input.SessionID))
	}
	if plan.OpenEvents && !input.EventStreamConnected {
		cmds = append(cmds, openEventsCmd(input.SessionAPI, input.SessionID))
	}
	return cmds
}

func (c defaultSessionBootstrapCoordinator) policyOrDefault() SessionBootstrapPolicy {
	if c.policy == nil {
		return defaultSessionBootstrapPolicy{}
	}
	return c.policy
}

func buildBootstrapCommands(
	plan sessionBootstrapPlan,
	sessionID, key string,
	lines int,
	loadCtx context.Context,
	historyAPI SessionHistoryAPI,
	sessionAPI SessionAPI,
) []tea.Cmd {
	cmds := make([]tea.Cmd, 0, 4)
	if plan.FetchHistory {
		cmds = append(cmds, fetchHistoryCmdWithContext(historyAPI, sessionID, key, lines, loadCtx))
	}
	if plan.FetchApprovals {
		cmds = append(cmds, fetchApprovalsCmdWithContext(sessionAPI, sessionID, loadCtx))
	}
	if plan.OpenItems {
		cmds = append(cmds, openItemsCmd(sessionAPI, sessionID))
	}
	if plan.OpenTail {
		cmds = append(cmds, openStreamCmd(sessionAPI, sessionID))
	}
	if plan.OpenEvents {
		cmds = append(cmds, openEventsCmd(sessionAPI, sessionID))
	}
	return cmds
}

func (m *Model) sessionBootstrapCoordinatorOrDefault() SessionBootstrapCoordinator {
	if m == nil || m.sessionBootstrapCoordinator == nil {
		return NewDefaultSessionBootstrapCoordinator(m.sessionBootstrapPolicyOrDefault())
	}
	return m.sessionBootstrapCoordinator
}
