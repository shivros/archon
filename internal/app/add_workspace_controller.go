package app

import (
	"fmt"
	"strings"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

type addWorkspaceHost interface {
	KeyResolver
	createWorkspaceCmd(path, sessionSubpath, name string, additionalDirectories, groupIDs []string) tea.Cmd
	exitAddWorkspace(status string)
	setStatus(status string)
	workspaceGroups() []*types.WorkspaceGroup
}

type AddWorkspaceController struct {
	input       *TextInput
	groupPicker *GroupPicker
	step        int
	path        string
	sub         string
	dirs        string
	name        string
	groupIDs    []string
}

func NewAddWorkspaceController(width int) *AddWorkspaceController {
	input := newAddInput(width)
	input.SetPlaceholder("/path/to/repo")
	picker := NewGroupPicker(width, 8)
	return &AddWorkspaceController{input: input, groupPicker: picker}
}

func (c *AddWorkspaceController) Resize(width int) {
	if c.input != nil {
		c.input.Resize(width)
	}
	if c.groupPicker != nil {
		c.groupPicker.SetSize(width, 8)
	}
}

func (c *AddWorkspaceController) Enter() {
	c.step = 0
	c.path = ""
	c.sub = ""
	c.dirs = ""
	c.name = ""
	c.groupIDs = nil
	c.prepareInput()
	if c.input != nil {
		c.input.Focus()
	}
}

func (c *AddWorkspaceController) Exit() {
	c.step = 0
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

func (c *AddWorkspaceController) Update(msg tea.Msg, host addWorkspaceHost) (bool, tea.Cmd) {
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
			host.exitAddWorkspace("add workspace canceled")
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

func (c *AddWorkspaceController) updateGroupPickerStep(msg tea.Msg, host addWorkspaceHost) (bool, tea.Cmd) {
	h := groupPickerStepHandler{
		picker:    c.groupPicker,
		keys:      host,
		setStatus: host.setStatus,
		onCancel:  func() { host.exitAddWorkspace("add workspace canceled") },
		onConfirm: func() tea.Cmd { return c.advance(host) },
	}
	return h.Update(msg)
}

func (c *AddWorkspaceController) View() string {
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

func (c *AddWorkspaceController) advance(host addWorkspaceHost) tea.Cmd {
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
		host.setStatus("add workspace: session subpath (optional)")
		return nil
	case 1:
		c.sub = strings.TrimSpace(c.value())
		c.step = 2
		c.prepareInput()
		host.setStatus("add workspace: additional directories (optional)")
		return nil
	case 2:
		c.dirs = strings.TrimSpace(c.value())
		c.step = 3
		c.prepareInput()
		host.setStatus("add workspace: name (optional)")
		return nil
	case 3:
		c.name = strings.TrimSpace(c.value())
		c.step = 4
		if c.input != nil {
			c.input.Blur()
		}
		if c.groupPicker != nil {
			c.groupPicker.ClearQuery()
			c.groupPicker.SetGroups(host.workspaceGroups(), nil)
		}
		host.setStatus("add workspace: groups (optional)")
		return nil
	case 4:
		if c.groupPicker != nil {
			c.groupIDs = c.groupPicker.SelectedIDs()
		}
		host.setStatus("creating workspace")
		return host.createWorkspaceCmd(c.path, c.sub, c.name, parseAdditionalDirectories(c.dirs), c.groupIDs)
	default:
		return nil
	}
}

func (c *AddWorkspaceController) prepareInput() {
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
	}
}

func renderAddField(input *TextInput, currentStep int, label, value string, step int) string {
	if currentStep == step {
		if input == nil {
			return fmt.Sprintf("%s: <empty>", label)
		}
		return fmt.Sprintf("%s: %s", label, input.View())
	}
	if strings.TrimSpace(value) == "" {
		value = "<empty>"
	}
	return fmt.Sprintf("%s: %s", label, value)
}

func (c *AddWorkspaceController) value() string {
	if c.input == nil {
		return ""
	}
	return c.input.Value()
}

func newAddInput(width int) *TextInput {
	return NewTextInput(width, TextInputConfig{Height: 1, SingleLine: true})
}

func parseAdditionalDirectories(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
