package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type composeHost interface {
	sendMessageCmd(sessionID, text string) tea.Cmd
	exitCompose(status string)
	setStatus(status string)
}

type ComposeController struct {
	input      textinput.Model
	sessionID  string
	sessionTag string
}

func NewComposeController(width int) *ComposeController {
	input := newAddInput(width)
	input.Placeholder = "type a message"
	return &ComposeController{input: input}
}

func (c *ComposeController) Resize(width int) {
	resizeAddInput(&c.input, width)
}

func (c *ComposeController) Enter(sessionID, sessionTag string) {
	c.sessionID = sessionID
	c.sessionTag = sessionTag
	c.input.SetValue("")
	c.input.Placeholder = "type a message"
	c.input.Focus()
}

func (c *ComposeController) Exit() {
	c.sessionID = ""
	c.sessionTag = ""
	c.input.SetValue("")
	c.input.Blur()
}

func (c *ComposeController) Update(msg tea.Msg, host composeHost) (bool, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			host.exitCompose("compose canceled")
			return true, nil
		case "enter":
			text := strings.TrimSpace(c.input.Value())
			if text == "" {
				host.setStatus("message is required")
				return true, nil
			}
			sessionID := c.sessionID
			host.setStatus("sending message")
			host.exitCompose("")
			return true, host.sendMessageCmd(sessionID, text)
		}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return true, cmd
}

func (c *ComposeController) View() string {
	label := "Compose"
	if strings.TrimSpace(c.sessionTag) != "" {
		label = "To: " + c.sessionTag
	}
	lines := []string{
		label,
		c.input.View(),
		"",
		"Enter to send • Esc to cancel",
	}
	return strings.Join(lines, "\n")
}
