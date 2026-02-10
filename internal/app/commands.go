package app

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"control/internal/client"
	"control/internal/types"

	tea "github.com/charmbracelet/bubbletea"
)

func fetchSessionsWithMetaCmd(api SessionListWithMetaAPI) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		sessions, meta, err := api.ListSessionsWithMeta(ctx)
		return sessionsWithMetaMsg{sessions: sessions, meta: meta, err: err}
	}
}

func fetchProviderOptionsCmd(api SessionProviderOptionsAPI, provider string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		options, err := api.GetProviderOptions(ctx, provider)
		return providerOptionsMsg{provider: provider, options: options, err: err}
	}
}

func fetchWorkspacesCmd(api WorkspaceListAPI) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		workspaces, err := api.ListWorkspaces(ctx)
		return workspacesMsg{workspaces: workspaces, err: err}
	}
}

func fetchWorkspaceGroupsCmd(api WorkspaceGroupListAPI) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		groups, err := api.ListWorkspaceGroups(ctx)
		return workspaceGroupsMsg{groups: groups, err: err}
	}
}

func fetchWorktreesCmd(api WorktreeListAPI, workspaceID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		worktrees, err := api.ListWorktrees(ctx, workspaceID)
		return worktreesMsg{workspaceID: workspaceID, worktrees: worktrees, err: err}
	}
}

func fetchNotesCmd(api NoteListAPI, scope noteScopeTarget) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		notes, err := api.ListNotes(ctx, scope.ToListRequest())
		return notesMsg{scope: scope, notes: notes, err: err}
	}
}

func createNoteCmd(api NoteCreateAPI, scope noteScopeTarget, body string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		note := &types.Note{
			Kind:        types.NoteKindNote,
			Scope:       scope.Scope,
			WorkspaceID: scope.WorkspaceID,
			WorktreeID:  scope.WorktreeID,
			SessionID:   scope.SessionID,
			Body:        strings.TrimSpace(body),
			Status:      types.NoteStatusIdea,
		}
		created, err := api.CreateNote(ctx, note)
		return noteCreatedMsg{note: created, scope: scope, err: err}
	}
}

func deleteNoteCmd(api NoteDeleteAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err := api.DeleteNote(ctx, id)
		return noteDeletedMsg{id: id, err: err}
	}
}

func pinSessionNoteCmd(api SessionPinAPI, sessionID string, block ChatBlock, snippet string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		created, err := api.PinSessionMessage(ctx, sessionID, client.PinSessionNoteRequest{
			SourceBlockID: strings.TrimSpace(block.ID),
			SourceRole:    strings.ToLower(strings.TrimSpace(chatRoleLabel(block.Role))),
			SourceSnippet: strings.TrimSpace(snippet),
		})
		return notePinnedMsg{note: created, sessionID: sessionID, err: err}
	}
}

func fetchAvailableWorktreesCmd(api AvailableWorktreeListAPI, workspaceID, workspacePath string) tea.Cmd {
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

func fetchAppStateCmd(api AppStateGetAPI) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		state, err := api.GetAppState(ctx)
		return appStateMsg{state: state, err: err}
	}
}

func createWorkspaceCmd(api WorkspaceCreateAPI, path, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		workspace, err := api.CreateWorkspace(ctx, &types.Workspace{
			Name:     name,
			RepoPath: path,
		})
		return createWorkspaceMsg{workspace: workspace, err: err}
	}
}

func createWorkspaceGroupCmd(api WorkspaceGroupCreateAPI, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		group, err := api.CreateWorkspaceGroup(ctx, &types.WorkspaceGroup{Name: name})
		return createWorkspaceGroupMsg{group: group, err: err}
	}
}

func updateWorkspaceGroupCmd(api WorkspaceGroupUpdateAPI, id, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		group, err := api.UpdateWorkspaceGroup(ctx, id, &types.WorkspaceGroup{Name: name})
		return updateWorkspaceGroupMsg{group: group, err: err}
	}
}

func deleteWorkspaceGroupCmd(api WorkspaceGroupDeleteAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err := api.DeleteWorkspaceGroup(ctx, id)
		return deleteWorkspaceGroupMsg{id: id, err: err}
	}
}

func updateWorkspaceCmd(api WorkspaceUpdateAPI, id, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		workspace, err := api.UpdateWorkspace(ctx, id, &types.Workspace{Name: name})
		return updateWorkspaceMsg{workspace: workspace, err: err}
	}
}

func updateWorkspaceGroupsCmd(api WorkspaceUpdateAPI, id string, groupIDs []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		workspace, err := api.UpdateWorkspace(ctx, id, &types.Workspace{GroupIDs: groupIDs})
		return updateWorkspaceMsg{workspace: workspace, err: err}
	}
}

func updateSessionCmd(api SessionUpdateAPI, id, title string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err := api.UpdateSession(ctx, id, client.UpdateSessionRequest{Title: title})
		return updateSessionMsg{id: id, err: err}
	}
}

func updateSessionRuntimeCmd(api SessionUpdateAPI, id string, runtimeOptions *types.SessionRuntimeOptions) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err := api.UpdateSession(ctx, id, client.UpdateSessionRequest{RuntimeOptions: types.CloneRuntimeOptions(runtimeOptions)})
		return updateSessionMsg{id: id, err: err}
	}
}

func assignGroupWorkspacesCmd(api WorkspaceUpdateAPI, groupID string, workspaceIDs []string, workspaces []*types.Workspace) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if strings.TrimSpace(groupID) == "" {
			return assignGroupWorkspacesMsg{groupID: groupID, err: errors.New("group id is required")}
		}
		selected := map[string]bool{}
		for _, id := range workspaceIDs {
			selected[strings.TrimSpace(id)] = true
		}
		updated := 0
		for _, ws := range workspaces {
			if ws == nil {
				continue
			}
			next := applyGroupAssignment(ws.GroupIDs, groupID, selected[ws.ID])
			if slicesEqual(ws.GroupIDs, next) {
				continue
			}
			if _, err := api.UpdateWorkspace(ctx, ws.ID, &types.Workspace{GroupIDs: next}); err != nil {
				return assignGroupWorkspacesMsg{groupID: groupID, err: err, updated: updated}
			}
			updated++
		}
		return assignGroupWorkspacesMsg{groupID: groupID, updated: updated}
	}
}

func applyGroupAssignment(current []string, groupID string, selected bool) []string {
	out := make([]string, 0, len(current)+1)
	found := false
	for _, id := range current {
		if id == groupID {
			found = true
			if selected {
				out = append(out, id)
			}
			continue
		}
		out = append(out, id)
	}
	if selected && !found {
		out = append(out, groupID)
	}
	return out
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func deleteWorkspaceCmd(api WorkspaceDeleteAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err := api.DeleteWorkspace(ctx, id)
		return deleteWorkspaceMsg{id: id, err: err}
	}
}

func createWorktreeCmd(api WorktreeCreateAPI, workspaceID string, req client.CreateWorktreeRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		worktree, err := api.CreateWorktree(ctx, workspaceID, req)
		return createWorktreeMsg{workspaceID: workspaceID, worktree: worktree, err: err}
	}
}

func addWorktreeCmd(api WorktreeAddAPI, workspaceID string, worktree *types.Worktree) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		created, err := api.AddWorktree(ctx, workspaceID, worktree)
		return addWorktreeMsg{workspaceID: workspaceID, worktree: created, err: err}
	}
}

func deleteWorktreeCmd(api WorktreeDeleteAPI, workspaceID, worktreeID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err := api.DeleteWorktree(ctx, workspaceID, worktreeID)
		return worktreeDeletedMsg{workspaceID: workspaceID, worktreeID: worktreeID, err: err}
	}
}

func fetchTailCmd(api SessionTailAPI, id, key string) tea.Cmd {
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

func fetchHistoryCmd(api SessionHistoryAPI, id, key string, lines int) tea.Cmd {
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

func openStreamCmd(api SessionTailStreamAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancelCtx := context.WithCancel(context.Background())
		ch, cancel, err := api.TailStream(ctx, id, "combined")
		if err != nil {
			cancelCtx()
		}
		return streamMsg{id: id, ch: ch, cancel: cancel, err: err}
	}
}

func openEventsCmd(api SessionEventStreamAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancelCtx := context.WithCancel(context.Background())
		ch, cancel, err := api.EventStream(ctx, id)
		if err != nil {
			cancelCtx()
		}
		return eventsMsg{id: id, ch: ch, cancel: cancel, err: err}
	}
}

func openItemsCmd(api SessionItemsStreamAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancelCtx := context.WithCancel(context.Background())
		ch, cancel, err := api.ItemsStream(ctx, id)
		if err != nil {
			cancelCtx()
		}
		return itemsStreamMsg{id: id, ch: ch, cancel: cancel, err: err}
	}
}

func fetchApprovalsCmd(api SessionApprovalsAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		approvals, err := api.ListApprovals(ctx, id)
		return approvalsMsg{id: id, approvals: approvals, err: err}
	}
}

func killSessionCmd(api SessionKillAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		err := api.KillSession(ctx, id)
		return killMsg{id: id, err: err}
	}
}

func markExitedCmd(api SessionMarkExitedAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		err := api.MarkSessionExited(ctx, id)
		return exitMsg{id: id, err: err}
	}
}

func markExitedManyCmd(api SessionMarkExitedAPI, ids []string) tea.Cmd {
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

func sendSessionCmd(api SessionSendAPI, id, text string, token int) tea.Cmd {
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

func interruptSessionCmd(api SessionInterruptAPI, id string) tea.Cmd {
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

func historyPollCmd(id, key string, attempt int, delay time.Duration, minAgents int) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return historyPollMsg{id: id, key: key, attempt: attempt, minAgents: minAgents}
	})
}

func approveSessionCmd(api SessionApproveAPI, id string, requestID int, decision string) tea.Cmd {
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

func startSessionCmd(api WorkspaceSessionStartAPI, workspaceID, worktreeID, provider, text string, runtimeOptions *types.SessionRuntimeOptions) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		req := client.StartSessionRequest{
			Provider:       provider,
			Text:           text,
			RuntimeOptions: types.CloneRuntimeOptions(runtimeOptions),
		}
		session, err := api.StartWorkspaceSession(ctx, workspaceID, worktreeID, req)
		return startSessionMsg{session: session, err: err}
	}
}
