package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type stubComposeControlActionDispatcher struct {
	cmd    tea.Cmd
	span   composeControlSpan
	called bool
}

func (s *stubComposeControlActionDispatcher) Execute(_ *Model, span composeControlSpan) tea.Cmd {
	s.called = true
	s.span = span
	return s.cmd
}

func TestWithComposeControlActionDispatcherConfiguresAndResetsDefault(t *testing.T) {
	custom := &stubComposeControlActionDispatcher{}
	m := NewModel(nil, WithComposeControlActionDispatcher(custom))
	if m.composeControlActionDispatcherOrDefault() != custom {
		t.Fatalf("expected custom compose control action dispatcher")
	}
	WithComposeControlActionDispatcher(nil)(&m)
	if _, ok := m.composeControlActionDispatcherOrDefault().(defaultComposeControlActionDispatcher); !ok {
		t.Fatalf("expected default compose control action dispatcher after reset, got %T", m.composeControlActionDispatcherOrDefault())
	}
}

func TestReduceComposeControlsLeftPressMouseUsesActionDispatcher(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	m.resize(120, 40)
	if line := m.composeControlsLine(); line == "" {
		t.Fatalf("expected compose controls line")
	}
	spans := m.composeControlSpans()
	if len(spans) == 0 {
		t.Fatalf("expected compose control spans")
	}
	custom := &stubComposeControlActionDispatcher{
		cmd: func() tea.Msg { return nil },
	}
	WithComposeControlActionDispatcher(custom)(&m)

	layout := m.resolveMouseLayout()
	x := layout.rightStart + spans[0].start
	y := m.composeControlsRow()
	handled := m.reduceComposeControlsLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected compose control click to be handled")
	}
	if !custom.called {
		t.Fatalf("expected custom dispatcher to be called")
	}
	if custom.span != spans[0] {
		t.Fatalf("expected clicked span %#v, got %#v", spans[0], custom.span)
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected dispatcher command to be queued")
	}
}

func TestDefaultComposeControlActionDispatcherIgnoresUnknownAction(t *testing.T) {
	dispatcher := newDefaultComposeControlActionDispatcher()
	m := NewModel(nil)
	cmd := dispatcher.Execute(&m, composeControlSpan{
		action: composeControlAction(999),
		kind:   composeOptionModel,
		start:  0,
		end:    3,
	})
	if cmd != nil {
		t.Fatalf("expected unknown action to return nil command")
	}
}

func TestComposeControlActionDispatcherOrDefaultHandlesNilModel(t *testing.T) {
	var m *Model
	if _, ok := m.composeControlActionDispatcherOrDefault().(defaultComposeControlActionDispatcher); !ok {
		t.Fatalf("expected default compose control action dispatcher for nil model")
	}
}

func TestComposeOptionControlActionExecutorReturnsNilForNilModelOrNoneKind(t *testing.T) {
	executor := composeOptionControlActionExecutor{}
	if cmd := executor.Execute(nil, composeControlSpan{kind: composeOptionModel}); cmd != nil {
		t.Fatalf("expected nil model to return nil command")
	}
	m := NewModel(nil)
	if cmd := executor.Execute(&m, composeControlSpan{kind: composeOptionNone}); cmd != nil {
		t.Fatalf("expected composeOptionNone to return nil command")
	}
}

func TestComposeOptionControlActionExecutorComposePath(t *testing.T) {
	executor := composeOptionControlActionExecutor{}
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "opencode"}

	cmd := executor.Execute(&m, composeControlSpan{kind: composeOptionModel})
	if cmd == nil {
		t.Fatalf("expected compose option executor to route to compose picker command")
	}
}

func TestComposeOptionControlActionExecutorGuidedWorkflowPath(t *testing.T) {
	executor := composeOptionControlActionExecutor{}
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	advanceGuidedWorkflowToComposerForTest(t, &m)
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	m.guidedWorkflow.SetProvider("opencode")
	m.newSession = &newSessionTarget{provider: "opencode"}

	cmd := executor.Execute(&m, composeControlSpan{kind: composeOptionModel})
	if cmd == nil {
		t.Fatalf("expected guided workflow option executor to route to setup picker command")
	}
}

func TestComposeInterruptControlActionExecutor(t *testing.T) {
	executor := composeInterruptControlActionExecutor{}
	if cmd := executor.Execute(nil, composeControlSpan{}); cmd != nil {
		t.Fatalf("expected nil model to return nil command")
	}
	m := newComposeInterruptTestModel("codex")
	m.startRequestActivity("s1", "codex")
	cmd := executor.Execute(m, composeControlSpan{})
	if cmd == nil {
		t.Fatalf("expected interrupt executor to return command")
	}
}
