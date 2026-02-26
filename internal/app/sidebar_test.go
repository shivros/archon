package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

type testSessionLayoutEngine struct {
	title string
	right string
}

func (e testSessionLayoutEngine) Layout(_, _ string, _ int) (string, string) {
	return e.title, e.right
}

func TestBuildSidebarItemsGroupsSessions(t *testing.T) {
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
		{ID: "ws2", Name: "Workspace Two"},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now.Add(-5 * time.Minute)},
		{ID: "s2", Status: types.SessionStatusExited, CreatedAt: now.Add(-4 * time.Minute)},
		{ID: "s3", Status: types.SessionStatusRunning, CreatedAt: now.Add(-3 * time.Minute)},
		{ID: "s4", Status: types.SessionStatusRunning, CreatedAt: now.Add(-2 * time.Minute)},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s3": {SessionID: "s3", WorkspaceID: "ws2"},
		"s4": {SessionID: "s4", WorkspaceID: "missing"},
	}

	items := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, false)
	if len(items) != 7 {
		t.Fatalf("expected 7 items, got %d", len(items))
	}

	ws1 := items[0].(*sidebarItem)
	if ws1.kind != sidebarWorkspace || ws1.workspace.ID != "ws1" || ws1.sessionCount != 1 {
		t.Fatalf("expected ws1 workspace item with 1 session")
	}
	s1 := items[1].(*sidebarItem)
	if !s1.isSession() || s1.session.ID != "s1" {
		t.Fatalf("expected s1 session under ws1")
	}
	ws2 := items[2].(*sidebarItem)
	if ws2.kind != sidebarWorkspace || ws2.workspace.ID != "ws2" || ws2.sessionCount != 1 {
		t.Fatalf("expected ws2 workspace item with 1 session")
	}
	s3 := items[3].(*sidebarItem)
	if !s3.isSession() || s3.session.ID != "s3" {
		t.Fatalf("expected s3 session under ws2")
	}
	wsUnassigned := items[4].(*sidebarItem)
	if wsUnassigned.kind != sidebarWorkspace || wsUnassigned.workspace.Name != unassignedWorkspaceTag {
		t.Fatalf("expected unassigned workspace")
	}
	s4 := items[5].(*sidebarItem)
	if !s4.isSession() || s4.session.ID != "s4" {
		t.Fatalf("expected s4 session under unassigned")
	}
	s2 := items[6].(*sidebarItem)
	if !s2.isSession() || s2.session.ID != "s2" {
		t.Fatalf("expected exited session s2 under unassigned")
	}
}

func TestSessionTitlePriority(t *testing.T) {
	session := &types.Session{ID: "s1", Title: "fallback"}
	meta := &types.SessionMeta{
		SessionID:    "s1",
		Title:        "from meta",
		InitialInput: "from input",
	}
	if got := sessionTitle(session, meta); got != "from meta" {
		t.Fatalf("expected meta title, got %q", got)
	}
	meta.Title = ""
	if got := sessionTitle(session, meta); got != "from input" {
		t.Fatalf("expected initial input title, got %q", got)
	}
	meta.InitialInput = ""
	if got := sessionTitle(session, meta); got != "fallback" {
		t.Fatalf("expected session title, got %q", got)
	}
	session.Title = ""
	if got := sessionTitle(session, nil); got != "s1" {
		t.Fatalf("expected session id fallback, got %q", got)
	}
}

func TestSelectSidebarIndexPrefersSelectionAndActiveWorkspace(t *testing.T) {
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
		{ID: "ws2", Name: "Workspace Two"},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning},
		{ID: "s2", Status: types.SessionStatusRunning},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws2"},
	}
	items := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, false)

	selected := "sess:s1"
	if idx := selectSidebarIndex(items, selected, "ws2", ""); idx != 1 {
		t.Fatalf("expected selected session index 1, got %d", idx)
	}

	if idx := selectSidebarIndex(items, "", "ws2", ""); idx != 3 {
		t.Fatalf("expected ws2 session index 3, got %d", idx)
	}
}

func TestBuildSidebarItemsShowDismissedToggle(t *testing.T) {
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
	}
	sessions := []*types.Session{
		{ID: "active", Status: types.SessionStatusInactive, CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "dismissed", Status: types.SessionStatusExited, CreatedAt: now.Add(-1 * time.Minute)},
	}
	dismissedAt := now.Add(-30 * time.Second)
	meta := map[string]*types.SessionMeta{
		"active":    {SessionID: "active", WorkspaceID: "ws1"},
		"dismissed": {SessionID: "dismissed", WorkspaceID: "ws1", DismissedAt: &dismissedAt},
	}

	hidden := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, false)
	if len(hidden) != 2 {
		t.Fatalf("expected workspace + active session when dismissed hidden, got %d", len(hidden))
	}

	visible := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, nil, meta, true)
	if len(visible) != 3 {
		t.Fatalf("expected workspace + both sessions when dismissed shown, got %d", len(visible))
	}
}

func TestResolveProviderBadgeUsesDefaults(t *testing.T) {
	codex := resolveProviderBadge("codex", nil)
	if codex.Prefix != "[CDX]" {
		t.Fatalf("expected codex prefix [CDX], got %q", codex.Prefix)
	}
	if codex.Color != "15" {
		t.Fatalf("expected codex color 15, got %q", codex.Color)
	}

	claude := resolveProviderBadge("claude", nil)
	if claude.Prefix != "[CLD]" {
		t.Fatalf("expected claude prefix [CLD], got %q", claude.Prefix)
	}
	if claude.Color != "208" {
		t.Fatalf("expected claude color 208, got %q", claude.Color)
	}

	kilocode := resolveProviderBadge("kilocode", nil)
	if kilocode.Prefix != "[KLO]" {
		t.Fatalf("expected kilocode prefix [KLO], got %q", kilocode.Prefix)
	}
	if kilocode.Color != "226" {
		t.Fatalf("expected kilocode color 226, got %q", kilocode.Color)
	}
}

func TestResolveProviderBadgeAppliesOverrides(t *testing.T) {
	overrides := normalizeProviderBadgeOverrides(map[string]*types.ProviderBadgeConfig{
		" CoDeX ": {
			Prefix: " [GPT] ",
			Color:  " 231 ",
		},
	})
	resolved := resolveProviderBadge("codex", overrides)
	if resolved.Prefix != "[GPT]" {
		t.Fatalf("expected override prefix [GPT], got %q", resolved.Prefix)
	}
	if resolved.Color != "231" {
		t.Fatalf("expected override color 231, got %q", resolved.Color)
	}
}

func TestResolveProviderBadgeUnknownProviderFallback(t *testing.T) {
	resolved := resolveProviderBadge("open code", nil)
	if resolved.Prefix != "[OPE]" {
		t.Fatalf("expected fallback prefix [OPE], got %q", resolved.Prefix)
	}
	if resolved.Color != defaultBadgeColor {
		t.Fatalf("expected fallback color %q, got %q", defaultBadgeColor, resolved.Color)
	}
}

func TestBuildSidebarItemsCollapsedWorkspaceHidesNestedItems(t *testing.T) {
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
	}
	worktrees := map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree One"},
		},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning},
		{ID: "s2", Status: types.SessionStatusRunning},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", WorktreeID: "wt1"},
	}

	items := buildSidebarItemsWithExpansion(workspaces, worktrees, sessions, nil, meta, false, sidebarExpansionResolver{
		workspace: map[string]bool{"ws1": false},
		defaultOn: true,
	})
	if len(items) != 1 {
		t.Fatalf("expected only workspace row when collapsed, got %d", len(items))
	}
	ws := items[0].(*sidebarItem)
	if ws.kind != sidebarWorkspace || ws.workspace == nil || ws.workspace.ID != "ws1" {
		t.Fatalf("expected workspace ws1 row")
	}
	if !ws.collapsible || ws.expanded {
		t.Fatalf("expected workspace ws1 collapsible and collapsed")
	}
}

func TestBuildSidebarItemsCollapsedWorktreeHidesWorktreeSessions(t *testing.T) {
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
	}
	worktrees := map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree One"},
		},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning},
		{ID: "s2", Status: types.SessionStatusRunning},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", WorktreeID: "wt1"},
	}

	items := buildSidebarItemsWithExpansion(workspaces, worktrees, sessions, nil, meta, false, sidebarExpansionResolver{
		workspace: map[string]bool{"ws1": true},
		worktree:  map[string]bool{"wt1": false},
		defaultOn: true,
	})
	if len(items) != 3 {
		t.Fatalf("expected workspace + direct session + collapsed worktree, got %d", len(items))
	}
	if items[0].(*sidebarItem).kind != sidebarWorkspace {
		t.Fatalf("expected first item workspace")
	}
	directSession := items[1].(*sidebarItem)
	if directSession.kind != sidebarSession || directSession.session == nil || directSession.session.ID != "s1" {
		t.Fatalf("expected direct workspace session s1")
	}
	wt := items[2].(*sidebarItem)
	if wt.kind != sidebarWorktree || wt.worktree == nil || wt.worktree.ID != "wt1" {
		t.Fatalf("expected collapsed worktree row wt1")
	}
	if !wt.collapsible || wt.expanded {
		t.Fatalf("expected worktree wt1 collapsible and collapsed")
	}
}

func TestBuildSidebarItemsNestsWorkflowOwnedSessionsWithoutDuplication(t *testing.T) {
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
	}
	sessions := []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning, CreatedAt: now.Add(-time.Minute)},
		{ID: "s2", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: "gwf-1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", TemplateName: "SOLID Phase Delivery", WorkspaceID: "ws1", Status: guidedworkflows.WorkflowRunStatusRunning},
	}

	items := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, sessions, workflows, meta, false)
	if len(items) != 4 {
		t.Fatalf("expected workspace + workflow + workflow child session + normal session, got %d", len(items))
	}
	if items[0].(*sidebarItem).kind != sidebarWorkspace {
		t.Fatalf("expected workspace row first")
	}
	if items[0].(*sidebarItem).depth != 0 {
		t.Fatalf("expected workspace depth 0, got %d", items[0].(*sidebarItem).depth)
	}
	if items[1].(*sidebarItem).kind != sidebarWorkflow {
		t.Fatalf("expected workflow row second")
	}
	if items[1].(*sidebarItem).depth != 1 {
		t.Fatalf("expected workflow depth 1, got %d", items[1].(*sidebarItem).depth)
	}
	if items[2].(*sidebarItem).kind != sidebarSession || items[2].(*sidebarItem).session.ID != "s1" {
		t.Fatalf("expected workflow-owned session nested under workflow")
	}
	if items[2].(*sidebarItem).depth != 2 {
		t.Fatalf("expected workflow session depth 2, got %d", items[2].(*sidebarItem).depth)
	}
	if items[3].(*sidebarItem).kind != sidebarSession || items[3].(*sidebarItem).session.ID != "s2" {
		t.Fatalf("expected regular session outside workflow")
	}
	if items[3].(*sidebarItem).depth != 1 {
		t.Fatalf("expected regular workspace session depth 1, got %d", items[3].(*sidebarItem).depth)
	}
}

func TestBuildSidebarItemsWorkflowDismissedToggle(t *testing.T) {
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-active", TemplateName: "SOLID", WorkspaceID: "ws1", CreatedAt: now, Status: guidedworkflows.WorkflowRunStatusRunning},
		{
			ID:           "gwf-dismissed",
			TemplateName: "SOLID",
			WorkspaceID:  "ws1",
			CreatedAt:    now.Add(-time.Minute),
			Status:       guidedworkflows.WorkflowRunStatusPaused,
			DismissedAt:  ptrTime(now.Add(-30 * time.Second)),
		},
	}

	hidden := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, nil, workflows, nil, false)
	if len(hidden) != 2 {
		t.Fatalf("expected workspace + active workflow when dismissed hidden, got %d", len(hidden))
	}
	if hidden[1].(*sidebarItem).kind != sidebarWorkflow || hidden[1].(*sidebarItem).workflowRunID() != "gwf-active" {
		t.Fatalf("expected active workflow when dismissed hidden")
	}

	visible := buildSidebarItems(workspaces, map[string][]*types.Worktree{}, nil, workflows, nil, true)
	if len(visible) != 3 {
		t.Fatalf("expected workspace + both workflows when dismissed shown, got %d", len(visible))
	}
}

func TestWorkflowRunStatusTextIncludesStoppedAndDismissed(t *testing.T) {
	if got := workflowRunStatusText(nil); got != "" {
		t.Fatalf("expected empty status for nil workflow run, got %q", got)
	}
	now := time.Now().UTC()
	cases := []struct {
		status guidedworkflows.WorkflowRunStatus
		want   string
	}{
		{status: guidedworkflows.WorkflowRunStatusCreated, want: "created"},
		{status: guidedworkflows.WorkflowRunStatusRunning, want: "running"},
		{status: guidedworkflows.WorkflowRunStatusPaused, want: "paused"},
		{status: guidedworkflows.WorkflowRunStatusStopped, want: "stopped"},
		{status: guidedworkflows.WorkflowRunStatusCompleted, want: "completed"},
		{status: guidedworkflows.WorkflowRunStatusFailed, want: "failed"},
	}
	for _, tc := range cases {
		run := &guidedworkflows.WorkflowRun{
			ID:     "gwf-1",
			Status: tc.status,
		}
		if got := workflowRunStatusText(run); got != tc.want {
			t.Fatalf("status %q: expected %q, got %q", tc.status, tc.want, got)
		}
	}
	run := &guidedworkflows.WorkflowRun{ID: "gwf-1"}
	run.Status = guidedworkflows.WorkflowRunStatus(" custom ")
	if got := workflowRunStatusText(run); got != "custom" {
		t.Fatalf("expected trimmed fallback workflow status text, got %q", got)
	}
	run.DismissedAt = &now
	if got := workflowRunStatusText(run); got != "dismissed" {
		t.Fatalf("expected dismissed workflow status text to take precedence, got %q", got)
	}
}

func ptrTime(value time.Time) *time.Time {
	ts := value.UTC()
	return &ts
}

func TestSidebarDelegateRenderSessionIncludesDepthIndent(t *testing.T) {
	now := time.Now().UTC()
	delegate := &sidebarDelegate{}
	model := list.New(nil, delegate, 120, 1)

	renderAtDepth := func(depth int) string {
		var buf bytes.Buffer
		delegate.Render(&buf, model, 0, &sidebarItem{
			kind: sidebarSession,
			session: &types.Session{
				ID:        "s1",
				Provider:  "codex",
				Status:    types.SessionStatusRunning,
				CreatedAt: now,
			},
			meta:  &types.SessionMeta{SessionID: "s1", Title: "workflow child"},
			depth: depth,
		})
		return xansi.Strip(buf.String())
	}

	plainDepthOne := renderAtDepth(1)
	plainDepthTwo := renderAtDepth(2)

	if !strings.HasPrefix(plainDepthOne, "   "+activeDot+" ") {
		t.Fatalf("expected depth 1 session to include leading indent, got %q", plainDepthOne)
	}
	if !strings.HasPrefix(plainDepthTwo, "     "+activeDot+" ") {
		t.Fatalf("expected depth 2 session to include additive leading indent, got %q", plainDepthTwo)
	}
}

func TestSidebarItemWorkflowTitleOmitsRunID(t *testing.T) {
	item := &sidebarItem{
		kind:       sidebarWorkflow,
		workflowID: "gwf-123",
	}
	if got := item.Title(); got != "Guided Workflow" {
		t.Fatalf("expected generic guided workflow title, got %q", got)
	}
}

func TestSidebarDelegateRenderWorkflowOmitsRunID(t *testing.T) {
	delegate := &sidebarDelegate{}
	model := list.New(nil, delegate, 120, 1)
	var buf bytes.Buffer
	delegate.Render(&buf, model, 0, &sidebarItem{
		kind: sidebarWorkflow,
		workflow: &guidedworkflows.WorkflowRun{
			ID:           "gwf-123",
			TemplateName: "SOLID Phase Delivery",
			Status:       guidedworkflows.WorkflowRunStatusRunning,
		},
		collapsible: true,
		expanded:    true,
		depth:       1,
	})
	plain := xansi.Strip(buf.String())
	if strings.Contains(plain, "gwf-123") {
		t.Fatalf("expected workflow row to hide run id, got %q", plain)
	}
}

func TestSidebarDelegateRenderWorktreeRowVariants(t *testing.T) {
	delegate := &sidebarDelegate{activeWorktreeID: "wt1"}
	model := list.New(nil, delegate, 120, 1)

	rendered := func(item *sidebarItem) string {
		var buf bytes.Buffer
		delegate.Render(&buf, model, 0, item)
		return xansi.Strip(buf.String())
	}

	collapsed := rendered(&sidebarItem{
		kind:         sidebarWorktree,
		worktree:     &types.Worktree{ID: "wt1", Name: "Feature A"},
		collapsible:  true,
		expanded:     false,
		depth:        1,
		sessionCount: 2,
	})
	if !strings.Contains(collapsed, "▸ Feature A (2)") {
		t.Fatalf("expected collapsed marker and count in worktree row, got %q", collapsed)
	}

	expanded := rendered(&sidebarItem{
		kind:        sidebarWorktree,
		worktree:    &types.Worktree{ID: "wt2", Name: "Feature B"},
		collapsible: true,
		expanded:    true,
		depth:       1,
	})
	if !strings.Contains(expanded, "▾ Feature B") {
		t.Fatalf("expected expanded marker in worktree row, got %q", expanded)
	}
}

func TestFormatSinceUsesCompactUnits(t *testing.T) {
	now := time.Now().UTC()
	if got := formatSince(ptrTime(now.Add(-30 * time.Second))); got != "just now" {
		t.Fatalf("expected just now for sub-minute delta, got %q", got)
	}
	if got := formatSince(ptrTime(now.Add(-2 * time.Minute))); got != "2m" {
		t.Fatalf("expected compact minutes, got %q", got)
	}
	if got := formatSince(ptrTime(now.Add(-3 * time.Hour))); got != "3h" {
		t.Fatalf("expected compact hours, got %q", got)
	}
	if got := formatSince(ptrTime(now.Add(-49 * time.Hour))); got != "2d" {
		t.Fatalf("expected compact days, got %q", got)
	}
}

func TestBuildSessionRightTextIncludesDismissedAndCompactTime(t *testing.T) {
	now := time.Now().UTC()
	session := &types.Session{
		ID:        "s1",
		Status:    types.SessionStatusExited,
		CreatedAt: now.Add(-5 * time.Minute),
	}
	dismissedAt := now.Add(-2 * time.Minute)
	meta := &types.SessionMeta{
		SessionID:   "s1",
		DismissedAt: &dismissedAt,
	}
	if got := buildSessionRightText(session, meta, now); got != "dismissed • 5m" {
		t.Fatalf("expected dismissed compact age text, got %q", got)
	}
}

func TestBuildSessionRightTextNonDismissed(t *testing.T) {
	now := time.Now().UTC()
	session := &types.Session{
		ID:        "s1",
		Status:    types.SessionStatusRunning,
		CreatedAt: now.Add(-3 * time.Minute),
	}
	if got := buildSessionRightText(session, nil, now); got != "3m" {
		t.Fatalf("expected compact age text, got %q", got)
	}
}

func TestSidebarDelegateBuildSessionRowViewModelUsesInjectedNow(t *testing.T) {
	base := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
	delegate := &sidebarDelegate{
		now: func() time.Time { return base.Add(8 * time.Minute) },
	}
	entry := &sidebarItem{
		kind: sidebarSession,
		session: &types.Session{
			ID:        "s1",
			Provider:  "codex",
			Status:    types.SessionStatusRunning,
			CreatedAt: base,
		},
		meta: &types.SessionMeta{
			SessionID: "s1",
		},
	}

	vm := delegate.buildSessionRowViewModel(entry, 80)
	if !strings.Contains(vm.RightText, "8m") {
		t.Fatalf("expected injected now to drive compact age, got right text %q", vm.RightText)
	}
}

func TestSidebarDelegateRenderSessionRowUsesInjectedLayoutEngine(t *testing.T) {
	delegate := &sidebarDelegate{
		sessionLayout: testSessionLayoutEngine{title: "LEFT", right: "  RIGHT"},
	}
	model := list.New(nil, delegate, 60, 1)
	var buf bytes.Buffer
	delegate.Render(&buf, model, 0, &sidebarItem{
		kind: sidebarSession,
		session: &types.Session{
			ID:        "s1",
			Provider:  "codex",
			Status:    types.SessionStatusRunning,
			CreatedAt: time.Now().UTC(),
		},
		meta: &types.SessionMeta{SessionID: "s1"},
	})
	plain := xansi.Strip(buf.String())
	if !strings.Contains(plain, "LEFT") || !strings.Contains(plain, "RIGHT") {
		t.Fatalf("expected injected layout output in rendered session row, got %q", plain)
	}
}

func TestRenderSessionColumnsEdgeCases(t *testing.T) {
	cases := []struct {
		name       string
		title      string
		right      string
		width      int
		wantTitle  string
		wantRight  string
		wantRightW int
	}{
		{name: "non-positive width", title: "abc", right: "1m", width: 0, wantTitle: "", wantRight: ""},
		{name: "empty right", title: "abcdef", right: "", width: 4, wantTitle: "abc…", wantRight: ""},
		{name: "right wider than width", title: "abcdef", right: "dismissed • 10m", width: 6, wantTitle: "", wantRightW: 6},
		{name: "max title zero", title: "abcdef", right: "1234", width: 5, wantTitle: "", wantRight: " 1234"},
		{name: "normal layout", title: "alpha", right: "5m", width: 10, wantTitle: "alpha", wantRight: "   5m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title, right := renderSessionColumns(tc.title, tc.right, tc.width)
			if tc.wantTitle != "" && title != tc.wantTitle {
				t.Fatalf("expected title %q, got %q", tc.wantTitle, title)
			}
			if tc.wantRight != "" && right != tc.wantRight {
				t.Fatalf("expected right %q, got %q", tc.wantRight, right)
			}
			if tc.wantRightW > 0 && xansi.StringWidth(right) != tc.wantRightW {
				t.Fatalf("expected right width %d, got %d (%q)", tc.wantRightW, xansi.StringWidth(right), right)
			}
		})
	}
}

func TestSidebarIsSelectedKeyGuards(t *testing.T) {
	var nilDelegate *sidebarDelegate
	if nilDelegate.isSelectedKey("ws:1") {
		t.Fatalf("expected nil delegate to return false")
	}
	delegate := &sidebarDelegate{selectedKeys: map[string]struct{}{"ws:1": {}}}
	if delegate.isSelectedKey(" ") {
		t.Fatalf("expected blank key to return false")
	}
	if !delegate.isSelectedKey("ws:1") {
		t.Fatalf("expected existing key to return true")
	}
}

func TestSidebarDelegateRenderSessionTimeIsRightAligned(t *testing.T) {
	now := time.Now().UTC()
	delegate := &sidebarDelegate{}
	model := list.New(nil, delegate, 70, 1)

	renderSession := func(title string) string {
		var buf bytes.Buffer
		delegate.Render(&buf, model, 0, &sidebarItem{
			kind: sidebarSession,
			session: &types.Session{
				ID:        "s1",
				Provider:  "codex",
				Status:    types.SessionStatusRunning,
				CreatedAt: now.Add(-5 * time.Minute),
			},
			meta: &types.SessionMeta{
				SessionID: "s1",
				Title:     title,
			},
			depth: 1,
		})
		return xansi.Strip(buf.String())
	}

	shortLine := renderSession("short")
	longLine := renderSession("this is a much longer session title used for alignment checks")

	shortIdx := strings.Index(shortLine, "5m")
	longIdx := strings.Index(longLine, "5m")
	if shortIdx < 0 || longIdx < 0 {
		t.Fatalf("expected both lines to include compact age value: short=%q long=%q", shortLine, longLine)
	}
	shortCol := xansi.StringWidth(shortLine[:shortIdx])
	longCol := xansi.StringWidth(longLine[:longIdx])
	if shortCol != longCol {
		t.Fatalf("expected age text to align to same visual column, got short=%d long=%d", shortCol, longCol)
	}
	if strings.Contains(shortLine, "ago") || strings.Contains(longLine, "ago") {
		t.Fatalf("expected compact age without ago suffix: short=%q long=%q", shortLine, longLine)
	}
}

func TestBuildSidebarItemsWithOptionsSortsWorkspacesByActivity(t *testing.T) {
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws-old", Name: "Old", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: "ws-new", Name: "New", CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)},
	}
	sessions := []*types.Session{
		{ID: "s-old", CreatedAt: now.Add(-80 * time.Minute), Status: types.SessionStatusRunning},
		{ID: "s-new", CreatedAt: now.Add(-10 * time.Minute), Status: types.SessionStatusRunning},
	}
	meta := map[string]*types.SessionMeta{
		"s-old": {SessionID: "s-old", WorkspaceID: "ws-old"},
		"s-new": {SessionID: "s-new", WorkspaceID: "ws-new"},
	}

	items := buildSidebarItemsWithOptions(
		workspaces,
		map[string][]*types.Worktree{},
		sessions,
		nil,
		meta,
		false,
		sidebarBuildOptions{
			expansion: sidebarExpansionResolver{defaultOn: true},
			sort:      sidebarSortState{Key: sidebarSortKeyActivity},
		},
	)
	if len(items) < 1 {
		t.Fatalf("expected workspace rows")
	}
	first := items[0].(*sidebarItem)
	if first.kind != sidebarWorkspace || first.workspace == nil || first.workspace.ID != "ws-new" {
		t.Fatalf("expected ws-new first by activity, got %#v", first)
	}
}

func TestBuildSidebarItemsWithOptionsFiltersByQueryAndKeepsAncestors(t *testing.T) {
	now := time.Now().UTC()
	workspaces := []*types.Workspace{
		{ID: "ws1", Name: "Payments"},
	}
	sessions := []*types.Session{
		{ID: "s1", CreatedAt: now, Status: types.SessionStatusRunning},
	}
	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Retry cleanup"},
	}
	items := buildSidebarItemsWithOptions(
		workspaces,
		map[string][]*types.Worktree{},
		sessions,
		nil,
		meta,
		false,
		sidebarBuildOptions{
			expansion:   sidebarExpansionResolver{defaultOn: true},
			sort:        defaultSidebarSortState(),
			filterQuery: "retry",
		},
	)
	if len(items) != 2 {
		t.Fatalf("expected workspace + matching session, got %d", len(items))
	}
	if got := items[0].(*sidebarItem).kind; got != sidebarWorkspace {
		t.Fatalf("expected ancestor workspace to be retained, got %v", got)
	}
	if got := items[1].(*sidebarItem).kind; got != sidebarSession {
		t.Fatalf("expected filtered session row, got %v", got)
	}
}
