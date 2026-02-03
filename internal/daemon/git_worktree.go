package daemon

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"

	"control/internal/types"
)

func listGitWorktrees(repoPath string) ([]*types.GitWorktree, error) {
	if strings.TrimSpace(repoPath) == "" {
		return nil, fmt.Errorf("repo path is required")
	}
	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git worktree list failed: %s", strings.TrimSpace(string(output)))
	}
	return parseGitWorktreeList(string(output)), nil
}

func createGitWorktree(repoPath, path, branch string) error {
	if strings.TrimSpace(repoPath) == "" {
		return fmt.Errorf("repo path is required")
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("worktree path is required")
	}
	args := []string{"-C", repoPath, "worktree", "add", path}
	if strings.TrimSpace(branch) != "" {
		args = append(args, branch)
	}
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func parseGitWorktreeList(output string) []*types.GitWorktree {
	var out []*types.GitWorktree
	scanner := bufio.NewScanner(strings.NewReader(output))
	var current *types.GitWorktree
	flush := func() {
		if current != nil && strings.TrimSpace(current.Path) != "" {
			out = append(out, current)
		}
		current = nil
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			flush()
			path := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
			current = &types.GitWorktree{Path: path}
			continue
		}
		if current == nil {
			continue
		}
		switch {
		case strings.HasPrefix(line, "branch "):
			branch := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			branch = strings.TrimPrefix(branch, "refs/heads/")
			current.Branch = branch
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
		case line == "detached":
			current.Detached = true
		}
	}
	flush()
	return out
}
