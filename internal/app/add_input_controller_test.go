package app

import (
	"strings"
	"testing"

	"control/internal/client"
	"control/internal/types"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAddWorkspaceControllerSupportsRemappedSubmit(t *testing.T) {
	controller := NewAddWorkspaceController(80)
	host := &stubAddWorkspaceHost{submitKey: "f6"}
	controller.Enter()
	if controller.input == nil {
		t.Fatalf("expected input")
	}
	controller.input.SetValue("/tmp/repo")

	handled, cmd := controller.Update(tea.KeyMsg{Type: tea.KeyF6}, host)
	if !handled {
		t.Fatalf("expected remapped submit key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command after first step")
	}
	if controller.step != 1 {
		t.Fatalf("expected second step after first submit, got %d", controller.step)
	}
	controller.input.SetValue("Repo Name")

	handled, cmd = controller.Update(tea.KeyMsg{Type: tea.KeyF6}, host)
	if !handled {
		t.Fatalf("expected remapped submit key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command from stub host")
	}
	if host.createPath != "/tmp/repo" || host.createName != "Repo Name" {
		t.Fatalf("unexpected create payload: path=%q name=%q", host.createPath, host.createName)
	}
}

func TestAddWorktreeControllerSupportsRemappedSubmit(t *testing.T) {
	controller := NewAddWorktreeController(80)
	host := &stubAddWorktreeHost{submitKey: "f6"}
	controller.Enter("ws1", "/tmp/repo")
	if controller.input == nil {
		t.Fatalf("expected input")
	}

	handled, cmd := controller.Update(tea.KeyMsg{Type: tea.KeyF6}, host)
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
	handled, cmd = controller.Update(tea.KeyMsg{Type: tea.KeyF6}, host)
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

type stubAddWorkspaceHost struct {
	submitKey  string
	status     string
	createPath string
	createName string
}

func (h *stubAddWorkspaceHost) createWorkspaceCmd(path, name string) tea.Cmd {
	h.createPath = path
	h.createName = name
	return nil
}

func (h *stubAddWorkspaceHost) exitAddWorkspace(status string) {
	h.status = status
}

func (h *stubAddWorkspaceHost) keyMatchesCommand(msg tea.KeyMsg, command, fallback string) bool {
	key := strings.TrimSpace(msg.String())
	if command == KeyCommandInputSubmit && strings.TrimSpace(h.submitKey) != "" {
		return key == strings.TrimSpace(h.submitKey)
	}
	return key == strings.TrimSpace(fallback)
}

func (h *stubAddWorkspaceHost) keyString(msg tea.KeyMsg) string {
	return msg.String()
}

func (h *stubAddWorkspaceHost) setStatus(status string) {
	h.status = status
}

type stubAddWorktreeHost struct {
	submitKey string
	status    string
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

func (h *stubAddWorktreeHost) keyMatchesCommand(msg tea.KeyMsg, command, fallback string) bool {
	key := strings.TrimSpace(msg.String())
	if command == KeyCommandInputSubmit && strings.TrimSpace(h.submitKey) != "" {
		return key == strings.TrimSpace(h.submitKey)
	}
	return key == strings.TrimSpace(fallback)
}

func (h *stubAddWorktreeHost) keyString(msg tea.KeyMsg) string {
	return msg.String()
}

func (h *stubAddWorktreeHost) setStatus(status string) {
	h.status = status
}
