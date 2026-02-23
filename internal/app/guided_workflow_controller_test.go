package app

import (
	"errors"
	"strings"
	"testing"

	"control/internal/guidedworkflows"
)

func TestGuidedWorkflowControllerLauncherQueryEditing(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{workspaceID: "ws1"})
	controller.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "bug_triage", Name: "Bug Triage"},
		{ID: "solid_phase_delivery", Name: "SOLID Phase Delivery"},
	})

	if got := controller.Query(); got != "" {
		t.Fatalf("expected empty initial query, got %q", got)
	}
	if !controller.AppendQuery("bug") {
		t.Fatalf("expected append query to succeed in launcher stage")
	}
	if got := controller.Query(); got != "bug" {
		t.Fatalf("expected query bug, got %q", got)
	}
	if !controller.BackspaceQuery() {
		t.Fatalf("expected backspace query to succeed in launcher stage")
	}
	if got := controller.Query(); got != "bu" {
		t.Fatalf("expected query bu after backspace, got %q", got)
	}
	if !controller.ClearQuery() {
		t.Fatalf("expected clear query to succeed")
	}
	if controller.ClearQuery() {
		t.Fatalf("expected second clear query to be a no-op")
	}

	if !controller.OpenSetup() {
		t.Fatalf("expected setup to open with selected template")
	}
	if got := controller.Query(); got != "" {
		t.Fatalf("expected query accessor to be stage-guarded, got %q", got)
	}
	if controller.AppendQuery("x") {
		t.Fatalf("expected append query to be blocked outside launcher")
	}
	if controller.BackspaceQuery() {
		t.Fatalf("expected backspace query to be blocked outside launcher")
	}
}

func TestGuidedWorkflowControllerCycleSensitivityAndErrorSetters(t *testing.T) {
	var nilController *GuidedWorkflowUIController
	nilController.CycleSensitivity(1)
	nilController.SetCreateError(errors.New("ignored"))
	nilController.SetStartError(errors.New("ignored"))
	nilController.SetDecisionError(errors.New("ignored"))
	nilController.SetSnapshotError(errors.New("ignored"))

	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{workspaceID: "ws1"})
	controller.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "solid_phase_delivery", Name: "SOLID Phase Delivery"},
	})
	if !controller.OpenSetup() {
		t.Fatalf("expected setup stage")
	}

	if controller.sensitivity != guidedPolicySensitivityBalanced {
		t.Fatalf("expected balanced sensitivity at setup start, got %v", controller.sensitivity)
	}
	controller.CycleSensitivity(1)
	if controller.sensitivity != guidedPolicySensitivityHigh {
		t.Fatalf("expected high sensitivity after +1, got %v", controller.sensitivity)
	}
	controller.CycleSensitivity(1)
	if controller.sensitivity != guidedPolicySensitivityLow {
		t.Fatalf("expected low sensitivity after wrap, got %v", controller.sensitivity)
	}
	controller.CycleSensitivity(-1)
	if controller.sensitivity != guidedPolicySensitivityHigh {
		t.Fatalf("expected high sensitivity after -1 from low, got %v", controller.sensitivity)
	}
	controller.CycleSensitivity(0)
	if controller.sensitivity != guidedPolicySensitivityHigh {
		t.Fatalf("expected zero delta to leave sensitivity unchanged")
	}
	controller.OpenLauncher()
	controller.CycleSensitivity(1)
	if controller.sensitivity != guidedPolicySensitivityHigh {
		t.Fatalf("expected non-setup cycle to be ignored")
	}

	controller.SetCreateError(errors.New("create failed"))
	if controller.lastError != "create failed" {
		t.Fatalf("expected create error text, got %q", controller.lastError)
	}
	controller.SetStartError(errors.New("start failed"))
	if controller.lastError != "start failed" {
		t.Fatalf("expected start error text, got %q", controller.lastError)
	}
	controller.SetDecisionError(errors.New("decision failed"))
	if controller.lastError != "decision failed" {
		t.Fatalf("expected decision error text, got %q", controller.lastError)
	}
	controller.refreshQueued = true
	controller.SetSnapshotError(errors.New("snapshot failed"))
	if controller.lastError != "snapshot failed" {
		t.Fatalf("expected snapshot error text, got %q", controller.lastError)
	}
	if controller.refreshQueued {
		t.Fatalf("expected snapshot error to clear refresh queued state")
	}
}

func TestGuidedWorkflowControllerLauncherTemplatePickerLayoutGuards(t *testing.T) {
	var nilController *GuidedWorkflowUIController
	if layout, ok := nilController.LauncherTemplatePickerLayout(); ok || layout.height != 0 {
		t.Fatalf("expected nil controller to return no picker layout")
	}

	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{workspaceID: "ws1"})
	if layout, ok := controller.LauncherTemplatePickerLayout(); ok || layout.height != 0 {
		t.Fatalf("expected no picker layout while loading")
	}

	controller.SetTemplateLoadError(errors.New("failed"))
	if layout, ok := controller.LauncherTemplatePickerLayout(); ok || layout.height != 0 {
		t.Fatalf("expected no picker layout while load error is present")
	}

	controller.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "bug_triage", Name: "Bug Triage"},
		{ID: "solid_phase_delivery", Name: "SOLID Phase Delivery"},
	})
	layout, ok := controller.LauncherTemplatePickerLayout()
	if !ok {
		t.Fatalf("expected picker layout once templates are available")
	}
	if layout.height < 2 {
		t.Fatalf("expected picker layout to include query + options, got %d", layout.height)
	}
	if layout.queryLine != "/" {
		t.Fatalf("expected picker query line anchor '/', got %q", layout.queryLine)
	}

	if !controller.SelectTemplateByRow(2) {
		t.Fatalf("expected select by row to work in launcher stage")
	}
	if controller.templateID != "solid_phase_delivery" {
		t.Fatalf("expected second template to be selected, got %q", controller.templateID)
	}
	if controller.SelectTemplateByRow(-1) {
		t.Fatalf("expected invalid row to be ignored")
	}

	if !controller.OpenSetup() {
		t.Fatalf("expected setup stage")
	}
	if _, ok := controller.LauncherTemplatePickerLayout(); ok {
		t.Fatalf("expected no launcher picker layout outside launcher stage")
	}
	if controller.SelectTemplateByRow(1) {
		t.Fatalf("expected select by row to be blocked outside launcher stage")
	}
}

func TestGuidedWorkflowControllerLauncherRequiresRawANSIRender(t *testing.T) {
	var nilController *GuidedWorkflowUIController
	if nilController.LauncherRequiresRawANSIRender() {
		t.Fatalf("expected nil controller to disable ANSI passthrough")
	}

	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{workspaceID: "ws1"})
	if controller.LauncherRequiresRawANSIRender() {
		t.Fatalf("expected loading launcher to disable ANSI passthrough")
	}

	controller.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "bug_triage", Name: "Bug Triage"},
		{ID: "solid_phase_delivery", Name: "SOLID Phase Delivery"},
	})
	if !controller.LauncherRequiresRawANSIRender() {
		t.Fatalf("expected loaded launcher picker to require ANSI passthrough")
	}

	controller.OpenSetup()
	if controller.LauncherRequiresRawANSIRender() {
		t.Fatalf("expected non-launcher stage to disable ANSI passthrough")
	}
}

func TestGuidedWorkflowControllerStepTraceChipVariants(t *testing.T) {
	controller := NewGuidedWorkflowUIController()

	if got := controller.stepTraceChip(guidedworkflows.StepRun{
		ExecutionState: guidedworkflows.StepExecutionStateLinked,
	}); got != "[session:linked]" {
		t.Fatalf("expected linked chip without session, got %q", got)
	}

	if got := controller.stepTraceChip(guidedworkflows.StepRun{
		ExecutionState: guidedworkflows.StepExecutionStateLinked,
		Execution:      &guidedworkflows.StepExecutionRef{SessionID: "s1"},
	}); got != "[session:s1]" {
		t.Fatalf("expected session-only linked chip, got %q", got)
	}

	if got := controller.stepTraceChip(guidedworkflows.StepRun{
		ExecutionState: guidedworkflows.StepExecutionStateLinked,
		Execution:      &guidedworkflows.StepExecutionRef{SessionID: "s1"},
		TurnID:         "turn-step",
	}); got != "[session:s1 turn:turn-step]" {
		t.Fatalf("expected linked chip to fall back to step turn, got %q", got)
	}

	if got := controller.stepTraceChip(guidedworkflows.StepRun{
		ExecutionState: guidedworkflows.StepExecutionStateUnavailable,
	}); got != "[session:unavailable]" {
		t.Fatalf("expected unavailable chip, got %q", got)
	}

	if got := controller.stepTraceChip(guidedworkflows.StepRun{
		ExecutionState: guidedworkflows.StepExecutionStateNone,
	}); got != "[session:none]" {
		t.Fatalf("expected none chip, got %q", got)
	}
}

func TestGuidedWorkflowControllerTurnLinkTargets(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.SetRun(&guidedworkflows.WorkflowRun{
		ID:     "gwf-1",
		Status: guidedworkflows.WorkflowRunStatusRunning,
		Phases: []guidedworkflows.PhaseRun{
			{
				ID:   "phase-1",
				Name: "Phase 1",
				Steps: []guidedworkflows.StepRun{
					{
						ID:   "step-1",
						Name: "Step 1",
						Execution: &guidedworkflows.StepExecutionRef{
							SessionID: "s1",
							TurnID:    "turn-1",
						},
					},
					{
						ID:   "step-2",
						Name: "Step 2",
						Execution: &guidedworkflows.StepExecutionRef{
							SessionID: "s1",
							TurnID:    "turn-1",
						},
					},
					{
						ID:   "step-3",
						Name: "Step 3",
						Execution: &guidedworkflows.StepExecutionRef{
							SessionID: "s2",
							TurnID:    "turn-2",
						},
					},
				},
			},
		},
	})

	targets := controller.TurnLinkTargets()
	if len(targets) != 2 {
		t.Fatalf("expected deduplicated turn link targets, got %#v", targets)
	}
	if targets[0].label != "user turn turn-1" || targets[0].sessionID != "s1" || targets[0].turnID != "turn-1" {
		t.Fatalf("unexpected first target: %#v", targets[0])
	}
	if targets[1].label != "user turn turn-2" || targets[1].sessionID != "s2" || targets[1].turnID != "turn-2" {
		t.Fatalf("unexpected second target: %#v", targets[1])
	}
}

func TestGuidedWorkflowStatusTextHelpersIncludeStoppedStates(t *testing.T) {
	if got := runStatusText(guidedworkflows.WorkflowRunStatusStopped); got != "stopped" {
		t.Fatalf("expected stopped run status text, got %q", got)
	}
	if got := runStatusText(guidedworkflows.WorkflowRunStatusRunning); got != "running" {
		t.Fatalf("expected running run status text, got %q", got)
	}
	if got := runStatusText(guidedworkflows.WorkflowRunStatusPaused); got != "paused (decision needed)" {
		t.Fatalf("expected paused run status text, got %q", got)
	}
	if got := runStatusText(guidedworkflows.WorkflowRunStatus(" custom ")); got != "custom" {
		t.Fatalf("expected trimmed fallback run status, got %q", got)
	}
	if got := stepStatusPrefix(guidedworkflows.StepRunStatusStopped); got != "[s]" {
		t.Fatalf("expected stopped step prefix [s], got %q", got)
	}
	if got := phaseStatusPrefix(guidedworkflows.PhaseRunStatusStopped); got != "[s]" {
		t.Fatalf("expected stopped phase prefix [s], got %q", got)
	}
	if got := phaseStatusPrefix(guidedworkflows.PhaseRunStatusRunning); got != "[>]" {
		t.Fatalf("expected running phase prefix [>], got %q", got)
	}
	if got := phaseStatusPrefix(guidedworkflows.PhaseRunStatusCompleted); got != "[x]" {
		t.Fatalf("expected completed phase prefix [x], got %q", got)
	}
	if got := phaseStatusPrefix(guidedworkflows.PhaseRunStatusFailed); got != "[!]" {
		t.Fatalf("expected failed phase prefix [!], got %q", got)
	}
	if got := phaseStatusPrefix(guidedworkflows.PhaseRunStatusPending); got != "[ ]" {
		t.Fatalf("expected pending phase prefix [ ], got %q", got)
	}
}

func TestGuidedWorkflowControllerRenderUsesDisplayPromptFallback(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.SetRun(&guidedworkflows.WorkflowRun{
		ID:                "gwf-1",
		Status:            guidedworkflows.WorkflowRunStatusRunning,
		TemplateName:      "SOLID",
		DisplayUserPrompt: "legacy intent from session metadata",
		Phases:            []guidedworkflows.PhaseRun{},
	})

	live := controller.renderLive()
	if !strings.Contains(live, "Original prompt: legacy intent from session metadata") {
		t.Fatalf("expected display prompt in live view, got %q", live)
	}
}

func TestGuidedWorkflowControllerRenderFallsBackToUserPromptForLegacyPayloads(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.SetRun(&guidedworkflows.WorkflowRun{
		ID:           "gwf-1",
		Status:       guidedworkflows.WorkflowRunStatusCompleted,
		TemplateName: "SOLID",
		UserPrompt:   "prompt from legacy daemon payload",
		Phases:       []guidedworkflows.PhaseRun{},
	})

	summary := controller.renderSummary()
	if !strings.Contains(summary, "### Original Prompt") || !strings.Contains(summary, "> prompt from legacy daemon payload") {
		t.Fatalf("expected user prompt fallback in summary view, got %q", summary)
	}
}

func TestGuidedWorkflowControllerRenderWorkflowPromptNilController(t *testing.T) {
	var controller *GuidedWorkflowUIController
	if got := controller.renderWorkflowPrompt(&guidedworkflows.WorkflowRun{UserPrompt: "x"}); got != workflowPromptUnavailable {
		t.Fatalf("expected unavailable fallback for nil controller, got %q", got)
	}
}

func TestGuidedWorkflowControllerRenderWorkflowPromptNilPresenterFallback(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.promptPresenter = nil
	run := &guidedworkflows.WorkflowRun{UserPrompt: "recover prompt from default presenter"}
	if got := controller.renderWorkflowPrompt(run); got != "recover prompt from default presenter" {
		t.Fatalf("expected fallback presenter output, got %q", got)
	}
}
