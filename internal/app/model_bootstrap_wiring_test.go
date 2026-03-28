package app

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

type captureBootstrapWiringCoordinator struct {
	t     *testing.T
	model *Model
	api   *transcriptAttachmentOpenAPIStub

	selectionCalled      bool
	selectionBuilderUsed bool
	selectionCtxMatch    bool
	selectionMsg         transcriptStreamMsg

	startCalled      bool
	startBuilderUsed bool
	startCtxMatch    bool
	startMsg         transcriptStreamMsg
}

func (c *captureBootstrapWiringCoordinator) BuildSelectionLoadCommands(input SelectionLoadBootstrapInput) []tea.Cmd {
	c.selectionCalled = true
	if input.OpenTranscriptCmdBuilder == nil {
		c.t.Fatalf("expected selection bootstrap input to include transcript open builder")
	}
	cmd := input.OpenTranscriptCmdBuilder(input.SessionID, input.AfterRevision, input.OpenSource)
	if cmd == nil {
		c.t.Fatalf("expected selection transcript open builder to return a command")
	}
	if c.model != nil {
		c.model.cancelRequestScope(requestScopeSessionLoad)
	}
	msg, ok := cmd().(transcriptStreamMsg)
	if !ok {
		c.t.Fatalf("expected selection builder command to emit transcriptStreamMsg")
	}
	c.selectionBuilderUsed = true
	c.selectionMsg = msg
	c.selectionCtxMatch = c.api != nil && c.api.streamCtx == input.LoadContext
	return nil
}

func (c *captureBootstrapWiringCoordinator) BuildSessionStartCommands(input SessionStartBootstrapInput) []tea.Cmd {
	c.startCalled = true
	if input.OpenTranscriptCmdBuilder == nil {
		c.t.Fatalf("expected start bootstrap input to include transcript open builder")
	}
	cmd := input.OpenTranscriptCmdBuilder(input.SessionID, input.AfterRevision, input.OpenSource)
	if cmd == nil {
		c.t.Fatalf("expected start transcript open builder to return a command")
	}
	if c.model != nil {
		c.model.cancelRequestScope(requestScopeSessionLoad)
	}
	msg, ok := cmd().(transcriptStreamMsg)
	if !ok {
		c.t.Fatalf("expected start builder command to emit transcriptStreamMsg")
	}
	c.startBuilderUsed = true
	c.startMsg = msg
	c.startCtxMatch = c.api != nil && c.api.streamCtx == input.LoadContext
	return nil
}

func (*captureBootstrapWiringCoordinator) BuildReconnectCommands(SessionReconnectBootstrapInput) []tea.Cmd {
	return nil
}

func TestLoadSelectedSessionProvidesBuilderBackedTranscriptOpenCommand(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := &transcriptAttachmentOpenAPIStub{}
	m.sessionTranscriptAPI = api
	coordinator := &captureBootstrapWiringCoordinator{t: t, model: &m, api: api}
	m.sessionBootstrapCoordinator = coordinator

	item := m.selectedItem()
	if item == nil || item.session == nil {
		t.Fatalf("expected selected session item")
	}
	_ = m.loadSelectedSession(item)

	if !coordinator.selectionCalled || !coordinator.selectionBuilderUsed {
		t.Fatalf("expected loadSelectedSession to call selection bootstrap builder path")
	}
	if !coordinator.selectionCtxMatch {
		t.Fatalf("expected selection builder stream open to use session-load context")
	}
	if !errors.Is(coordinator.selectionMsg.err, context.Canceled) {
		t.Fatalf("expected selection builder command to observe canceled session-load context, got %v", coordinator.selectionMsg.err)
	}
	if coordinator.selectionMsg.generation == 0 {
		t.Fatalf("expected selection builder transcript open to be generation-aware")
	}
	if coordinator.selectionMsg.source != transcriptAttachmentSourceSelectionLoad {
		t.Fatalf("expected selection builder source %q, got %q", transcriptAttachmentSourceSelectionLoad, coordinator.selectionMsg.source)
	}
}

func TestStartSessionProvidesBuilderBackedTranscriptOpenCommand(t *testing.T) {
	m := NewModel(nil)
	api := &transcriptAttachmentOpenAPIStub{}
	m.sessionTranscriptAPI = api
	coordinator := &captureBootstrapWiringCoordinator{t: t, model: &m, api: api}
	m.sessionBootstrapCoordinator = coordinator

	handled, _ := m.reduceStateMessages(startSessionMsg{
		session: &types.Session{
			ID:       "s1",
			Provider: "codex",
			Status:   types.SessionStatusRunning,
			Title:    "Session",
		},
	})
	if !handled {
		t.Fatalf("expected start session message to be handled")
	}

	if !coordinator.startCalled || !coordinator.startBuilderUsed {
		t.Fatalf("expected startSession path to call start bootstrap builder path")
	}
	if !coordinator.startCtxMatch {
		t.Fatalf("expected start builder stream open to use session-load context")
	}
	if !errors.Is(coordinator.startMsg.err, context.Canceled) {
		t.Fatalf("expected start builder command to observe canceled session-load context, got %v", coordinator.startMsg.err)
	}
	if coordinator.startMsg.generation == 0 {
		t.Fatalf("expected start builder transcript open to be generation-aware")
	}
	if coordinator.startMsg.source != transcriptAttachmentSourceSessionStart {
		t.Fatalf("expected start builder source %q, got %q", transcriptAttachmentSourceSessionStart, coordinator.startMsg.source)
	}
}
