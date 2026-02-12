package app

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

type TextInput struct {
	input       textarea.Model
	height      int
	width       int
	singleLine  bool
	allSelected bool
	undoHistory []string
	redoHistory []string
}

type TextInputConfig struct {
	Height     int
	SingleLine bool
}

func DefaultTextInputConfig() TextInputConfig {
	return TextInputConfig{Height: 3}
}

func NewTextInput(width int, cfg TextInputConfig) *TextInput {
	input := textarea.New()
	input.Prompt = ""
	input.ShowLineNumbers = false
	input.CharLimit = 0
	applyWordNavigationAliases(&input)
	height := cfg.Height
	if height <= 0 {
		height = 1
	}
	input.SetHeight(height)
	inputWidth := setTextareaWidth(&input, width)
	return &TextInput{input: input, height: height, width: inputWidth, singleLine: cfg.SingleLine}
}

func (c *TextInput) Resize(width int) {
	c.width = setTextareaWidth(&c.input, width)
	c.input.SetHeight(c.height)
}

func (c *TextInput) Focus() {
	c.input.Focus()
}

func (c *TextInput) Blur() {
	c.input.Blur()
}

func (c *TextInput) SetPlaceholder(value string) {
	c.input.Placeholder = value
}

func (c *TextInput) SetValue(value string) {
	c.input.SetValue(value)
	c.sanitize()
	c.allSelected = false
}

func (c *TextInput) Value() string {
	return c.input.Value()
}

func (c *TextInput) Clear() {
	c.input.SetValue("")
	c.allSelected = false
}

func (c *TextInput) Update(msg tea.Msg) tea.Cmd {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		processed, consume := c.preprocessKeyMsg(keyMsg)
		if consume {
			return nil
		}
		msg = processed
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	c.sanitize()
	return cmd
}

func (c *TextInput) View() string {
	return c.input.View()
}

func (c *TextInput) Height() int {
	if c == nil {
		return 0
	}
	return c.height
}

func (c *TextInput) Focused() bool {
	if c == nil {
		return false
	}
	return c.input.Focused()
}

func (c *TextInput) CanScroll() bool {
	if c == nil || c.height <= 0 || c.width <= 0 {
		return false
	}
	lines := wrappedLineCount(c.input.Value(), c.width)
	return lines > c.height
}

func (c *TextInput) Scroll(lines int) tea.Cmd {
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

func (c *TextInput) InsertNewline() tea.Cmd {
	if c == nil {
		return nil
	}
	return c.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

func (c *TextInput) MoveWordLeft() tea.Cmd {
	if c == nil {
		return nil
	}
	c.allSelected = false
	return c.Update(tea.KeyMsg{Type: tea.KeyLeft, Alt: true})
}

func (c *TextInput) MoveWordRight() tea.Cmd {
	if c == nil {
		return nil
	}
	c.allSelected = false
	return c.Update(tea.KeyMsg{Type: tea.KeyRight, Alt: true})
}

func (c *TextInput) DeleteWordLeft() tea.Cmd {
	if c == nil {
		return nil
	}
	return c.Update(tea.KeyMsg{Type: tea.KeyBackspace, Alt: true})
}

func (c *TextInput) DeleteWordRight() tea.Cmd {
	if c == nil {
		return nil
	}
	return c.Update(tea.KeyMsg{Type: tea.KeyDelete, Alt: true})
}

func (c *TextInput) SelectAll() bool {
	if c == nil {
		return false
	}
	c.allSelected = true
	return true
}

func (c *TextInput) Undo() bool {
	if c == nil || len(c.undoHistory) == 0 {
		return false
	}
	current := c.input.Value()
	prev := c.undoHistory[len(c.undoHistory)-1]
	c.undoHistory = c.undoHistory[:len(c.undoHistory)-1]
	c.pushRedoHistory(current)
	c.input.SetValue(prev)
	c.sanitize()
	c.allSelected = false
	return true
}

func (c *TextInput) Redo() bool {
	if c == nil || len(c.redoHistory) == 0 {
		return false
	}
	current := c.input.Value()
	next := c.redoHistory[len(c.redoHistory)-1]
	c.redoHistory = c.redoHistory[:len(c.redoHistory)-1]
	c.pushUndoHistory(current)
	c.input.SetValue(next)
	c.sanitize()
	c.allSelected = false
	return true
}

const textInputHistoryLimit = 200

func (c *TextInput) preprocessKeyMsg(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	if c == nil {
		return msg, false
	}
	if c.singleLine && isNewlineKey(msg) {
		return msg, true
	}
	recordedMutation := false
	if c.allSelected {
		switch {
		case isDeletionKey(msg):
			c.recordMutationSnapshot()
			recordedMutation = true
			c.input.SetValue("")
			c.allSelected = false
			c.sanitize()
			return msg, true
		case isMutationKey(msg):
			c.recordMutationSnapshot()
			recordedMutation = true
			c.input.SetValue("")
			c.allSelected = false
		case isNavigationKey(msg):
			c.allSelected = false
		}
	}
	if isMutationKey(msg) && !recordedMutation {
		c.recordMutationSnapshot()
	}
	return msg, false
}

func (c *TextInput) recordMutationSnapshot() {
	if c == nil {
		return
	}
	c.pushUndoHistory(c.input.Value())
	c.redoHistory = nil
}

func (c *TextInput) pushUndoHistory(value string) {
	if c == nil {
		return
	}
	if n := len(c.undoHistory); n > 0 && c.undoHistory[n-1] == value {
		return
	}
	c.undoHistory = append(c.undoHistory, value)
	if len(c.undoHistory) > textInputHistoryLimit {
		c.undoHistory = c.undoHistory[len(c.undoHistory)-textInputHistoryLimit:]
	}
}

func (c *TextInput) pushRedoHistory(value string) {
	if c == nil {
		return
	}
	if n := len(c.redoHistory); n > 0 && c.redoHistory[n-1] == value {
		return
	}
	c.redoHistory = append(c.redoHistory, value)
	if len(c.redoHistory) > textInputHistoryLimit {
		c.redoHistory = c.redoHistory[len(c.redoHistory)-textInputHistoryLimit:]
	}
}

func isMutationKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes, tea.KeySpace, tea.KeyEnter, tea.KeyBackspace, tea.KeyDelete:
		return true
	}
	switch msg.String() {
	case "ctrl+h", "ctrl+d", "ctrl+k", "ctrl+u", "ctrl+w", "alt+backspace", "alt+delete", "alt+d", "ctrl+v":
		return true
	default:
		return false
	}
}

func isDeletionKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyDelete:
		return true
	}
	switch msg.String() {
	case "ctrl+h", "ctrl+d", "ctrl+k", "ctrl+u", "ctrl+w", "alt+backspace", "alt+delete", "alt+d":
		return true
	default:
		return false
	}
}

func isNavigationKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown, tea.KeyHome, tea.KeyEnd:
		return true
	}
	switch msg.String() {
	case "ctrl+left", "ctrl+right", "alt+left", "alt+right", "alt+b", "alt+f", "ctrl+b", "ctrl+f", "ctrl+a", "ctrl+e":
		return true
	default:
		return false
	}
}

func isNewlineKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyEnter {
		return true
	}
	switch msg.String() {
	case "shift+enter", "ctrl+enter":
		return true
	default:
		return false
	}
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

func (c *TextInput) sanitize() {
	if c == nil {
		return
	}
	value := c.input.Value()
	if value == "" {
		return
	}
	cleaned := sanitizeChatInput(value, c.singleLine)
	if cleaned != value {
		c.input.SetValue(cleaned)
	}
}

func sanitizeChatInput(value string, singleLine bool) string {
	value = ansiEscapeRE.ReplaceAllString(value, "")
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if r == '\n' {
			if singleLine {
				b.WriteRune(' ')
			} else {
				b.WriteRune(r)
			}
			continue
		}
		if r < 32 || r == 127 {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func applyWordNavigationAliases(input *textarea.Model) {
	if input == nil {
		return
	}
	appendBindingKey(&input.KeyMap.WordBackward, "ctrl+left")
	appendBindingKey(&input.KeyMap.WordForward, "ctrl+right")
	appendBindingKey(&input.KeyMap.DeleteWordBackward, "ctrl+backspace")
	appendBindingKey(&input.KeyMap.DeleteWordForward, "ctrl+delete")
}

func appendBindingKey(binding *key.Binding, alias string) {
	if binding == nil {
		return
	}
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return
	}
	keys := binding.Keys()
	for _, entry := range keys {
		if strings.EqualFold(strings.TrimSpace(entry), alias) {
			return
		}
	}
	keys = append(keys, alias)
	binding.SetKeys(keys...)
}

// Backward-compatible aliases while the input refactor migrates call sites.
type ChatInput = TextInput
type ChatInputConfig = TextInputConfig

func DefaultChatInputConfig() ChatInputConfig {
	return DefaultTextInputConfig()
}

func NewChatInput(width int, cfg ChatInputConfig) *ChatInput {
	return NewTextInput(width, cfg)
}
