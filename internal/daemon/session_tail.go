package daemon

import (
	"context"
	"encoding/json"
	"os"
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
	baseDir, err := s.sessionsBaseDir()
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

func (s *SessionService) readSessionItems(id string, lines int) ([]map[string]any, bool, error) {
	baseDir, err := s.sessionsBaseDir()
	if err != nil {
		return nil, false, err
	}
	sessionDir := filepath.Join(baseDir, id)
	itemsPath := filepath.Join(sessionDir, "items.jsonl")
	if _, err := os.Stat(itemsPath); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	rawLines, truncated, err := tailLines(itemsPath, lines)
	if err != nil {
		return nil, false, err
	}
	items := make([]map[string]any, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		if payload != nil {
			items = append(items, payload)
		}
	}
	return items, truncated, nil
}

func (s *SessionService) appendSessionItems(id string, items []map[string]any) error {
	if strings.TrimSpace(id) == "" || len(items) == 0 {
		return nil
	}
	baseDir, err := s.sessionsBaseDir()
	if err != nil {
		return err
	}
	sessionDir := filepath.Join(baseDir, id)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return err
	}
	itemsPath := filepath.Join(sessionDir, "items.jsonl")
	file, err := os.OpenFile(itemsPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, item := range items {
		if item == nil {
			continue
		}
		data, err := json.Marshal(item)
		if err != nil {
			continue
		}
		data = append(data, '\n')
		if _, err := file.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func (s *SessionService) sessionsBaseDir() (string, error) {
	if s != nil && s.manager != nil {
		if baseDir := s.manager.SessionsBaseDir(); baseDir != "" {
			return baseDir, nil
		}
	}
	return config.SessionsDir()
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
	if s.codexPool == nil {
		s.codexPool = NewCodexHistoryPool(s.logger)
	}
	thread, err := s.codexPool.ReadThread(ctx, session.Cwd, codexHome, threadID)
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
