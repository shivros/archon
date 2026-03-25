package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"control/internal/guidedworkflows"

	xansi "github.com/charmbracelet/x/ansi"
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

	if !controller.OpenProvider() {
		t.Fatalf("expected provider stage to open with selected template")
	}
	if !controller.OpenPolicy() {
		t.Fatalf("expected policy stage to open with selected provider")
	}
	if !controller.OpenSetup() {
		t.Fatalf("expected setup to open from policy stage")
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
	if !controller.OpenProvider() || !controller.OpenPolicy() {
		t.Fatalf("expected provider/policy stages")
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
	controller.OpenSetup()
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

	if !controller.OpenProvider() || !controller.OpenPolicy() || !controller.OpenSetup() {
		t.Fatalf("expected provider/policy/setup stages")
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

	controller.OpenProvider()
	if !controller.LauncherRequiresRawANSIRender() {
		t.Fatalf("expected picker stages to preserve ANSI passthrough")
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

func TestGuidedWorkflowStatusTextHelpersIncludeExplicitRunStates(t *testing.T) {
	if got := workflowRunDetailedStatusText(nil); got != "" {
		t.Fatalf("expected empty detailed status text for nil run, got %q", got)
	}
	cases := []struct {
		status guidedworkflows.WorkflowRunStatus
		want   string
	}{
		{status: guidedworkflows.WorkflowRunStatusCreated, want: "created"},
		{status: guidedworkflows.WorkflowRunStatusQueued, want: "queued (waiting for dependencies)"},
		{status: guidedworkflows.WorkflowRunStatusRunning, want: "running"},
		{status: guidedworkflows.WorkflowRunStatusPaused, want: "paused (decision needed)"},
		{status: guidedworkflows.WorkflowRunStatusStopped, want: "stopped"},
		{status: guidedworkflows.WorkflowRunStatusCompleted, want: "completed"},
		{status: guidedworkflows.WorkflowRunStatusFailed, want: "failed"},
	}
	for _, tc := range cases {
		run := &guidedworkflows.WorkflowRun{Status: tc.status}
		if got := workflowRunDetailedStatusText(run); got != tc.want {
			t.Fatalf("status %q: expected %q, got %q", tc.status, tc.want, got)
		}
	}
	if got := workflowRunDetailedStatusText(&guidedworkflows.WorkflowRun{
		Status: guidedworkflows.WorkflowRunStatus(" custom "),
	}); got != "custom" {
		t.Fatalf("expected trimmed fallback run status, got %q", got)
	}
	dismissedRun := &guidedworkflows.WorkflowRun{
		Status:      guidedworkflows.WorkflowRunStatusCompleted,
		DismissedAt: ptrTime(time.Now().UTC()),
	}
	if got := workflowRunDetailedStatusText(dismissedRun); got != "dismissed" {
		t.Fatalf("expected dismissed run label to take precedence, got %q", got)
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

func TestGuidedWorkflowControllerRenderLiveUsesDetailedWorkflowStatusLabels(t *testing.T) {
	cases := []struct {
		name   string
		status guidedworkflows.WorkflowRunStatus
		want   string
	}{
		{name: "queued", status: guidedworkflows.WorkflowRunStatusQueued, want: "Status: queued (waiting for dependencies)"},
		{name: "paused", status: guidedworkflows.WorkflowRunStatusPaused, want: "Status: paused (decision needed)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			controller := NewGuidedWorkflowUIController()
			controller.SetRun(&guidedworkflows.WorkflowRun{
				ID:           "gwf-1",
				Status:       tc.status,
				TemplateName: "SOLID",
				Phases:       []guidedworkflows.PhaseRun{},
			})

			live := controller.renderLive()
			if !strings.Contains(live, tc.want) {
				t.Fatalf("expected live view to include %q, got %q", tc.want, live)
			}
		})
	}
}

func TestGuidedWorkflowControllerRenderSummaryUsesDetailedWorkflowStatusLabels(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.SetRun(&guidedworkflows.WorkflowRun{
		ID:           "gwf-1",
		Status:       guidedworkflows.WorkflowRunStatusCompleted,
		TemplateName: "SOLID",
		Phases:       []guidedworkflows.PhaseRun{},
	})

	summary := controller.renderSummary()
	if !strings.Contains(summary, "Final status: completed") {
		t.Fatalf("expected summary to include completed status label, got %q", summary)
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

func TestGuidedWorkflowControllerRenderLiveWrapsPhaseProgressInMonospaceBlock(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.SetRun(&guidedworkflows.WorkflowRun{
		ID:           "gwf-1",
		Status:       guidedworkflows.WorkflowRunStatusRunning,
		TemplateName: "SOLID",
		Phases: []guidedworkflows.PhaseRun{
			{
				ID:     "phase-1",
				Name:   "Discovery",
				Status: guidedworkflows.PhaseRunStatusRunning,
				Steps: []guidedworkflows.StepRun{
					{ID: "step-1", Name: "Map current state", Status: guidedworkflows.StepRunStatusCompleted},
					{ID: "step-2", Name: "Refine live renderer", Status: guidedworkflows.StepRunStatusRunning},
				},
			},
			{
				ID:     "phase-2",
				Name:   "Validation",
				Status: guidedworkflows.PhaseRunStatusPending,
				Steps:  []guidedworkflows.StepRun{{ID: "step-3", Name: "Run focused tests", Status: guidedworkflows.StepRunStatusPending}},
			},
		},
	})

	live := controller.renderLive()
	if !strings.Contains(live, "### Phase Progress  \n\n```text") {
		t.Fatalf("expected phase progress to render in a monospace block, got %q", live)
	}
	if !strings.Contains(live, "[>] Phase 1  Discovery") {
		t.Fatalf("expected first phase heading in live output, got %q", live)
	}
	if !strings.Contains(live, "  > [x] Step 1.1  Map current state [session:none]") {
		t.Fatalf("expected selected step line in live output, got %q", live)
	}
}

func TestGuidedWorkflowControllerRenderLivePreservesPhaseProgressLineBoundaries(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.SetRun(&guidedworkflows.WorkflowRun{
		ID:           "gwf-1",
		Status:       guidedworkflows.WorkflowRunStatusRunning,
		TemplateName: "SOLID",
		Phases: []guidedworkflows.PhaseRun{
			{
				ID:     "phase-1",
				Name:   "Discovery",
				Status: guidedworkflows.PhaseRunStatusRunning,
				Steps: []guidedworkflows.StepRun{
					{ID: "step-1", Name: "Map current state", Status: guidedworkflows.StepRunStatusCompleted},
					{ID: "step-2", Name: "Refine live renderer", Status: guidedworkflows.StepRunStatusRunning},
				},
			},
		},
	})

	rendered := xansi.Strip(renderMarkdown(controller.renderLive(), 120))
	lines := strings.Split(rendered, "\n")
	phaseLine := findGuidedWorkflowLineContaining(lines, "Phase 1  Discovery")
	stepOneLine := findGuidedWorkflowLineContaining(lines, "Step 1.1  Map current state")
	stepTwoLine := findGuidedWorkflowLineContaining(lines, "Step 1.2  Refine live renderer")
	if phaseLine < 0 || stepOneLine < 0 || stepTwoLine < 0 {
		t.Fatalf("expected rendered markdown to keep phase progress lines visible, got %#v", lines)
	}
	if phaseLine == stepOneLine || stepOneLine == stepTwoLine || phaseLine == stepTwoLine {
		t.Fatalf("expected phase progress entries on separate lines, got %#v", lines)
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

func TestGuidedWorkflowControllerRenderSetupOmitsLegacyWorkflowPromptMetadata(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{workspaceID: "ws1"})
	controller.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "solid_phase_delivery", Name: "SOLID Phase Delivery"},
	})
	if !controller.OpenProvider() || !controller.OpenPolicy() || !controller.OpenSetup() {
		t.Fatalf("expected launcher/provider/policy/setup flow")
	}

	setup := controller.Render()
	if !strings.Contains(setup, "### Launch Selections") {
		t.Fatalf("expected launch selections section, got %q", setup)
	}
	if strings.Contains(setup, "### Workflow Prompt") {
		t.Fatalf("expected workflow prompt metadata block removed, got %q", setup)
	}
	if strings.Contains(setup, "### Runtime Options") {
		t.Fatalf("expected runtime options summary block removed, got %q", setup)
	}
	if strings.Contains(setup, "### Controls") {
		t.Fatalf("expected controls legend removed from setup metadata, got %q", setup)
	}
}

func TestGuidedWorkflowControllerRenderSetupShowsReadOnlyFollowUpDependency(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{
		workspaceID:      "ws1",
		followUpRunID:    "gwf-1",
		followUpRunLabel: "Workflow A",
		dependencyLocked: true,
	})
	controller.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "solid_phase_delivery", Name: "SOLID Phase Delivery"},
	})
	if !controller.OpenProvider() || !controller.OpenPolicy() || !controller.OpenSetup() {
		t.Fatalf("expected launcher/provider/policy/setup flow")
	}

	setup := controller.Render()
	if !strings.Contains(setup, "- Depends on: Workflow A (gwf-1) (read-only)") {
		t.Fatalf("expected read-only dependency note in setup, got %q", setup)
	}
}

func TestGuidedWorkflowControllerBuildCreateRequestIncludesFollowUpDependency(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{
		workspaceID:      "ws1",
		worktreeID:       "wt1",
		sessionID:        "s1",
		followUpRunID:    "gwf-1",
		followUpRunLabel: "Workflow A",
	})
	controller.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "solid_phase_delivery", Name: "SOLID Phase Delivery"},
	})
	if !controller.OpenProvider() || !controller.OpenPolicy() || !controller.OpenSetup() {
		t.Fatalf("expected launcher/provider/policy/setup flow")
	}

	req := controller.BuildCreateRequest()
	if len(req.DependsOnRunIDs) != 1 || req.DependsOnRunIDs[0] != "gwf-1" {
		t.Fatalf("expected create request to include follow-up dependency, got %#v", req.DependsOnRunIDs)
	}
}

func TestGuidedWorkflowControllerDependencyPickerTypeAheadSelection(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{workspaceID: "ws-target"})
	controller.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "solid_phase_delivery", Name: "SOLID Phase Delivery"},
	})
	if !controller.OpenProvider() || !controller.OpenPolicy() || !controller.OpenSetup() {
		t.Fatalf("expected launcher/provider/policy/setup flow")
	}
	controller.SetDependencyOptions([]guidedWorkflowDependencyOption{
		{runID: "gwf-a", label: "gwf-a [running] Source A", search: "gwf-a running source a"},
		{runID: "gwf-b", label: "gwf-b [completed] Source B", search: "gwf-b completed source b"},
	})
	if !controller.OpenDependencyPicker() {
		t.Fatalf("expected dependency picker to open")
	}
	if !controller.AppendQuery("gwf-b") {
		t.Fatalf("expected dependency query append")
	}
	if !controller.ConfirmDependencySelection() {
		t.Fatalf("expected dependency selection confirmation")
	}
	req := controller.BuildCreateRequest()
	if len(req.DependsOnRunIDs) != 1 || req.DependsOnRunIDs[0] != "gwf-b" {
		t.Fatalf("expected dependency gwf-b after picker selection, got %#v", req.DependsOnRunIDs)
	}
	setup := controller.Render()
	if !strings.Contains(setup, "- Depends on: gwf-b [completed] Source B") {
		t.Fatalf("expected selected dependency in setup summary, got %q", setup)
	}
}

func TestGuidedWorkflowControllerDependencyPickerReadOnlyGuard(t *testing.T) {
	controller := NewGuidedWorkflowUIController()
	controller.Enter(guidedWorkflowLaunchContext{
		workspaceID:      "ws1",
		followUpRunID:    "gwf-1",
		followUpRunLabel: "Workflow A",
		dependencyLocked: true,
	})
	controller.SetTemplates([]guidedworkflows.WorkflowTemplate{
		{ID: "solid_phase_delivery", Name: "SOLID Phase Delivery"},
	})
	if !controller.OpenProvider() || !controller.OpenPolicy() || !controller.OpenSetup() {
		t.Fatalf("expected launcher/provider/policy/setup flow")
	}
	if controller.OpenDependencyPicker() {
		t.Fatalf("did not expect read-only follow-up context to open dependency picker")
	}
}

func findGuidedWorkflowLineContaining(lines []string, target string) int {
	target = strings.TrimSpace(target)
	if target == "" {
		return -1
	}
	for i, line := range lines {
		if strings.Contains(strings.TrimSpace(line), target) {
			return i
		}
	}
	return -1
}
