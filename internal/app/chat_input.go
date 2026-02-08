package app

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

type ChatInput struct {
	input  textarea.Model
	height int
	width  int
}

type ChatInputConfig struct {
	Height int
}

func DefaultChatInputConfig() ChatInputConfig {
	return ChatInputConfig{Height: 3}
}

func NewChatInput(width int, cfg ChatInputConfig) *ChatInput {
	input := textarea.New()
	input.Prompt = ""
	input.ShowLineNumbers = false
	input.CharLimit = 0
	height := cfg.Height
	if height <= 0 {
		height = 1
	}
	input.SetHeight(height)
	inputWidth := setTextareaWidth(&input, width)
	return &ChatInput{input: input, height: height, width: inputWidth}
}

func (c *ChatInput) Resize(width int) {
	c.width = setTextareaWidth(&c.input, width)
	c.input.SetHeight(c.height)
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
	c.sanitize()
	return cmd
}

func (c *ChatInput) View() string {
	return c.input.View()
}

func (c *ChatInput) Height() int {
	if c == nil {
		return 0
	}
	return c.height
}

func (c *ChatInput) Focused() bool {
	if c == nil {
		return false
	}
	return c.input.Focused()
}

func (c *ChatInput) CanScroll() bool {
	if c == nil || c.height <= 0 || c.width <= 0 {
		return false
	}
	lines := wrappedLineCount(c.input.Value(), c.width)
	return lines > c.height
}

func (c *ChatInput) Scroll(lines int) tea.Cmd {
	if c == nil || lines == 0 {
		return nil
	}
	wasFocused := c.input.Focused()
	if !wasFocused {
		c.input.Focus()
	}
	var cmd tea.Cmd
	steps := lines
	if steps < 0 {
		steps = -steps
	}
	for i := 0; i < steps; i++ {
		key := tea.KeyDown
		if lines < 0 {
			key = tea.KeyUp
		}
		c.input, cmd = c.input.Update(tea.KeyMsg{Type: key})
	}
	c.sanitize()
	if !wasFocused {
		c.input.Blur()
	}
	return cmd
}

func setTextareaWidth(input *textarea.Model, width int) int {
	if input == nil {
		return 0
	}
	if width > 4 {
		width = width - 4
	}
	width = max(1, width)
	input.SetWidth(width)
	return width
}

func wrappedLineCount(value string, width int) int {
	if width <= 0 {
		return 1
	}
	if value == "" {
		return 1
	}
	lines := strings.Split(value, "\n")
	count := 0
	for _, line := range lines {
		if line == "" {
			count++
			continue
		}
		w := runewidth.StringWidth(line)
		if w == 0 {
			count++
			continue
		}
		count += (w-1)/width + 1
	}
	return max(1, count)
}

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func (c *ChatInput) sanitize() {
	if c == nil {
		return
	}
	value := c.input.Value()
	if value == "" {
		return
	}
	cleaned := sanitizeChatInput(value)
	if cleaned != value {
		c.input.SetValue(cleaned)
	}
}

func sanitizeChatInput(value string) string {
	value = ansiEscapeRE.ReplaceAllString(value, "")
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if r == '\n' {
			b.WriteRune(r)
			continue
		}
		if r < 32 || r == 127 {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
