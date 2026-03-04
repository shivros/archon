package app

import (
	"strconv"
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

func TestSidebarControllerScrollingEnabledViewOnly(t *testing.T) {
	controller := NewSidebarController()
	controller.SetSize(32, 6)
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions, meta := makeSidebarSessionFixtures("ws1", 20)
	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, "ws1", "", false)
	if !controller.SelectBySessionID("s20") {
		t.Fatalf("expected to select session s20")
	}

	if got := controller.ScrollbarWidth(); got != 1 {
		t.Fatalf("expected sidebar scrollbar width when scrolling is enabled, got %d", got)
	}
	header := controller.headerRows()
	beforeTop := controller.ItemAtRow(header)
	if beforeTop == nil {
		t.Fatalf("expected a visible sidebar row")
	}
	selectedBefore := controller.SelectedSessionID()
	if !controller.Scroll(2) {
		t.Fatalf("expected sidebar scroll to adjust viewport")
	}
	if got := controller.SelectedSessionID(); got != selectedBefore {
		t.Fatalf("expected sidebar scroll to preserve selection, got %q want %q", got, selectedBefore)
	}
	afterTop := controller.ItemAtRow(header)
	if afterTop == nil {
		t.Fatalf("expected visible row after scroll")
	}
	if afterTop.key() == beforeTop.key() {
		t.Fatalf("expected sidebar scroll to move viewport")
	}
}

func TestSidebarControllerScrollbarSelectMovesViewportOnly(t *testing.T) {
	controller := NewSidebarController()
	controller.SetSize(32, 6)
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions, meta := makeSidebarSessionFixtures("ws1", 24)
	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, "ws1", "", false)
	if !controller.SelectBySessionID("s24") {
		t.Fatalf("expected to select session s24")
	}
	header := controller.headerRows()
	beforeTop := controller.ItemAtRow(header)
	if beforeTop == nil {
		t.Fatalf("expected a visible sidebar row before scrollbar selection")
	}
	selectedBefore := controller.SelectedSessionID()
	targetRow := header + max(1, controller.list.Height()-header-1)
	if !controller.ScrollbarSelect(targetRow) {
		t.Fatalf("expected scrollbar select to adjust viewport")
	}
	if got := controller.SelectedSessionID(); got != selectedBefore {
		t.Fatalf("expected scrollbar selection to preserve session, got %q want %q", got, selectedBefore)
	}
	afterTop := controller.ItemAtRow(header)
	if afterTop == nil {
		t.Fatalf("expected visible row after scrollbar selection")
	}
	if afterTop.key() == beforeTop.key() {
		t.Fatalf("expected scrollbar selection to move viewport")
	}
}

func TestSidebarControllerUnreadSessions(t *testing.T) {
	controller := NewSidebarController()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "s2", Status: types.SessionStatusRunning, CreatedAt: now.Add(-1 * time.Minute)},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", LastTurnID: "turn-2"},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, "ws1", "", false)
	if controller.delegate.isUnread("s1") || controller.delegate.isUnread("s2") {
		t.Fatalf("did not expect unread sessions on initial load")
	}
	if !controller.SelectBySessionID("s1") {
		t.Fatalf("expected to select session s1")
	}

	updatedMeta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", LastTurnID: "turn-3"},
	}
	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, updatedMeta, "ws1", "", false)
	if !controller.delegate.isUnread("s2") {
		t.Fatalf("expected session s2 to be unread after new activity")
	}

	if !controller.SelectBySessionID("s2") {
		t.Fatalf("expected to select session s2")
	}
	if controller.delegate.isUnread("s2") {
		t.Fatalf("expected session s2 unread flag to clear after selection")
	}
}

func TestSidebarControllerToggleFocusedSelection(t *testing.T) {
	controller := NewSidebarController()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
		{ID: "s2", Status: types.SessionStatusRunning, CreatedAt: now.Add(-time.Minute)},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1"},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, "ws1", "", false)
	if !controller.SelectBySessionID("s1") {
		t.Fatalf("expected to select s1")
	}
	if !controller.ToggleFocusedSelection() {
		t.Fatalf("expected toggle for focused selection")
	}
	if !controller.SelectBySessionID("s2") {
		t.Fatalf("expected to select s2")
	}
	if !controller.ToggleFocusedSelection() {
		t.Fatalf("expected second toggle for focused selection")
	}
	selected := controller.SelectedItems()
	if len(selected) != 2 {
		t.Fatalf("expected two selected sidebar items, got %d", len(selected))
	}
	if !controller.ClearSelectedKeys() {
		t.Fatalf("expected clear selected keys to return true")
	}
	if len(controller.SelectedItems()) != 0 {
		t.Fatalf("expected no selected items after clear")
	}
}

func TestSidebarControllerSelectedKeysAndIsKeySelected(t *testing.T) {
	controller := NewSidebarController()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
		{ID: "s2", Status: types.SessionStatusRunning, CreatedAt: now.Add(-time.Minute)},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1"},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, "ws1", "", false)
	if !controller.SelectBySessionID("s1") || !controller.ToggleFocusedSelection() {
		t.Fatalf("expected s1 to be included in multi-select")
	}
	if !controller.SelectBySessionID("s2") || !controller.ToggleFocusedSelection() {
		t.Fatalf("expected s2 to be included in multi-select")
	}

	keys := controller.SelectedKeys()
	if len(keys) != 2 {
		t.Fatalf("expected two selected keys, got %d", len(keys))
	}
	if !controller.IsKeySelected("sess:s1") {
		t.Fatalf("expected sess:s1 to be selected")
	}
	if !controller.IsKeySelected("sess:s2") {
		t.Fatalf("expected sess:s2 to be selected")
	}
	if controller.IsKeySelected("sess:missing") {
		t.Fatalf("did not expect sess:missing to be selected")
	}

	keys[0] = "mutated"
	fresh := controller.SelectedKeys()
	for _, key := range fresh {
		if key == "mutated" {
			t.Fatalf("expected SelectedKeys to return a defensive copy")
		}
	}
}

func TestSidebarControllerPrunesSelectedKeysOnApply(t *testing.T) {
	controller := NewSidebarController()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	initialSessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
		{ID: "s2", Status: types.SessionStatusRunning, CreatedAt: now.Add(-time.Minute)},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1"},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, initialSessions, nil, meta, "ws1", "", false)
	if !controller.SelectBySessionID("s1") || !controller.ToggleFocusedSelection() {
		t.Fatalf("expected to select s1 into multi-select")
	}
	if !controller.SelectBySessionID("s2") || !controller.ToggleFocusedSelection() {
		t.Fatalf("expected to select s2 into multi-select")
	}
	remaining := []*types.Session{{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now}}
	controller.Apply(workspaces, map[string][]*types.Worktree{}, remaining, nil, map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}, "ws1", "", false)
	selected := controller.SelectedItems()
	if len(selected) != 1 {
		t.Fatalf("expected pruned selection length 1, got %d", len(selected))
	}
	if selected[0].session == nil || selected[0].session.ID != "s1" {
		t.Fatalf("expected s1 to remain selected, got %#v", selected[0])
	}
}

func TestSidebarControllerToggleSelectedContainer(t *testing.T) {
	controller := NewSidebarController()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, "ws1", "", false)
	if item := controller.SelectedItem(); item == nil || item.kind != sidebarSession {
		t.Fatalf("expected session to be selected by default")
	}
	controller.Select(0)
	if item := controller.SelectedItem(); item == nil || item.kind != sidebarWorkspace {
		t.Fatalf("expected workspace to be selected")
	}
	if !controller.ToggleSelectedContainer() {
		t.Fatalf("expected workspace toggle to change expansion state")
	}
	items := controller.Items()
	if len(items) != 1 {
		t.Fatalf("expected collapsed workspace to hide session, got %d rows", len(items))
	}
	if got := controller.SelectedItem(); got == nil || got.kind != sidebarWorkspace {
		t.Fatalf("expected workspace row to remain selected")
	}
}

func TestSidebarControllerSetAllWorkspacesExpanded(t *testing.T) {
	controller := NewSidebarController()
	controller.SetExpandByDefault(false)
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace A"},
		{ID: "ws2", Name: "Workspace B"},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
		{ID: "s2", Status: types.SessionStatusRunning, CreatedAt: now.Add(-time.Minute)},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws2"},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, "", "", false)
	if !controller.SetAllWorkspacesExpanded(false) {
		t.Fatalf("expected bulk workspace collapse to change state")
	}
	if controller.IsWorkspaceExpanded("ws1") || controller.IsWorkspaceExpanded("ws2") {
		t.Fatalf("expected all workspaces collapsed")
	}
	if got := len(controller.Items()); got != 2 {
		t.Fatalf("expected only workspace rows when collapsed, got %d", got)
	}
	if !controller.SetAllWorkspacesExpanded(true) {
		t.Fatalf("expected bulk workspace expand to change state")
	}
	if !controller.IsWorkspaceExpanded("ws1") || !controller.IsWorkspaceExpanded("ws2") {
		t.Fatalf("expected all workspaces expanded")
	}
	if got := len(controller.Items()); got != 4 {
		t.Fatalf("expected workspace and session rows when expanded, got %d", got)
	}
	if controller.SetAllWorkspacesExpanded(true) {
		t.Fatalf("expected no-op all-workspace expand to return false")
	}
}

func TestSidebarControllerSetAllWorkspacesExpandedNilController(t *testing.T) {
	var controller *SidebarController
	if controller.SetAllWorkspacesExpanded(true) {
		t.Fatalf("expected nil controller to return false")
	}
}

func TestSidebarControllerSetWorktreesExpandedForWorktree(t *testing.T) {
	controller := NewSidebarController()
	controller.SetExpandByDefault(false)
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace A"},
		{ID: "ws2", Name: "Workspace B"},
	}
	worktrees := map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Feature 1"},
			{ID: "wt2", WorkspaceID: "ws1", Name: "Feature 2"},
		},
		"ws2": {
			{ID: "wt3", WorkspaceID: "ws2", Name: "Feature 3"},
		},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
		{ID: "s2", Status: types.SessionStatusRunning, CreatedAt: now.Add(-time.Minute)},
		{ID: "s3", Status: types.SessionStatusRunning, CreatedAt: now.Add(-2 * time.Minute)},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", WorktreeID: "wt2"},
		"s3": {SessionID: "s3", WorkspaceID: "ws2", WorktreeID: "wt3"},
	}

	controller.Apply(workspaces, worktrees, sessions, nil, meta, "", "", false)
	if !controller.SetAllWorkspacesExpanded(true) {
		t.Fatalf("expected workspaces to expand for scoped worktree test")
	}
	if !controller.SetWorktreeExpanded("wt1", true) || !controller.SetWorktreeExpanded("wt2", true) || !controller.SetWorktreeExpanded("wt3", true) {
		t.Fatalf("expected baseline worktree expansion setup")
	}
	if !controller.SetWorktreesExpandedForWorktree("wt1", false) {
		t.Fatalf("expected bulk worktree collapse to change state")
	}
	if controller.IsWorktreeExpanded("wt1") || controller.IsWorktreeExpanded("wt2") {
		t.Fatalf("expected ws1 worktrees collapsed")
	}
	if !controller.IsWorktreeExpanded("wt3") {
		t.Fatalf("expected ws2 worktree to remain expanded")
	}
	if !controller.SetWorktreesExpandedForWorktree("wt1", true) {
		t.Fatalf("expected bulk worktree expand to change state")
	}
	if !controller.IsWorktreeExpanded("wt1") || !controller.IsWorktreeExpanded("wt2") {
		t.Fatalf("expected ws1 worktrees expanded")
	}
	if !controller.IsWorktreeExpanded("wt3") {
		t.Fatalf("expected ws2 worktree to remain expanded")
	}
}

func TestSidebarControllerSetWorktreesExpandedForWorktreeRejectsUnknown(t *testing.T) {
	controller := NewSidebarController()
	if controller.SetWorktreesExpandedForWorktree("", true) {
		t.Fatalf("expected blank worktree id to be rejected")
	}
	controller.Apply([]*types.Workspace{{ID: "ws1", Name: "Workspace"}}, map[string][]*types.Worktree{
		"ws1": {{ID: "wt1", WorkspaceID: "ws1", Name: "Feature"}},
	}, nil, nil, nil, "", "", false)
	if controller.SetWorktreesExpandedForWorktree("wt-missing", true) {
		t.Fatalf("expected unknown worktree id to be rejected")
	}
}

func TestSidebarControllerSelectBySessionIDAutoExpandsParents(t *testing.T) {
	controller := NewSidebarController()
	controller.SetExpandByDefault(false)
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	worktrees := map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree"},
		},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1"},
	}

	controller.Apply(workspaces, worktrees, sessions, nil, meta, "ws1", "wt1", false)
	if len(controller.Items()) != 1 {
		t.Fatalf("expected only workspace row when default collapsed")
	}
	if !controller.SelectBySessionID("s1") {
		t.Fatalf("expected SelectBySessionID to auto-expand and select s1")
	}
	if got := controller.SelectedSessionID(); got != "s1" {
		t.Fatalf("expected selected session s1, got %q", got)
	}
	if !controller.IsWorkspaceExpanded("ws1") {
		t.Fatalf("expected workspace ws1 to be expanded")
	}
	if !controller.IsWorktreeExpanded("wt1") {
		t.Fatalf("expected worktree wt1 to be expanded")
	}
}
func TestSidebarControllerSelectByKeySessionAutoExpandsParents(t *testing.T) {
	controller := NewSidebarController()
	controller.SetExpandByDefault(false)
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	worktrees := map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree"},
		},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1"},
	}

	controller.Apply(workspaces, worktrees, sessions, nil, meta, "ws1", "wt1", false)
	if !controller.SelectByKey("sess:s1") {
		t.Fatalf("expected SelectByKey(sess:s1) to select hidden session")
	}
	if got := controller.SelectedSessionID(); got != "s1" {
		t.Fatalf("expected selected session s1, got %q", got)
	}
}

func TestSidebarControllerSelectByKeyWorktreeAutoExpandsWorkspace(t *testing.T) {
	controller := NewSidebarController()
	controller.SetExpandByDefault(false)
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	worktrees := map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree"},
		},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1"},
	}

	controller.Apply(workspaces, worktrees, sessions, nil, meta, "ws1", "wt1", false)
	if !controller.CanSelectKey("wt:wt1") {
		t.Fatalf("expected CanSelectKey(wt:wt1) to be true")
	}
	if !controller.SelectByKey("wt:wt1") {
		t.Fatalf("expected SelectByKey(wt:wt1) to select hidden worktree")
	}
	item := controller.SelectedItem()
	if item == nil || item.kind != sidebarWorktree || item.worktree == nil || item.worktree.ID != "wt1" {
		t.Fatalf("expected selected worktree wt1, got %#v", item)
	}
}
func makeSidebarSessionFixtures(workspaceID string, count int) ([]*types.Session, map[string]*types.SessionMeta) {
	now := time.Now().UTC()
	sessions := make([]*types.Session, 0, count)
	meta := make(map[string]*types.SessionMeta, count)
	for i := 1; i <= count; i++ {
		id := "s" + strconv.Itoa(i)
		sessions = append(sessions, &types.Session{
			ID:        id,
			Status:    types.SessionStatusRunning,
			CreatedAt: now.Add(time.Duration(-count+i) * time.Minute),
		})
		meta[id] = &types.SessionMeta{
			SessionID:   id,
			WorkspaceID: workspaceID,
			LastTurnID:  "turn-" + strconv.Itoa(i),
		}
	}
	return sessions, meta
}

func TestSidebarControllerSelectBySessionIDAutoExpandsWorkflowParent(t *testing.T) {
	controller := NewSidebarController()
	controller.SetExpandByDefault(false)
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: "gwf-1"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, workflows, meta, "ws1", "", false)
	if len(controller.Items()) != 1 {
		t.Fatalf("expected only workspace row when default collapsed")
	}
	if !controller.SelectBySessionID("s1") {
		t.Fatalf("expected SelectBySessionID to auto-expand and select s1")
	}
	if got := controller.SelectedSessionID(); got != "s1" {
		t.Fatalf("expected selected session s1, got %q", got)
	}
	if !controller.IsWorkflowExpanded("gwf-1") {
		t.Fatalf("expected workflow gwf-1 to be expanded")
	}
}

func TestSidebarControllerSelectByWorkflowIDAutoExpandsParent(t *testing.T) {
	controller := NewSidebarController()
	controller.SetExpandByDefault(false)
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, nil, workflows, map[string]*types.SessionMeta{}, "ws1", "", false)
	if len(controller.Items()) != 1 {
		t.Fatalf("expected only workspace row when default collapsed")
	}
	if !controller.SelectByWorkflowID("gwf-1") {
		t.Fatalf("expected SelectByWorkflowID to auto-expand and select workflow")
	}
	item := controller.SelectedItem()
	if item == nil || item.kind != sidebarWorkflow || item.workflowRunID() != "gwf-1" {
		t.Fatalf("expected workflow gwf-1 selected, got %#v", item)
	}
	if !controller.IsWorkspaceExpanded("ws1") {
		t.Fatalf("expected workspace ws1 expanded")
	}
}

func TestSidebarControllerBackgroundApplyKeepsSessionSelectedWhenNestingUnderWorkflow(t *testing.T) {
	controller := NewSidebarController()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	baseMeta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}

	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, baseMeta, "ws1", "", false)
	if !controller.SelectBySessionID("s1") {
		t.Fatalf("expected s1 selected before workflow nesting")
	}

	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning, CreatedAt: now},
	}
	controller.SetWorkflowExpanded("gwf-1", false)
	nestedMeta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: "gwf-1"},
	}
	item := controller.ApplyWithReason(
		workspaces,
		map[string][]*types.Worktree{},
		sessions,
		workflows,
		nestedMeta,
		"ws1",
		"",
		false,
		sidebarApplyReasonBackground,
	)
	if item == nil || !item.isSession() || item.session == nil || item.session.ID != "s1" {
		t.Fatalf("expected session s1 to stay selected, got %#v", item)
	}
	if got := controller.SelectedSessionID(); got != "s1" {
		t.Fatalf("expected selected session s1, got %q", got)
	}
	if !controller.IsWorkflowExpanded("gwf-1") {
		t.Fatalf("expected workflow gwf-1 to auto-expand to preserve selection")
	}
}

func TestSidebarControllerApplySelectionDecisionNilControllerNoPanic(t *testing.T) {
	var controller *SidebarController
	controller.applySelectionDecision(sidebarSelectionDecision{Candidates: []string{"sess:s1"}})
}

func TestSidebarControllerApplySelectionDecisionNoCandidatesKeepsSelection(t *testing.T) {
	controller := NewSidebarController()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions := []*types.Session{{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now}}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, "ws1", "", false)
	if !controller.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	before := controller.SelectedKey()
	controller.applySelectionDecision(sidebarSelectionDecision{})
	if got := controller.SelectedKey(); got != before {
		t.Fatalf("expected selection unchanged, got %q want %q", got, before)
	}
}

func TestSidebarControllerApplySelectionDecisionFallsBackToFirstVisibleItem(t *testing.T) {
	controller := NewSidebarController()
	now := time.Now().UTC()
	workspaces := []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	sessions := []*types.Session{{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now}}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	controller.Apply(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, "ws1", "", false)
	if !controller.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	controller.applySelectionDecision(sidebarSelectionDecision{Candidates: []string{"sess:missing"}})
	item := controller.SelectedItem()
	if item == nil || item.kind != sidebarWorkspace || item.workspace == nil || item.workspace.ID != "ws1" {
		t.Fatalf("expected fallback to first visible workspace row, got %#v", item)
	}
}
