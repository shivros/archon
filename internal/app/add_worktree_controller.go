package app

import (
	"fmt"
	"strconv"
	"strings"

	"control/internal/client"
	"control/internal/types"

	"github.com/charmbracelet/bubbles/textinput"
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
	setStatus(status string)
}

type AddWorktreeController struct {
	input         textinput.Model
	step          int
	mode          worktreeAddMode
	choice        int
	path          string
	branch        string
	name          string
	available     []*types.GitWorktree
	workspaceID   string
	workspacePath string
}

func NewAddWorktreeController(width int) *AddWorktreeController {
	input := newAddInput(width)
	input.Placeholder = ""
	return &AddWorktreeController{
		input: input,
		mode:  worktreeModeUnset,
	}
}

func (c *AddWorktreeController) Resize(width int) {
	resizeAddInput(&c.input, width)
}

func (c *AddWorktreeController) Enter(workspaceID, workspacePath string) {
	c.step = 0
	c.workspaceID = workspaceID
	c.workspacePath = workspacePath
	c.mode = worktreeModeUnset
	c.choice = 0
	c.path = ""
	c.branch = ""
	c.name = ""
	c.available = nil
	c.prepareInput()
	c.input.Blur()
}

func (c *AddWorktreeController) Exit() {
	c.step = 0
	c.workspaceID = ""
	c.workspacePath = ""
	c.mode = worktreeModeUnset
	c.choice = 0
	c.path = ""
	c.branch = ""
	c.name = ""
	c.available = nil
	c.input.SetValue("")
	c.input.Blur()
}

func (c *AddWorktreeController) SetAvailable(available []*types.GitWorktree, existing []*types.Worktree, workspacePath string) int {
	c.available = filterAvailableWorktrees(available, existing, workspacePath)
	return len(c.available)
}

func (c *AddWorktreeController) Update(msg tea.Msg, host addWorktreeHost) (bool, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if handled, cmd := c.handleKey(keyMsg, host); handled {
			return true, cmd
		}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return true, cmd
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
	switch keyMsg.String() {
	case "esc":
		host.exitAddWorktree("add worktree canceled")
		return true, nil
	case "enter":
		return true, c.advance(host)
	case "ctrl+b":
		// Swallow global hotkey while typing.
		return true, nil
	case "ctrl+r":
		if c.mode == worktreeModeExisting && c.step == 0 {
			return true, host.fetchAvailableWorktreesCmd(c.workspaceID, c.workspacePath)
		}
		return true, nil
	}

	if c.mode != worktreeModeUnset {
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
	c.input.SetValue("")
	switch c.mode {
	case worktreeModeUnset:
		c.input.Placeholder = ""
		c.input.Blur()
	case worktreeModeNew:
		c.input.Focus()
		switch c.step {
		case 0:
			c.input.Placeholder = "/path/to/worktree"
		case 1:
			c.input.Placeholder = "(optional) branch name"
		case 2:
			c.input.Placeholder = "(optional) display name"
		}
	case worktreeModeExisting:
		c.input.Focus()
		switch c.step {
		case 0:
			c.input.Placeholder = "number"
		case 1:
			c.input.Placeholder = "(optional) display name"
		}
	}
}

func (c *AddWorktreeController) advance(host addWorktreeHost) tea.Cmd {
	value := strings.TrimSpace(c.input.Value())
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
			index := parseIndex(value)
			if index < 0 || index >= len(c.available) {
				host.setStatus("select a valid worktree number")
				return nil
			}
			c.path = c.available[index].Path
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

func (c *AddWorktreeController) renderStep() string {
	switch c.mode {
	case worktreeModeUnset:
		options := []string{"New worktree", "Existing worktree"}
		lines := []string{"Mode:"}
		for i, option := range options {
			prefix := "  "
			if i == c.choice {
				prefix = "> "
			}
			lines = append(lines, prefix+option)
		}
		lines = append(lines, "", "Use j/k to select, Enter to continue")
		return strings.Join(lines, "\n")
	case worktreeModeNew:
		return strings.Join([]string{
			renderAddField(&c.input, c.step, "Path", c.path, 0),
			renderAddField(&c.input, c.step, "Branch", c.branch, 1),
			renderAddField(&c.input, c.step, "Name", c.name, 2),
		}, "\n")
	case worktreeModeExisting:
		lines := []string{"Worktrees:"}
		if len(c.available) == 0 {
			lines = append(lines, "  (none found)")
		} else {
			for i, wt := range c.available {
				label := wt.Path
				if wt.Branch != "" {
					label += " (" + wt.Branch + ")"
				}
				lines = append(lines, fmt.Sprintf("  %d) %s", i+1, label))
			}
		}
		lines = append(lines, "")
		selectLine := renderAddField(&c.input, c.step, "Select", c.path, 0)
		if c.step == 0 {
			selectLine = fmt.Sprintf("Select: %s", c.input.View())
		}
		lines = append(lines, selectLine)
		if c.step >= 1 {
			lines = append(lines, renderAddField(&c.input, c.step, "Name", c.name, 1))
		}
		return strings.Join(lines, "\n")
	}
	return ""
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
