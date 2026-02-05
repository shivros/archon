package app

import (
	"context"
	"log"
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

func fetchTailCmd(api SessionAPI, id, key string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		resp, err := api.TailItems(ctx, id, defaultTailLines)
		if err != nil {
			return tailMsg{id: id, err: err, key: key}
		}
		return tailMsg{id: id, items: resp.Items, key: key}
	}
}

func fetchHistoryCmd(api SessionAPI, id, key string, lines int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		resp, err := api.History(ctx, id, lines)
		if err != nil {
			return historyMsg{id: id, err: err, key: key}
		}
		return historyMsg{id: id, items: resp.Items, key: key}
	}
}

func openStreamCmd(api SessionAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ch, cancel, err := api.TailStream(ctx, id, "combined")
		return streamMsg{id: id, ch: ch, cancel: cancel, err: err}
	}
}

func openEventsCmd(api SessionAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ch, cancel, err := api.EventStream(ctx, id)
		return eventsMsg{id: id, ch: ch, cancel: cancel, err: err}
	}
}

func fetchApprovalsCmd(api SessionAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		approvals, err := api.ListApprovals(ctx, id)
		return approvalsMsg{id: id, approvals: approvals, err: err}
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

func sendSessionCmd(api SessionAPI, id, text string, token int) tea.Cmd {
	return func() tea.Msg {
		log.Printf("ui send: id=%s text_len=%d", id, len(text))
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		resp, err := api.SendMessage(ctx, id, client.SendSessionRequest{Text: text})
		turnID := ""
		if resp != nil {
			turnID = resp.TurnID
		}
		return sendMsg{id: id, turnID: turnID, text: text, err: err, token: token}
	}
}

func interruptSessionCmd(api SessionAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		err := api.InterruptSession(ctx, id)
		return interruptMsg{id: id, err: err}
	}
}

func debounceSelectCmd(id string, seq int, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return selectDebounceMsg{id: id, seq: seq}
	})
}

func approveSessionCmd(api SessionAPI, id string, requestID int, decision string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		req := client.ApproveSessionRequest{
			RequestID: requestID,
			Decision:  decision,
		}
		err := api.ApproveSession(ctx, id, req)
		return approvalMsg{id: id, requestID: requestID, decision: decision, err: err}
	}
}

func startSessionCmd(api SessionAPI, workspaceID, worktreeID, provider, text string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		req := client.StartSessionRequest{
			Provider: provider,
			Text:     text,
		}
		session, err := api.StartWorkspaceSession(ctx, workspaceID, worktreeID, req)
		return startSessionMsg{session: session, err: err}
	}
}
