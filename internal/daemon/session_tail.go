package daemon

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"control/internal/config"
	"control/internal/types"
)

func sortSessionsByCreatedAt(sessions []*types.Session) {
	sort.Slice(sessions, func(i, j int) bool {
		left := sessions[i]
		right := sessions[j]
		if left == nil || right == nil {
			return left != nil
		}
		return left.CreatedAt.After(right.CreatedAt)
	})
}

func logLinesToItems(lines []string) []map[string]any {
	items := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		items = append(items, map[string]any{
			"type":   "log",
			"stream": "combined",
			"text":   line,
		})
	}
	return items
}

func (s *SessionService) readSessionLogs(id string, lines int) ([]string, bool, string, error) {
	baseDir, err := config.SessionsDir()
	if err != nil {
		return nil, false, "", err
	}
	sessionDir := filepath.Join(baseDir, id)
	stdoutPath := filepath.Join(sessionDir, "stdout.log")
	stderrPath := filepath.Join(sessionDir, "stderr.log")

	stdoutLines, stdoutTrunc, err := tailLines(stdoutPath, lines)
	if err != nil {
		return nil, false, "", err
	}
	stderrLines, stderrTrunc, err := tailLines(stderrPath, lines)
	if err != nil {
		return nil, false, "", err
	}
	combined := append(stdoutLines, stderrLines...)
	return combined, stdoutTrunc || stderrTrunc, "stdout_then_stderr", nil
}

func (s *SessionService) tailCodexThread(ctx context.Context, session *types.Session, threadID string, lines int) ([]map[string]any, error) {
	if session == nil {
		return nil, invalidError("session is required", nil)
	}
	if session.Cwd == "" {
		return nil, invalidError("session cwd is required", nil)
	}
	if strings.TrimSpace(threadID) == "" {
		threadID = session.ID
	}
	workspacePath := ""
	if s.stores != nil && s.stores.SessionMeta != nil && s.stores.Workspaces != nil {
		if meta, ok, err := s.stores.SessionMeta.Get(ctx, session.ID); err == nil && ok && meta != nil {
			if ws, ok, err := s.stores.Workspaces.Get(ctx, meta.WorkspaceID); err == nil && ok && ws != nil {
				workspacePath = ws.RepoPath
			}
		}
	}
	codexHome := resolveCodexHome(session.Cwd, workspacePath)
	client, err := startCodexAppServer(ctx, session.Cwd, codexHome)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	thread, err := client.ReadThread(ctx, threadID)
	if err != nil {
		return nil, err
	}
	items := flattenCodexItems(thread)
	return items, nil
}

func flattenCodexItems(thread *codexThread) []map[string]any {
	if thread == nil {
		return nil
	}
	items := make([]map[string]any, 0)
	for _, turn := range thread.Turns {
		for _, item := range turn.Items {
			items = append(items, item)
		}
	}
	return items
}
