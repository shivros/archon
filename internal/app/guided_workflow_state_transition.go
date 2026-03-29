package app

import (
	"control/internal/guidedworkflows"
)

type GuidedWorkflowReflowInput struct {
	BeforeInputLines int
	AfterInputLines  int
	Width            int
	Height           int
}

type GuidedWorkflowReflowPolicy interface {
	ShouldReflow(input GuidedWorkflowReflowInput) bool
}

type defaultGuidedWorkflowReflowPolicy struct{}

func (defaultGuidedWorkflowReflowPolicy) ShouldReflow(input GuidedWorkflowReflowInput) bool {
	if input.Width <= 0 || input.Height <= 0 {
		return false
	}
	return input.BeforeInputLines != input.AfterInputLines
}

func WithGuidedWorkflowReflowPolicy(policy GuidedWorkflowReflowPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.guidedWorkflowReflowPolicy = defaultGuidedWorkflowReflowPolicy{}
			return
		}
		m.guidedWorkflowReflowPolicy = policy
	}
}

func (m *Model) guidedWorkflowReflowPolicyOrDefault() GuidedWorkflowReflowPolicy {
	if m == nil || m.guidedWorkflowReflowPolicy == nil {
		return defaultGuidedWorkflowReflowPolicy{}
	}
	return m.guidedWorkflowReflowPolicy
}

type GuidedWorkflowInteractiveStateTransitionGateway interface {
	ApplyRun(run *guidedworkflows.WorkflowRun)
	ApplySnapshot(run *guidedworkflows.WorkflowRun, timeline []guidedworkflows.RunTimelineEvent)
}

type GuidedWorkflowPreviewStateTransitionGateway interface {
	ApplyPreview(run *guidedworkflows.WorkflowRun)
	ApplyPreviewSnapshot(run *guidedworkflows.WorkflowRun, timeline []guidedworkflows.RunTimelineEvent)
}

type GuidedWorkflowStateTransitionGateway interface {
	GuidedWorkflowInteractiveStateTransitionGateway
	GuidedWorkflowPreviewStateTransitionGateway
}

type defaultGuidedWorkflowStateTransitionGateway struct {
	model *Model
}

func NewDefaultGuidedWorkflowStateTransitionGateway(model *Model) GuidedWorkflowStateTransitionGateway {
	return defaultGuidedWorkflowStateTransitionGateway{model: model}
}

func (g defaultGuidedWorkflowStateTransitionGateway) ApplyRun(run *guidedworkflows.WorkflowRun) {
	g.apply(func(controller *GuidedWorkflowUIController) {
		controller.SetRun(run)
	})
}

func (g defaultGuidedWorkflowStateTransitionGateway) ApplySnapshot(run *guidedworkflows.WorkflowRun, timeline []guidedworkflows.RunTimelineEvent) {
	g.apply(func(controller *GuidedWorkflowUIController) {
		controller.SetSnapshot(run, timeline)
	})
}

func (g defaultGuidedWorkflowStateTransitionGateway) ApplyPreview(run *guidedworkflows.WorkflowRun) {
	if g.model == nil || g.model.guidedWorkflow == nil {
		return
	}
	g.model.guidedWorkflow.SetRun(run)
}

func (g defaultGuidedWorkflowStateTransitionGateway) ApplyPreviewSnapshot(run *guidedworkflows.WorkflowRun, timeline []guidedworkflows.RunTimelineEvent) {
	m := g.model
	if m == nil || m.guidedWorkflow == nil {
		return
	}
	m.guidedWorkflow.SetSnapshot(run, timeline)
	m.renderWorkflowPreviewContent()
}

func (g defaultGuidedWorkflowStateTransitionGateway) apply(update func(controller *GuidedWorkflowUIController)) {
	m := g.model
	if m == nil || m.guidedWorkflow == nil {
		return
	}
	beforeInputLines := m.modeInputLineCount()
	if update != nil {
		update(m.guidedWorkflow)
	}
	m.renderGuidedWorkflowContent()
	afterInputLines := m.modeInputLineCount()
	if !m.guidedWorkflowReflowPolicyOrDefault().ShouldReflow(GuidedWorkflowReflowInput{
		BeforeInputLines: beforeInputLines,
		AfterInputLines:  afterInputLines,
		Width:            m.width,
		Height:           m.height,
	}) {
		return
	}
	m.resize(m.width, m.height)
}

func WithGuidedWorkflowStateTransitionGateway(gateway GuidedWorkflowStateTransitionGateway) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if gateway == nil {
			defaultGateway := NewDefaultGuidedWorkflowStateTransitionGateway(m)
			m.guidedWorkflowInteractiveStateTransitionGateway = defaultGateway
			m.guidedWorkflowPreviewStateTransitionGateway = defaultGateway
			return
		}
		m.guidedWorkflowInteractiveStateTransitionGateway = gateway
		m.guidedWorkflowPreviewStateTransitionGateway = gateway
	}
}

func WithGuidedWorkflowInteractiveStateTransitionGateway(gateway GuidedWorkflowInteractiveStateTransitionGateway) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if gateway == nil {
			m.guidedWorkflowInteractiveStateTransitionGateway = NewDefaultGuidedWorkflowStateTransitionGateway(m)
			return
		}
		m.guidedWorkflowInteractiveStateTransitionGateway = gateway
	}
}

func WithGuidedWorkflowPreviewStateTransitionGateway(gateway GuidedWorkflowPreviewStateTransitionGateway) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if gateway == nil {
			m.guidedWorkflowPreviewStateTransitionGateway = NewDefaultGuidedWorkflowStateTransitionGateway(m)
			return
		}
		m.guidedWorkflowPreviewStateTransitionGateway = gateway
	}
}

func (m *Model) guidedWorkflowInteractiveStateTransitionGatewayOrDefault() GuidedWorkflowInteractiveStateTransitionGateway {
	if m == nil || m.guidedWorkflowInteractiveStateTransitionGateway == nil {
		return NewDefaultGuidedWorkflowStateTransitionGateway(m)
	}
	return m.guidedWorkflowInteractiveStateTransitionGateway
}

func (m *Model) guidedWorkflowPreviewStateTransitionGatewayOrDefault() GuidedWorkflowPreviewStateTransitionGateway {
	if m == nil || m.guidedWorkflowPreviewStateTransitionGateway == nil {
		return NewDefaultGuidedWorkflowStateTransitionGateway(m)
	}
	return m.guidedWorkflowPreviewStateTransitionGateway
}
