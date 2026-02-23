package app

import (
	"errors"
	"fmt"
	"strings"

	"control/internal/guidedworkflows"
	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

type selectionActionConfirmSpec struct {
	title       string
	message     string
	confirmText string
	cancelText  string
}

type selectionAction interface {
	Validate(*Model) error
	ConfirmSpec(*Model) selectionActionConfirmSpec
	Execute(*Model) tea.Cmd
}

type deleteWorkspaceSelectionAction struct {
	workspaceID string
}

func (a deleteWorkspaceSelectionAction) Validate(_ *Model) error {
	if strings.TrimSpace(a.workspaceID) == "" || strings.TrimSpace(a.workspaceID) == unassignedWorkspaceID {
		return errors.New("select a workspace to delete")
	}
	return nil
}

func (a deleteWorkspaceSelectionAction) ConfirmSpec(m *Model) selectionActionConfirmSpec {
	message := "Delete workspace?"
	if m != nil {
		if ws := m.workspaceByID(a.workspaceID); ws != nil && strings.TrimSpace(ws.Name) != "" {
			message = fmt.Sprintf("Delete workspace %q?", ws.Name)
		}
	}
	return selectionActionConfirmSpec{
		title:       "Delete Workspace",
		message:     message,
		confirmText: "Delete",
		cancelText:  "Cancel",
	}
}

func (a deleteWorkspaceSelectionAction) Execute(m *Model) tea.Cmd {
	if m == nil {
		return nil
	}
	m.setStatusMessage("deleting workspace")
	return deleteWorkspaceCmd(m.workspaceAPI, a.workspaceID)
}

type deleteWorktreeSelectionAction struct {
	workspaceID string
	worktreeID  string
}

func (a deleteWorktreeSelectionAction) Validate(_ *Model) error {
	if strings.TrimSpace(a.workspaceID) == "" || strings.TrimSpace(a.worktreeID) == "" {
		return errors.New("select a worktree to delete")
	}
	return nil
}

func (a deleteWorktreeSelectionAction) ConfirmSpec(m *Model) selectionActionConfirmSpec {
	message := "Delete worktree?"
	if m != nil {
		if wt := m.worktreeByID(a.worktreeID); wt != nil && strings.TrimSpace(wt.Name) != "" {
			message = fmt.Sprintf("Delete worktree %q?", wt.Name)
		}
	}
	return selectionActionConfirmSpec{
		title:       "Delete Worktree",
		message:     message,
		confirmText: "Delete",
		cancelText:  "Cancel",
	}
}

func (a deleteWorktreeSelectionAction) Execute(m *Model) tea.Cmd {
	if m == nil {
		return nil
	}
	m.setStatusMessage("deleting worktree")
	return deleteWorktreeCmd(m.workspaceAPI, a.workspaceID, a.worktreeID)
}

type dismissSessionSelectionAction struct {
	sessionID string
}

func (a dismissSessionSelectionAction) Validate(_ *Model) error {
	if strings.TrimSpace(a.sessionID) == "" {
		return errors.New("select a session to dismiss")
	}
	return nil
}

func (a dismissSessionSelectionAction) ConfirmSpec(_ *Model) selectionActionConfirmSpec {
	return selectionActionConfirmSpec{
		title:       "Dismiss Sessions",
		message:     "Dismiss session?",
		confirmText: "Dismiss",
		cancelText:  "Cancel",
	}
}

func (a dismissSessionSelectionAction) Execute(m *Model) tea.Cmd {
	if m == nil {
		return nil
	}
	m.setStatusMessage("dismissing " + a.sessionID)
	return dismissSessionCmd(m.sessionAPI, a.sessionID)
}

type dismissWorkflowSelectionAction struct {
	runID string
}

func (a dismissWorkflowSelectionAction) Validate(_ *Model) error {
	if strings.TrimSpace(a.runID) == "" {
		return errors.New("select a workflow to dismiss")
	}
	return nil
}

func (a dismissWorkflowSelectionAction) ConfirmSpec(_ *Model) selectionActionConfirmSpec {
	return selectionActionConfirmSpec{
		title:       "Dismiss Workflow",
		message:     "Dismiss workflow?",
		confirmText: "Dismiss",
		cancelText:  "Cancel",
	}
}

func (a dismissWorkflowSelectionAction) Execute(m *Model) tea.Cmd {
	if m == nil {
		return nil
	}
	m.setStatusMessage("dismissing workflow " + a.runID)
	return dismissWorkflowRunCmd(m.guidedWorkflowAPI, a.runID)
}

func resolveDismissOrDeleteSelectionAction(item *sidebarItem) (selectionAction, error) {
	if item == nil {
		return nil, errors.New("select an item to dismiss or delete")
	}
	switch item.kind {
	case sidebarWorkspace:
		if item.workspace == nil {
			return nil, errors.New("select a workspace to delete")
		}
		return deleteWorkspaceSelectionAction{workspaceID: item.workspace.ID}, nil
	case sidebarWorktree:
		if item.worktree == nil {
			return nil, errors.New("select a worktree to delete")
		}
		return deleteWorktreeSelectionAction{
			workspaceID: item.worktree.WorkspaceID,
			worktreeID:  item.worktree.ID,
		}, nil
	case sidebarSession:
		if item.session == nil {
			return nil, errors.New("select a session to dismiss")
		}
		return dismissSessionSelectionAction{sessionID: item.session.ID}, nil
	case sidebarWorkflow:
		return dismissWorkflowSelectionAction{runID: item.workflowRunID()}, nil
	default:
		return nil, errors.New("select an item to dismiss or delete")
	}
}

type selectionCommandKind int

const (
	selectionCommandDismissDelete selectionCommandKind = iota
	selectionCommandInterruptStop
	selectionCommandKill
)

type selectionOperationKind int

const (
	selectionOperationDeleteWorkspace selectionOperationKind = iota
	selectionOperationDeleteWorktree
	selectionOperationDismissSession
	selectionOperationDismissWorkflow
	selectionOperationInterruptSession
	selectionOperationStopWorkflow
	selectionOperationKillSession
)

type selectionOperation struct {
	kind        selectionOperationKind
	label       string
	workspaceID string
	worktreeID  string
	sessionID   string
	runID       string
}

func (o selectionOperation) confirmGroup() string {
	switch o.kind {
	case selectionOperationDeleteWorkspace, selectionOperationDeleteWorktree:
		return "Delete"
	case selectionOperationDismissSession, selectionOperationDismissWorkflow:
		return "Dismiss"
	case selectionOperationInterruptSession:
		return "Interrupt"
	case selectionOperationStopWorkflow:
		return "Stop"
	case selectionOperationKillSession:
		return "Kill"
	default:
		return "Apply"
	}
}

func (o selectionOperation) execute(m *Model) tea.Cmd {
	if m == nil {
		return nil
	}
	switch o.kind {
	case selectionOperationDeleteWorkspace:
		return deleteWorkspaceCmd(m.workspaceAPI, o.workspaceID)
	case selectionOperationDeleteWorktree:
		return deleteWorktreeCmd(m.workspaceAPI, o.workspaceID, o.worktreeID)
	case selectionOperationDismissSession:
		return dismissSessionCmd(m.sessionAPI, o.sessionID)
	case selectionOperationDismissWorkflow:
		return dismissWorkflowRunCmd(m.guidedWorkflowAPI, o.runID)
	case selectionOperationInterruptSession:
		return interruptSessionCmd(m.sessionAPI, o.sessionID)
	case selectionOperationStopWorkflow:
		return stopWorkflowRunCmd(m.guidedWorkflowAPI, o.runID)
	case selectionOperationKillSession:
		return killSessionCmd(m.sessionAPI, o.sessionID)
	default:
		return nil
	}
}

func (o selectionOperation) uniqueKey() string {
	switch o.kind {
	case selectionOperationDeleteWorkspace:
		return fmt.Sprintf("delete-workspace:%s", strings.TrimSpace(o.workspaceID))
	case selectionOperationDeleteWorktree:
		return fmt.Sprintf("delete-worktree:%s:%s", strings.TrimSpace(o.workspaceID), strings.TrimSpace(o.worktreeID))
	case selectionOperationDismissSession:
		return fmt.Sprintf("dismiss-session:%s", strings.TrimSpace(o.sessionID))
	case selectionOperationDismissWorkflow:
		return fmt.Sprintf("dismiss-workflow:%s", strings.TrimSpace(o.runID))
	case selectionOperationInterruptSession:
		return fmt.Sprintf("interrupt-session:%s", strings.TrimSpace(o.sessionID))
	case selectionOperationStopWorkflow:
		return fmt.Sprintf("stop-workflow:%s", strings.TrimSpace(o.runID))
	case selectionOperationKillSession:
		return fmt.Sprintf("kill-session:%s", strings.TrimSpace(o.sessionID))
	default:
		return ""
	}
}

type SelectionOperationPlan struct {
	Command      selectionCommandKind
	Operations   []selectionOperation
	SkippedCount int
}

type SelectionOperationPlanningContext interface {
	WorkflowRunStatus(runID string) (guidedworkflows.WorkflowRunStatus, bool)
}

type SelectionOperationExecutionContext interface {
	CommandForSelectionOperation(operation selectionOperation) tea.Cmd
	SetSelectionOperationStatus(message string)
}

type SelectionOperationResolver interface {
	Resolve(command selectionCommandKind, item *sidebarItem, context SelectionOperationPlanningContext) (selectionOperation, bool)
}

type SelectionOperationPlanner interface {
	Plan(command selectionCommandKind, items []*sidebarItem, context SelectionOperationPlanningContext) (SelectionOperationPlan, error)
}

type SelectionConfirmationPresenter interface {
	ConfirmSpec(plan SelectionOperationPlan) selectionActionConfirmSpec
}

type SelectionOperationExecutor interface {
	Execute(plan SelectionOperationPlan, context SelectionOperationExecutionContext) tea.Cmd
}

type SelectionCommandProfile interface {
	Kind() selectionCommandKind
	EmptySelectionError() string
	NoActionableError() string
	ConfirmSpecBase() selectionActionConfirmSpec
	ExecutionStatus(operationCount int) string
}

type SelectionCommandProfileCatalog interface {
	Profile(kind selectionCommandKind) (SelectionCommandProfile, bool)
}

type selectionCommandProfile struct {
	kind              selectionCommandKind
	emptySelectionErr string
	noActionableErr   string
	confirmSpec       selectionActionConfirmSpec
	executionStatusFn func(operationCount int) string
}

func (p selectionCommandProfile) Kind() selectionCommandKind {
	return p.kind
}

func (p selectionCommandProfile) EmptySelectionError() string {
	return strings.TrimSpace(p.emptySelectionErr)
}

func (p selectionCommandProfile) NoActionableError() string {
	return strings.TrimSpace(p.noActionableErr)
}

func (p selectionCommandProfile) ConfirmSpecBase() selectionActionConfirmSpec {
	return p.confirmSpec
}

func (p selectionCommandProfile) ExecutionStatus(operationCount int) string {
	if p.executionStatusFn == nil {
		return fmt.Sprintf("processing %d item(s)", operationCount)
	}
	return p.executionStatusFn(operationCount)
}

type selectionCommandProfileCatalog struct {
	byKind map[selectionCommandKind]SelectionCommandProfile
}

func (c selectionCommandProfileCatalog) Profile(kind selectionCommandKind) (SelectionCommandProfile, bool) {
	if c.byKind == nil {
		return nil, false
	}
	profile, ok := c.byKind[kind]
	return profile, ok
}

type defaultSelectionOperationPlanner struct {
	resolvers []SelectionOperationResolver
	profiles  SelectionCommandProfileCatalog
}

type defaultSelectionConfirmationPresenter struct {
	profiles SelectionCommandProfileCatalog
}

type defaultSelectionOperationExecutor struct {
	profiles SelectionCommandProfileCatalog
}

type workspaceSelectionOperationResolver struct{}
type worktreeSelectionOperationResolver struct{}
type sessionSelectionOperationResolver struct{}
type workflowSelectionOperationResolver struct{}

type emptySelectionOperationPlanningContext struct{}

func (emptySelectionOperationPlanningContext) WorkflowRunStatus(string) (guidedworkflows.WorkflowRunStatus, bool) {
	return "", false
}

type modelSelectionOperationPlanningContext struct {
	model *Model
}

func (c modelSelectionOperationPlanningContext) WorkflowRunStatus(runID string) (guidedworkflows.WorkflowRunStatus, bool) {
	if c.model == nil {
		return "", false
	}
	return c.model.workflowRunStatus(strings.TrimSpace(runID))
}

type modelSelectionOperationExecutionContext struct {
	model *Model
}

func (c modelSelectionOperationExecutionContext) CommandForSelectionOperation(operation selectionOperation) tea.Cmd {
	if c.model == nil {
		return nil
	}
	switch operation.kind {
	case selectionOperationDeleteWorkspace:
		return deleteWorkspaceCmd(c.model.workspaceAPI, operation.workspaceID)
	case selectionOperationDeleteWorktree:
		return deleteWorktreeCmd(c.model.workspaceAPI, operation.workspaceID, operation.worktreeID)
	case selectionOperationDismissSession:
		return dismissSessionCmd(c.model.sessionAPI, operation.sessionID)
	case selectionOperationDismissWorkflow:
		return dismissWorkflowRunCmd(c.model.guidedWorkflowAPI, operation.runID)
	case selectionOperationInterruptSession:
		return interruptSessionCmd(c.model.sessionAPI, operation.sessionID)
	case selectionOperationStopWorkflow:
		return stopWorkflowRunCmd(c.model.guidedWorkflowAPI, operation.runID)
	case selectionOperationKillSession:
		return killSessionCmd(c.model.sessionAPI, operation.sessionID)
	default:
		return nil
	}
}

func (c modelSelectionOperationExecutionContext) SetSelectionOperationStatus(message string) {
	if c.model == nil || strings.TrimSpace(message) == "" {
		return
	}
	c.model.setStatusMessage(message)
}

type selectionBatchAction struct {
	command      selectionCommandKind
	operations   []selectionOperation
	skippedCount int
	presenter    SelectionConfirmationPresenter
	executor     SelectionOperationExecutor
}

func NewDefaultSelectionOperationPlanner() SelectionOperationPlanner {
	return defaultSelectionOperationPlanner{
		resolvers: []SelectionOperationResolver{
			workspaceSelectionOperationResolver{},
			worktreeSelectionOperationResolver{},
			sessionSelectionOperationResolver{},
			workflowSelectionOperationResolver{},
		},
		profiles: defaultSelectionCommandProfileCatalog(),
	}
}

func NewDefaultSelectionConfirmationPresenter() SelectionConfirmationPresenter {
	return defaultSelectionConfirmationPresenter{
		profiles: defaultSelectionCommandProfileCatalog(),
	}
}

func NewDefaultSelectionOperationExecutor() SelectionOperationExecutor {
	return defaultSelectionOperationExecutor{
		profiles: defaultSelectionCommandProfileCatalog(),
	}
}

func (p defaultSelectionOperationPlanner) Plan(command selectionCommandKind, items []*sidebarItem, context SelectionOperationPlanningContext) (SelectionOperationPlan, error) {
	items = uniqueSidebarItems(items)
	profile := selectionCommandProfileFor(p.profiles, command)
	if len(items) == 0 {
		return SelectionOperationPlan{}, errors.New(profile.EmptySelectionError())
	}
	if context == nil {
		context = emptySelectionOperationPlanningContext{}
	}
	operations := make([]selectionOperation, 0, len(items))
	seen := map[string]struct{}{}
	skippedCount := 0
	for _, item := range items {
		operation, ok := resolveSelectionOperationFromResolvers(p.resolvers, command, item, context)
		if !ok {
			skippedCount++
			continue
		}
		key := operation.uniqueKey()
		if key == "" {
			skippedCount++
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		operations = append(operations, operation)
	}
	if len(operations) == 0 {
		return SelectionOperationPlan{}, errors.New(profile.NoActionableError())
	}
	return SelectionOperationPlan{
		Command:      command,
		Operations:   operations,
		SkippedCount: skippedCount,
	}, nil
}

func (p defaultSelectionConfirmationPresenter) ConfirmSpec(plan SelectionOperationPlan) selectionActionConfirmSpec {
	profile := selectionCommandProfileFor(p.profiles, plan.Command)
	base := profile.ConfirmSpecBase()
	grouped := map[string][]string{}
	orderedGroups := []string{}
	for _, operation := range plan.Operations {
		group := operation.confirmGroup()
		if _, ok := grouped[group]; !ok {
			orderedGroups = append(orderedGroups, group)
		}
		grouped[group] = append(grouped[group], strings.TrimSpace(operation.label))
	}
	message := strings.TrimSpace(base.message)
	if message == "" {
		message = "Apply selection action?"
	}
	if len(orderedGroups) > 0 {
		var b strings.Builder
		b.WriteString(message)
		for _, group := range orderedGroups {
			labels := grouped[group]
			if len(labels) == 0 {
				continue
			}
			b.WriteString("\n\n")
			b.WriteString(group)
			b.WriteString(":")
			const previewLimit = 20
			previewCount := min(len(labels), previewLimit)
			for i := 0; i < previewCount; i++ {
				b.WriteString("\n- ")
				b.WriteString(labels[i])
			}
			if len(labels) > previewLimit {
				b.WriteString(fmt.Sprintf("\n- ...and %d more", len(labels)-previewLimit))
			}
		}
		if plan.SkippedCount > 0 {
			b.WriteString(fmt.Sprintf("\n\nSkipped %d selected item(s) that are not applicable.", plan.SkippedCount))
		}
		message = b.String()
	}
	base.message = message
	return base
}

func (e defaultSelectionOperationExecutor) Execute(plan SelectionOperationPlan, context SelectionOperationExecutionContext) tea.Cmd {
	if context == nil || len(plan.Operations) == 0 {
		return nil
	}
	profile := selectionCommandProfileFor(e.profiles, plan.Command)
	context.SetSelectionOperationStatus(profile.ExecutionStatus(len(plan.Operations)))
	cmds := make([]tea.Cmd, 0, len(plan.Operations))
	for _, operation := range plan.Operations {
		if cmd := context.CommandForSelectionOperation(operation); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (workspaceSelectionOperationResolver) Resolve(command selectionCommandKind, item *sidebarItem, _ SelectionOperationPlanningContext) (selectionOperation, bool) {
	if item == nil || item.kind != sidebarWorkspace || item.workspace == nil || command != selectionCommandDismissDelete {
		return selectionOperation{}, false
	}
	workspaceID := strings.TrimSpace(item.workspace.ID)
	if workspaceID == "" || workspaceID == unassignedWorkspaceID {
		return selectionOperation{}, false
	}
	return selectionOperation{
		kind:        selectionOperationDeleteWorkspace,
		label:       selectionItemLabel(item),
		workspaceID: workspaceID,
	}, true
}

func (worktreeSelectionOperationResolver) Resolve(command selectionCommandKind, item *sidebarItem, _ SelectionOperationPlanningContext) (selectionOperation, bool) {
	if item == nil || item.kind != sidebarWorktree || item.worktree == nil || command != selectionCommandDismissDelete {
		return selectionOperation{}, false
	}
	workspaceID := strings.TrimSpace(item.worktree.WorkspaceID)
	worktreeID := strings.TrimSpace(item.worktree.ID)
	if workspaceID == "" || worktreeID == "" {
		return selectionOperation{}, false
	}
	return selectionOperation{
		kind:        selectionOperationDeleteWorktree,
		label:       selectionItemLabel(item),
		workspaceID: workspaceID,
		worktreeID:  worktreeID,
	}, true
}

func (sessionSelectionOperationResolver) Resolve(command selectionCommandKind, item *sidebarItem, _ SelectionOperationPlanningContext) (selectionOperation, bool) {
	if item == nil || item.kind != sidebarSession || item.session == nil {
		return selectionOperation{}, false
	}
	sessionID := strings.TrimSpace(item.session.ID)
	if sessionID == "" {
		return selectionOperation{}, false
	}
	switch command {
	case selectionCommandDismissDelete:
		return selectionOperation{
			kind:      selectionOperationDismissSession,
			label:     selectionItemLabel(item),
			sessionID: sessionID,
		}, true
	case selectionCommandInterruptStop:
		if !isSessionInterruptible(item.session.Status) {
			return selectionOperation{}, false
		}
		return selectionOperation{
			kind:      selectionOperationInterruptSession,
			label:     selectionItemLabel(item),
			sessionID: sessionID,
		}, true
	case selectionCommandKill:
		if !isSessionKillable(item.session.Status) {
			return selectionOperation{}, false
		}
		return selectionOperation{
			kind:      selectionOperationKillSession,
			label:     selectionItemLabel(item),
			sessionID: sessionID,
		}, true
	default:
		return selectionOperation{}, false
	}
}

func (workflowSelectionOperationResolver) Resolve(command selectionCommandKind, item *sidebarItem, context SelectionOperationPlanningContext) (selectionOperation, bool) {
	if item == nil || item.kind != sidebarWorkflow {
		return selectionOperation{}, false
	}
	runID := strings.TrimSpace(item.workflowRunID())
	if runID == "" {
		return selectionOperation{}, false
	}
	switch command {
	case selectionCommandDismissDelete:
		return selectionOperation{
			kind:  selectionOperationDismissWorkflow,
			label: selectionItemLabel(item),
			runID: runID,
		}, true
	case selectionCommandInterruptStop:
		status := workflowStatusForPlanning(item, context)
		if !isWorkflowStoppable(status) {
			return selectionOperation{}, false
		}
		return selectionOperation{
			kind:  selectionOperationStopWorkflow,
			label: selectionItemLabel(item),
			runID: runID,
		}, true
	default:
		return selectionOperation{}, false
	}
}

func resolveSelectionOperationFromResolvers(resolvers []SelectionOperationResolver, command selectionCommandKind, item *sidebarItem, context SelectionOperationPlanningContext) (selectionOperation, bool) {
	for _, resolver := range resolvers {
		if resolver == nil {
			continue
		}
		if operation, ok := resolver.Resolve(command, item, context); ok {
			return operation, true
		}
	}
	return selectionOperation{}, false
}

func defaultSelectionCommandProfileCatalog() SelectionCommandProfileCatalog {
	return selectionCommandProfileCatalog{
		byKind: map[selectionCommandKind]SelectionCommandProfile{
			selectionCommandDismissDelete: selectionCommandProfile{
				kind:              selectionCommandDismissDelete,
				emptySelectionErr: "select an item to dismiss or delete",
				noActionableErr:   "selection has no dismissable or deletable items",
				confirmSpec: selectionActionConfirmSpec{
					title:       "Dismiss / Delete Selection",
					message:     "Dismiss/delete selected items?",
					confirmText: "Apply",
					cancelText:  "Cancel",
				},
				executionStatusFn: func(operationCount int) string {
					return fmt.Sprintf("processing dismiss/delete for %d item(s)", operationCount)
				},
			},
			selectionCommandInterruptStop: selectionCommandProfile{
				kind:              selectionCommandInterruptStop,
				emptySelectionErr: "select interruptible sessions or stoppable workflows",
				noActionableErr:   "selection has no interruptible or stoppable items",
				confirmSpec: selectionActionConfirmSpec{
					title:       "Interrupt / Stop Selection",
					message:     "Interrupt/stop selected items?",
					confirmText: "Apply",
					cancelText:  "Cancel",
				},
				executionStatusFn: func(operationCount int) string {
					return fmt.Sprintf("processing interrupt/stop for %d item(s)", operationCount)
				},
			},
			selectionCommandKill: selectionCommandProfile{
				kind:              selectionCommandKill,
				emptySelectionErr: "select killable sessions",
				noActionableErr:   "selection has no killable items",
				confirmSpec: selectionActionConfirmSpec{
					title:       "Kill Selection",
					message:     "Kill selected sessions?",
					confirmText: "Kill",
					cancelText:  "Cancel",
				},
				executionStatusFn: func(operationCount int) string {
					return fmt.Sprintf("processing kill for %d item(s)", operationCount)
				},
			},
		},
	}
}

func selectionCommandProfileFor(catalog SelectionCommandProfileCatalog, kind selectionCommandKind) SelectionCommandProfile {
	if catalog != nil {
		if profile, ok := catalog.Profile(kind); ok && profile != nil {
			return profile
		}
	}
	return selectionCommandProfile{
		kind:              kind,
		emptySelectionErr: "select an item to act on",
		noActionableErr:   "selection has no actionable items",
		confirmSpec: selectionActionConfirmSpec{
			title:       "Apply Selection Action",
			message:     "Apply selection action?",
			confirmText: "Apply",
			cancelText:  "Cancel",
		},
	}
}

func (a selectionBatchAction) plan() SelectionOperationPlan {
	return SelectionOperationPlan{
		Command:      a.command,
		Operations:   append([]selectionOperation(nil), a.operations...),
		SkippedCount: a.skippedCount,
	}
}

func (a selectionBatchAction) Validate(_ *Model) error {
	if len(a.operations) == 0 {
		return errors.New("selection has no actionable items")
	}
	return nil
}

func (a selectionBatchAction) ConfirmSpec(_ *Model) selectionActionConfirmSpec {
	presenter := a.presenter
	if presenter == nil {
		presenter = NewDefaultSelectionConfirmationPresenter()
	}
	return presenter.ConfirmSpec(a.plan())
}

func (a selectionBatchAction) Execute(m *Model) tea.Cmd {
	executor := a.executor
	if executor == nil {
		executor = NewDefaultSelectionOperationExecutor()
	}
	return executor.Execute(a.plan(), modelSelectionOperationExecutionContext{model: m})
}

func resolveDismissOrDeleteSelectionActionForItems(items []*sidebarItem) (selectionAction, error) {
	return resolveSelectionActionForCommand(nil, selectionCommandDismissDelete, items)
}

func resolveInterruptOrStopSelectionAction(m *Model, items []*sidebarItem) (selectionAction, error) {
	return resolveSelectionActionForCommand(m, selectionCommandInterruptStop, items)
}

func resolveKillSelectionAction(m *Model, items []*sidebarItem) (selectionAction, error) {
	return resolveSelectionActionForCommand(m, selectionCommandKill, items)
}

func resolveSelectionActionForCommand(m *Model, command selectionCommandKind, items []*sidebarItem) (selectionAction, error) {
	items = uniqueSidebarItems(items)
	if command == selectionCommandDismissDelete && len(items) == 1 {
		return resolveDismissOrDeleteSelectionAction(items[0])
	}
	planner := selectionOperationPlannerForModel(m)
	plan, err := planner.Plan(command, items, modelSelectionOperationPlanningContext{model: m})
	if err != nil {
		return nil, err
	}
	return selectionBatchAction{
		command:      plan.Command,
		operations:   plan.Operations,
		skippedCount: plan.SkippedCount,
		presenter:    selectionConfirmationPresenterForModel(m),
		executor:     selectionOperationExecutorForModel(m),
	}, nil
}

func selectionItemLabel(item *sidebarItem) string {
	if item == nil {
		return "Unknown"
	}
	name := strings.TrimSpace(item.Title())
	switch item.kind {
	case sidebarWorkspace:
		if name == "" && item.workspace != nil {
			name = strings.TrimSpace(item.workspace.ID)
		}
		if name == "" {
			name = "Workspace"
		}
		return fmt.Sprintf("Workspace %q", name)
	case sidebarWorktree:
		if name == "" && item.worktree != nil {
			name = strings.TrimSpace(item.worktree.ID)
		}
		if name == "" {
			name = "Worktree"
		}
		return fmt.Sprintf("Worktree %q", name)
	case sidebarSession:
		if name == "" && item.session != nil {
			name = strings.TrimSpace(item.session.ID)
		}
		if name == "" {
			name = "Session"
		}
		return fmt.Sprintf("Session %q", name)
	case sidebarWorkflow:
		if name == "" {
			name = strings.TrimSpace(item.workflowRunID())
		}
		if name == "" {
			name = "Guided Workflow"
		}
		return fmt.Sprintf("Workflow %q", name)
	default:
		if name == "" {
			name = "Item"
		}
		return fmt.Sprintf("Item %q", name)
	}
}

func uniqueSidebarItems(items []*sidebarItem) []*sidebarItem {
	if len(items) <= 1 {
		return items
	}
	out := make([]*sidebarItem, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if item == nil {
			continue
		}
		key := strings.TrimSpace(item.key())
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func workflowStatusForPlanning(item *sidebarItem, context SelectionOperationPlanningContext) guidedworkflows.WorkflowRunStatus {
	if item == nil {
		return ""
	}
	if item.workflow != nil && strings.TrimSpace(string(item.workflow.Status)) != "" {
		return item.workflow.Status
	}
	if context == nil {
		return ""
	}
	status, ok := context.WorkflowRunStatus(strings.TrimSpace(item.workflowRunID()))
	if !ok {
		return ""
	}
	return status
}

func selectionOperationPlannerForModel(m *Model) SelectionOperationPlanner {
	if m == nil || m.selectionOperationPlanner == nil {
		return NewDefaultSelectionOperationPlanner()
	}
	return m.selectionOperationPlanner
}

func selectionConfirmationPresenterForModel(m *Model) SelectionConfirmationPresenter {
	if m == nil || m.selectionConfirmationPresenter == nil {
		return NewDefaultSelectionConfirmationPresenter()
	}
	return m.selectionConfirmationPresenter
}

func selectionOperationExecutorForModel(m *Model) SelectionOperationExecutor {
	if m == nil || m.selectionOperationExecutor == nil {
		return NewDefaultSelectionOperationExecutor()
	}
	return m.selectionOperationExecutor
}

func WithSelectionOperationPlanner(planner SelectionOperationPlanner) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if planner == nil {
			m.selectionOperationPlanner = NewDefaultSelectionOperationPlanner()
			return
		}
		m.selectionOperationPlanner = planner
	}
}

func WithSelectionConfirmationPresenter(presenter SelectionConfirmationPresenter) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if presenter == nil {
			m.selectionConfirmationPresenter = NewDefaultSelectionConfirmationPresenter()
			return
		}
		m.selectionConfirmationPresenter = presenter
	}
}

func WithSelectionOperationExecutor(executor SelectionOperationExecutor) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if executor == nil {
			m.selectionOperationExecutor = NewDefaultSelectionOperationExecutor()
			return
		}
		m.selectionOperationExecutor = executor
	}
}

func isWorkflowStoppable(status guidedworkflows.WorkflowRunStatus) bool {
	switch status {
	case guidedworkflows.WorkflowRunStatusStopped,
		guidedworkflows.WorkflowRunStatusCompleted,
		guidedworkflows.WorkflowRunStatusFailed:
		return false
	default:
		return true
	}
}

func isSessionInterruptible(status types.SessionStatus) bool {
	switch status {
	case types.SessionStatusExited,
		types.SessionStatusFailed,
		types.SessionStatusKilled,
		types.SessionStatusOrphaned:
		return false
	default:
		return true
	}
}

func isSessionKillable(status types.SessionStatus) bool {
	return isSessionInterruptible(status)
}

func (m *Model) openSelectionActionConfirm(action selectionAction) {
	if m == nil {
		return
	}
	if action == nil {
		m.setValidationStatus("select an item to act on")
		return
	}
	if m.confirm == nil {
		return
	}
	if err := action.Validate(m); err != nil {
		m.setValidationStatus(err.Error())
		return
	}
	spec := action.ConfirmSpec(m)
	if strings.TrimSpace(spec.title) == "" {
		spec.title = "Confirm"
	}
	if strings.TrimSpace(spec.message) == "" {
		spec.message = "Are you sure?"
	}
	if strings.TrimSpace(spec.confirmText) == "" {
		spec.confirmText = "Confirm"
	}
	if strings.TrimSpace(spec.cancelText) == "" {
		spec.cancelText = "Cancel"
	}
	m.pendingSelectionAction = action
	m.pendingConfirm = confirmAction{}
	if m.menu != nil {
		m.menu.CloseAll()
	}
	if m.contextMenu != nil {
		m.contextMenu.Close()
	}
	m.confirm.Open(spec.title, spec.message, spec.confirmText, spec.cancelText)
}
