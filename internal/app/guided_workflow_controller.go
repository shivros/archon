package app

import (
	"fmt"
	"strings"
	"time"

	"control/internal/client"
	"control/internal/guidedworkflows"
)

type guidedWorkflowStage int

const (
	guidedWorkflowStageLauncher guidedWorkflowStage = iota
	guidedWorkflowStageSetup
	guidedWorkflowStageLive
	guidedWorkflowStageSummary
)

type guidedPolicySensitivity int

const (
	guidedPolicySensitivityBalanced guidedPolicySensitivity = iota
	guidedPolicySensitivityLow
	guidedPolicySensitivityHigh
)

type guidedWorkflowLaunchContext struct {
	workspaceID string
	worktreeID  string
	sessionID   string
}

type GuidedWorkflowUIController struct {
	stage         guidedWorkflowStage
	context       guidedWorkflowLaunchContext
	templateID    string
	templateName  string
	defaultPreset guidedPolicySensitivity
	sensitivity   guidedPolicySensitivity
	userPrompt    string
	run           *guidedworkflows.WorkflowRun
	timeline      []guidedworkflows.RunTimelineEvent
	lastError     string
	refreshQueued bool
	lastRefreshAt time.Time
	selectedPhase int
	selectedStep  int
}

func NewGuidedWorkflowUIController() *GuidedWorkflowUIController {
	return &GuidedWorkflowUIController{
		stage:         guidedWorkflowStageLauncher,
		templateID:    guidedworkflows.TemplateIDSolidPhaseDelivery,
		templateName:  "SOLID Phase Delivery",
		defaultPreset: guidedPolicySensitivityBalanced,
		sensitivity:   guidedPolicySensitivityBalanced,
	}
}

func (c *GuidedWorkflowUIController) Enter(context guidedWorkflowLaunchContext) {
	if c == nil {
		return
	}
	c.stage = guidedWorkflowStageLauncher
	c.context = context
	c.templateID = guidedworkflows.TemplateIDSolidPhaseDelivery
	c.templateName = "SOLID Phase Delivery"
	c.sensitivity = c.defaultPreset
	c.userPrompt = ""
	c.run = nil
	c.timeline = nil
	c.lastError = ""
	c.refreshQueued = false
	c.lastRefreshAt = time.Time{}
	c.selectedPhase = 0
	c.selectedStep = 0
}

func (c *GuidedWorkflowUIController) Exit() {
	if c == nil {
		return
	}
	c.Enter(guidedWorkflowLaunchContext{})
}

func (c *GuidedWorkflowUIController) Stage() guidedWorkflowStage {
	if c == nil {
		return guidedWorkflowStageLauncher
	}
	return c.stage
}

func (c *GuidedWorkflowUIController) OpenSetup() {
	if c == nil {
		return
	}
	c.stage = guidedWorkflowStageSetup
	c.lastError = ""
}

func (c *GuidedWorkflowUIController) OpenLauncher() {
	if c == nil {
		return
	}
	c.stage = guidedWorkflowStageLauncher
}

func (c *GuidedWorkflowUIController) SetDefaultSensitivity(sensitivity guidedPolicySensitivity) {
	if c == nil {
		return
	}
	switch sensitivity {
	case guidedPolicySensitivityLow, guidedPolicySensitivityBalanced, guidedPolicySensitivityHigh:
		c.defaultPreset = sensitivity
	default:
		c.defaultPreset = guidedPolicySensitivityBalanced
	}
	if c.stage == guidedWorkflowStageLauncher || c.stage == guidedWorkflowStageSetup {
		c.sensitivity = c.defaultPreset
	}
}

func (c *GuidedWorkflowUIController) CycleSensitivity(delta int) {
	if c == nil || c.stage != guidedWorkflowStageSetup || delta == 0 {
		return
	}
	order := []guidedPolicySensitivity{
		guidedPolicySensitivityLow,
		guidedPolicySensitivityBalanced,
		guidedPolicySensitivityHigh,
	}
	current := 1
	for idx, value := range order {
		if value == c.sensitivity {
			current = idx
			break
		}
	}
	next := (current + delta + len(order)) % len(order)
	c.sensitivity = order[next]
}

func (c *GuidedWorkflowUIController) BeginStart() {
	if c == nil {
		return
	}
	c.lastError = ""
	c.run = nil
	c.timeline = nil
	c.refreshQueued = false
	c.lastRefreshAt = time.Time{}
	c.selectedPhase = 0
	c.selectedStep = 0
}

func (c *GuidedWorkflowUIController) SetUserPrompt(text string) {
	if c == nil {
		return
	}
	c.userPrompt = text
}

func (c *GuidedWorkflowUIController) UserPrompt() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.userPrompt)
}

func (c *GuidedWorkflowUIController) SetCreateError(err error) {
	if c == nil {
		return
	}
	c.lastError = errorText(err)
}

func (c *GuidedWorkflowUIController) SetStartError(err error) {
	if c == nil {
		return
	}
	c.lastError = errorText(err)
}

func (c *GuidedWorkflowUIController) SetDecisionError(err error) {
	if c == nil {
		return
	}
	c.lastError = errorText(err)
}

func (c *GuidedWorkflowUIController) SetSnapshotError(err error) {
	if c == nil {
		return
	}
	c.lastError = errorText(err)
	c.refreshQueued = false
}

func (c *GuidedWorkflowUIController) SetRun(run *guidedworkflows.WorkflowRun) {
	if c == nil {
		return
	}
	c.run = cloneWorkflowRun(run)
	c.lastError = ""
	c.refreshQueued = false
	c.syncStepSelection()
	if c.run == nil {
		return
	}
	switch c.run.Status {
	case guidedworkflows.WorkflowRunStatusCompleted, guidedworkflows.WorkflowRunStatusFailed:
		c.stage = guidedWorkflowStageSummary
	default:
		c.stage = guidedWorkflowStageLive
	}
}

func (c *GuidedWorkflowUIController) SetSnapshot(run *guidedworkflows.WorkflowRun, timeline []guidedworkflows.RunTimelineEvent) {
	if c == nil {
		return
	}
	c.run = cloneWorkflowRun(run)
	c.timeline = cloneRunTimeline(timeline)
	c.lastError = ""
	c.refreshQueued = false
	c.syncStepSelection()
	if c.run == nil {
		return
	}
	switch c.run.Status {
	case guidedworkflows.WorkflowRunStatusCompleted, guidedworkflows.WorkflowRunStatusFailed:
		c.stage = guidedWorkflowStageSummary
	default:
		c.stage = guidedWorkflowStageLive
	}
}

func (c *GuidedWorkflowUIController) RunID() string {
	if c == nil || c.run == nil {
		return ""
	}
	return strings.TrimSpace(c.run.ID)
}

func (c *GuidedWorkflowUIController) MarkRefreshQueued(at time.Time) {
	if c == nil {
		return
	}
	c.refreshQueued = true
	c.lastRefreshAt = at.UTC()
}

func (c *GuidedWorkflowUIController) CanRefresh(now time.Time, interval time.Duration) bool {
	if c == nil || c.stage != guidedWorkflowStageLive {
		return false
	}
	if strings.TrimSpace(c.RunID()) == "" || c.refreshQueued {
		return false
	}
	if c.run == nil {
		return false
	}
	switch c.run.Status {
	case guidedworkflows.WorkflowRunStatusCompleted, guidedworkflows.WorkflowRunStatusFailed:
		return false
	}
	if interval <= 0 {
		return true
	}
	if c.lastRefreshAt.IsZero() {
		return true
	}
	return now.Sub(c.lastRefreshAt) >= interval
}

func (c *GuidedWorkflowUIController) NeedsDecision() bool {
	if c == nil || c.run == nil {
		return false
	}
	if c.run.Status != guidedworkflows.WorkflowRunStatusPaused {
		return false
	}
	if c.run.LatestDecision == nil {
		return false
	}
	return c.run.LatestDecision.Metadata.Action == guidedworkflows.CheckpointActionPause
}

func (c *GuidedWorkflowUIController) MoveStepSelection(delta int) {
	if c == nil || c.run == nil || delta == 0 {
		return
	}
	steps := c.stepLocations()
	if len(steps) == 0 {
		c.selectedPhase = 0
		c.selectedStep = 0
		return
	}
	current := 0
	for idx, location := range steps {
		if location.phase == c.selectedPhase && location.step == c.selectedStep {
			current = idx
			break
		}
	}
	next := (current + delta + len(steps)) % len(steps)
	c.selectedPhase = steps[next].phase
	c.selectedStep = steps[next].step
}

func (c *GuidedWorkflowUIController) SelectedStepSessionID() string {
	_, step, ok := c.selectedStepRef()
	if !ok {
		return ""
	}
	if step.Execution != nil {
		if sessionID := strings.TrimSpace(step.Execution.SessionID); sessionID != "" {
			return sessionID
		}
	}
	return ""
}

func (c *GuidedWorkflowUIController) BuildCreateRequest() client.CreateWorkflowRunRequest {
	req := client.CreateWorkflowRunRequest{
		TemplateID:  strings.TrimSpace(c.templateID),
		WorkspaceID: strings.TrimSpace(c.context.workspaceID),
		WorktreeID:  strings.TrimSpace(c.context.worktreeID),
		SessionID:   strings.TrimSpace(c.context.sessionID),
		UserPrompt:  strings.TrimSpace(c.userPrompt),
	}
	if override := policyOverrideForSensitivity(c.sensitivity); override != nil {
		req.PolicyOverrides = override
	}
	return req
}

func (c *GuidedWorkflowUIController) BuildDecisionRequest(action guidedworkflows.DecisionAction) client.WorkflowRunDecisionRequest {
	req := client.WorkflowRunDecisionRequest{Action: action}
	if c == nil || c.run == nil || c.run.LatestDecision == nil {
		return req
	}
	req.DecisionID = strings.TrimSpace(c.run.LatestDecision.ID)
	return req
}

func (c *GuidedWorkflowUIController) RecommendedDecisionAction() guidedworkflows.DecisionAction {
	if c == nil || c.run == nil || c.run.LatestDecision == nil {
		return guidedworkflows.DecisionActionApproveContinue
	}
	meta := c.run.LatestDecision.Metadata
	if meta.HardGateTriggered {
		return guidedworkflows.DecisionActionRequestRevision
	}
	switch meta.Severity {
	case guidedworkflows.DecisionSeverityHigh, guidedworkflows.DecisionSeverityCritical:
		return guidedworkflows.DecisionActionRequestRevision
	default:
		return guidedworkflows.DecisionActionApproveContinue
	}
}

func (c *GuidedWorkflowUIController) Render() string {
	if c == nil {
		return "Guided workflow unavailable."
	}
	switch c.stage {
	case guidedWorkflowStageSetup:
		return c.renderSetup()
	case guidedWorkflowStageLive:
		return c.renderLive()
	case guidedWorkflowStageSummary:
		return c.renderSummary()
	default:
		return c.renderLauncher()
	}
}

func (c *GuidedWorkflowUIController) renderLauncher() string {
	lines := []string{
		"Workflow Launcher",
		"",
		"Start a guided workflow run manually from the selected context.",
		"",
		"Context",
		fmt.Sprintf("- Workspace: %s", valueOrFallback(c.context.workspaceID, "(not set)")),
		fmt.Sprintf("- Worktree: %s", valueOrFallback(c.context.worktreeID, "(not set)")),
		fmt.Sprintf("- Task/Session: %s", valueOrFallback(c.context.sessionID, "(not set)")),
		"",
		"Controls",
		"- enter: continue to run setup",
		"- esc: close launcher",
	}
	if text := strings.TrimSpace(c.lastError); text != "" {
		lines = append(lines, "", "Error: "+text)
	}
	return joinGuidedWorkflowLines(lines)
}

func (c *GuidedWorkflowUIController) renderSetup() string {
	sensitivity := c.sensitivityLabel()
	chars, linesCount := promptStats(c.userPrompt)
	lines := []string{
		"Run Setup",
		"",
		fmt.Sprintf("Template: %s (%s)", valueOrFallback(c.templateName, "SOLID Phase Delivery"), valueOrFallback(c.templateID, guidedworkflows.TemplateIDSolidPhaseDelivery)),
		fmt.Sprintf("Policy sensitivity: %s", sensitivity),
		"",
		"Workflow prompt (required)",
		"- Input focus: active in the framed task description panel below",
		fmt.Sprintf("- Prompt stats: %d chars across %d lines", chars, linesCount),
		"- Paste support: uses the same editor behavior as chat/notes input",
	}
	lines = append(lines,
		"",
		"Sensitivity presets",
		"- low: fewer pauses, higher continue tolerance",
		"- balanced: default confidence-weighted policy",
		"- high: stricter checkpointing and earlier pauses",
		"",
		"Controls",
		"- type/paste: edit workflow prompt",
		"- up/down: change sensitivity",
		"- enter: create and start run",
		"- esc: back to launcher",
	)
	if text := strings.TrimSpace(c.lastError); text != "" {
		lines = append(lines, "", "Error: "+text)
	}
	return joinGuidedWorkflowLines(lines)
}

func (c *GuidedWorkflowUIController) renderLive() string {
	run := c.run
	if run == nil {
		return "Live Timeline\n\nWaiting for run state..."
	}
	lines := []string{
		"Live Timeline",
		"",
		"Run Overview",
		fmt.Sprintf("- Run: %s", valueOrFallback(run.ID, "(pending)")),
		fmt.Sprintf("- Status: %s", runStatusText(run.Status)),
		fmt.Sprintf("- Template: %s", valueOrFallback(run.TemplateName, run.TemplateID)),
		fmt.Sprintf("- Checkpoint style: %s", valueOrFallback(run.CheckpointStyle, guidedworkflows.DefaultCheckpointStyle)),
		fmt.Sprintf("- Policy sensitivity: %s", c.sensitivityLabel()),
	}
	if explain := c.decisionExplanation(); explain != "" {
		lines = append(lines, fmt.Sprintf("- Decision explanation: %s", explain))
	}
	lines = append(lines, "", "Phase Progress")
	lines = append(lines, c.renderPhaseProgress()...)
	lines = append(lines, "", "Execution Details")
	lines = append(lines, c.renderExecutionDetails()...)
	lines = append(lines, "", "Artifacts / Timeline")
	lines = append(lines, c.renderTimeline()...)
	lines = append(lines, "", "Controls")
	lines = append(lines, "- j/down: next step details")
	lines = append(lines, "- k/up: previous step details")
	lines = append(lines, "- o: open selected step session")
	lines = append(lines, "- r: refresh timeline")
	lines = append(lines, "- esc: close guided workflow view")
	if c.NeedsDecision() {
		lines = append(lines, "", "Decision Inbox")
		lines = append(lines, c.renderDecisionInbox()...)
	}
	if text := strings.TrimSpace(c.lastError); text != "" {
		lines = append(lines, "", "Error: "+text)
	}
	return joinGuidedWorkflowLines(lines)
}

func (c *GuidedWorkflowUIController) renderSummary() string {
	run := c.run
	if run == nil {
		return "Post-run Summary\n\nNo run data."
	}
	completedSteps := 0
	totalSteps := 0
	for _, phase := range run.Phases {
		for _, step := range phase.Steps {
			totalSteps++
			if step.Status == guidedworkflows.StepRunStatusCompleted {
				completedSteps++
			}
		}
	}
	lines := []string{
		"Post-run Summary",
		"",
		"Outcome",
		fmt.Sprintf("- Run: %s", valueOrFallback(run.ID, "(unknown)")),
		fmt.Sprintf("- Final status: %s", runStatusText(run.Status)),
		fmt.Sprintf("- Completed steps: %d/%d", completedSteps, totalSteps),
		fmt.Sprintf("- Decisions requested: %d", len(run.CheckpointDecisions)),
	}
	linkedSteps, unavailableSteps := c.traceabilityCounts()
	lines = append(lines, fmt.Sprintf("- Traceability: %d/%d linked (%d unavailable)", linkedSteps, totalSteps, unavailableSteps))
	if run.CompletedAt != nil {
		lines = append(lines, fmt.Sprintf("- Completed at: %s", run.CompletedAt.UTC().Format(time.RFC3339)))
	}
	if strings.TrimSpace(run.LastError) != "" {
		lines = append(lines, fmt.Sprintf("- Failure detail: %s", strings.TrimSpace(run.LastError)))
	}
	if explain := c.decisionExplanation(); explain != "" {
		lines = append(lines, fmt.Sprintf("- Final decision explanation: %s", explain))
	}
	lines = append(lines, "", "Controls", "- enter: close summary", "- esc: close summary")
	return joinGuidedWorkflowLines(lines)
}

func joinGuidedWorkflowLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			out = append(out, "")
			continue
		}
		// Guided workflow content is rendered through markdown; add hard line
		// breaks so single-line fields don't collapse into one paragraph.
		out = append(out, line+"  ")
	}
	return strings.Join(out, "\n")
}

func (c *GuidedWorkflowUIController) renderPhaseProgress() []string {
	if c == nil || c.run == nil || len(c.run.Phases) == 0 {
		return []string{"- No phase data"}
	}
	lines := make([]string, 0, len(c.run.Phases)*2)
	for phaseIdx, phase := range c.run.Phases {
		phasePrefix := phaseStatusPrefix(phase.Status)
		lines = append(lines, fmt.Sprintf("%s %d. %s", phasePrefix, phaseIdx+1, valueOrFallback(phase.Name, phase.ID)))
		for stepIdx, step := range phase.Steps {
			selected := " "
			if phaseIdx == c.selectedPhase && stepIdx == c.selectedStep {
				selected = ">"
			}
			traceChip := c.stepTraceChip(step)
			lines = append(lines, fmt.Sprintf("  %s %s %s %s", selected, stepStatusPrefix(step.Status), valueOrFallback(step.Name, step.ID), traceChip))
		}
	}
	return lines
}

func (c *GuidedWorkflowUIController) renderExecutionDetails() []string {
	phase, step, ok := c.selectedStepRef()
	if !ok {
		return []string{"- Select a step to inspect execution details"}
	}
	lines := []string{
		fmt.Sprintf("- Step: %s / %s", valueOrFallback(phase.Name, phase.ID), valueOrFallback(step.Name, step.ID)),
		fmt.Sprintf("- Status: %s", strings.TrimSpace(string(step.Status))),
		fmt.Sprintf("- Execution state: %s", c.stepExecutionStateLabel(*step)),
	}
	if text := strings.TrimSpace(step.ExecutionMessage); text != "" {
		lines = append(lines, fmt.Sprintf("- Execution message: %s", text))
	}
	sessionID := ""
	if step.Execution != nil {
		sessionID = strings.TrimSpace(step.Execution.SessionID)
	}
	if sessionID != "" {
		lines = append(lines, fmt.Sprintf("- Session: %s", sessionID))
	} else {
		lines = append(lines, "- Session: (none)")
	}
	if step.Execution != nil {
		lines = append(lines, fmt.Sprintf("- Provider/model: %s / %s",
			valueOrFallback(step.Execution.Provider, "(unknown)"),
			valueOrFallback(step.Execution.Model, "(default)"),
		))
		lines = append(lines, fmt.Sprintf("- Turn id: %s", valueOrFallback(step.Execution.TurnID, step.TurnID)))
		lines = append(lines, fmt.Sprintf("- Trace id: %s", valueOrFallback(step.Execution.TraceID, "(none)")))
		prompt := strings.TrimSpace(step.Execution.PromptSnapshot)
		if prompt == "" {
			prompt = strings.TrimSpace(step.Prompt)
		}
		if prompt != "" {
			lines = append(lines, fmt.Sprintf("- Prompt snapshot: %s", truncateRunes(prompt, 160)))
		}
	} else {
		lines = append(lines, fmt.Sprintf("- Turn id: %s", valueOrFallback(step.TurnID, "(none)")))
	}
	return lines
}

func (c *GuidedWorkflowUIController) renderTimeline() []string {
	if c == nil || len(c.timeline) == 0 {
		return []string{"- No events yet"}
	}
	limit := min(12, len(c.timeline))
	start := len(c.timeline) - limit
	lines := make([]string, 0, limit)
	for i := start; i < len(c.timeline); i++ {
		event := c.timeline[i]
		stamp := event.At.UTC().Format("15:04:05")
		message := strings.TrimSpace(event.Message)
		if message == "" {
			message = strings.TrimSpace(event.Type)
		}
		lines = append(lines, fmt.Sprintf("- %s %s", stamp, valueOrFallback(message, "(event)")))
	}
	return lines
}

func (c *GuidedWorkflowUIController) renderDecisionInbox() []string {
	if c == nil || c.run == nil || c.run.LatestDecision == nil {
		return []string{"- No pending decision"}
	}
	decision := c.run.LatestDecision
	reasonLine := "no explicit reason"
	if len(decision.Metadata.Reasons) > 0 {
		parts := make([]string, 0, len(decision.Metadata.Reasons))
		for _, reason := range decision.Metadata.Reasons {
			if text := strings.TrimSpace(reason.Message); text != "" {
				parts = append(parts, text)
			} else if code := strings.TrimSpace(reason.Code); code != "" {
				parts = append(parts, code)
			}
		}
		if len(parts) > 0 {
			reasonLine = strings.Join(parts, "; ")
		}
	}
	return []string{
		fmt.Sprintf("- Why paused: %s", reasonLine),
		fmt.Sprintf("- Confidence/score: %.2f / %.2f", decision.Metadata.Confidence, decision.Metadata.Score),
		fmt.Sprintf("- Severity/Tier: %s / %s", decision.Metadata.Severity, decision.Metadata.Tier),
		fmt.Sprintf("- Recommended action: %s", decisionActionText(c.RecommendedDecisionAction())),
		"- Actions: a approve/continue, v request revision, p pause run",
	}
}

func (c *GuidedWorkflowUIController) stepTraceChip(step guidedworkflows.StepRun) string {
	switch c.normalizedStepExecutionState(step) {
	case guidedworkflows.StepExecutionStateLinked:
		sessionID := ""
		turnID := strings.TrimSpace(step.TurnID)
		if step.Execution != nil {
			sessionID = strings.TrimSpace(step.Execution.SessionID)
			if turnID == "" {
				turnID = strings.TrimSpace(step.Execution.TurnID)
			}
		}
		if sessionID == "" {
			return "[session:linked]"
		}
		if turnID == "" {
			return fmt.Sprintf("[session:%s]", sessionID)
		}
		return fmt.Sprintf("[session:%s turn:%s]", sessionID, turnID)
	case guidedworkflows.StepExecutionStateUnavailable:
		return "[session:unavailable]"
	default:
		return "[session:none]"
	}
}

func (c *GuidedWorkflowUIController) stepExecutionStateLabel(step guidedworkflows.StepRun) string {
	return string(c.normalizedStepExecutionState(step))
}

func (c *GuidedWorkflowUIController) normalizedStepExecutionState(step guidedworkflows.StepRun) guidedworkflows.StepExecutionState {
	switch step.ExecutionState {
	case guidedworkflows.StepExecutionStateLinked, guidedworkflows.StepExecutionStateUnavailable, guidedworkflows.StepExecutionStateNone:
		return step.ExecutionState
	}
	if step.Execution != nil && strings.TrimSpace(step.Execution.SessionID) != "" {
		return guidedworkflows.StepExecutionStateLinked
	}
	if strings.TrimSpace(step.ExecutionMessage) != "" {
		return guidedworkflows.StepExecutionStateUnavailable
	}
	return guidedworkflows.StepExecutionStateNone
}

func (c *GuidedWorkflowUIController) traceabilityCounts() (linked int, unavailable int) {
	if c == nil || c.run == nil {
		return 0, 0
	}
	for _, phase := range c.run.Phases {
		for _, step := range phase.Steps {
			switch c.normalizedStepExecutionState(step) {
			case guidedworkflows.StepExecutionStateLinked:
				linked++
			case guidedworkflows.StepExecutionStateUnavailable:
				unavailable++
			}
		}
	}
	return linked, unavailable
}

type stepLocation struct {
	phase int
	step  int
}

func (c *GuidedWorkflowUIController) stepLocations() []stepLocation {
	if c == nil || c.run == nil {
		return nil
	}
	locations := make([]stepLocation, 0, 16)
	for phaseIdx, phase := range c.run.Phases {
		for stepIdx := range phase.Steps {
			locations = append(locations, stepLocation{phase: phaseIdx, step: stepIdx})
		}
	}
	return locations
}

func (c *GuidedWorkflowUIController) syncStepSelection() {
	if c == nil || c.run == nil {
		c.selectedPhase = 0
		c.selectedStep = 0
		return
	}
	if c.selectedPhase >= 0 && c.selectedPhase < len(c.run.Phases) {
		phase := c.run.Phases[c.selectedPhase]
		if c.selectedStep >= 0 && c.selectedStep < len(phase.Steps) {
			return
		}
	}
	if c.run.CurrentPhaseIndex >= 0 && c.run.CurrentPhaseIndex < len(c.run.Phases) {
		phase := c.run.Phases[c.run.CurrentPhaseIndex]
		if c.run.CurrentStepIndex >= 0 && c.run.CurrentStepIndex < len(phase.Steps) {
			c.selectedPhase = c.run.CurrentPhaseIndex
			c.selectedStep = c.run.CurrentStepIndex
			return
		}
	}
	locations := c.stepLocations()
	if len(locations) == 0 {
		c.selectedPhase = 0
		c.selectedStep = 0
		return
	}
	c.selectedPhase = locations[0].phase
	c.selectedStep = locations[0].step
}

func (c *GuidedWorkflowUIController) selectedStepRef() (*guidedworkflows.PhaseRun, *guidedworkflows.StepRun, bool) {
	if c == nil || c.run == nil {
		return nil, nil, false
	}
	if c.selectedPhase < 0 || c.selectedPhase >= len(c.run.Phases) {
		return nil, nil, false
	}
	phase := &c.run.Phases[c.selectedPhase]
	if c.selectedStep < 0 || c.selectedStep >= len(phase.Steps) {
		return nil, nil, false
	}
	return phase, &phase.Steps[c.selectedStep], true
}

func (c *GuidedWorkflowUIController) sensitivityLabel() string {
	switch c.sensitivity {
	case guidedPolicySensitivityLow:
		return "Low"
	case guidedPolicySensitivityHigh:
		return "High"
	default:
		return "Balanced"
	}
}

func (c *GuidedWorkflowUIController) decisionExplanation() string {
	if c == nil || c.run == nil || c.run.LatestDecision == nil {
		return ""
	}
	decision := c.run.LatestDecision
	parts := make([]string, 0, len(decision.Metadata.Reasons))
	for _, reason := range decision.Metadata.Reasons {
		text := strings.TrimSpace(reason.Message)
		if text == "" {
			text = strings.TrimSpace(reason.Code)
		}
		if text != "" {
			parts = append(parts, text)
		}
	}
	base := "no triggers provided"
	if len(parts) > 0 {
		base = strings.Join(parts, "; ")
	}
	switch decision.Metadata.Action {
	case guidedworkflows.CheckpointActionPause:
		return "paused because " + base
	case guidedworkflows.CheckpointActionContinue:
		return "continued because " + base
	default:
		return base
	}
}

func runStatusText(status guidedworkflows.WorkflowRunStatus) string {
	switch status {
	case guidedworkflows.WorkflowRunStatusCreated:
		return "created"
	case guidedworkflows.WorkflowRunStatusRunning:
		return "running"
	case guidedworkflows.WorkflowRunStatusPaused:
		return "paused (decision needed)"
	case guidedworkflows.WorkflowRunStatusCompleted:
		return "completed"
	case guidedworkflows.WorkflowRunStatusFailed:
		return "failed"
	default:
		return strings.TrimSpace(string(status))
	}
}

func stepStatusPrefix(status guidedworkflows.StepRunStatus) string {
	switch status {
	case guidedworkflows.StepRunStatusCompleted:
		return "[x]"
	case guidedworkflows.StepRunStatusRunning:
		return "[>]"
	case guidedworkflows.StepRunStatusFailed:
		return "[!]"
	default:
		return "[ ]"
	}
}

func phaseStatusPrefix(status guidedworkflows.PhaseRunStatus) string {
	switch status {
	case guidedworkflows.PhaseRunStatusCompleted:
		return "[x]"
	case guidedworkflows.PhaseRunStatusRunning:
		return "[>]"
	case guidedworkflows.PhaseRunStatusFailed:
		return "[!]"
	default:
		return "[ ]"
	}
}

func decisionActionText(action guidedworkflows.DecisionAction) string {
	switch action {
	case guidedworkflows.DecisionActionApproveContinue:
		return "approve and continue"
	case guidedworkflows.DecisionActionRequestRevision:
		return "request revision"
	case guidedworkflows.DecisionActionPauseRun:
		return "pause run"
	default:
		return strings.TrimSpace(string(action))
	}
}

func policyOverrideForSensitivity(sensitivity guidedPolicySensitivity) *guidedworkflows.CheckpointPolicyOverride {
	return guidedworkflows.PolicyOverrideForPreset(policyPresetForSensitivity(sensitivity))
}

func guidedPolicySensitivityFromPreset(preset string) guidedPolicySensitivity {
	normalized, ok := guidedworkflows.NormalizePolicyPreset(preset)
	if !ok {
		return guidedPolicySensitivityBalanced
	}
	switch normalized {
	case guidedworkflows.PolicyPresetLow:
		return guidedPolicySensitivityLow
	case guidedworkflows.PolicyPresetHigh:
		return guidedPolicySensitivityHigh
	default:
		return guidedPolicySensitivityBalanced
	}
}

func policyPresetForSensitivity(sensitivity guidedPolicySensitivity) guidedworkflows.PolicyPreset {
	switch sensitivity {
	case guidedPolicySensitivityLow:
		return guidedworkflows.PolicyPresetLow
	case guidedPolicySensitivityHigh:
		return guidedworkflows.PolicyPresetHigh
	default:
		return guidedworkflows.PolicyPresetBalanced
	}
}

func promptStats(text string) (chars int, lines int) {
	chars = len([]rune(text))
	lines = 1
	if text == "" {
		return chars, lines
	}
	lines = len(strings.Split(text, "\n"))
	return chars, lines
}

func cloneWorkflowRun(run *guidedworkflows.WorkflowRun) *guidedworkflows.WorkflowRun {
	if run == nil {
		return nil
	}
	cloned := *run
	cloned.Phases = make([]guidedworkflows.PhaseRun, len(run.Phases))
	for i := range run.Phases {
		phase := run.Phases[i]
		cloned.Phases[i] = phase
		if phase.Steps != nil {
			cloned.Phases[i].Steps = append([]guidedworkflows.StepRun(nil), phase.Steps...)
			for stepIdx := range cloned.Phases[i].Steps {
				step := &cloned.Phases[i].Steps[stepIdx]
				if step.Execution != nil {
					execution := *step.Execution
					step.Execution = &execution
				}
				if step.ExecutionAttempts != nil {
					step.ExecutionAttempts = append([]guidedworkflows.StepExecutionRef(nil), step.ExecutionAttempts...)
				}
			}
		}
	}
	if run.CheckpointDecisions != nil {
		cloned.CheckpointDecisions = append([]guidedworkflows.CheckpointDecision(nil), run.CheckpointDecisions...)
	}
	if run.LatestDecision != nil {
		decision := *run.LatestDecision
		if run.LatestDecision.Metadata.Reasons != nil {
			decision.Metadata.Reasons = append([]guidedworkflows.CheckpointReason(nil), run.LatestDecision.Metadata.Reasons...)
		}
		cloned.LatestDecision = &decision
	}
	return &cloned
}

func cloneRunTimeline(events []guidedworkflows.RunTimelineEvent) []guidedworkflows.RunTimelineEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]guidedworkflows.RunTimelineEvent, len(events))
	copy(out, events)
	return out
}

func valueOrFallback(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func truncateRunes(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || text == "" {
		return text
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}
