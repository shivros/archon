package app

import (
	"strings"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

type editWorkspaceHost interface {
	KeyResolver
	updateWorkspaceCmd(id string, patch *types.WorkspacePatch) tea.Cmd
	exitEditWorkspace(status string)
	setStatus(status string)
	workspaceGroups() []*types.WorkspaceGroup
}

type EditWorkspaceController struct {
	input       *TextInput
	groupPicker *GroupPicker
	step        int
	workspaceID string
	path        string
	sub         string
	dirs        string
	name        string
	groupIDs    []string
}

func NewEditWorkspaceController(width int) *EditWorkspaceController {
	input := newAddInput(width)
	input.SetPlaceholder("/path/to/repo")
	picker := NewGroupPicker(width, 8)
	return &EditWorkspaceController{input: input, groupPicker: picker}
}

func (c *EditWorkspaceController) Resize(width int) {
	if c.input != nil {
		c.input.Resize(width)
	}
	if c.groupPicker != nil {
		c.groupPicker.SetSize(width, 8)
	}
}

func (c *EditWorkspaceController) Enter(workspaceID string, workspace *types.Workspace) bool {
	c.step = 0
	c.workspaceID = strings.TrimSpace(workspaceID)
	c.path = ""
	c.sub = ""
	c.dirs = ""
	c.name = ""
	c.groupIDs = nil
	if c.workspaceID == "" {
		return false
	}
	if workspace == nil {
		c.prepareInput()
		if c.input != nil {
			c.input.Focus()
		}
		return true
	}
	if strings.TrimSpace(workspace.ID) != "" {
		c.workspaceID = strings.TrimSpace(workspace.ID)
	}
	c.path = strings.TrimSpace(workspace.RepoPath)
	c.sub = strings.TrimSpace(workspace.SessionSubpath)
	if len(workspace.AdditionalDirectories) > 0 {
		c.dirs = strings.Join(workspace.AdditionalDirectories, ", ")
	}
	c.name = strings.TrimSpace(workspace.Name)
	c.groupIDs = append([]string(nil), workspace.GroupIDs...)
	c.prepareInput()
	if c.input != nil {
		c.input.Focus()
	}
	return true
}

func (c *EditWorkspaceController) Exit() {
	c.step = 0
	c.workspaceID = ""
	c.path = ""
	c.sub = ""
	c.dirs = ""
	c.name = ""
	c.groupIDs = nil
	if c.input != nil {
		c.input.SetValue("")
		c.input.Blur()
	}
	if c.groupPicker != nil {
		c.groupPicker.ClearQuery()
		c.groupPicker.SetGroups(nil, nil)
	}
}

func (c *EditWorkspaceController) Update(msg tea.Msg, host editWorkspaceHost) (bool, tea.Cmd) {
	if c.step == 4 {
		return c.updateGroupPickerStep(msg, host)
	}
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if host.keyMatchesCommand(keyMsg, KeyCommandToggleSidebar, "ctrl+b") {
			// Swallow global hotkey while typing.
			return true, nil
		}
		switch host.keyString(keyMsg) {
		case "esc":
			host.exitEditWorkspace("edit workspace canceled")
			return true, nil
		}
		controller := textInputModeController{
			input:             c.input,
			keyString:         host.keyString,
			keyMatchesCommand: host.keyMatchesCommand,
			onSubmit: func(string) tea.Cmd {
				return c.advance(host)
			},
		}
		return controller.Update(keyMsg)
	}
	if c.input != nil {
		return true, c.input.Update(msg)
	}
	return true, nil
}

func (c *EditWorkspaceController) updateGroupPickerStep(msg tea.Msg, host editWorkspaceHost) (bool, tea.Cmd) {
	h := groupPickerStepHandler{
		picker:    c.groupPicker,
		keys:      host,
		setStatus: host.setStatus,
		onCancel:  func() { host.exitEditWorkspace("edit workspace canceled") },
		onConfirm: func() tea.Cmd { return c.advance(host) },
	}
	return h.Update(msg)
}

func (c *EditWorkspaceController) View() string {
	lines := []string{
		renderAddField(c.input, c.step, "Path", c.path, 0),
		renderAddField(c.input, c.step, "Session Subpath", c.sub, 1),
		renderAddField(c.input, c.step, "Additional Dirs", c.dirs, 2),
		renderAddField(c.input, c.step, "Name", c.name, 3),
	}
	if c.step == 4 {
		lines = append(lines, "Groups:")
		if c.groupPicker != nil {
			lines = append(lines, c.groupPicker.View())
		}
		lines = append(lines, "", "Space to toggle • Enter to continue • Esc to cancel")
	} else {
		lines = append(lines, "", "Enter to continue • Esc to cancel")
	}
	return strings.Join(lines, "\n")
}

func (c *EditWorkspaceController) advance(host editWorkspaceHost) tea.Cmd {
	switch c.step {
	case 0:
		path := strings.TrimSpace(c.value())
		if path == "" {
			host.setStatus("path is required")
			return nil
		}
		c.path = path
		c.step = 1
		c.prepareInput()
		host.setStatus("edit workspace: session subpath (optional)")
		return nil
	case 1:
		c.sub = strings.TrimSpace(c.value())
		c.step = 2
		c.prepareInput()
		host.setStatus("edit workspace: additional directories (optional)")
		return nil
	case 2:
		c.dirs = strings.TrimSpace(c.value())
		c.step = 3
		c.prepareInput()
		host.setStatus("edit workspace: name (optional)")
		return nil
	case 3:
		c.name = strings.TrimSpace(c.value())
		c.step = 4
		if c.input != nil {
			c.input.Blur()
		}
		selected := map[string]bool{}
		for _, id := range c.groupIDs {
			selected[id] = true
		}
		if c.groupPicker != nil {
			c.groupPicker.ClearQuery()
			c.groupPicker.SetGroups(host.workspaceGroups(), selected)
		}
		host.setStatus("edit workspace: groups (optional)")
		return nil
	case 4:
		if strings.TrimSpace(c.workspaceID) == "" {
			host.setStatus("no workspace selected")
			return nil
		}
		if c.groupPicker != nil {
			c.groupIDs = c.groupPicker.SelectedIDs()
		}
		host.setStatus("updating workspace")
		return host.updateWorkspaceCmd(c.workspaceID, workspacePatchFromForm(workspaceFormData{
			Path:                     c.path,
			SessionSubpath:           c.sub,
			AdditionalDirectoriesRaw: c.dirs,
			Name:                     c.name,
			GroupIDs:                 c.groupIDs,
		}))
	default:
		return nil
	}
}

func (c *EditWorkspaceController) prepareInput() {
	if c.input == nil {
		return
	}
	switch c.step {
	case 0:
		c.input.SetPlaceholder("/path/to/repo")
		c.input.SetValue(c.path)
	case 1:
		c.input.SetPlaceholder("packages/pennies (optional)")
		c.input.SetValue(c.sub)
	case 2:
		c.input.SetPlaceholder("../backend, ../shared (optional)")
		c.input.SetValue(c.dirs)
	case 3:
		c.input.SetPlaceholder("optional name")
		c.input.SetValue(c.name)
	default:
		c.input.SetValue("")
	}
}

func (c *EditWorkspaceController) value() string {
	if c.input == nil {
		return ""
	}
	return c.input.Value()
}
