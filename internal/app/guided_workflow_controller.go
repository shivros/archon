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
	workspaceID   string
	workspaceName string
	worktreeID    string
	worktreeName  string
	sessionID     string
	sessionName   string
}

type guidedWorkflowTemplateOption struct {
	id          string
	name        string
	description string
}

type guidedWorkflowLauncherTemplatePickerLayout struct {
	queryLine string
	height    int
}

type guidedWorkflowTurnLinkTarget struct {
	label     string
	sessionID string
	turnID    string
}

type GuidedWorkflowUIController struct {
	stage           guidedWorkflowStage
	context         guidedWorkflowLaunchContext
	templateID      string
	templateName    string
	templatePicker  guidedWorkflowTemplatePicker
	defaultPreset   guidedPolicySensitivity
	sensitivity     guidedPolicySensitivity
	userPrompt      string
	resumeMessage   string
	run             *guidedworkflows.WorkflowRun
	timeline        []guidedworkflows.RunTimelineEvent
	lastError       string
	refreshQueued   bool
	lastRefreshAt   time.Time
	selectedPhase   int
	selectedStep    int
	userTurnLink    WorkflowUserTurnLinkBuilder
	promptPresenter workflowPromptPresenter
}

func NewGuidedWorkflowUIController() *GuidedWorkflowUIController {
	return &GuidedWorkflowUIController{
		stage:           guidedWorkflowStageLauncher,
		templateID:      "",
		templateName:    "",
		templatePicker:  newGuidedWorkflowTemplatePicker(),
		defaultPreset:   guidedPolicySensitivityBalanced,
		sensitivity:     guidedPolicySensitivityBalanced,
		userTurnLink:    NewArchonWorkflowUserTurnLinkBuilder(),
		promptPresenter: newWorkflowPromptPresenter(),
	}
}

func (c *GuidedWorkflowUIController) Enter(context guidedWorkflowLaunchContext) {
	if c == nil {
		return
	}
	c.stage = guidedWorkflowStageLauncher
	c.context = context
	c.templateID = ""
	c.templateName = ""
	c.templatePicker.Reset()
	c.sensitivity = c.defaultPreset
	c.userPrompt = ""
	c.resumeMessage = guidedworkflows.DefaultResumeFailedRunMessage
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

func (c *GuidedWorkflowUIController) OpenSetup() bool {
	if c == nil || c.stage != guidedWorkflowStageLauncher {
		return false
	}
	if c.templatePicker.Loading() || strings.TrimSpace(c.templateID) == "" {
		return false
	}
	c.stage = guidedWorkflowStageSetup
	c.lastError = ""
	return true
}

func (c *GuidedWorkflowUIController) OpenLauncher() {
	if c == nil {
		return
	}
	c.stage = guidedWorkflowStageLauncher
}

func (c *GuidedWorkflowUIController) BeginTemplateLoad() {
	if c == nil {
		return
	}
	c.templatePicker.BeginLoad()
}

func (c *GuidedWorkflowUIController) SetTemplateLoadError(err error) {
	if c == nil {
		return
	}
	c.templatePicker.SetError(err)
}

func (c *GuidedWorkflowUIController) SetTemplates(raw []guidedworkflows.WorkflowTemplate) {
	if c == nil {
		return
	}
	c.templatePicker.SetTemplates(raw, c.templateID)
	c.syncTemplateSelection()
}

func (c *GuidedWorkflowUIController) MoveTemplateSelection(delta int) bool {
	if c == nil || c.stage != guidedWorkflowStageLauncher || delta == 0 {
		return false
	}
	if !c.templatePicker.Move(delta) {
		return false
	}
	c.syncTemplateSelection()
	return true
}

func (c *GuidedWorkflowUIController) SetTemplatePickerSize(width, height int) {
	if c == nil {
		return
	}
	c.templatePicker.SetSize(width, height)
}

func (c *GuidedWorkflowUIController) SetUserTurnLinkBuilder(builder WorkflowUserTurnLinkBuilder) {
	if c == nil {
		return
	}
	c.userTurnLink = workflowUserTurnLinkBuilderOrDefault(builder)
}

func (c *GuidedWorkflowUIController) Query() string {
	if c == nil || c.stage != guidedWorkflowStageLauncher {
		return ""
	}
	return c.templatePicker.Query()
}

func (c *GuidedWorkflowUIController) AppendQuery(text string) bool {
	if c == nil || c.stage != guidedWorkflowStageLauncher {
		return false
	}
	if !c.templatePicker.AppendQuery(text) {
		return false
	}
	c.syncTemplateSelection()
	return true
}

func (c *GuidedWorkflowUIController) BackspaceQuery() bool {
	if c == nil || c.stage != guidedWorkflowStageLauncher {
		return false
	}
	if !c.templatePicker.BackspaceQuery() {
		return false
	}
	c.syncTemplateSelection()
	return true
}

func (c *GuidedWorkflowUIController) ClearQuery() bool {
	if c == nil || c.stage != guidedWorkflowStageLauncher {
		return false
	}
	if !c.templatePicker.ClearQuery() {
		return false
	}
	c.syncTemplateSelection()
	return true
}

func (c *GuidedWorkflowUIController) SelectTemplateByRow(row int) bool {
	if c == nil || c.stage != guidedWorkflowStageLauncher {
		return false
	}
	if !c.templatePicker.HandleClick(row) {
		return false
	}
	c.syncTemplateSelection()
	return true
}

func (c *GuidedWorkflowUIController) LauncherTemplatePickerLayout() (guidedWorkflowLauncherTemplatePickerLayout, bool) {
	if c == nil || c.stage != guidedWorkflowStageLauncher {
		return guidedWorkflowLauncherTemplatePickerLayout{}, false
	}
	if c.templatePicker.Loading() || c.templatePicker.Error() != "" || len(c.templatePicker.Options()) == 0 {
		return guidedWorkflowLauncherTemplatePickerLayout{}, false
	}
	view := strings.TrimSpace(c.templatePicker.View())
	if view == "" {
		return guidedWorkflowLauncherTemplatePickerLayout{}, false
	}
	lines := strings.Split(view, "\n")
	return guidedWorkflowLauncherTemplatePickerLayout{
		queryLine: strings.TrimSpace(renderPickerQueryLine(c.templatePicker.Query())),
		height:    len(lines),
	}, true
}

func (c *GuidedWorkflowUIController) LauncherRequiresRawANSIRender() bool {
	if c == nil || c.stage != guidedWorkflowStageLauncher {
		return false
	}
	if c.templatePicker.Loading() || c.templatePicker.Error() != "" || len(c.templatePicker.Options()) == 0 {
		return false
	}
	return strings.Contains(c.templatePicker.View(), "\x1b[")
}

func (c *GuidedWorkflowUIController) TemplatesLoading() bool {
	if c == nil {
		return false
	}
	return c.templatePicker.Loading()
}

func (c *GuidedWorkflowUIController) TemplateLoadError() string {
	if c == nil {
		return ""
	}
	return c.templatePicker.Error()
}

func (c *GuidedWorkflowUIController) HasTemplateSelection() bool {
	if c == nil {
		return false
	}
	return c.templatePicker.HasSelection()
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

func (c *GuidedWorkflowUIController) CanResumeFailedRun() bool {
	return c != nil && c.run != nil && c.run.Status == guidedworkflows.WorkflowRunStatusFailed
}

func (c *GuidedWorkflowUIController) SetResumeMessage(text string) {
	if c == nil {
		return
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		c.resumeMessage = guidedworkflows.DefaultResumeFailedRunMessage
		return
	}
	c.resumeMessage = trimmed
}

func (c *GuidedWorkflowUIController) ResumeMessage() string {
	if c == nil {
		return guidedworkflows.DefaultResumeFailedRunMessage
	}
	trimmed := strings.TrimSpace(c.resumeMessage)
	if trimmed == "" {
		return guidedworkflows.DefaultResumeFailedRunMessage
	}
	return trimmed
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

func (c *GuidedWorkflowUIController) SetResumeError(err error) {
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
		if c.run.Status == guidedworkflows.WorkflowRunStatusFailed && strings.TrimSpace(c.resumeMessage) == "" {
			c.resumeMessage = guidedworkflows.DefaultResumeFailedRunMessage
		}
	default:
		c.stage = guidedWorkflowStageLive
		c.resumeMessage = ""
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
		if c.run.Status == guidedworkflows.WorkflowRunStatusFailed && strings.TrimSpace(c.resumeMessage) == "" {
			c.resumeMessage = guidedworkflows.DefaultResumeFailedRunMessage
		}
	default:
		c.stage = guidedWorkflowStageLive
		c.resumeMessage = ""
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
	sessionID, _ := stepSessionAndTurn(*step)
	return sessionID
}

func (c *GuidedWorkflowUIController) SelectedStepTurnID() string {
	_, step, ok := c.selectedStepRef()
	if !ok {
		return ""
	}
	_, turnID := stepSessionAndTurn(*step)
	return turnID
}

func (c *GuidedWorkflowUIController) TurnLinkTargets() []guidedWorkflowTurnLinkTarget {
	if c == nil || c.run == nil {
		return nil
	}
	targets := make([]guidedWorkflowTurnLinkTarget, 0, 16)
	seen := map[string]struct{}{}
	for _, phase := range c.run.Phases {
		for _, step := range phase.Steps {
			sessionID, turnID := stepSessionAndTurn(step)
			if sessionID == "" || turnID == "" {
				continue
			}
			label := workflowUserTurnLinkLabel(c.stepUserTurnLink(step))
			if label == "" {
				continue
			}
			key := sessionID + "\x00" + turnID + "\x00" + label
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, guidedWorkflowTurnLinkTarget{
				label:     label,
				sessionID: sessionID,
				turnID:    turnID,
			})
		}
	}
	return targets
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

func (c *GuidedWorkflowUIController) BuildResumeRequest() client.WorkflowRunResumeRequest {
	return client.WorkflowRunResumeRequest{
		ResumeFailed: true,
		Message:      c.ResumeMessage(),
	}
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
		"# Workflow Launcher",
		"",
		"Launch a guided workflow from the selected context.",
		"",
		"### Launch Context",
		fmt.Sprintf("- Workspace: %s", c.context.workspaceDisplay()),
		fmt.Sprintf("- Worktree: %s", c.context.worktreeDisplay()),
		fmt.Sprintf("- Task/Session: %s", c.context.sessionDisplay()),
		"",
		"### Template Picker",
		"- Type to filter templates. Use up/down to select.",
	}
	options := c.templatePicker.Options()
	switch {
	case c.templatePicker.Loading():
		lines = append(lines, "- Loading workflow templates...")
	case c.templatePicker.Error() != "":
		lines = append(lines, "- Template load failed: "+c.templatePicker.Error())
	case len(options) == 0:
		lines = append(lines, "- No templates available.")
	default:
		if pickerView := strings.TrimSpace(c.templatePicker.View()); pickerView != "" {
			lines = append(lines, "")
			lines = append(lines, strings.Split(pickerView, "\n")...)
		}
		if selected, ok := c.templatePicker.Selected(); ok {
			lines = append(lines,
				"",
				"### Selected Template",
				fmt.Sprintf("- Name: %s", valueOrFallback(selected.name, selected.id)),
				fmt.Sprintf("- ID: %s", valueOrFallback(selected.id, "(not set)")),
			)
			if text := strings.TrimSpace(selected.description); text != "" {
				lines = append(lines, fmt.Sprintf("- Description: %s", text))
			}
		}
	}
	lines = append(lines,
		"",
		"### Controls",
		"- up/down: choose template",
		"- type/backspace/ctrl+u: filter templates",
		"- enter: continue to run setup",
		"- ctrl+r: reload templates",
		"- esc: close launcher",
	)
	if text := strings.TrimSpace(c.lastError); text != "" {
		lines = append(lines, "", "Error: "+text)
	}
	return joinGuidedWorkflowLines(lines)
}

func (c *GuidedWorkflowUIController) renderSetup() string {
	sensitivity := c.sensitivityLabel()
	chars, linesCount := promptStats(c.userPrompt)
	lines := []string{
		"# Run Setup",
		"",
		"### Selected Template",
		fmt.Sprintf("- Name: %s", valueOrFallback(c.templateName, "(none selected)")),
		fmt.Sprintf("- ID: %s", valueOrFallback(c.templateID, "(not selected)")),
		fmt.Sprintf("- Policy sensitivity: %s", sensitivity),
		"",
		"### Workflow Prompt (Required)",
		"- Input focus: active in the framed task description panel below",
		fmt.Sprintf("- Prompt stats: %d chars across %d lines", chars, linesCount),
		"- Paste support: uses the same editor behavior as chat/notes input",
	}
	lines = append(lines,
		"",
		"### Sensitivity Presets",
		"- low: fewer pauses, higher continue tolerance",
		"- balanced: default confidence-weighted policy",
		"- high: stricter checkpointing and earlier pauses",
		"",
		"### Controls",
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
		return "# Live Timeline\n\nWaiting for run state..."
	}
	lines := []string{
		"# Live Timeline",
		"",
		"### Run Overview",
		fmt.Sprintf("- Run: %s", valueOrFallback(run.ID, "(pending)")),
		fmt.Sprintf("- Status: %s", runStatusText(run.Status)),
		fmt.Sprintf("- Template: %s", valueOrFallback(run.TemplateName, run.TemplateID)),
		fmt.Sprintf("- Original prompt: %s", c.renderWorkflowPrompt(run)),
		fmt.Sprintf("- Checkpoint style: %s", valueOrFallback(run.CheckpointStyle, guidedworkflows.DefaultCheckpointStyle)),
		fmt.Sprintf("- Policy sensitivity: %s", c.sensitivityLabel()),
	}
	if explain := c.decisionExplanation(); explain != "" {
		lines = append(lines, fmt.Sprintf("- Decision explanation: %s", explain))
	}
	lines = append(lines, "", "### Phase Progress")
	lines = append(lines, c.renderPhaseProgress()...)
	lines = append(lines, "", "### Execution Details")
	lines = append(lines, c.renderExecutionDetails()...)
	lines = append(lines, "", "### Artifacts / Timeline")
	lines = append(lines, c.renderTimeline()...)
	lines = append(lines, "", "### Controls")
	lines = append(lines, "- j/down: next step details")
	lines = append(lines, "- k/up: previous step details")
	lines = append(lines, "- o: open selected step user turn")
	lines = append(lines, "- r: refresh timeline")
	lines = append(lines, "- esc: close guided workflow view")
	if c.NeedsDecision() {
		lines = append(lines, "", "### Decision Inbox")
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
		return "# Post-run Summary\n\nNo run data."
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
		"# Post-run Summary",
		"",
		"### Original Prompt",
	}
	lines = append(lines, c.renderWorkflowPromptSummaryBlock(run)...)
	lines = append(lines,
		"",
		"### Outcome",
		fmt.Sprintf("- Run: %s", valueOrFallback(run.ID, "(unknown)")),
		fmt.Sprintf("- Final status: %s", runStatusText(run.Status)),
		fmt.Sprintf("- Completed steps: %d/%d", completedSteps, totalSteps),
		fmt.Sprintf("- Decisions requested: %d", len(run.CheckpointDecisions)),
	)
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
	lines = append(lines, "", "### Step Links")
	lines = append(lines, c.renderStepLinksSummary()...)
	if c.CanResumeFailedRun() {
		lines = append(lines,
			"",
			"### Resume Failed Run",
			"- Edit the resume message in the input panel below before submitting.",
			"- Enter submits a resume request from the last in-flight workflow step.",
		)
		lines = append(lines, "", "### Controls", "- enter: resume run", "- esc: close summary")
	} else {
		lines = append(lines, "", "### Controls", "- enter: close summary", "- esc: close summary")
	}
	return joinGuidedWorkflowLines(lines)
}

func (c *GuidedWorkflowUIController) renderWorkflowPrompt(run *guidedworkflows.WorkflowRun) string {
	if c == nil {
		return workflowPromptUnavailable
	}
	presenter := c.promptPresenter
	if presenter == nil {
		presenter = newWorkflowPromptPresenter()
	}
	return presenter.Present(run)
}

func (c *GuidedWorkflowUIController) renderWorkflowPromptSummaryBlock(run *guidedworkflows.WorkflowRun) []string {
	prompt := c.resolveWorkflowPromptRaw(run)
	if prompt == "" {
		prompt = c.renderWorkflowPrompt(run)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = workflowPromptUnavailable
	}
	lines := strings.Split(prompt, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			out = append(out, ">")
			continue
		}
		out = append(out, "> "+line)
	}
	return out
}

func (c *GuidedWorkflowUIController) resolveWorkflowPromptRaw(run *guidedworkflows.WorkflowRun) string {
	if run == nil {
		return ""
	}
	if prompt := strings.TrimSpace(run.DisplayUserPrompt); prompt != "" {
		return prompt
	}
	return strings.TrimSpace(run.UserPrompt)
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
	lines = append(lines, fmt.Sprintf("- User turn link: %s", c.stepUserTurnLink(*step)))
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
		link := ""
		if _, step, ok := c.stepRefByID(event.PhaseID, event.StepID); ok {
			link = c.stepUserTurnLink(*step)
		}
		if link != unavailableUserTurnLink {
			message = strings.TrimSpace(message + " · " + link)
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

func (c *GuidedWorkflowUIController) renderStepLinksSummary() []string {
	if c == nil || c.run == nil || len(c.run.Phases) == 0 {
		return []string{"- No steps available"}
	}
	lines := make([]string, 0, 16)
	for _, phase := range c.run.Phases {
		phaseName := valueOrFallback(phase.Name, phase.ID)
		for _, step := range phase.Steps {
			stepName := valueOrFallback(step.Name, step.ID)
			stepLinkLabel := workflowUserTurnLinkLabel(c.stepUserTurnLink(step))
			if stepLinkLabel == "" {
				stepLinkLabel = unavailableUserTurnLink
			}
			lines = append(lines, fmt.Sprintf("- %s / %s: %s", phaseName, stepName, stepLinkLabel))
		}
	}
	return lines
}

func (c *GuidedWorkflowUIController) stepRefByID(phaseID, stepID string) (*guidedworkflows.PhaseRun, *guidedworkflows.StepRun, bool) {
	if c == nil || c.run == nil {
		return nil, nil, false
	}
	phaseID = strings.TrimSpace(phaseID)
	stepID = strings.TrimSpace(stepID)
	if phaseID == "" || stepID == "" {
		return nil, nil, false
	}
	for i := range c.run.Phases {
		phase := &c.run.Phases[i]
		if strings.TrimSpace(phase.ID) != phaseID {
			continue
		}
		for j := range phase.Steps {
			step := &phase.Steps[j]
			if strings.TrimSpace(step.ID) == stepID {
				return phase, step, true
			}
		}
	}
	return nil, nil, false
}

func (c *GuidedWorkflowUIController) stepUserTurnLink(step guidedworkflows.StepRun) string {
	if c == nil {
		return unavailableUserTurnLink
	}
	sessionID, turnID := stepSessionAndTurn(step)
	return workflowUserTurnLinkBuilderOrDefault(c.userTurnLink).BuildUserTurnLink(sessionID, turnID)
}

func (c *GuidedWorkflowUIController) stepTraceChip(step guidedworkflows.StepRun) string {
	switch c.normalizedStepExecutionState(step) {
	case guidedworkflows.StepExecutionStateLinked:
		sessionID, turnID := stepSessionAndTurn(step)
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

func (c *GuidedWorkflowUIController) syncTemplateSelection() {
	if c == nil {
		return
	}
	selection, ok := c.templatePicker.Selected()
	if !ok {
		c.templateID = ""
		c.templateName = ""
		return
	}
	c.templateID = strings.TrimSpace(selection.id)
	c.templateName = strings.TrimSpace(selection.name)
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

func (c guidedWorkflowLaunchContext) workspaceDisplay() string {
	return guidedContextDisplay(c.workspaceName, c.workspaceID)
}

func (c guidedWorkflowLaunchContext) worktreeDisplay() string {
	return guidedContextDisplay(c.worktreeName, c.worktreeID)
}

func (c guidedWorkflowLaunchContext) sessionDisplay() string {
	return guidedContextDisplay(c.sessionName, c.sessionID)
}

func guidedContextDisplay(name, id string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	id = strings.TrimSpace(id)
	if id != "" {
		return id
	}
	return "(not set)"
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
