package app

import (
	"net/url"
	"testing"

	"control/internal/guidedworkflows"
)

type stubWorkflowUserTurnLinkBuilder struct {
	calls     int
	sessionID string
	turnID    string
	link      string
}

func (s *stubWorkflowUserTurnLinkBuilder) BuildUserTurnLink(sessionID, turnID string) string {
	s.calls++
	s.sessionID = sessionID
	s.turnID = turnID
	return s.link
}

func TestStepSessionAndTurnResolvesExecutionThenStepFallback(t *testing.T) {
	stepWithExecutionTurn := guidedworkflows.StepRun{
		TurnID: "step-turn",
		Execution: &guidedworkflows.StepExecutionRef{
			SessionID: "s1",
			TurnID:    "execution-turn",
		},
	}
	sessionID, turnID := stepSessionAndTurn(stepWithExecutionTurn)
	if sessionID != "s1" || turnID != "execution-turn" {
		t.Fatalf("expected execution turn resolution, got session=%q turn=%q", sessionID, turnID)
	}

	stepWithFallbackTurn := guidedworkflows.StepRun{
		TurnID: "step-turn",
		Execution: &guidedworkflows.StepExecutionRef{
			SessionID: "s2",
		},
	}
	sessionID, turnID = stepSessionAndTurn(stepWithFallbackTurn)
	if sessionID != "s2" || turnID != "step-turn" {
		t.Fatalf("expected step turn fallback, got session=%q turn=%q", sessionID, turnID)
	}
}

func TestArchonWorkflowUserTurnLinkBuilderBuildsAndValidatesLinks(t *testing.T) {
	builder := NewArchonWorkflowUserTurnLinkBuilder()
	if got := builder.BuildUserTurnLink("s1", "turn-1"); got != "[user turn turn-1](archon://session/s1?turn=turn-1)" {
		t.Fatalf("unexpected link: %q", got)
	}
	if got := builder.BuildUserTurnLink("session/alpha", "turn 1?x"); got != "[user turn turn 1?x](archon://session/session%2Falpha?turn="+url.QueryEscape("turn 1?x")+")" {
		t.Fatalf("expected escaped link, got %q", got)
	}
	if got := builder.BuildUserTurnLink(" ", "turn-1"); got != unavailableUserTurnLink {
		t.Fatalf("expected unavailable for missing session, got %q", got)
	}
	if got := builder.BuildUserTurnLink("s1", " "); got != unavailableUserTurnLink {
		t.Fatalf("expected unavailable for missing turn, got %q", got)
	}
}

func TestGuidedWorkflowControllerUsesConfiguredUserTurnLinkBuilder(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	stub := &stubWorkflowUserTurnLinkBuilder{link: "[custom](archon://custom)"}
	controller.SetUserTurnLinkBuilder(stub)

	step := guidedworkflows.StepRun{
		Execution: &guidedworkflows.StepExecutionRef{
			SessionID: "s1",
			TurnID:    "turn-42",
		},
	}
	if got := controller.stepUserTurnLink(step); got != "[custom](archon://custom)" {
		t.Fatalf("expected custom link from builder, got %q", got)
	}
	if stub.calls != 1 || stub.sessionID != "s1" || stub.turnID != "turn-42" {
		t.Fatalf("unexpected builder call: %#v", stub)
	}

	controller.SetUserTurnLinkBuilder(nil)
	if got := controller.stepUserTurnLink(step); got != "[user turn turn-42](archon://session/s1?turn=turn-42)" {
		t.Fatalf("expected default archon link after reset, got %q", got)
	}
}

func TestGuidedWorkflowControllerUserTurnLinkNilReceiverGuards(t *testing.T) {
	var nilController *GuidedWorkflowUIController
	nilController.SetUserTurnLinkBuilder(&stubWorkflowUserTurnLinkBuilder{link: "[ignored](archon://ignored)"})

	got := nilController.stepUserTurnLink(guidedworkflows.StepRun{
		Execution: &guidedworkflows.StepExecutionRef{
			SessionID: "s1",
			TurnID:    "turn-1",
		},
	})
	if got != unavailableUserTurnLink {
		t.Fatalf("expected unavailable link for nil controller, got %q", got)
	}
}

func TestWorkflowUserTurnLinkLabelParsesMarkdownAndFallbacks(t *testing.T) {
	if got := workflowUserTurnLinkLabel("[user turn turn-1](archon://session/s1?turn=turn-1)"); got != "user turn turn-1" {
		t.Fatalf("expected markdown label extraction, got %q", got)
	}
	if got := workflowUserTurnLinkLabel("[custom label](archon://custom)"); got != "custom label" {
		t.Fatalf("expected custom markdown label extraction, got %q", got)
	}
	if got := workflowUserTurnLinkLabel("plain label"); got != "plain label" {
		t.Fatalf("expected plain link passthrough, got %q", got)
	}
	if got := workflowUserTurnLinkLabel(unavailableUserTurnLink); got != "" {
		t.Fatalf("expected unavailable link to return empty label, got %q", got)
	}
}
