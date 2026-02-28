package app

import (
	"strings"
	"testing"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

func TestEditWorkspaceControllerSubmitsAllWorkspaceFields(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	host := &stubEditWorkspaceHost{
		groups: []*types.WorkspaceGroup{
			{ID: "g1", Name: "Alpha"},
		},
	}
	ok := controller.Enter("ws1", &types.Workspace{
		ID:                    "ws1",
		RepoPath:              "/tmp/repo",
		SessionSubpath:        "packages/pennies",
		AdditionalDirectories: []string{"../backend", "../shared"},
		Name:                  "Repo",
		GroupIDs:              []string{"g1"},
	})
	if !ok {
		t.Fatalf("expected edit workspace controller to enter with workspace")
	}
	if controller.input == nil {
		t.Fatalf("expected input")
	}
	controller.input.SetValue("/tmp/repo-2")

	handled, cmd := controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command after path step")
	}
	if controller.step != 1 {
		t.Fatalf("expected subpath step, got %d", controller.step)
	}

	controller.input.SetValue("")
	handled, cmd = controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command after subpath step")
	}
	if controller.step != 2 {
		t.Fatalf("expected additional dirs step, got %d", controller.step)
	}

	controller.input.SetValue("")
	handled, cmd = controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command after additional dirs step")
	}
	if controller.step != 3 {
		t.Fatalf("expected name step, got %d", controller.step)
	}

	controller.input.SetValue("Renamed Repo")
	// Step 3 → step 4 (group picker)
	handled, cmd = controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command after name step")
	}
	if controller.step != 4 {
		t.Fatalf("expected group picker step, got %d", controller.step)
	}

	// Step 4: Enter confirms group picker (pre-selected groups remain)
	handled, cmd = controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	if !handled {
		t.Fatalf("expected enter to be handled on group picker step")
	}
	if cmd != nil {
		t.Fatalf("expected no async command from stub host")
	}
	if host.updateID != "ws1" {
		t.Fatalf("expected update workspace id ws1, got %q", host.updateID)
	}
	if host.updatePatch == nil {
		t.Fatalf("expected non-nil workspace patch")
	}
	if host.updatePatch.RepoPath == nil || *host.updatePatch.RepoPath != "/tmp/repo-2" {
		t.Fatalf("expected updated path, got %#v", host.updatePatch.RepoPath)
	}
	if host.updatePatch.SessionSubpath == nil || *host.updatePatch.SessionSubpath != "" {
		t.Fatalf("expected cleared session subpath, got %#v", host.updatePatch.SessionSubpath)
	}
	if host.updatePatch.AdditionalDirectories == nil || len(*host.updatePatch.AdditionalDirectories) != 0 {
		t.Fatalf("expected additional directories to clear, got %#v", host.updatePatch.AdditionalDirectories)
	}
	if host.updatePatch.Name == nil || *host.updatePatch.Name != "Renamed Repo" {
		t.Fatalf("expected updated name, got %#v", host.updatePatch.Name)
	}
	if host.updatePatch.GroupIDs == nil || len(*host.updatePatch.GroupIDs) != 1 || (*host.updatePatch.GroupIDs)[0] != "g1" {
		t.Fatalf("expected pre-selected group g1 in patch, got %#v", host.updatePatch.GroupIDs)
	}
}

func TestEditWorkspaceControllerAllowsIDWithoutPrefill(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	ok := controller.Enter("ws1", nil)
	if !ok {
		t.Fatalf("expected controller to enter with workspace id only")
	}
	if controller.workspaceID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", controller.workspaceID)
	}
	if got := strings.TrimSpace(controller.path); got != "" {
		t.Fatalf("expected empty path prefill when workspace payload is missing, got %q", got)
	}
}

func TestEditWorkspaceControllerExitClearsInputAndState(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	ok := controller.Enter("ws1", &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
		Name:     "Repo",
	})
	if !ok {
		t.Fatalf("expected controller to enter")
	}
	if controller.input == nil {
		t.Fatalf("expected input")
	}
	controller.input.SetValue("/tmp/repo-updated")
	controller.Exit()
	if controller.workspaceID != "" {
		t.Fatalf("expected workspace id to clear")
	}
	if controller.path != "" || controller.sub != "" || controller.dirs != "" || controller.name != "" {
		t.Fatalf("expected controller fields to clear")
	}
	if got := controller.input.Value(); got != "" {
		t.Fatalf("expected input value to clear, got %q", got)
	}
}

func TestEditWorkspaceControllerUpdateEscCancels(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	host := &stubEditWorkspaceHost{}
	ok := controller.Enter("ws1", &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
	})
	if !ok {
		t.Fatalf("expected controller to enter")
	}

	handled, cmd := controller.Update(tea.KeyPressMsg{Code: tea.KeyEsc}, host)
	if !handled {
		t.Fatalf("expected esc to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command on esc")
	}
	if host.status != "edit workspace canceled" {
		t.Fatalf("unexpected cancel status: %q", host.status)
	}
}

func TestEditWorkspaceControllerViewContainsInstructions(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	ok := controller.Enter("ws1", &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
	})
	if !ok {
		t.Fatalf("expected controller to enter")
	}
	view := controller.View()
	if !strings.Contains(view, "Path:") {
		t.Fatalf("expected path field in view: %q", view)
	}
	if !strings.Contains(view, "Enter to continue") {
		t.Fatalf("expected footer instructions in view: %q", view)
	}
}

func TestEditWorkspaceControllerUpdateHandlesNonKeyMessages(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	host := &stubEditWorkspaceHost{}
	ok := controller.Enter("ws1", &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
	})
	if !ok {
		t.Fatalf("expected controller to enter")
	}
	handled, cmd := controller.Update(tea.WindowSizeMsg{Width: 80, Height: 20}, host)
	if !handled {
		t.Fatalf("expected non-key message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for non-key message")
	}
}

func TestEditWorkspaceControllerEnterRejectsBlankWorkspaceID(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	ok := controller.Enter("   ", &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
	})
	if ok {
		t.Fatalf("expected enter to reject blank workspace id")
	}
}

func TestEditWorkspaceControllerUpdateSwallowsToggleSidebarHotkey(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	host := &stubEditWorkspaceHost{stubKeyResolver: stubKeyResolver{toggleSidebarMatch: true}}
	ok := controller.Enter("ws1", &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
	})
	if !ok {
		t.Fatalf("expected controller to enter")
	}
	if controller.step != 0 {
		t.Fatalf("expected initial step 0, got %d", controller.step)
	}
	handled, cmd := controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	if !handled {
		t.Fatalf("expected key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when toggle sidebar key is swallowed")
	}
	if controller.step != 0 {
		t.Fatalf("expected step to remain unchanged, got %d", controller.step)
	}
}

func TestEditWorkspaceControllerAdvanceRequiresPath(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	host := &stubEditWorkspaceHost{}
	ok := controller.Enter("ws1", &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
	})
	if !ok {
		t.Fatalf("expected controller to enter")
	}

	controller.input.SetValue("   ")
	handled, cmd := controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	if !handled {
		t.Fatalf("expected submit to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when path is invalid")
	}
	if host.status != "path is required" {
		t.Fatalf("unexpected status: %q", host.status)
	}
	if controller.step != 0 {
		t.Fatalf("expected controller to remain on path step, got %d", controller.step)
	}
}

func TestEditWorkspaceControllerAdvanceRequiresWorkspaceIDOnSubmit(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	host := &stubEditWorkspaceHost{}
	ok := controller.Enter("ws1", &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
	})
	if !ok {
		t.Fatalf("expected controller to enter")
	}

	// The workspace ID check now happens at step 4 (group picker confirm).
	controller.step = 4
	controller.workspaceID = "   "
	handled, cmd := controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	if !handled {
		t.Fatalf("expected submit to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when workspace id is missing")
	}
	if host.status != "no workspace selected" {
		t.Fatalf("unexpected status: %q", host.status)
	}
	if host.updatePatch != nil {
		t.Fatalf("expected no patch submission when workspace id is missing")
	}
}

func TestWorkspacePatchFromFormClearsEmptyAdditionalDirectories(t *testing.T) {
	patch := workspacePatchFromForm(workspaceFormData{Path: "/tmp/repo", AdditionalDirectoriesRaw: " ", Name: "Repo"})
	if patch == nil || patch.AdditionalDirectories == nil {
		t.Fatalf("expected non-nil additional directories patch")
	}
	if len(*patch.AdditionalDirectories) != 0 {
		t.Fatalf("expected empty slice for clear semantics, got %#v", patch.AdditionalDirectories)
	}
}

func TestWorkspacePatchFromFormTrimsValuesAndParsesDirectories(t *testing.T) {
	patch := workspacePatchFromForm(workspaceFormData{
		Path:                     " /tmp/repo ",
		SessionSubpath:           " packages/pennies ",
		AdditionalDirectoriesRaw: " ../backend,  ../shared  ",
		Name:                     " Repo ",
	})
	if patch == nil {
		t.Fatalf("expected non-nil patch")
	}
	if patch.RepoPath == nil || *patch.RepoPath != "/tmp/repo" {
		t.Fatalf("unexpected repo path: %#v", patch.RepoPath)
	}
	if patch.SessionSubpath == nil || *patch.SessionSubpath != "packages/pennies" {
		t.Fatalf("unexpected session subpath: %#v", patch.SessionSubpath)
	}
	if patch.Name == nil || *patch.Name != "Repo" {
		t.Fatalf("unexpected name: %#v", patch.Name)
	}
	if patch.AdditionalDirectories == nil {
		t.Fatalf("expected additional directories")
	}
	if len(*patch.AdditionalDirectories) != 2 {
		t.Fatalf("expected 2 additional directories, got %#v", patch.AdditionalDirectories)
	}
	if (*patch.AdditionalDirectories)[0] != "../backend" || (*patch.AdditionalDirectories)[1] != "../shared" {
		t.Fatalf("unexpected additional directories order/content: %#v", patch.AdditionalDirectories)
	}
}

func TestEditWorkspaceControllerPrepareInputDefaultStepClearsValue(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	controller.input.SetValue("stale")
	controller.step = 99

	controller.prepareInput()

	if got := controller.input.Value(); got != "" {
		t.Fatalf("expected default step to clear input, got %q", got)
	}
}

func TestEditWorkspaceControllerValueReturnsEmptyWhenInputMissing(t *testing.T) {
	controller := &EditWorkspaceController{}
	if got := controller.value(); got != "" {
		t.Fatalf("expected empty value when input missing, got %q", got)
	}
}

func TestEditWorkspaceControllerUpdateHandlesNilInput(t *testing.T) {
	controller := &EditWorkspaceController{}
	host := &stubEditWorkspaceHost{}
	handled, cmd := controller.Update(tea.WindowSizeMsg{Width: 80, Height: 20}, host)
	if !handled {
		t.Fatalf("expected nil-input update to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command for nil-input update")
	}
}

func TestEditWorkspaceControllerGroupPickerPreSelectsExistingGroups(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	host := &stubEditWorkspaceHost{
		groups: []*types.WorkspaceGroup{
			{ID: "g1", Name: "Alpha"},
			{ID: "g2", Name: "Beta"},
		},
	}
	ok := controller.Enter("ws1", &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
		Name:     "Repo",
		GroupIDs: []string{"g2"},
	})
	if !ok {
		t.Fatalf("expected controller to enter")
	}
	controller.input.SetValue("/tmp/repo")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // step 0 → 1
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // step 1 → 2
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // step 2 → 3
	controller.input.SetValue("Repo")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // step 3 → 4

	if controller.step != 4 {
		t.Fatalf("expected step 4, got %d", controller.step)
	}

	// Confirm without changing selection — pre-selected g2 should carry through
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)

	if host.updatePatch == nil || host.updatePatch.GroupIDs == nil {
		t.Fatalf("expected group IDs in patch")
	}
	if len(*host.updatePatch.GroupIDs) != 1 || (*host.updatePatch.GroupIDs)[0] != "g2" {
		t.Fatalf("expected pre-selected group g2 in patch, got %#v", host.updatePatch.GroupIDs)
	}
}

func TestEditWorkspaceControllerGroupPickerToggle(t *testing.T) {
	controller := NewEditWorkspaceController(80)
	host := &stubEditWorkspaceHost{
		groups: []*types.WorkspaceGroup{
			{ID: "g1", Name: "Alpha"},
			{ID: "g2", Name: "Beta"},
		},
	}
	ok := controller.Enter("ws1", &types.Workspace{
		ID:       "ws1",
		RepoPath: "/tmp/repo",
		Name:     "Repo",
	})
	if !ok {
		t.Fatalf("expected controller to enter")
	}
	controller.input.SetValue("/tmp/repo")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)
	controller.input.SetValue("Repo")
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host) // → step 4

	// Toggle first group
	controller.Update(tea.KeyPressMsg{Code: ' '}, host)

	// Confirm
	controller.Update(tea.KeyPressMsg{Code: tea.KeyEnter}, host)

	if host.updatePatch == nil || host.updatePatch.GroupIDs == nil {
		t.Fatalf("expected group IDs in patch")
	}
	if len(*host.updatePatch.GroupIDs) != 1 || (*host.updatePatch.GroupIDs)[0] != "g1" {
		t.Fatalf("expected toggled group g1, got %#v", host.updatePatch.GroupIDs)
	}
}

type stubEditWorkspaceHost struct {
	stubKeyResolver
	status      string
	updateID    string
	updatePatch *types.WorkspacePatch
	groups      []*types.WorkspaceGroup
}

func (h *stubEditWorkspaceHost) updateWorkspaceCmd(id string, patch *types.WorkspacePatch) tea.Cmd {
	h.updateID = id
	h.updatePatch = patch
	return nil
}

func (h *stubEditWorkspaceHost) exitEditWorkspace(status string) {
	h.status = status
}

func (h *stubEditWorkspaceHost) setStatus(status string) {
	h.status = status
}

func (h *stubEditWorkspaceHost) workspaceGroups() []*types.WorkspaceGroup {
	return h.groups
}
