package app

import (
	"context"
	"time"

	"control/internal/client"
	"control/internal/types"

	tea "github.com/charmbracelet/bubbletea"
)

func fetchSessionsWithMetaCmd(api SessionAPI) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		sessions, meta, err := api.ListSessionsWithMeta(ctx)
		return sessionsWithMetaMsg{sessions: sessions, meta: meta, err: err}
	}
}

func fetchWorkspacesCmd(api WorkspaceAPI) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		workspaces, err := api.ListWorkspaces(ctx)
		return workspacesMsg{workspaces: workspaces, err: err}
	}
}

func fetchWorktreesCmd(api WorkspaceAPI, workspaceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		worktrees, err := api.ListWorktrees(ctx, workspaceID)
		return worktreesMsg{workspaceID: workspaceID, worktrees: worktrees, err: err}
	}
}

func fetchAvailableWorktreesCmd(api WorkspaceAPI, workspaceID, workspacePath string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		worktrees, err := api.ListAvailableWorktrees(ctx, workspaceID)
		return availableWorktreesMsg{
			workspaceID:   workspaceID,
			workspacePath: workspacePath,
			worktrees:     worktrees,
			err:           err,
		}
	}
}

func fetchAppStateCmd(api StateAPI) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		state, err := api.GetAppState(ctx)
		return appStateMsg{state: state, err: err}
	}
}

func createWorkspaceCmd(api WorkspaceAPI, path, name, provider string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		workspace, err := api.CreateWorkspace(ctx, &types.Workspace{
			Name:     name,
			RepoPath: path,
			Provider: provider,
		})
		return createWorkspaceMsg{workspace: workspace, err: err}
	}
}

func createWorktreeCmd(api WorkspaceAPI, workspaceID string, req client.CreateWorktreeRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		worktree, err := api.CreateWorktree(ctx, workspaceID, req)
		return createWorktreeMsg{workspaceID: workspaceID, worktree: worktree, err: err}
	}
}

func addWorktreeCmd(api WorkspaceAPI, workspaceID string, worktree *types.Worktree) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		created, err := api.AddWorktree(ctx, workspaceID, worktree)
		return addWorktreeMsg{workspaceID: workspaceID, worktree: created, err: err}
	}
}

func fetchTailCmd(api SessionAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		resp, err := api.TailItems(ctx, id, defaultTailLines)
		if err != nil {
			return tailMsg{id: id, err: err}
		}
		return tailMsg{id: id, items: resp.Items}
	}
}

func openStreamCmd(api SessionAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ch, cancel, err := api.TailStream(ctx, id, "combined")
		return streamMsg{id: id, ch: ch, cancel: cancel, err: err}
	}
}

func killSessionCmd(api SessionAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		err := api.KillSession(ctx, id)
		return killMsg{id: id, err: err}
	}
}

func markExitedCmd(api SessionAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		err := api.MarkSessionExited(ctx, id)
		return exitMsg{id: id, err: err}
	}
}

func markExitedManyCmd(api SessionAPI, ids []string) tea.Cmd {
	return func() tea.Msg {
		if len(ids) == 0 {
			return bulkExitMsg{ids: ids}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		for _, id := range ids {
			if err := api.MarkSessionExited(ctx, id); err != nil {
				return bulkExitMsg{ids: ids, err: err}
			}
		}
		return bulkExitMsg{ids: ids}
	}
}
