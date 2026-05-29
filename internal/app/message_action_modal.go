package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// MessageActionModalAction represents an action the user can take on a
// selected chat message via the action popup.
type MessageActionModalAction int

const (
	MessageActionNone MessageActionModalAction = iota
	MessageActionCopy
	MessageActionPin
	MessageActionToggleReasoning
)

type messageActionItem struct {
	Label  string
	Action MessageActionModalAction
	Key    string // keyboard shortcut hint displayed to user
}

const (
	messageActionMaxWidth = 40
)

// MessageActionModalController manages a center-screen popup that appears
// when a user clicks a chat message (or presses 'v'). It shows contextual
// actions like Copy, Pin, and Toggle Reasoning — inspired by Open Code's
// message interaction pattern.
type MessageActionModalController struct {
	active     bool
	blockIndex int
	items      []messageActionItem
	selected   int
}

func NewMessageActionModalController() *MessageActionModalController {
	return &MessageActionModalController{}
}

func (m *MessageActionModalController) IsOpen() bool {
	return m != nil && m.active
}

func (m *MessageActionModalController) BlockIndex() int {
	if m == nil {
		return -1
	}
	return m.blockIndex
}

// Open shows the action modal for the given block index. The caller passes in
// role/reasoning metadata so the modal can tailor its items.
func (m *MessageActionModalController) Open(blockIndex int, role ChatRole, reasoningExpanded bool) {
	if m == nil {
		return
	}
	m.active = true
	m.blockIndex = blockIndex
	m.selected = 0

	m.items = []messageActionItem{
		{Label: "Copy", Action: MessageActionCopy, Key: "y"},
		{Label: "Pin", Action: MessageActionPin, Key: "p"},
	}

	// Only show reasoning toggle for assistant messages that have a reasoning section.
	if role == ChatRoleAgent {
		label := "Expand Reasoning"
		if reasoningExpanded {
			label = "Collapse Reasoning"
		}
		m.items = append(m.items, messageActionItem{
			Label:  label,
			Action: MessageActionToggleReasoning,
		})
	}
}

func (m *MessageActionModalController) Close() {
	if m == nil {
		return
	}
	m.active = false
	m.blockIndex = -1
	m.items = nil
	m.selected = 0
}

func (m *MessageActionModalController) HandleKey(msg tea.KeyMsg) (bool, MessageActionModalAction) {
	if m == nil || !m.active {
		return false, MessageActionNone
	}
	switch msg.String() {
	case "esc", "q":
		return true, MessageActionNone
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
		return true, MessageActionNone
	case "down", "j":
		if m.selected < len(m.items)-1 {
			m.selected++
		}
		return true, MessageActionNone
	case "enter":
		if len(m.items) == 0 || m.selected < 0 || m.selected >= len(m.items) {
			return true, MessageActionNone
		}
		return true, m.items[m.selected].Action
	case "y":
		// Shortcut: copy directly
		return true, MessageActionCopy
	case "p":
		// Shortcut: pin directly
		return true, MessageActionPin
	}
	return false, MessageActionNone
}

func (m *MessageActionModalController) HandleMouse(msg tea.MouseMsg, maxWidth, maxHeight int) (bool, MessageActionModalAction) {
	if m == nil || !m.active {
		return false, MessageActionNone
	}
	if _, ok := msg.(tea.MouseClickMsg); !ok {
		return false, MessageActionNone
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return false, MessageActionNone
	}
	x, y, width, height := m.layout(maxWidth, maxHeight)
	if mouse.X < x || mouse.X >= x+width || mouse.Y < y || mouse.Y >= y+height {
		return false, MessageActionNone
	}
	row := mouse.Y - y
	if row <= 0 {
		// Header row click — no action
		return true, MessageActionNone
	}
	idx := row - 1
	if idx < 0 || idx >= len(m.items) {
		return true, MessageActionNone
	}
	m.selected = idx
	return true, m.items[idx].Action
}

func (m *MessageActionModalController) Contains(x, y, maxWidth, maxHeight int) bool {
	if m == nil || !m.active {
		return false
	}
	bx, by, bw, bh := m.layout(maxWidth, maxHeight)
	return x >= bx && x < bx+bw && y >= by && y < by+bh
}

func (m *MessageActionModalController) ViewBlock(maxWidth, maxHeight int) (string, int, int) {
	if m == nil || !m.active {
		return "", 0, 0
	}
	x, y, width, _ := m.layout(maxWidth, maxHeight)
	contentWidth := max(1, width-2)

	header := "Message Actions"
	headerLine := " " + padToWidth(truncateToWidth(header, contentWidth), contentWidth) + " "
	lines := []string{contextMenuHeaderStyle.Render(headerLine)}

	for i, item := range m.items {
		label := item.Label
		if item.Key != "" {
			label = fmt.Sprintf("(%s) %s", item.Key, label)
		}
		label = truncateToWidth(label, contentWidth)
		line := " " + padToWidth(label, contentWidth) + " "
		if i == m.selected {
			line = selectedStyle.Render(line)
		} else {
			line = menuDropStyle.Render(line)
		}
		lines = append(lines, line)
	}

	block := confirmDialogBorderStyle.Render(strings.Join(lines, "\n"))
	return block, x, y
}

func (m *MessageActionModalController) layout(maxWidth, maxHeight int) (int, int, int, int) {
	width := m.menuWidth()
	if maxWidth > 0 && width > maxWidth {
		width = maxWidth
	}
	height := len(m.items) + 1
	if height < 3 {
		height = 3
	}

	minRow := 1
	if maxHeight <= 0 {
		minRow = 0
	}

	x := 0
	y := minRow
	if maxWidth > 0 {
		x = (maxWidth - width) / 2
		if x < 0 {
			x = 0
		}
	}
	if maxHeight > 0 {
		y = (maxHeight-height)/2 + minRow
		if y < minRow {
			y = minRow
		}
	}
	return x, y, width, height + 2 // +2 for border
}

func (m *MessageActionModalController) menuWidth() int {
	if m == nil {
		return minListWidth
	}
	maxContentWidth := xansi.StringWidth("Message Actions")
	for _, item := range m.items {
		label := item.Label
		if item.Key != "" {
			label = fmt.Sprintf("(%s) %s", item.Key, label)
		}
		if w := xansi.StringWidth(label); w > maxContentWidth {
			maxContentWidth = w
		}
	}
	width := maxContentWidth + 2 // padding
	if width < minListWidth {
		width = minListWidth
	}
	if messageActionMaxWidth > 0 && width > messageActionMaxWidth {
		width = messageActionMaxWidth
	}
	return width
}

// String provides a debug representation.
func (m *MessageActionModalController) String() string {
	if m == nil || !m.active {
		return "MessageActionModal(closed)"
	}
	var items []string
	for _, item := range m.items {
		items = append(items, item.Label)
	}
	return fmt.Sprintf("MessageActionModal(block=%d, selected=%d, items=%v)", m.blockIndex, m.selected, items)
}
