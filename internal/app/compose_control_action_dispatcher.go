package app

import tea "charm.land/bubbletea/v2"

type ComposeControlActionDispatcher interface {
	Execute(m *Model, span composeControlSpan) tea.Cmd
}

type composeControlActionExecutor interface {
	Execute(m *Model, span composeControlSpan) tea.Cmd
}

type defaultComposeControlActionDispatcher struct {
	executors map[composeControlAction]composeControlActionExecutor
}

type composeOptionControlActionExecutor struct{}

type composeInterruptControlActionExecutor struct{}

func newDefaultComposeControlActionDispatcher() ComposeControlActionDispatcher {
	optionExecutor := composeOptionControlActionExecutor{}
	return defaultComposeControlActionDispatcher{
		executors: map[composeControlAction]composeControlActionExecutor{
			composeControlActionNone:          optionExecutor,
			composeControlActionOpenOption:    optionExecutor,
			composeControlActionInterruptTurn: composeInterruptControlActionExecutor{},
		},
	}
}

func WithComposeControlActionDispatcher(dispatcher ComposeControlActionDispatcher) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if dispatcher == nil {
			m.composeControlActionDispatcher = newDefaultComposeControlActionDispatcher()
			return
		}
		m.composeControlActionDispatcher = dispatcher
	}
}

func (m *Model) composeControlActionDispatcherOrDefault() ComposeControlActionDispatcher {
	if m == nil || m.composeControlActionDispatcher == nil {
		return newDefaultComposeControlActionDispatcher()
	}
	return m.composeControlActionDispatcher
}

func (d defaultComposeControlActionDispatcher) Execute(m *Model, span composeControlSpan) tea.Cmd {
	executor, ok := d.executors[span.action]
	if !ok || executor == nil {
		return nil
	}
	return executor.Execute(m, span)
}

func (composeOptionControlActionExecutor) Execute(m *Model, span composeControlSpan) tea.Cmd {
	if m == nil || span.kind == composeOptionNone {
		return nil
	}
	if m.mode == uiModeGuidedWorkflow {
		return m.requestGuidedWorkflowComposeOptionPicker(span.kind)
	}
	return m.requestComposeOptionPicker(span.kind)
}

func (composeInterruptControlActionExecutor) Execute(m *Model, _ composeControlSpan) tea.Cmd {
	if m == nil {
		return nil
	}
	return m.requestComposeInterruptCmd()
}
