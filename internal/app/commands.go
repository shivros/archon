package app

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"control/internal/client"
	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

type fetchSessionsOptions struct {
	refresh              bool
	workspaceID          string
	includeDismissed     bool
	includeWorkflowOwned bool
}

const recentsCompletionWatchTimeout = 15 * time.Second

func commandParentContext(parent context.Context) context.Context {
	if parent == nil {
		return context.Background()
	}
	return parent
}

func commandWithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(commandParentContext(parent), timeout)
}

func fetchSessionsWithMetaCmd(api SessionListWithMetaQueryAPI, options ...fetchSessionsOptions) tea.Cmd {
	opts := fetchSessionsOptions{}
	if len(options) > 0 {
		opts = options[0]
	}
	return func() tea.Msg {
		timeout := 4 * time.Second
		if opts.refresh {
			timeout = 95 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		var (
			sessions []*types.Session
			meta     []*types.SessionMeta
			err      error
		)
		sessions, meta, err = api.ListSessionsWithMetaQuery(ctx, SessionListQuery{
			Refresh:              opts.refresh,
			WorkspaceID:          opts.workspaceID,
			IncludeDismissed:     opts.includeDismissed,
			IncludeWorkflowOwned: opts.includeWorkflowOwned,
		})
		return sessionsWithMetaMsg{sessions: sessions, meta: meta, err: err}
	}
}

func fetchProviderOptionsCmd(api SessionProviderOptionsAPI, provider string) tea.Cmd {
	return fetchProviderOptionsCmdWithContext(api, provider, nil)
}

func fetchProviderOptionsCmdWithContext(api SessionProviderOptionsAPI, provider string, parent context.Context) tea.Cmd {
	return func() tea.Msg {
		timeout := 4 * time.Second
		switch strings.ToLower(strings.TrimSpace(provider)) {
		case "opencode", "kilocode":
			// Give local server auto-start and cold plugin init enough time.
			timeout = 90 * time.Second
		}
		ctx, cancel := commandWithTimeout(parent, timeout)
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

func fetchWorktreesCmdWithContext(api WorktreeListAPI, workspaceID string, parent context.Context) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := commandWithTimeout(parent, 4*time.Second)
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

func notesPanelReflowCmd() tea.Cmd {
	return func() tea.Msg {
		return notesPanelReflowMsg{}
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

func moveNoteCmd(api NoteUpdateAPI, previous *types.Note, target noteScopeTarget) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		if previous == nil {
			return noteMovedMsg{err: errors.New("note is required")}
		}
		noteID := strings.TrimSpace(previous.ID)
		if noteID == "" {
			return noteMovedMsg{err: errors.New("note id is required")}
		}
		patch := &types.Note{
			Scope:       target.Scope,
			WorkspaceID: strings.TrimSpace(target.WorkspaceID),
			WorktreeID:  strings.TrimSpace(target.WorktreeID),
			SessionID:   strings.TrimSpace(target.SessionID),
		}
		updated, err := api.UpdateNote(ctx, noteID, patch)
		prev := cloneNoteForMessage(previous)
		return noteMovedMsg{note: updated, previous: prev, err: err}
	}
}

func cloneNoteForMessage(note *types.Note) *types.Note {
	if note == nil {
		return nil
	}
	cloned := *note
	if note.Tags != nil {
		cloned.Tags = append([]string(nil), note.Tags...)
	}
	if note.Source != nil {
		source := *note.Source
		cloned.Source = &source
	}
	return &cloned
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
		return appStateInitialLoadMsg{state: state, err: err}
	}
}

func createWorkspaceCmd(api WorkspaceCreateAPI, path, sessionSubpath, name string, additionalDirectories, groupIDs []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		workspace, err := api.CreateWorkspace(ctx, &types.Workspace{
			Name:                  name,
			RepoPath:              path,
			SessionSubpath:        sessionSubpath,
			AdditionalDirectories: append([]string(nil), additionalDirectories...),
			GroupIDs:              append([]string(nil), groupIDs...),
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

func updateWorkspaceCmd(api WorkspaceUpdateAPI, id string, patch *types.WorkspacePatch) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		workspace, err := api.UpdateWorkspace(ctx, id, patch)
		return updateWorkspaceMsg{workspace: workspace, err: err}
	}
}

func updateWorkspaceGroupsCmd(api WorkspaceUpdateAPI, id string, groupIDs []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		normalized := normalizePatchStringSlice(groupIDs)
		workspace, err := api.UpdateWorkspace(ctx, id, &types.WorkspacePatch{GroupIDs: &normalized})
		return updateWorkspaceMsg{workspace: workspace, err: err}
	}
}

func normalizePatchStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	return append(out, values...)
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

func createWorkflowRunCmd(api GuidedWorkflowRunAPI, req client.CreateWorkflowRunRequest) tea.Cmd {
	return func() tea.Msg {
		if api == nil {
			return workflowRunCreatedMsg{err: errors.New("guided workflow api is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		run, err := api.CreateWorkflowRun(ctx, req)
		return workflowRunCreatedMsg{run: run, err: err}
	}
}

func fetchWorkflowTemplatesCmd(api GuidedWorkflowTemplateAPI) tea.Cmd {
	return func() tea.Msg {
		if api == nil {
			return workflowTemplatesMsg{err: errors.New("guided workflow api is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		templates, err := api.ListWorkflowTemplates(ctx)
		return workflowTemplatesMsg{templates: templates, err: err}
	}
}

func fetchWorkflowRunsCmd(api GuidedWorkflowRunAPI, includeDismissed bool) tea.Cmd {
	return func() tea.Msg {
		if api == nil {
			return workflowRunsMsg{err: errors.New("guided workflow api is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		runs, err := api.ListWorkflowRunsWithOptions(ctx, includeDismissed)
		return workflowRunsMsg{runs: runs, err: err}
	}
}

func startWorkflowRunCmd(api GuidedWorkflowRunAPI, runID string) tea.Cmd {
	return func() tea.Msg {
		if api == nil {
			return workflowRunStartedMsg{err: errors.New("guided workflow api is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		run, err := api.StartWorkflowRun(ctx, runID)
		return workflowRunStartedMsg{run: run, err: err}
	}
}

func stopWorkflowRunCmd(api GuidedWorkflowRunAPI, runID string) tea.Cmd {
	return func() tea.Msg {
		if api == nil {
			return workflowRunStoppedMsg{err: errors.New("guided workflow api is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		run, err := api.StopWorkflowRun(ctx, runID)
		return workflowRunStoppedMsg{run: run, err: err}
	}
}

func resumeFailedWorkflowRunCmd(api GuidedWorkflowRunAPI, runID string, req client.WorkflowRunResumeRequest) tea.Cmd {
	return func() tea.Msg {
		if api == nil {
			return workflowRunResumedMsg{err: errors.New("guided workflow api is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		run, err := api.ResumeFailedWorkflowRun(ctx, runID, req)
		return workflowRunResumedMsg{run: run, err: err}
	}
}

func renameWorkflowRunCmd(api GuidedWorkflowRunAPI, runID, name string) tea.Cmd {
	runID = strings.TrimSpace(runID)
	name = strings.TrimSpace(name)
	return func() tea.Msg {
		if api == nil {
			return workflowRunRenamedMsg{err: errors.New("guided workflow api is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		run, err := api.RenameWorkflowRun(ctx, runID, name)
		return workflowRunRenamedMsg{run: run, err: err}
	}
}

func fetchWorkflowRunSnapshotCmd(api GuidedWorkflowRunAPI, runID string) tea.Cmd {
	return func() tea.Msg {
		if api == nil {
			return workflowRunSnapshotMsg{err: errors.New("guided workflow api is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		run, err := api.GetWorkflowRun(ctx, runID)
		if err != nil {
			return workflowRunSnapshotMsg{err: err}
		}
		timeline, err := api.GetWorkflowRunTimeline(ctx, runID)
		return workflowRunSnapshotMsg{run: run, timeline: timeline, err: err}
	}
}

func decideWorkflowRunCmd(api GuidedWorkflowRunAPI, runID string, req client.WorkflowRunDecisionRequest) tea.Cmd {
	return func() tea.Msg {
		if api == nil {
			return workflowRunDecisionMsg{err: errors.New("guided workflow api is unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		run, err := api.DecideWorkflowRun(ctx, runID, req)
		return workflowRunDecisionMsg{run: run, err: err}
	}
}

func dismissWorkflowRunCmd(api GuidedWorkflowRunAPI, runID string) tea.Cmd {
	runID = strings.TrimSpace(runID)
	return func() tea.Msg {
		if api == nil {
			return workflowRunVisibilityMsg{runID: runID, err: errors.New("guided workflow api is unavailable"), dismissed: true}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		run, err := api.DismissWorkflowRun(ctx, runID)
		return workflowRunVisibilityMsg{runID: runID, run: run, err: err, dismissed: true}
	}
}

func undismissWorkflowRunCmd(api GuidedWorkflowRunAPI, runID string) tea.Cmd {
	runID = strings.TrimSpace(runID)
	return func() tea.Msg {
		if api == nil {
			return workflowRunVisibilityMsg{runID: runID, err: errors.New("guided workflow api is unavailable"), dismissed: false}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		run, err := api.UndismissWorkflowRun(ctx, runID)
		return workflowRunVisibilityMsg{runID: runID, run: run, err: err, dismissed: false}
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
			normalized := normalizePatchStringSlice(next)
			if _, err := api.UpdateWorkspace(ctx, ws.ID, &types.WorkspacePatch{GroupIDs: &normalized}); err != nil {
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

func updateWorktreeCmd(api WorktreeUpdateAPI, workspaceID, worktreeID, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		worktree, err := api.UpdateWorktree(ctx, workspaceID, worktreeID, &types.Worktree{Name: name})
		return updateWorktreeMsg{workspaceID: workspaceID, worktree: worktree, err: err}
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

func fetchHistoryCmdWithContext(api SessionHistoryAPI, id, key string, lines int, parent context.Context) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := commandWithTimeout(parent, 8*time.Second)
		defer cancel()
		resp, err := api.History(ctx, id, lines)
		if err != nil {
			return historyMsg{id: id, err: err, key: key, requestedLines: lines}
		}
		return historyMsg{id: id, items: resp.Items, key: key, requestedLines: lines}
	}
}

func fetchTranscriptSnapshotCmdWithContext(api SessionTranscriptSnapshotAPI, id, key string, lines int, parent context.Context) tea.Cmd {
	return fetchTranscriptSnapshotCmdWithContextAndRequest(api, id, key, lines, parent, transcriptSnapshotRequest{})
}

type transcriptSnapshotRequest struct {
	Source        TranscriptAttachmentSource
	Authoritative bool
}

func fetchTranscriptSnapshotCmdWithContextAndRequest(
	api SessionTranscriptSnapshotAPI,
	id, key string,
	lines int,
	parent context.Context,
	request transcriptSnapshotRequest,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := commandWithTimeout(parent, 8*time.Second)
		defer cancel()
		resp, err := api.GetTranscriptSnapshot(ctx, id, lines)
		if err != nil {
			return transcriptSnapshotMsg{
				id:             id,
				key:            key,
				err:            err,
				requestedLines: lines,
				source:         normalizeTranscriptAttachmentSource(request.Source),
				authoritative:  request.Authoritative,
			}
		}
		return transcriptSnapshotMsg{
			id:             id,
			key:            key,
			snapshot:       resp,
			requestedLines: lines,
			source:         normalizeTranscriptAttachmentSource(request.Source),
			authoritative:  request.Authoritative,
		}
	}
}

func retryTranscriptSnapshotCmdWithDelay(
	api SessionTranscriptSnapshotAPI,
	id, key string,
	lines int,
	parent context.Context,
	delay time.Duration,
	request transcriptSnapshotRequest,
) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		cmd := fetchTranscriptSnapshotCmdWithContextAndRequest(api, id, key, lines, parent, request)
		if cmd == nil {
			return nil
		}
		return cmd()
	})
}

func fetchRecentsPreviewCmd(api SessionHistoryAPI, id, revision string, lines int) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		resp, err := api.History(ctx, id, lines)
		if err != nil {
			return recentsPreviewMsg{id: id, revision: revision, err: err}
		}
		text := ""
		if resp != nil {
			text = latestAssistantBlockText(itemsToBlocks(resp.Items))
		}
		return recentsPreviewMsg{id: id, revision: revision, text: text}
	}
}

func openTranscriptStreamCmd(api SessionTranscriptStreamAPI, id, afterRevision string) tea.Cmd {
	return openTranscriptStreamCmdWithContextAndRequest(api, id, afterRevision, nil, transcriptStreamOpenRequest{})
}

func openTranscriptStreamCmdWithContext(api SessionTranscriptStreamAPI, id, afterRevision string, parent context.Context) tea.Cmd {
	return openTranscriptStreamCmdWithContextAndRequest(api, id, afterRevision, parent, transcriptStreamOpenRequest{})
}

type transcriptStreamOpenRequest struct {
	Source     TranscriptAttachmentSource
	Generation uint64
}

func openTranscriptStreamCmdWithRequest(api SessionTranscriptStreamAPI, id, afterRevision string, request transcriptStreamOpenRequest) tea.Cmd {
	return openTranscriptStreamCmdWithContextAndRequest(api, id, afterRevision, nil, request)
}

func openTranscriptStreamCmdWithContextAndRequest(
	api SessionTranscriptStreamAPI,
	id, afterRevision string,
	parent context.Context,
	request transcriptStreamOpenRequest,
) tea.Cmd {
	return func() tea.Msg {
		ch, cancel, err := api.TranscriptStream(commandParentContext(parent), id, afterRevision)
		return transcriptStreamMsg{
			id:         id,
			ch:         ch,
			cancel:     cancel,
			err:        err,
			revision:   afterRevision,
			source:     normalizeTranscriptAttachmentSource(request.Source),
			generation: request.Generation,
		}
	}
}

func watchRecentsTurnCompletionCmd(api SessionTranscriptAPI, signalPolicy RecentsCompletionSignalPolicy, id, expectedTurn string) tea.Cmd {
	return watchRecentsTurnCompletionCmdWithContext(api, signalPolicy, id, expectedTurn, nil)
}

func watchRecentsTurnCompletionCmdWithContext(api SessionTranscriptAPI, signalPolicy RecentsCompletionSignalPolicy, id, expectedTurn string, parent context.Context) tea.Cmd {
	if signalPolicy == nil {
		signalPolicy = transcriptEventRecentsCompletionSignalPolicy{}
	}
	return func() tea.Msg {
		id = strings.TrimSpace(id)
		expectedTurn = strings.TrimSpace(expectedTurn)
		if id == "" {
			return recentsTurnCompletedMsg{id: id, expectedTurn: expectedTurn, err: errors.New("session id is required")}
		}
		ctx, cancel := commandWithTimeout(parent, recentsCompletionWatchTimeout)
		defer cancel()
		ch, streamCancel, err := api.TranscriptStream(ctx, id, "")
		if err != nil {
			return recentsTurnCompletedMsg{id: id, expectedTurn: expectedTurn, err: err}
		}
		defer streamCancel()
		for {
			select {
			case <-ctx.Done():
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return recentsTurnCompletedMsg{id: id, expectedTurn: expectedTurn}
				}
				return recentsTurnCompletedMsg{id: id, expectedTurn: expectedTurn, err: ctx.Err()}
			case event, ok := <-ch:
				if !ok {
					return recentsTurnCompletedMsg{id: id, expectedTurn: expectedTurn}
				}
				turnID, matched := signalPolicy.CompletionFromTranscriptEvent(event)
				if !matched {
					continue
				}
				return recentsTurnCompletedMsg{
					id:           id,
					expectedTurn: expectedTurn,
					turnID:       turnID,
					matched:      true,
				}
			}
		}
	}
}

func openDebugStreamCmd(api SessionDebugStreamAPI, id string) tea.Cmd {
	return openDebugStreamCmdWithContext(api, id, nil)
}

func openDebugStreamCmdWithContext(api SessionDebugStreamAPI, id string, parent context.Context) tea.Cmd {
	return func() tea.Msg {
		ch, cancel, err := api.DebugStream(commandParentContext(parent), id)
		return debugStreamMsg{id: id, ch: ch, cancel: cancel, err: err}
	}
}

func openMetadataStreamCmd(api MetadataStreamAPI, afterRevision string) tea.Cmd {
	return func() tea.Msg {
		if api == nil {
			return metadataStreamMsg{err: errors.New("metadata stream api is unavailable"), afterRevision: afterRevision}
		}
		ch, cancel, err := api.MetadataStream(context.Background(), afterRevision)
		return metadataStreamMsg{ch: ch, cancel: cancel, err: err, afterRevision: afterRevision}
	}
}

func reconnectMetadataStreamCmd(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return metadataStreamReconnectMsg{}
	})
}

func fetchApprovalsCmdWithContext(api SessionApprovalsAPI, id string, parent context.Context) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := commandWithTimeout(parent, 4*time.Second)
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

func dismissSessionCmd(api SessionDismissAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		err := api.DismissSession(ctx, id)
		return dismissMsg{id: id, err: err}
	}
}

func dismissManySessionsCmd(api SessionDismissAPI, ids []string) tea.Cmd {
	return func() tea.Msg {
		if len(ids) == 0 {
			return bulkDismissMsg{ids: ids}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		for _, id := range ids {
			if err := api.DismissSession(ctx, id); err != nil {
				return bulkDismissMsg{ids: ids, err: err}
			}
		}
		return bulkDismissMsg{ids: ids}
	}
}

func undismissSessionCmd(api SessionUndismissAPI, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		err := api.UndismissSession(ctx, id)
		return undismissMsg{id: id, err: err}
	}
}

func sendSessionCmd(api SessionSendAPI, id, text string, token int) tea.Cmd {
	return func() tea.Msg {
		log.Printf("ui send: id=%s text_len=%d", id, len(text))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
	return interruptSessionCmdWithContext(api, id, nil)
}

func interruptSessionCmdWithContext(api SessionInterruptAPI, id string, parent context.Context) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := commandWithTimeout(parent, 4*time.Second)
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

func approveSessionCmd(api SessionApproveAPI, id string, requestID int, decision string, responses []string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		req := client.ApproveSessionRequest{
			RequestID: requestID,
			Decision:  decision,
			Responses: responses,
		}
		err := api.ApproveSession(ctx, id, req)
		return approvalMsg{id: id, requestID: requestID, decision: decision, err: err}
	}
}

func startSessionCmd(api WorkspaceSessionStartAPI, workspaceID, worktreeID, provider, text string, runtimeOptions *types.SessionRuntimeOptions) tea.Cmd {
	return startSessionCmdWithContext(api, workspaceID, worktreeID, provider, text, runtimeOptions, nil)
}

func startSessionCmdWithContext(api WorkspaceSessionStartAPI, workspaceID, worktreeID, provider, text string, runtimeOptions *types.SessionRuntimeOptions, parent context.Context) tea.Cmd {
	return func() tea.Msg {
		timeout := 8 * time.Second
		switch strings.ToLower(strings.TrimSpace(provider)) {
		case "opencode", "kilocode":
			// OpenCode/Kilo cold starts can take longer on first run.
			timeout = 90 * time.Second
		}
		ctx, cancel := commandWithTimeout(parent, timeout)
		defer cancel()
		req := client.StartSessionRequest{
			Provider:       provider,
			Text:           text,
			RuntimeOptions: types.CloneRuntimeOptions(runtimeOptions),
		}
		session, err := api.StartWorkspaceSession(ctx, workspaceID, worktreeID, req)
		return startSessionMsg{session: session, prompt: text, err: err}
	}
}
