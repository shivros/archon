package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type addWorkspaceHost interface {
	createWorkspaceCmd(path, name string) tea.Cmd
	exitAddWorkspace(status string)
	setStatus(status string)
}

type AddWorkspaceController struct {
	input textinput.Model
	step  int
	path  string
	name  string
}

func NewAddWorkspaceController(width int) *AddWorkspaceController {
	input := newAddInput(width)
	input.Placeholder = "/path/to/repo"
	return &AddWorkspaceController{input: input}
}

func (c *AddWorkspaceController) Resize(width int) {
	resizeAddInput(&c.input, width)
}

func (c *AddWorkspaceController) Enter() {
	c.step = 0
	c.path = ""
	c.name = ""
	c.prepareInput()
	c.input.Focus()
}

func (c *AddWorkspaceController) Exit() {
	c.step = 0
	c.path = ""
	c.name = ""
	c.input.SetValue("")
	c.input.Blur()
}

func (c *AddWorkspaceController) Update(msg tea.Msg, host addWorkspaceHost) (bool, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			host.exitAddWorkspace("add workspace canceled")
			return true, nil
		case "enter":
			return true, c.advance(host)
		case "ctrl+b":
			// Swallow global hotkey while typing.
			return true, nil
		}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return true, cmd
}

func (c *AddWorkspaceController) View() string {
	lines := []string{
		renderAddField(&c.input, c.step, "Path", c.path, 0),
		renderAddField(&c.input, c.step, "Name", c.name, 1),
		"",
		"Enter to continue • Esc to cancel",
	}
	return strings.Join(lines, "\n")
}

func (c *AddWorkspaceController) advance(host addWorkspaceHost) tea.Cmd {
	switch c.step {
	case 0:
		path := strings.TrimSpace(c.input.Value())
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
		c.name = strings.TrimSpace(c.input.Value())
		host.setStatus("creating workspace")
		return host.createWorkspaceCmd(c.path, c.name)
	default:
		return nil
	}
}

func (c *AddWorkspaceController) prepareInput() {
	switch c.step {
	case 0:
		c.input.Placeholder = "/path/to/repo"
		c.input.SetValue(c.path)
	case 1:
		c.input.Placeholder = "optional name"
		c.input.SetValue(c.name)
	}
}

func renderAddField(input *textinput.Model, currentStep int, label, value string, step int) string {
	if currentStep == step {
		return fmt.Sprintf("%s: %s", label, input.View())
	}
	if strings.TrimSpace(value) == "" {
		value = "<empty>"
	}
	return fmt.Sprintf("%s: %s", label, value)
}

func newAddInput(width int) textinput.Model {
	input := textinput.New()
	input.Prompt = ""
	input.CharLimit = 0
	input.Width = max(0, width-4)
	return input
}

func resizeAddInput(input *textinput.Model, width int) {
	if width > 4 {
		input.Width = width - 4
	}
}
