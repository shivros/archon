package app

type InputFrameTarget string

const (
	InputFrameTargetCompose             InputFrameTarget = "compose"
	InputFrameTargetAddNote             InputFrameTarget = "add_note"
	InputFrameTargetGuidedWorkflowSetup InputFrameTarget = "guided_workflow_setup"
	InputFrameTargetApprovalResponse    InputFrameTarget = "approval_response"
	InputFrameTargetRecentsReply        InputFrameTarget = "recents_reply"
	InputFrameTargetSearch              InputFrameTarget = "search"
)

type InputFramePolicy interface {
	FrameForTarget(target InputFrameTarget) InputPanelFrame
}

type defaultInputFramePolicy struct{}

func NewDefaultInputFramePolicy() InputFramePolicy {
	return defaultInputFramePolicy{}
}

func (defaultInputFramePolicy) FrameForTarget(target InputFrameTarget) InputPanelFrame {
	switch target {
	case InputFrameTargetCompose, InputFrameTargetAddNote, InputFrameTargetGuidedWorkflowSetup:
		return LipglossInputPanelFrame{Style: guidedWorkflowPromptFrameStyle}
	default:
		return nil
	}
}

func WithInputFramePolicy(policy InputFramePolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.inputFramePolicy = NewDefaultInputFramePolicy()
			return
		}
		m.inputFramePolicy = policy
	}
}

func (m *Model) inputFramePolicyOrDefault() InputFramePolicy {
	if m == nil || m.inputFramePolicy == nil {
		return NewDefaultInputFramePolicy()
	}
	return m.inputFramePolicy
}

func (m *Model) inputFrame(target InputFrameTarget) InputPanelFrame {
	return m.inputFramePolicyOrDefault().FrameForTarget(target)
}
