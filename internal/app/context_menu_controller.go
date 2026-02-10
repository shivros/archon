package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
)

type ContextMenuAction int

const (
	ContextMenuNone ContextMenuAction = iota
	ContextMenuWorkspaceCreate
	ContextMenuWorkspaceRename
	ContextMenuWorkspaceEditGroups
	ContextMenuWorkspaceOpenNotes
	ContextMenuWorkspaceAddNote
	ContextMenuWorkspaceAddWorktree
	ContextMenuWorkspaceCopyPath
	ContextMenuWorkspaceDelete
	ContextMenuWorktreeAdd
	ContextMenuWorktreeOpenNotes
	ContextMenuWorktreeAddNote
	ContextMenuWorktreeCopyPath
	ContextMenuWorktreeDelete
	ContextMenuSessionChat
	ContextMenuSessionRename
	ContextMenuSessionOpenNotes
	ContextMenuSessionAddNote
	ContextMenuSessionDismiss
	ContextMenuSessionKill
	ContextMenuSessionInterrupt
	ContextMenuSessionCopyID
)

type contextMenuItem struct {
	Label  string
	Action ContextMenuAction
}

type contextMenuTargetKind int

const (
	contextTargetNone contextMenuTargetKind = iota
	contextTargetWorkspace
	contextTargetWorktree
	contextTargetSession
)

type ContextMenuController struct {
	active      bool
	targetKind  contextMenuTargetKind
	targetID    string
	workspaceID string
	worktreeID  string
	sessionID   string
	targetLabel string
	items       []contextMenuItem
	selected    int
	x           int
	y           int
}

func NewContextMenuController() *ContextMenuController {
	return &ContextMenuController{}
}

func (c *ContextMenuController) IsOpen() bool {
	return c != nil && c.active
}

func (c *ContextMenuController) TargetID() string {
	if c == nil {
		return ""
	}
	return c.targetID
}

func (c *ContextMenuController) WorkspaceID() string {
	if c == nil {
		return ""
	}
	return c.workspaceID
}

func (c *ContextMenuController) WorktreeID() string {
	if c == nil {
		return ""
	}
	return c.worktreeID
}

func (c *ContextMenuController) SessionID() string {
	if c == nil {
		return ""
	}
	return c.sessionID
}

func (c *ContextMenuController) Close() {
	if c == nil {
		return
	}
	c.active = false
	c.targetKind = contextTargetNone
	c.targetID = ""
	c.workspaceID = ""
	c.worktreeID = ""
	c.sessionID = ""
	c.targetLabel = ""
	c.items = nil
	c.selected = 0
}

func (c *ContextMenuController) OpenWorkspace(id, label string, x, y int) {
	if c == nil {
		return
	}
	c.active = true
	c.targetKind = contextTargetWorkspace
	c.targetID = id
	c.workspaceID = id
	c.targetLabel = strings.TrimSpace(label)
	c.items = []contextMenuItem{
		{Label: "Create Workspace", Action: ContextMenuWorkspaceCreate},
		{Label: "Rename Workspace", Action: ContextMenuWorkspaceRename},
		{Label: "Edit Workspace Groups", Action: ContextMenuWorkspaceEditGroups},
		{Label: "Open Notes", Action: ContextMenuWorkspaceOpenNotes},
		{Label: "Add Note", Action: ContextMenuWorkspaceAddNote},
		{Label: "Add Worktree", Action: ContextMenuWorkspaceAddWorktree},
		{Label: "Copy Workspace Path", Action: ContextMenuWorkspaceCopyPath},
		{Label: "Delete Workspace", Action: ContextMenuWorkspaceDelete},
	}
	c.selected = 0
	c.x = x
	c.y = y
}

func (c *ContextMenuController) OpenWorktree(worktreeID, workspaceID, label string, x, y int) {
	if c == nil {
		return
	}
	c.active = true
	c.targetKind = contextTargetWorktree
	c.targetID = worktreeID
	c.worktreeID = worktreeID
	c.workspaceID = workspaceID
	c.targetLabel = strings.TrimSpace(label)
	c.items = []contextMenuItem{
		{Label: "Add Worktree", Action: ContextMenuWorktreeAdd},
		{Label: "Open Notes", Action: ContextMenuWorktreeOpenNotes},
		{Label: "Add Note", Action: ContextMenuWorktreeAddNote},
		{Label: "Copy Worktree Path", Action: ContextMenuWorktreeCopyPath},
		{Label: "Delete Worktree", Action: ContextMenuWorktreeDelete},
	}
	c.selected = 0
	c.x = x
	c.y = y
}

func (c *ContextMenuController) OpenSession(sessionID, workspaceID, worktreeID, label string, x, y int) {
	if c == nil {
		return
	}
	c.active = true
	c.targetKind = contextTargetSession
	c.targetID = sessionID
	c.sessionID = sessionID
	c.workspaceID = workspaceID
	c.worktreeID = worktreeID
	c.targetLabel = strings.TrimSpace(label)
	c.items = []contextMenuItem{
		{Label: "Chat", Action: ContextMenuSessionChat},
		{Label: "Rename Session", Action: ContextMenuSessionRename},
		{Label: "Open Notes", Action: ContextMenuSessionOpenNotes},
		{Label: "Add Note", Action: ContextMenuSessionAddNote},
		{Label: "Dismiss Session", Action: ContextMenuSessionDismiss},
		{Label: "Kill Session", Action: ContextMenuSessionKill},
		{Label: "Interrupt Session", Action: ContextMenuSessionInterrupt},
		{Label: "Copy Session ID", Action: ContextMenuSessionCopyID},
	}
	c.selected = 0
	c.x = x
	c.y = y
}

func (c *ContextMenuController) HandleKey(msg tea.KeyMsg) (bool, ContextMenuAction) {
	if c == nil || !c.active {
		return false, ContextMenuNone
	}
	switch msg.String() {
	case "esc":
		c.Close()
		return true, ContextMenuNone
	case "up", "k":
		if c.selected > 0 {
			c.selected--
		}
		return true, ContextMenuNone
	case "down", "j":
		if c.selected < len(c.items)-1 {
			c.selected++
		}
		return true, ContextMenuNone
	case "enter":
		if len(c.items) == 0 || c.selected < 0 || c.selected >= len(c.items) {
			return true, ContextMenuNone
		}
		action := c.items[c.selected].Action
		return true, action
	}
	return false, ContextMenuNone
}

func (c *ContextMenuController) HandleMouse(msg tea.MouseMsg, maxWidth, maxHeight int) (bool, ContextMenuAction) {
	if c == nil || !c.active {
		return false, ContextMenuNone
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return false, ContextMenuNone
	}
	x, y, width, height := c.layout(maxWidth, maxHeight)
	if msg.X < x || msg.X >= x+width || msg.Y < y || msg.Y >= y+height {
		return false, ContextMenuNone
	}
	row := msg.Y - y
	if row <= 0 {
		return true, ContextMenuNone
	}
	idx := row - 1
	if idx < 0 || idx >= len(c.items) {
		return true, ContextMenuNone
	}
	c.selected = idx
	return true, c.items[idx].Action
}

func (c *ContextMenuController) View(maxWidth, maxHeight int) (string, int) {
	if c == nil || !c.active {
		return "", 0
	}
	x, y, width, _ := c.layout(maxWidth, maxHeight)
	contentWidth := max(1, width-2)
	header := c.headerLabel()
	header = truncateToWidth(header, contentWidth)
	headerLine := " " + padToWidth(header, contentWidth) + " "
	lines := []string{contextMenuHeaderStyle.Render(headerLine)}
	for i, item := range c.items {
		label := truncateToWidth(item.Label, contentWidth)
		line := " " + padToWidth(label, contentWidth) + " "
		if i == c.selected {
			line = selectedStyle.Render(line)
		} else {
			line = menuDropStyle.Render(line)
		}
		lines = append(lines, line)
	}
	block := strings.Join(lines, "\n")
	if x > 0 {
		block = indentBlock(block, x)
	}
	return block, y
}

func (c *ContextMenuController) Contains(x, y, maxWidth, maxHeight int) bool {
	if c == nil || !c.active {
		return false
	}
	bx, by, bw, bh := c.layout(maxWidth, maxHeight)
	return x >= bx && x < bx+bw && y >= by && y < by+bh
}

func (c *ContextMenuController) headerLabel() string {
	if c == nil {
		return "Workspace"
	}
	label := strings.TrimSpace(c.targetLabel)
	if label == "" {
		return "Workspace"
	}
	switch c.targetKind {
	case contextTargetWorkspace:
		return fmt.Sprintf("Workspace: %s", label)
	case contextTargetWorktree:
		return fmt.Sprintf("Worktree: %s", label)
	case contextTargetSession:
		return fmt.Sprintf("Session: %s", label)
	default:
		return label
	}
}

func (c *ContextMenuController) layout(maxWidth, maxHeight int) (int, int, int, int) {
	if c == nil || !c.active {
		return 0, 0, 0, 0
	}
	width := c.menuWidth()
	if maxWidth > 0 && width > maxWidth {
		width = maxWidth
	}
	height := len(c.items) + 1
	if maxHeight > 0 && height > maxHeight {
		height = maxHeight
	}
	x := c.x
	y := c.y
	minRow := 1
	if maxHeight <= 0 {
		minRow = 0
	}
	if y < minRow {
		y = minRow
	}
	if maxWidth > 0 {
		maxX := maxWidth - width
		if maxX < 0 {
			maxX = 0
		}
		x = clamp(x, 0, maxX)
	}
	if maxHeight > 0 {
		maxY := maxHeight - height
		if maxY < minRow {
			maxY = minRow
		}
		y = clamp(y, minRow, maxY)
	}
	return x, y, width, height
}

func (c *ContextMenuController) menuWidth() int {
	if c == nil {
		return minListWidth
	}
	maxWidth := xansi.StringWidth(c.headerLabel())
	for _, item := range c.items {
		if w := xansi.StringWidth(item.Label); w > maxWidth {
			maxWidth = w
		}
	}
	width := maxWidth + 2
	if width < minListWidth {
		width = minListWidth
	}
	return width
}

func indentBlock(block string, spaces int) string {
	if spaces <= 0 {
		return block
	}
	prefix := strings.Repeat(" ", spaces)
	lines := strings.Split(block, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
