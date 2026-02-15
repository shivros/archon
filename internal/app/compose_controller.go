package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type composeHost interface {
	sendMessageCmd(sessionID, text string) tea.Cmd
	exitCompose(status string)
	setStatus(status string)
}

type ComposeController struct {
	input      *TextInput
	sessionID  string
	sessionTag string
}

func NewComposeController(width int) *ComposeController {
	input := NewTextInput(width, TextInputConfig{Height: 1, SingleLine: true})
	input.SetPlaceholder("type a message")
	return &ComposeController{input: input}
}

func (c *ComposeController) Resize(width int) {
	if c.input != nil {
		c.input.Resize(width)
	}
}

func (c *ComposeController) Enter(sessionID, sessionTag string) {
	c.sessionID = sessionID
	c.sessionTag = sessionTag
	if c.input != nil {
		c.input.SetValue("")
		c.input.SetPlaceholder("type a message")
		c.input.Focus()
	}
}

func (c *ComposeController) Exit() {
	c.sessionID = ""
	c.sessionTag = ""
	if c.input != nil {
		c.input.SetValue("")
		c.input.Blur()
	}
}

func (c *ComposeController) SetSession(sessionID, sessionTag string) {
	c.sessionID = sessionID
	c.sessionTag = sessionTag
}

func (c *ComposeController) Update(msg tea.Msg, host composeHost) (bool, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "esc":
			host.exitCompose("compose canceled")
			return true, nil
		case "enter":
			text := strings.TrimSpace(c.value())
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
	if c.input != nil {
		return true, c.input.Update(msg)
	}
	return true, nil
}

func (c *ComposeController) View() string {
	label := "Compose"
	if strings.TrimSpace(c.sessionTag) != "" {
		label = "To: " + c.sessionTag
	}
	lines := []string{
		label,
		c.viewInput(),
		"",
		"Enter to send • Esc to cancel",
	}
	return strings.Join(lines, "\n")
}

func (c *ComposeController) value() string {
	if c.input == nil {
		return ""
	}
	return c.input.Value()
}

func (c *ComposeController) viewInput() string {
	if c.input == nil {
		return ""
	}
	return c.input.View()
}
