package app

import (
	"fmt"
	"strconv"
	"strings"

	"control/internal/client"
	"control/internal/types"

	tea "github.com/charmbracelet/bubbletea"
)

type worktreeAddMode int

const (
	worktreeModeUnset worktreeAddMode = iota
	worktreeModeNew
	worktreeModeExisting
)

type addWorktreeHost interface {
	addWorktreeCmd(workspaceID string, worktree *types.Worktree) tea.Cmd
	createWorktreeCmd(workspaceID string, req client.CreateWorktreeRequest) tea.Cmd
	exitAddWorktree(status string)
	fetchAvailableWorktreesCmd(workspaceID, workspacePath string) tea.Cmd
	keyMatchesCommand(msg tea.KeyMsg, command, fallback string) bool
	keyString(msg tea.KeyMsg) string
	setStatus(status string)
}

type AddWorktreeController struct {
	input         *TextInput
	step          int
	mode          worktreeAddMode
	choice        int
	listIndex     int
	listOffset    int
	listHeight    int
	path          string
	branch        string
	name          string
	available     []*types.GitWorktree
	workspaceID   string
	workspacePath string
}

func NewAddWorktreeController(width int) *AddWorktreeController {
	input := newAddInput(width)
	input.SetPlaceholder("")
	return &AddWorktreeController{
		input:      input,
		mode:       worktreeModeUnset,
		listHeight: 8,
	}
}

func (c *AddWorktreeController) Resize(width int) {
	if c.input != nil {
		c.input.Resize(width)
	}
}

func (c *AddWorktreeController) SetListHeight(height int) {
	if height < 3 {
		height = 3
	}
	c.listHeight = height
	c.ensureVisible()
}

func (c *AddWorktreeController) Enter(workspaceID, workspacePath string) {
	c.step = 0
	c.workspaceID = workspaceID
	c.workspacePath = workspacePath
	c.mode = worktreeModeUnset
	c.choice = 0
	c.listIndex = 0
	c.listOffset = 0
	c.path = ""
	c.branch = ""
	c.name = ""
	c.available = nil
	c.prepareInput()
	if c.input != nil {
		c.input.Blur()
	}
}

func (c *AddWorktreeController) Exit() {
	c.step = 0
	c.workspaceID = ""
	c.workspacePath = ""
	c.mode = worktreeModeUnset
	c.choice = 0
	c.listIndex = 0
	c.listOffset = 0
	c.path = ""
	c.branch = ""
	c.name = ""
	c.available = nil
	if c.input != nil {
		c.input.SetValue("")
		c.input.Blur()
	}
}

func (c *AddWorktreeController) SetAvailable(available []*types.GitWorktree, existing []*types.Worktree, workspacePath string) int {
	c.available = filterAvailableWorktrees(available, existing, workspacePath)
	if c.listIndex >= len(c.available) {
		c.listIndex = 0
	}
	c.ensureVisible()
	return len(c.available)
}

func (c *AddWorktreeController) Update(msg tea.Msg, host addWorktreeHost) (bool, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if handled, cmd := c.handleKey(keyMsg, host); handled {
			return true, cmd
		}
		if c.mode == worktreeModeExisting && c.step == 0 {
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
	if c.mode == worktreeModeExisting && c.step == 0 {
		return true, nil
	}
	if c.input != nil {
		return true, c.input.Update(msg)
	}
	return true, nil
}

func (c *AddWorktreeController) View() string {
	lines := []string{
		c.renderStep(),
		"",
		"Enter to continue • Esc to cancel",
	}
	return strings.Join(lines, "\n")
}

func (c *AddWorktreeController) handleKey(keyMsg tea.KeyMsg, host addWorktreeHost) (bool, tea.Cmd) {
	if host.keyMatchesCommand(keyMsg, KeyCommandToggleSidebar, "ctrl+b") {
		// Swallow global hotkey while typing.
		return true, nil
	}

	switch host.keyString(keyMsg) {
	case "esc":
		host.exitAddWorktree("add worktree canceled")
		return true, nil
	case "enter":
		return true, c.advance(host)
	case "ctrl+r":
		if c.mode == worktreeModeExisting && c.step == 0 {
			return true, host.fetchAvailableWorktreesCmd(c.workspaceID, c.workspacePath)
		}
		return true, nil
	}

	if c.mode != worktreeModeUnset {
		if c.mode == worktreeModeExisting && c.step == 0 {
			switch keyMsg.String() {
			case "j", "down":
				c.setListIndex(c.listIndex + 1)
				return true, nil
			case "k", "up":
				c.setListIndex(c.listIndex - 1)
				return true, nil
			case "pgdown":
				c.setListIndex(c.listIndex + c.listHeight)
				return true, nil
			case "pgup":
				c.setListIndex(c.listIndex - c.listHeight)
				return true, nil
			case "home":
				c.setListIndex(0)
				return true, nil
			case "end":
				c.setListIndex(len(c.available) - 1)
				return true, nil
			}
		}
		return false, nil
	}

	switch keyMsg.String() {
	case "j", "down":
		c.choice++
		if c.choice > 1 {
			c.choice = 1
		}
		return true, nil
	case "k", "up":
		c.choice--
		if c.choice < 0 {
			c.choice = 0
		}
		return true, nil
	}

	if keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) > 0 {
		switch strings.ToLower(string(keyMsg.Runes[0])) {
		case "n":
			c.choice = 0
			return true, c.advance(host)
		case "e":
			c.choice = 1
			return true, c.advance(host)
		}
	}

	return false, nil
}

func (c *AddWorktreeController) prepareInput() {
	if c.input == nil {
		return
	}
	c.input.SetValue("")
	switch c.mode {
	case worktreeModeUnset:
		c.input.SetPlaceholder("")
		c.input.Blur()
	case worktreeModeNew:
		c.input.Focus()
		switch c.step {
		case 0:
			c.input.SetPlaceholder("/path/to/worktree")
		case 1:
			c.input.SetPlaceholder("(optional) branch name")
		case 2:
			c.input.SetPlaceholder("(optional) display name")
		}
	case worktreeModeExisting:
		c.input.Focus()
		switch c.step {
		case 0:
			c.input.SetPlaceholder("number")
		case 1:
			c.input.SetPlaceholder("(optional) display name")
		}
	}
}

func (c *AddWorktreeController) advance(host addWorktreeHost) tea.Cmd {
	value := strings.TrimSpace(c.value())
	switch c.mode {
	case worktreeModeUnset:
		switch strings.ToLower(value) {
		case "":
			if c.choice == 0 {
				c.mode = worktreeModeNew
				c.step = 0
				c.prepareInput()
				host.setStatus("add worktree: enter path")
				return nil
			}
			c.mode = worktreeModeExisting
			c.step = 0
			c.prepareInput()
			host.setStatus("loading worktrees")
			return host.fetchAvailableWorktreesCmd(c.workspaceID, c.workspacePath)
		case "n", "new":
			c.choice = 0
			c.mode = worktreeModeNew
			c.step = 0
			c.prepareInput()
			host.setStatus("add worktree: enter path")
			return nil
		case "e", "existing":
			c.choice = 1
			c.mode = worktreeModeExisting
			c.step = 0
			c.prepareInput()
			host.setStatus("loading worktrees")
			return host.fetchAvailableWorktreesCmd(c.workspaceID, c.workspacePath)
		default:
			host.setStatus("select new or existing")
			return nil
		}
	case worktreeModeNew:
		switch c.step {
		case 0:
			if value == "" {
				host.setStatus("path is required")
				return nil
			}
			c.path = value
			c.step = 1
			c.prepareInput()
			host.setStatus("add worktree: branch (optional)")
			return nil
		case 1:
			c.branch = value
			c.step = 2
			c.prepareInput()
			host.setStatus("add worktree: name (optional)")
			return nil
		case 2:
			c.name = value
			req := client.CreateWorktreeRequest{
				Path:   c.path,
				Branch: c.branch,
				Name:   c.name,
			}
			return host.createWorktreeCmd(c.workspaceID, req)
		}
	case worktreeModeExisting:
		switch c.step {
		case 0:
			if len(c.available) == 0 {
				host.setStatus("no worktrees available")
				return nil
			}
			c.path = c.available[c.listIndex].Path
			c.step = 1
			c.prepareInput()
			host.setStatus("add worktree: name (optional)")
			return nil
		case 1:
			c.name = value
			return host.addWorktreeCmd(c.workspaceID, &types.Worktree{
				Name: c.name,
				Path: c.path,
			})
		}
	}
	return nil
}

func (c *AddWorktreeController) value() string {
	if c.input == nil {
		return ""
	}
	return c.input.Value()
}

func (c *AddWorktreeController) renderStep() string {
	switch c.mode {
	case worktreeModeUnset:
		options := []string{"New worktree", "Existing worktree"}
		lines := []string{"Mode:"}
		for i, option := range options {
			line := "  " + option
			if i == c.choice {
				line = selectedStyle.Render(line)
			}
			lines = append(lines, line)
		}
		lines = append(lines, "", "Use j/k to select, Enter to continue")
		return strings.Join(lines, "\n")
	case worktreeModeNew:
		return strings.Join([]string{
			renderAddField(c.input, c.step, "Path", c.path, 0),
			renderAddField(c.input, c.step, "Branch", c.branch, 1),
			renderAddField(c.input, c.step, "Name", c.name, 2),
		}, "\n")
	case worktreeModeExisting:
		lines := []string{"Worktrees:"}
		if len(c.available) == 0 {
			lines = append(lines, "  (none found)")
		} else {
			start, end := c.visibleRange()
			for i := start; i < end; i++ {
				wt := c.available[i]
				label := wt.Path
				if wt.Branch != "" {
					label += " (" + wt.Branch + ")"
				}
				line := fmt.Sprintf("  %s", label)
				if i == c.listIndex {
					line = selectedStyle.Render(line)
				}
				lines = append(lines, line)
			}
		}
		lines = append(lines, "", "Use j/k/↑/↓ to select • Enter to continue • Click to select")
		if c.step >= 1 {
			selected := c.path
			if strings.TrimSpace(selected) == "" && c.listIndex >= 0 && c.listIndex < len(c.available) {
				selected = c.available[c.listIndex].Path
			}
			if selected != "" {
				lines = append(lines, fmt.Sprintf("Selected: %s", selected))
			}
			lines = append(lines, renderAddField(c.input, c.step, "Name", c.name, 1))
		}
		return strings.Join(lines, "\n")
	}
	return ""
}

func (c *AddWorktreeController) HandleClick(row int, host addWorktreeHost) (bool, tea.Cmd) {
	if c.mode != worktreeModeExisting || c.step != 0 {
		return false, nil
	}
	if len(c.available) == 0 {
		return false, nil
	}
	listStart := 1
	idx := c.listOffset + (row - listStart)
	if idx < 0 || idx >= len(c.available) {
		return false, nil
	}
	c.setListIndex(idx)
	return true, c.advance(host)
}

func (c *AddWorktreeController) Scroll(delta int) bool {
	if c.mode != worktreeModeExisting || c.step != 0 {
		return false
	}
	if len(c.available) == 0 {
		return false
	}
	c.setListIndex(c.listIndex + delta)
	return true
}

func (c *AddWorktreeController) setListIndex(idx int) {
	c.listIndex = idx
	c.ensureVisible()
}

func (c *AddWorktreeController) ensureVisible() {
	if len(c.available) == 0 {
		c.listIndex = 0
		c.listOffset = 0
		return
	}
	if c.listIndex < 0 {
		c.listIndex = 0
	}
	if c.listIndex >= len(c.available) {
		c.listIndex = len(c.available) - 1
	}
	if c.listHeight <= 0 {
		c.listHeight = 8
	}
	if c.listIndex < c.listOffset {
		c.listOffset = c.listIndex
	}
	if c.listIndex >= c.listOffset+c.listHeight {
		c.listOffset = c.listIndex - c.listHeight + 1
	}
	maxOffset := len(c.available) - c.listHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if c.listOffset > maxOffset {
		c.listOffset = maxOffset
	}
}

func (c *AddWorktreeController) visibleRange() (int, int) {
	if len(c.available) == 0 {
		return 0, 0
	}
	if c.listHeight <= 0 || len(c.available) <= c.listHeight {
		return 0, len(c.available)
	}
	c.ensureVisible()
	end := c.listOffset + c.listHeight
	if end > len(c.available) {
		end = len(c.available)
	}
	return c.listOffset, end
}

func filterAvailableWorktrees(available []*types.GitWorktree, existing []*types.Worktree, workspacePath string) []*types.GitWorktree {
	existingPaths := make(map[string]struct{}, len(existing))
	for _, wt := range existing {
		if wt == nil {
			continue
		}
		existingPaths[wt.Path] = struct{}{}
	}
	workspacePath = strings.TrimSpace(workspacePath)
	out := make([]*types.GitWorktree, 0, len(available))
	for _, wt := range available {
		if wt == nil {
			continue
		}
		if _, ok := existingPaths[wt.Path]; ok {
			continue
		}
		if workspacePath != "" && wt.Path == workspacePath {
			continue
		}
		out = append(out, wt)
	}
	return out
}

func parseIndex(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return -1
	}
	idx, err := strconv.Atoi(value)
	if err != nil || idx <= 0 {
		return -1
	}
	return idx - 1
}
