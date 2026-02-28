package app

import (
	"strings"
	"testing"

	"control/internal/client"
	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

func TestAddWorkspaceControllerSupportsRemappedSubmit(t *testing.T) {
	controller := NewAddWorkspaceController(80)
	host := &stubAddWorkspaceHost{
		stubKeyResolver: stubKeyResolver{submitKey: "f6"},
		groups: []*types.WorkspaceGroup{
			{ID: "g1", Name: "Group 1"},
		},
	}
	controller.Enter()
	if controller.input == nil {
		t.Fatalf("expected input")
	}
	controller.input.SetValue("/tmp/repo")

	handled, cmd := controller.Update(tea.KeyPressMsg{Code: tea.KeyF6}, host)
	if !handled {
		t.Fatalf("expected remapped submit key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command after first step")
	}
	if controller.step != 1 {
		t.Fatalf("expected second step after first submit, got %d", controller.step)
	}
	controller.input.SetValue("packages/pennies")

	handled, cmd = controller.Update(tea.KeyPressMsg{Code: tea.KeyF6}, host)
	if !handled {
		t.Fatalf("expected remapped submit key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command after second step")
	}
	if controller.step != 2 {
		t.Fatalf("expected additional directories step after second submit, got %d", controller.step)
	}
	controller.input.SetValue("../backend, ../shared")

	handled, cmd = controller.Update(tea.KeyPressMsg{Code: tea.KeyF6}, host)
	if !handled {
		t.Fatalf("expected remapped submit key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command after third step")
	}
	if controller.step != 3 {
		t.Fatalf("expected name step after third submit, got %d", controller.step)
	}
	controller.input.SetValue("Repo Name")

	// Step 3 → step 4 (group picker), not final submission
	handled, cmd = controller.Update(tea.KeyPressMsg{Code: tea.KeyF6}, host)
	if !handled {
		t.Fatalf("expected remapped submit key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command after name step")
	}
	if controller.step != 4 {
		t.Fatalf("expected group picker step after name submit, got %d", controller.step)
	}

	// Step 4: Enter confirms group picker (no groups toggled)
	handled, cmd = controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	if !handled {
		t.Fatalf("expected enter to be handled on group picker step")
	}
	if cmd != nil {
		t.Fatalf("expected no async command from stub host")
	}
	if host.createPath != "/tmp/repo" || host.createSub != "packages/pennies" || host.createName != "Repo Name" {
		t.Fatalf("unexpected create payload: path=%q sub=%q name=%q", host.createPath, host.createSub, host.createName)
	}
	if len(host.createDirs) != 2 || host.createDirs[0] != "../backend" || host.createDirs[1] != "../shared" {
		t.Fatalf("unexpected additional directories: %#v", host.createDirs)
	}
	if len(host.createGroupIDs) != 0 {
		t.Fatalf("expected no group IDs when none toggled, got %#v", host.createGroupIDs)
	}
}

func TestAddWorkspaceControllerGroupPickerToggle(t *testing.T) {
	controller := NewAddWorkspaceController(80)
	host := &stubAddWorkspaceHost{
		groups: []*types.WorkspaceGroup{
			{ID: "g1", Name: "Alpha"},
			{ID: "g2", Name: "Beta"},
		},
	}
	controller.Enter()
	controller.input.SetValue("/tmp/repo")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // step 0 → 1
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // step 1 → 2
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // step 2 → 3
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // step 3 → 4

	if controller.step != 4 {
		t.Fatalf("expected step 4, got %d", controller.step)
	}

	// Toggle first group
	controller.Update(tea.KeyPressMsg{Code: ' '}, host)

	// Confirm
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)

	if len(host.createGroupIDs) != 1 || host.createGroupIDs[0] != "g1" {
		t.Fatalf("expected group g1 to be selected, got %#v", host.createGroupIDs)
	}
}

func TestAddWorkspaceControllerGroupPickerEscClearsQuery(t *testing.T) {
	controller := NewAddWorkspaceController(80)
	host := &stubAddWorkspaceHost{
		groups: []*types.WorkspaceGroup{
			{ID: "g1", Name: "Alpha"},
		},
	}
	controller.Enter()
	controller.input.SetValue("/tmp/repo")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // → step 4

	// Type a query character
	controller.Update(tea.KeyPressMsg{Code: 'x'}, host)
	if controller.groupPicker.Query() == "" {
		t.Fatalf("expected non-empty query after typing")
	}

	// Esc should clear query, not cancel
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEsc}, host)
	if controller.groupPicker.Query() != "" {
		t.Fatalf("expected query to clear on esc, got %q", controller.groupPicker.Query())
	}
	if controller.step != 4 {
		t.Fatalf("expected to remain on step 4 after esc clears query, got %d", controller.step)
	}
}

func TestAddWorkspaceControllerGroupPickerEscCancels(t *testing.T) {
	controller := NewAddWorkspaceController(80)
	host := &stubAddWorkspaceHost{
		groups: []*types.WorkspaceGroup{
			{ID: "g1", Name: "Alpha"},
		},
	}
	controller.Enter()
	controller.input.SetValue("/tmp/repo")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // → step 4

	// Esc with no query should cancel
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEsc}, host)
	if host.status != "add workspace canceled" {
		t.Fatalf("expected cancel status, got %q", host.status)
	}
}

func TestAddWorkspaceControllerGroupPickerPasteFilters(t *testing.T) {
	controller := NewAddWorkspaceController(80)
	host := &stubAddWorkspaceHost{
		groups: []*types.WorkspaceGroup{
			{ID: "g1", Name: "Alpha"},
			{ID: "g2", Name: "Beta"},
		},
	}
	controller.Enter()
	controller.input.SetValue("/tmp/repo")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // → step 4

	if controller.step != 4 {
		t.Fatalf("expected step 4, got %d", controller.step)
	}

	handled, _ := controller.Update(tea.PasteMsg{Content: "bet"}, host)
	if !handled {
		t.Fatalf("expected paste to be handled at group picker step")
	}
	if got := controller.groupPicker.Query(); got != "bet" {
		t.Fatalf("expected query 'bet' after paste, got %q", got)
	}
}

func TestAddWorkspaceControllerGroupPickerDownKeyMovesSelection(t *testing.T) {
	controller := NewAddWorkspaceController(80)
	host := &stubAddWorkspaceHost{
		groups: []*types.WorkspaceGroup{
			{ID: "g1", Name: "Alpha"},
			{ID: "g2", Name: "Beta"},
		},
	}
	controller.Enter()
	controller.input.SetValue("/tmp/repo")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // → step 4

	if controller.step != 4 {
		t.Fatalf("expected step 4, got %d", controller.step)
	}

	// Move down to second group, toggle it
	controller.Update(tea.KeyPressMsg{Code: tea.KeyDown}, host)
	controller.Update(tea.KeyPressMsg{Code: ' '}, host)

	// Confirm
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)

	if len(host.createGroupIDs) != 1 || host.createGroupIDs[0] != "g2" {
		t.Fatalf("expected group g2 to be selected after down+toggle, got %#v", host.createGroupIDs)
	}
}

func TestAddWorkspaceControllerClearCommandClearsInput(t *testing.T) {
	controller := NewAddWorkspaceController(80)
	host := &stubAddWorkspaceHost{}
	controller.Enter()
	if controller.input == nil {
		t.Fatalf("expected input")
	}
	controller.input.SetValue("/tmp/repo")

	handled, cmd := controller.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, host)
	if !handled {
		t.Fatalf("expected clear command to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for clear action")
	}
	if got := controller.input.Value(); got != "" {
		t.Fatalf("expected add workspace input to clear, got %q", got)
	}
	if controller.step != 0 {
		t.Fatalf("expected clear command to keep current step, got %d", controller.step)
	}
}

func TestAddWorkspaceControllerSupportsRemappedClearCommand(t *testing.T) {
	controller := NewAddWorkspaceController(80)
	host := &stubAddWorkspaceHost{stubKeyResolver: stubKeyResolver{clearKey: "f7"}}
	controller.Enter()
	if controller.input == nil {
		t.Fatalf("expected input")
	}
	controller.input.SetValue("/tmp/repo")

	handled, _ := controller.Update(tea.KeyPressMsg{Code: tea.KeyF7}, host)
	if !handled {
		t.Fatalf("expected remapped clear command to be handled")
	}
	if got := controller.input.Value(); got != "" {
		t.Fatalf("expected add workspace input to clear, got %q", got)
	}
}

func TestAddWorktreeControllerSupportsRemappedSubmit(t *testing.T) {
	controller := NewAddWorktreeController(80)
	host := &stubAddWorktreeHost{stubKeyResolver: stubKeyResolver{submitKey: "f6"}}
	controller.Enter("ws1", "/tmp/repo")
	if controller.input == nil {
		t.Fatalf("expected input")
	}

	handled, cmd := controller.Update(tea.KeyPressMsg{Code: tea.KeyF6}, host)
	if !handled {
		t.Fatalf("expected remapped submit key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command when selecting mode")
	}
	if controller.mode != worktreeModeNew || controller.step != 0 {
		t.Fatalf("expected new-worktree step 0, got mode=%v step=%d", controller.mode, controller.step)
	}

	controller.input.SetValue("/tmp/repo-wt")
	handled, cmd = controller.Update(tea.KeyPressMsg{Code: tea.KeyF6}, host)
	if !handled {
		t.Fatalf("expected remapped submit key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command after path step")
	}
	if controller.step != 1 {
		t.Fatalf("expected branch step after path submit, got %d", controller.step)
	}
}

func TestParseAdditionalDirectories(t *testing.T) {
	got := parseAdditionalDirectories(" ../backend, ,../shared ,,  ")
	if len(got) != 2 {
		t.Fatalf("expected two directories, got %#v", got)
	}
	if got[0] != "../backend" || got[1] != "../shared" {
		t.Fatalf("unexpected parsed directories: %#v", got)
	}
}

func TestAddWorktreeControllerExistingTypeAheadSelectsFilteredEntry(t *testing.T) {
	controller := NewAddWorktreeController(80)
	host := &stubAddWorktreeHost{}
	controller.Enter("ws1", "/tmp/repo")
	controller.mode = worktreeModeExisting
	controller.step = 0
	controller.SetAvailable([]*types.GitWorktree{
		{Path: "/tmp/repo/feature-ui", Branch: "feature-ui"},
		{Path: "/tmp/repo/fix-api", Branch: "fix-api"},
		{Path: "/tmp/repo/chore", Branch: "chore"},
	}, nil, "/tmp/repo")
	if !controller.appendQuery("fxapi") {
		t.Fatalf("expected type-ahead query to filter worktrees")
	}
	if len(controller.filtered) != 1 {
		t.Fatalf("expected one filtered worktree, got %d", len(controller.filtered))
	}
	if _, cmd := controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host); cmd != nil {
		t.Fatalf("expected no async command when picking existing worktree")
	}
	if controller.step != 1 {
		t.Fatalf("expected controller to advance to name step, got %d", controller.step)
	}
	if controller.path != "/tmp/repo/fix-api" {
		t.Fatalf("expected filtered worktree to be selected, got %q", controller.path)
	}
}

func TestAddWorktreeControllerExistingPasteUpdatesFilterQuery(t *testing.T) {
	controller := NewAddWorktreeController(80)
	host := &stubAddWorktreeHost{}
	controller.Enter("ws1", "/tmp/repo")
	controller.mode = worktreeModeExisting
	controller.step = 0
	controller.SetAvailable([]*types.GitWorktree{
		{Path: "/tmp/repo/feature-ui", Branch: "feature-ui"},
		{Path: "/tmp/repo/fix-api", Branch: "fix-api"},
		{Path: "/tmp/repo/chore", Branch: "chore"},
	}, nil, "/tmp/repo")

	handled, _ := controller.Update(tea.PasteMsg{Content: " \x1b[31mfxapi\x1b[0m\n "}, host)
	if !handled {
		t.Fatalf("expected paste to be handled while filtering existing worktrees")
	}
	if got := controller.query; got != "fxapi" {
		t.Fatalf("expected sanitized query, got %q", got)
	}
	if len(controller.filtered) != 1 {
		t.Fatalf("expected one filtered worktree, got %d", len(controller.filtered))
	}
	if idx := controller.selectedAvailableIndex(); idx < 0 || idx >= len(controller.available) {
		t.Fatalf("expected selected index in range, got %d", idx)
	} else if controller.available[idx].Path != "/tmp/repo/fix-api" {
		t.Fatalf("expected filtered worktree to be fix-api, got %q", controller.available[idx].Path)
	}
}

func TestAddWorktreeControllerExistingClearCommandClearsFilterQuery(t *testing.T) {
	controller := NewAddWorktreeController(80)
	host := &stubAddWorktreeHost{}
	controller.Enter("ws1", "/tmp/repo")
	controller.mode = worktreeModeExisting
	controller.step = 0
	controller.SetAvailable([]*types.GitWorktree{
		{Path: "/tmp/repo/feature-ui", Branch: "feature-ui"},
		{Path: "/tmp/repo/fix-api", Branch: "fix-api"},
	}, nil, "/tmp/repo")
	if !controller.appendQuery("fxapi") {
		t.Fatalf("expected query to be initialized")
	}

	handled, cmd := controller.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, host)
	if !handled {
		t.Fatalf("expected clear command to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for clear action")
	}
	if got := controller.query; got != "" {
		t.Fatalf("expected existing-worktree filter query to clear, got %q", got)
	}
}

// stubKeyResolver provides a shared KeyResolver implementation for test stubs.
// Set submitKey, clearKey, or toggleSidebarMatch to simulate remapped keybindings.
type stubKeyResolver struct {
	submitKey          string
	clearKey           string
	toggleSidebarMatch bool
}

func (r *stubKeyResolver) keyString(msg tea.KeyMsg) string {
	return msg.String()
}

func (r *stubKeyResolver) keyMatchesCommand(msg tea.KeyMsg, command, fallback string) bool {
	if command == KeyCommandToggleSidebar && r.toggleSidebarMatch {
		return true
	}
	key := strings.TrimSpace(msg.String())
	if command == KeyCommandInputSubmit && strings.TrimSpace(r.submitKey) != "" {
		return key == strings.TrimSpace(r.submitKey)
	}
	if command == KeyCommandInputClear && strings.TrimSpace(r.clearKey) != "" {
		return key == strings.TrimSpace(r.clearKey)
	}
	return key == strings.TrimSpace(fallback)
}

type stubAddWorkspaceHost struct {
	stubKeyResolver
	status         string
	createPath     string
	createSub      string
	createDirs     []string
	createName     string
	createGroupIDs []string
	groups         []*types.WorkspaceGroup
}

func (h *stubAddWorkspaceHost) createWorkspaceCmd(path, sessionSubpath, name string, additionalDirectories, groupIDs []string) tea.Cmd {
	h.createPath = path
	h.createSub = sessionSubpath
	h.createDirs = append([]string(nil), additionalDirectories...)
	h.createName = name
	h.createGroupIDs = append([]string(nil), groupIDs...)
	return nil
}

func (h *stubAddWorkspaceHost) exitAddWorkspace(status string) {
	h.status = status
}

func (h *stubAddWorkspaceHost) setStatus(status string) {
	h.status = status
}

func (h *stubAddWorkspaceHost) workspaceGroups() []*types.WorkspaceGroup {
	return h.groups
}

type stubAddWorktreeHost struct {
	stubKeyResolver
	status string
}

func (h *stubAddWorktreeHost) addWorktreeCmd(workspaceID string, worktree *types.Worktree) tea.Cmd {
	return nil
}

func (h *stubAddWorktreeHost) createWorktreeCmd(workspaceID string, req client.CreateWorktreeRequest) tea.Cmd {
	return nil
}

func (h *stubAddWorktreeHost) exitAddWorktree(status string) {
	h.status = status
}

func (h *stubAddWorktreeHost) fetchAvailableWorktreesCmd(workspaceID, workspacePath string) tea.Cmd {
	return nil
}

func (h *stubAddWorktreeHost) setStatus(status string) {
	h.status = status
}
