package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

type confirmChoice int

const (
	confirmChoiceNone confirmChoice = iota
	confirmChoiceConfirm
	confirmChoiceCancel
)

const (
	confirmMaxWidth = 60
)

type ConfirmController struct {
	active       bool
	title        string
	message      string
	confirmLabel string
	cancelLabel  string
	selected     int
}

func NewConfirmController() *ConfirmController {
	return &ConfirmController{}
}

func (c *ConfirmController) IsOpen() bool {
	return c != nil && c.active
}

func (c *ConfirmController) Open(title, message, confirmLabel, cancelLabel string) {
	if c == nil {
		return
	}
	c.active = true
	c.title = strings.TrimSpace(title)
	c.message = strings.TrimSpace(message)
	if confirmLabel == "" {
		confirmLabel = "Confirm"
	}
	if cancelLabel == "" {
		cancelLabel = "Cancel"
	}
	c.confirmLabel = confirmLabel
	c.cancelLabel = cancelLabel
	c.selected = 0
}

func (c *ConfirmController) Close() {
	if c == nil {
		return
	}
	c.active = false
	c.title = ""
	c.message = ""
	c.confirmLabel = ""
	c.cancelLabel = ""
	c.selected = 0
}

func (c *ConfirmController) HandleKey(msg tea.KeyMsg) (bool, confirmChoice) {
	if c == nil || !c.active {
		return false, confirmChoiceNone
	}
	switch msg.String() {
	case "esc", "q":
		return true, confirmChoiceCancel
	case "left", "h":
		c.selected = 0
		return true, confirmChoiceNone
	case "right", "l":
		c.selected = 1
		return true, confirmChoiceNone
	case "tab":
		if c.selected == 0 {
			c.selected = 1
		} else {
			c.selected = 0
		}
		return true, confirmChoiceNone
	case "y":
		return true, confirmChoiceConfirm
	case "n":
		return true, confirmChoiceCancel
	case "enter":
		if c.selected == 0 {
			return true, confirmChoiceConfirm
		}
		return true, confirmChoiceCancel
	}
	return false, confirmChoiceNone
}

func (c *ConfirmController) HandleMouse(msg tea.MouseMsg, maxWidth, maxHeight int) (bool, confirmChoice) {
	if c == nil || !c.active {
		return false, confirmChoiceNone
	}
	if _, ok := msg.(tea.MouseClickMsg); !ok {
		return false, confirmChoiceNone
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return false, confirmChoiceNone
	}
	x, y, width, height := c.layout(maxWidth, maxHeight)
	if mouse.X < x || mouse.X >= x+width || mouse.Y < y || mouse.Y >= y+height {
		return false, confirmChoiceNone
	}
	buttonRow := y + height - 2
	if mouse.Y != buttonRow {
		return true, confirmChoiceNone
	}
	contentX := x + 1
	contentWidth := max(1, width-2)
	if mouse.X < contentX || mouse.X >= contentX+contentWidth {
		return true, confirmChoiceNone
	}
	mid := contentX + contentWidth/2
	if mouse.X < mid {
		c.selected = 0
		return true, confirmChoiceConfirm
	}
	c.selected = 1
	return true, confirmChoiceCancel
}

func (c *ConfirmController) Contains(x, y, maxWidth, maxHeight int) bool {
	if c == nil || !c.active {
		return false
	}
	bx, by, bw, bh := c.layout(maxWidth, maxHeight)
	return x >= bx && x < bx+bw && y >= by && y < by+bh
}

func (c *ConfirmController) View(maxWidth, maxHeight int) (string, int) {
	if c == nil || !c.active {
		return "", 0
	}
	x, y, width, height := c.layout(maxWidth, maxHeight)
	innerWidth := max(1, width-2)
	contentWidth := max(1, innerWidth-2)
	title := c.title
	if title == "" {
		title = "Confirm"
	}
	title = truncateToWidth(title, contentWidth)
	lines := []string{contextMenuHeaderStyle.Render(" " + padToWidth(title, contentWidth) + " ")}

	message := strings.TrimSpace(c.message)
	if message != "" {
		wrapped := xansi.Hardwrap(message, contentWidth, true)
		for _, line := range strings.Split(wrapped, "\n") {
			line = truncateToWidth(line, contentWidth)
			lines = append(lines, menuDropStyle.Render(" "+padToWidth(line, contentWidth)+" "))
		}
	}

	confirm := "[" + c.confirmLabel + "]"
	cancel := "[" + c.cancelLabel + "]"
	leftWidth := contentWidth / 2
	rightWidth := contentWidth - leftWidth
	confirm = truncateToWidth(confirm, leftWidth)
	cancel = truncateToWidth(cancel, rightWidth)
	confirm = padToWidth(confirm, leftWidth)
	cancel = padToWidth(cancel, rightWidth)
	if c.selected == 0 {
		confirm = selectedStyle.Render(confirm)
		cancel = menuDropStyle.Render(cancel)
	} else {
		confirm = menuDropStyle.Render(confirm)
		cancel = selectedStyle.Render(cancel)
	}
	buttonLine := " " + confirm + cancel + " "
	if xansi.StringWidth(buttonLine) < innerWidth {
		buttonLine = padToWidth(buttonLine, innerWidth)
	}
	lines = append(lines, buttonLine)

	block := confirmDialogBorderStyle.Render(strings.Join(lines, "\n"))
	if x > 0 {
		block = indentBlock(block, x)
	}
	if height < len(lines)+2 {
		height = len(lines) + 2
	}
	return block, y
}

func (c *ConfirmController) layout(maxWidth, maxHeight int) (int, int, int, int) {
	width := c.menuWidth()
	if maxWidth > 0 && width > maxWidth {
		width = maxWidth
	}
	height := c.menuHeight(width)
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
	return x, y, width, height
}

func (c *ConfirmController) menuWidth() int {
	width := minListWidth
	if c == nil {
		return width
	}
	if confirmMaxWidth > 0 && width > confirmMaxWidth {
		width = confirmMaxWidth
	}
	contentWidth := 0
	title := strings.TrimSpace(c.title)
	if title == "" {
		title = "Confirm"
	}
	if w := xansi.StringWidth(title); w > contentWidth {
		contentWidth = w
	}
	if w := xansi.StringWidth(c.message); w > contentWidth {
		contentWidth = w
	}
	buttonWidth := xansi.StringWidth(c.confirmLabel) + xansi.StringWidth(c.cancelLabel) + 6
	if buttonWidth > contentWidth {
		contentWidth = buttonWidth
	}
	if contentWidth+4 > width {
		width = contentWidth + 4
	}
	if confirmMaxWidth > 0 && width > confirmMaxWidth {
		width = confirmMaxWidth
	}
	return width
}

func (c *ConfirmController) menuHeight(width int) int {
	innerWidth := max(1, width-2)
	contentWidth := max(1, innerWidth-2)
	height := 2
	if strings.TrimSpace(c.message) != "" {
		height += len(strings.Split(xansi.Hardwrap(c.message, contentWidth, true), "\n"))
	}
	return height + 2
}
