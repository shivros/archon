package app

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

type SelectionLoadBootstrapInput struct {
	Provider      string
	Status        types.SessionStatus
	SessionID     string
	SessionKey    string
	InitialLines  int
	AfterRevision string
	LoadContext   context.Context
	TranscriptAPI SessionTranscriptAPI
	SessionAPI    SessionAPI
}

type SessionStartBootstrapInput struct {
	Provider      string
	Status        types.SessionStatus
	SessionID     string
	SessionKey    string
	InitialLines  int
	AfterRevision string
	LoadContext   context.Context
	TranscriptAPI SessionTranscriptAPI
	SessionAPI    SessionAPI
}

type SessionReconnectBootstrapInput struct {
	Provider                  string
	SessionID                 string
	AfterRevision             string
	TranscriptAPI             SessionTranscriptAPI
	TranscriptStreamConnected bool
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
	return buildBootstrapCommands(plan, input.SessionID, input.SessionKey, input.AfterRevision, input.InitialLines, input.LoadContext, input.TranscriptAPI, input.SessionAPI)
}

func (c defaultSessionBootstrapCoordinator) BuildSessionStartCommands(input SessionStartBootstrapInput) []tea.Cmd {
	plan := c.policyOrDefault().SessionStartPlan(input.Provider, input.Status)
	return buildBootstrapCommands(plan, input.SessionID, input.SessionKey, input.AfterRevision, input.InitialLines, input.LoadContext, input.TranscriptAPI, input.SessionAPI)
}

func (c defaultSessionBootstrapCoordinator) BuildReconnectCommands(input SessionReconnectBootstrapInput) []tea.Cmd {
	plan := c.policyOrDefault().SendReconnectPlan(input.Provider)
	cmds := make([]tea.Cmd, 0, 1)
	if plan.OpenTranscript && !input.TranscriptStreamConnected && input.TranscriptAPI != nil {
		cmds = append(cmds, openTranscriptStreamCmd(input.TranscriptAPI, input.SessionID, input.AfterRevision))
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
	afterRevision string,
	lines int,
	loadCtx context.Context,
	transcriptAPI SessionTranscriptAPI,
	sessionAPI SessionAPI,
) []tea.Cmd {
	cmds := make([]tea.Cmd, 0, 4)
	if plan.FetchTranscript && transcriptAPI != nil {
		cmds = append(cmds, fetchTranscriptSnapshotCmdWithContext(transcriptAPI, sessionID, key, lines, loadCtx))
	}
	if plan.FetchApprovals {
		cmds = append(cmds, fetchApprovalsCmdWithContext(sessionAPI, sessionID, loadCtx))
	}
	if plan.OpenTranscript && transcriptAPI != nil {
		cmds = append(cmds, openTranscriptStreamCmd(transcriptAPI, sessionID, afterRevision))
	}
	return cmds
}

func (m *Model) sessionBootstrapCoordinatorOrDefault() SessionBootstrapCoordinator {
	if m == nil || m.sessionBootstrapCoordinator == nil {
		return NewDefaultSessionBootstrapCoordinator(m.sessionBootstrapPolicyOrDefault())
	}
	return m.sessionBootstrapCoordinator
}
