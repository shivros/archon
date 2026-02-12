package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type addWorkspaceHost interface {
	createWorkspaceCmd(path, name string) tea.Cmd
	exitAddWorkspace(status string)
	keyMatchesCommand(msg tea.KeyMsg, command, fallback string) bool
	keyString(msg tea.KeyMsg) string
	setStatus(status string)
}

type AddWorkspaceController struct {
	input *TextInput
	step  int
	path  string
	name  string
}

func NewAddWorkspaceController(width int) *AddWorkspaceController {
	input := newAddInput(width)
	input.SetPlaceholder("/path/to/repo")
	return &AddWorkspaceController{input: input}
}

func (c *AddWorkspaceController) Resize(width int) {
	if c.input != nil {
		c.input.Resize(width)
	}
}

func (c *AddWorkspaceController) Enter() {
	c.step = 0
	c.path = ""
	c.name = ""
	c.prepareInput()
	if c.input != nil {
		c.input.Focus()
	}
}

func (c *AddWorkspaceController) Exit() {
	c.step = 0
	c.path = ""
	c.name = ""
	if c.input != nil {
		c.input.SetValue("")
		c.input.Blur()
	}
}

func (c *AddWorkspaceController) Update(msg tea.Msg, host addWorkspaceHost) (bool, tea.Cmd) {
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

func (c *AddWorkspaceController) View() string {
	lines := []string{
		renderAddField(c.input, c.step, "Path", c.path, 0),
		renderAddField(c.input, c.step, "Name", c.name, 1),
		"",
		"Enter to continue • Esc to cancel",
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
		host.setStatus("add workspace: name (optional)")
		return nil
	case 1:
		c.name = strings.TrimSpace(c.value())
		host.setStatus("creating workspace")
		return host.createWorkspaceCmd(c.path, c.name)
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
