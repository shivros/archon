package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

type testSidebarSelectionReader struct {
	selected map[string]struct{}
}

func (r testSidebarSelectionReader) SelectedKeyCount() int {
	return len(r.selected)
}

func (r testSidebarSelectionReader) SingleSelectedKey() string {
	if len(r.selected) != 1 {
		return ""
	}
	for key := range r.selected {
		return key
	}
	return ""
}

func (r testSidebarSelectionReader) HasSelectedKeys() bool {
	return len(r.selected) > 0
}

func (r testSidebarSelectionReader) IsKeySelected(key string) bool {
	_, ok := r.selected[key]
	return ok
}

func (r testSidebarSelectionReader) SelectedKeys() []string {
	out := make([]string, 0, len(r.selected))
	for key := range r.selected {
		out = append(out, key)
	}
	return out
}

type testSidebarSelectionController struct {
	testSidebarSelectionReader
	selectedKey     string
	selectByKeyArgs []string
	toggleCalls     int
	clearCalls      int
	rangeCalls      int
	lastRangeAnchor string
	lastRangeTarget string
	selectByKeyOK   bool
	toggleOK        bool
	rangeOK         bool
}

func (c *testSidebarSelectionController) SelectByKey(key string) bool {
	key = stringTrimSpace(key)
	if key == "" {
		return false
	}
	c.selectByKeyArgs = append(c.selectByKeyArgs, key)
	c.selectedKey = key
	return c.selectByKeyOK
}

func (c *testSidebarSelectionController) ToggleFocusedSelection() bool {
	c.toggleCalls++
	key := stringTrimSpace(c.selectedKey)
	if key == "" {
		return false
	}
	if !c.toggleOK {
		return false
	}
	if c.selected == nil {
		c.selected = map[string]struct{}{}
	}
	if _, ok := c.selected[key]; ok {
		delete(c.selected, key)
		return true
	}
	c.selected[key] = struct{}{}
	return true
}

func (c *testSidebarSelectionController) ClearSelectedKeys() bool {
	c.clearCalls++
	c.selected = map[string]struct{}{}
	return true
}

func (c *testSidebarSelectionController) AddSelectionRangeByKeys(anchorKey, targetKey string) bool {
	c.rangeCalls++
	c.lastRangeAnchor = stringTrimSpace(anchorKey)
	c.lastRangeTarget = stringTrimSpace(targetKey)
	if !c.rangeOK {
		return false
	}
	if c.selected == nil {
		c.selected = map[string]struct{}{}
	}
	if c.lastRangeAnchor != "" {
		c.selected[c.lastRangeAnchor] = struct{}{}
	}
	if c.lastRangeTarget != "" {
		c.selected[c.lastRangeTarget] = struct{}{}
	}
	return true
}

func TestDefaultSidebarSelectionIntentPolicyResolveIntent(t *testing.T) {
	policy := defaultSidebarSelectionIntentPolicy{}

	if got := policy.ResolveIntent(nil, "sess:s1", tea.Mouse{}); got.kind != sidebarSelectionIntentNone {
		t.Fatalf("expected none intent when sidebar is nil, got %#v", got)
	}
	if got := policy.ResolveIntent(testSidebarSelectionReader{}, "", tea.Mouse{}); got.kind != sidebarSelectionIntentNone {
		t.Fatalf("expected none intent when clicked key is empty, got %#v", got)
	}

	reader := testSidebarSelectionReader{selected: map[string]struct{}{"sess:s1": {}}}
	if got := policy.ResolveIntent(reader, "sess:s2", tea.Mouse{Mod: tea.ModShift}); got.kind != sidebarSelectionIntentRangeAdd || got.anchorKey != "sess:s1" || got.targetKey != "sess:s2" {
		t.Fatalf("expected shift intent to be range add, got %#v", got)
	}

	reader = testSidebarSelectionReader{selected: map[string]struct{}{"sess:s1": {}, "sess:s2": {}}}
	if got := policy.ResolveIntent(reader, "sess:s3", tea.Mouse{Mod: tea.ModShift}); got.kind != sidebarSelectionIntentReplace {
		t.Fatalf("expected shift intent with multi-select to fallback to replace, got %#v", got)
	}
	if got := policy.ResolveIntent(reader, "sess:s3", tea.Mouse{Mod: tea.ModCtrl}); got.kind != sidebarSelectionIntentToggle {
		t.Fatalf("expected ctrl intent to toggle, got %#v", got)
	}
	if got := policy.ResolveIntent(reader, "sess:s3", tea.Mouse{}); got.kind != sidebarSelectionIntentReplace {
		t.Fatalf("expected plain click intent to replace, got %#v", got)
	}
}

func TestDefaultSidebarSelectionServiceApplyIntent(t *testing.T) {
	service := defaultSidebarSelectionService{}
	controller := &testSidebarSelectionController{
		testSidebarSelectionReader: testSidebarSelectionReader{
			selected: map[string]struct{}{"sess:s1": {}, "sess:s2": {}},
		},
		selectByKeyOK: true,
		toggleOK:      true,
		rangeOK:       true,
	}

	if service.ApplyIntent(nil, sidebarSelectionIntent{kind: sidebarSelectionIntentReplace, targetKey: "sess:s3"}) {
		t.Fatalf("expected nil sidebar to reject intent")
	}
	if !service.ApplyIntent(controller, sidebarSelectionIntent{kind: sidebarSelectionIntentReplace, targetKey: "sess:s3"}) {
		t.Fatalf("expected replace intent to apply")
	}
	if controller.clearCalls != 1 {
		t.Fatalf("expected replace intent to clear selection set once, got %d", controller.clearCalls)
	}
	if len(controller.selectByKeyArgs) != 1 || controller.selectByKeyArgs[0] != "sess:s3" {
		t.Fatalf("expected replace intent to select target key, got %#v", controller.selectByKeyArgs)
	}

	controller.selected = map[string]struct{}{"sess:s3": {}}
	if !service.ApplyIntent(controller, sidebarSelectionIntent{kind: sidebarSelectionIntentReplace, targetKey: "sess:s3"}) {
		t.Fatalf("expected replace intent to keep existing selected target")
	}
	if controller.clearCalls != 1 {
		t.Fatalf("expected replace intent not to clear already-selected target")
	}

	if !service.ApplyIntent(controller, sidebarSelectionIntent{kind: sidebarSelectionIntentToggle, targetKey: "sess:s4"}) {
		t.Fatalf("expected toggle intent to apply")
	}
	if controller.toggleCalls != 1 {
		t.Fatalf("expected toggle intent to invoke toggle once, got %d", controller.toggleCalls)
	}

	if !service.ApplyIntent(controller, sidebarSelectionIntent{kind: sidebarSelectionIntentRangeAdd, anchorKey: "sess:s1", targetKey: "sess:s4"}) {
		t.Fatalf("expected range intent to apply")
	}
	if controller.rangeCalls != 1 || controller.lastRangeAnchor != "sess:s1" || controller.lastRangeTarget != "sess:s4" {
		t.Fatalf("expected range intent to forward anchor/target, got calls=%d anchor=%q target=%q", controller.rangeCalls, controller.lastRangeAnchor, controller.lastRangeTarget)
	}

	if service.ApplyIntent(controller, sidebarSelectionIntent{kind: sidebarSelectionIntentNone}) {
		t.Fatalf("expected none intent to be rejected")
	}

	if service.ApplyIntent(controller, sidebarSelectionIntent{kind: sidebarSelectionIntentReplace, targetKey: ""}) {
		t.Fatalf("expected replace intent with empty target to be rejected")
	}

	controller.selectByKeyOK = false
	if service.ApplyIntent(controller, sidebarSelectionIntent{kind: sidebarSelectionIntentToggle, targetKey: "sess:s5"}) {
		t.Fatalf("expected toggle intent to reject when select-by-key fails")
	}
	controller.selectByKeyOK = true

	if service.ApplyIntent(controller, sidebarSelectionIntent{kind: sidebarSelectionIntentRangeAdd, anchorKey: "", targetKey: "sess:s5"}) {
		t.Fatalf("expected range intent with empty anchor to be rejected")
	}
	if service.ApplyIntent(controller, sidebarSelectionIntent{kind: sidebarSelectionIntentRangeAdd, anchorKey: "sess:s1", targetKey: ""}) {
		t.Fatalf("expected range intent with empty target to be rejected")
	}

	controller.selectByKeyOK = false
	if service.ApplyIntent(controller, sidebarSelectionIntent{kind: sidebarSelectionIntentRangeAdd, anchorKey: "sess:s1", targetKey: "sess:s5"}) {
		t.Fatalf("expected range intent to reject when select-by-key fails")
	}
}

type testSidebarSelectionIntentPolicy struct {
	intent sidebarSelectionIntent
	calls  int
}

func (p *testSidebarSelectionIntentPolicy) ResolveIntent(sidebar SidebarSelectionReader, clickedKey string, mouse tea.Mouse) sidebarSelectionIntent {
	p.calls++
	return p.intent
}

type testSidebarSelectionService struct {
	calls      int
	lastIntent sidebarSelectionIntent
	lastTarget SidebarSelectionController
	result     bool
}

func (s *testSidebarSelectionService) ApplyIntent(sidebar SidebarSelectionController, intent sidebarSelectionIntent) bool {
	s.calls++
	s.lastTarget = sidebar
	s.lastIntent = intent
	return s.result
}

func TestModelSidebarSelectionOptionsAndDefaults(t *testing.T) {
	var nilModel *Model
	if policy := nilModel.sidebarSelectionIntentPolicyOrDefault(); policy == nil {
		t.Fatalf("expected default selection policy for nil model")
	}
	if service := nilModel.sidebarSelectionServiceOrDefault(); service == nil {
		t.Fatalf("expected default selection service for nil model")
	}

	m := NewModel(nil)
	if m.sidebarSelectionIntentPolicy == nil || m.sidebarSelectionService == nil {
		t.Fatalf("expected non-nil default selection dependencies")
	}

	customPolicy := &testSidebarSelectionIntentPolicy{
		intent: sidebarSelectionIntent{kind: sidebarSelectionIntentReplace, targetKey: "sess:s1"},
	}
	customService := &testSidebarSelectionService{result: true}
	WithSidebarSelectionIntentPolicy(customPolicy)(&m)
	WithSidebarSelectionService(customService)(&m)
	if m.sidebarSelectionIntentPolicy != customPolicy {
		t.Fatalf("expected custom selection policy assignment")
	}
	if m.sidebarSelectionService != customService {
		t.Fatalf("expected custom selection service assignment")
	}

	WithSidebarSelectionIntentPolicy(nil)(&m)
	WithSidebarSelectionService(nil)(&m)
	if _, ok := m.sidebarSelectionIntentPolicy.(defaultSidebarSelectionIntentPolicy); !ok {
		t.Fatalf("expected nil selection policy option to restore default policy, got %T", m.sidebarSelectionIntentPolicy)
	}
	if _, ok := m.sidebarSelectionService.(defaultSidebarSelectionService); !ok {
		t.Fatalf("expected nil selection service option to restore default service, got %T", m.sidebarSelectionService)
	}
}

func TestMouseReducerSidebarSelectionUsesPolicyAndService(t *testing.T) {
	policy := &testSidebarSelectionIntentPolicy{
		intent: sidebarSelectionIntent{
			kind:      sidebarSelectionIntentToggle,
			targetKey: "sess:s1",
		},
	}
	service := &testSidebarSelectionService{result: true}
	m := NewModel(
		nil,
		WithSidebarSelectionIntentPolicy(policy),
		WithSidebarSelectionService(service),
	)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	layout := m.resolveMouseLayout()

	row := -1
	for y := 0; y < 20; y++ {
		entry := m.sidebar.ItemAtRow(y)
		if entry != nil && entry.kind == sidebarSession && entry.session != nil && entry.session.ID == "s1" {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected visible s1 row")
	}
	handled := m.reduceSidebarSelectionLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 6, Y: row}, layout)
	if !handled {
		t.Fatalf("expected sidebar click to be handled")
	}
	if policy.calls != 1 {
		t.Fatalf("expected policy to be called once, got %d", policy.calls)
	}
	if service.calls != 1 {
		t.Fatalf("expected service to be called once, got %d", service.calls)
	}
	if service.lastTarget != m.sidebar {
		t.Fatalf("expected service target to be model sidebar")
	}
	if service.lastIntent.kind != sidebarSelectionIntentToggle || service.lastIntent.targetKey != "sess:s1" {
		t.Fatalf("unexpected intent forwarded to service: %#v", service.lastIntent)
	}
}

func TestMouseReducerSidebarSelectionNoopIntentStillHandled(t *testing.T) {
	policy := &testSidebarSelectionIntentPolicy{
		intent: sidebarSelectionIntent{kind: sidebarSelectionIntentNone},
	}
	service := &testSidebarSelectionService{result: false}
	m := NewModel(
		nil,
		WithSidebarSelectionIntentPolicy(policy),
		WithSidebarSelectionService(service),
	)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	layout := m.resolveMouseLayout()

	row := -1
	for y := 0; y < 20; y++ {
		entry := m.sidebar.ItemAtRow(y)
		if entry != nil && entry.kind == sidebarSession && entry.session != nil && entry.session.ID == "s1" {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected visible s1 row")
	}
	beforeKey := m.sidebar.SelectedKey()
	handled := m.reduceSidebarSelectionLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 6, Y: row}, layout)
	if !handled {
		t.Fatalf("expected sidebar click to be handled")
	}
	if policy.calls != 1 || service.calls != 1 {
		t.Fatalf("expected one policy and service call, got policy=%d service=%d", policy.calls, service.calls)
	}
	if got := m.sidebar.SelectedKey(); got != beforeKey {
		t.Fatalf("expected noop selection service to keep focused key, got %q want %q", got, beforeKey)
	}
}

func stringTrimSpace(value string) string {
	return strings.TrimSpace(value)
}
