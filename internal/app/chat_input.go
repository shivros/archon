package app

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type ChatInput struct {
	input textinput.Model
}

func NewChatInput(width int) *ChatInput {
	input := newAddInput(width)
	input.Placeholder = ""
	return &ChatInput{input: input}
}

func (c *ChatInput) Resize(width int) {
	resizeAddInput(&c.input, width)
}

func (c *ChatInput) Focus() {
	c.input.Focus()
}

func (c *ChatInput) Blur() {
	c.input.Blur()
}

func (c *ChatInput) SetPlaceholder(value string) {
	c.input.Placeholder = value
}

func (c *ChatInput) SetValue(value string) {
	c.input.SetValue(value)
}

func (c *ChatInput) Value() string {
	return c.input.Value()
}

func (c *ChatInput) Clear() {
	c.input.SetValue("")
}

func (c *ChatInput) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return cmd
}

func (c *ChatInput) View() string {
	return c.input.View()
}
