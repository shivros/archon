package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"control/internal/config"
	"control/internal/logging"
	"control/internal/types"
)

const codexTimestampCacheFileName = "codex_item_timestamps.json"

var codexTimestampCacheMu sync.Mutex

type codexHistoryTimestampStats struct {
	Total                uint64
	NativeTimestampCount uint64
	CacheHitCount        uint64
	DaemonFilledCount    uint64
}

type codexHistoryTimestampCache struct {
	Items map[string]string `json:"items"`
}

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
	items = openCodeCompactShadowItems(items)
	return items, truncated, nil
}

func (s *SessionService) readSessionDebug(id string, lines int) ([]types.DebugEvent, bool, error) {
	baseDir, err := s.sessionsBaseDir()
	if err != nil {
		return nil, false, err
	}
	sessionDir := filepath.Join(baseDir, id)
	debugPath := filepath.Join(sessionDir, "debug.jsonl")
	if _, err := os.Stat(debugPath); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	rawLines, truncated, err := tailLines(debugPath, lines)
	if err != nil {
		return nil, false, err
	}
	events := make([]types.DebugEvent, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event types.DebugEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type == "" {
			event.Type = "debug"
		}
		if event.SessionID == "" {
			event.SessionID = strings.TrimSpace(id)
		}
		events = append(events, event)
	}
	return events, truncated, nil
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
	appended := make([]map[string]any, 0, len(items))
	for _, item := range items {
		prepared := prepareItemForPersistence(item, time.Now().UTC())
		if prepared == nil {
			continue
		}
		data, err := json.Marshal(prepared)
		if err != nil {
			continue
		}
		data = append(data, '\n')
		if _, err := file.Write(data); err != nil {
			return err
		}
		appended = append(appended, prepared)
	}
	if len(appended) > 0 && s.manager != nil {
		s.manager.BroadcastItems(id, appended)
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
	items := flattenCodexItemsWithLimit(thread, lines)
	items, stats, hydrateErr := s.hydrateCodexHistoryItemTimestamps(session.ID, items)
	if hydrateErr != nil {
		s.logger.Warn(
			"codex_history_timestamp_cache_error",
			logging.F("session_id", strings.TrimSpace(session.ID)),
			logging.F("thread_id", strings.TrimSpace(threadID)),
			logging.F("error", hydrateErr),
		)
	}
	if stats.Total > 0 {
		missingNative := stats.Total - stats.NativeTimestampCount
		s.logger.Info(
			"codex_history_timestamp_stats",
			logging.F("session_id", strings.TrimSpace(session.ID)),
			logging.F("thread_id", strings.TrimSpace(threadID)),
			logging.F("items_total", stats.Total),
			logging.F("native_timestamp_count", stats.NativeTimestampCount),
			logging.F("native_timestamp_pct", percentage(stats.NativeTimestampCount, stats.Total)),
			logging.F("missing_native_timestamp_count", missingNative),
			logging.F("missing_native_timestamp_pct", percentage(missingNative, stats.Total)),
			logging.F("cache_hit_count", stats.CacheHitCount),
			logging.F("cache_hit_pct", percentage(stats.CacheHitCount, stats.Total)),
			logging.F("daemon_filled_count", stats.DaemonFilledCount),
			logging.F("daemon_filled_pct", percentage(stats.DaemonFilledCount, stats.Total)),
		)
	}
	return items, nil
}

func flattenCodexItemsWithLimit(thread *codexThread, limit int) []map[string]any {
	return trimItemsToLimit(flattenCodexItems(thread), limit)
}

func flattenCodexItems(thread *codexThread) []map[string]any {
	if thread == nil {
		return nil
	}
	items := make([]map[string]any, 0)
	for _, turn := range thread.Turns {
		turnID := strings.TrimSpace(turn.ID)
		for _, item := range turn.Items {
			if item == nil {
				continue
			}
			clone := cloneItemMap(item)
			if turnID != "" {
				if strings.TrimSpace(asString(clone["turn_id"])) == "" && strings.TrimSpace(asString(clone["turnID"])) == "" {
					clone["turn_id"] = turnID
				}
			}
			items = append(items, clone)
		}
	}
	return items
}

func (s *SessionService) hydrateCodexHistoryItemTimestamps(sessionID string, items []map[string]any) ([]map[string]any, codexHistoryTimestampStats, error) {
	stats := codexHistoryTimestampStats{
		Total: uint64(len(items)),
	}
	if len(items) == 0 {
		return nil, stats, nil
	}

	cachePath, pathErr := s.codexTimestampCachePath(sessionID)
	cache := codexHistoryTimestampCache{
		Items: map[string]string{},
	}

	codexTimestampCacheMu.Lock()
	defer codexTimestampCacheMu.Unlock()

	if pathErr == nil {
		loaded, err := readCodexHistoryTimestampCache(cachePath)
		if err != nil {
			pathErr = err
		} else if loaded != nil {
			cache = *loaded
		}
	}

	now := time.Now().UTC()
	updated := false
	enriched := make([]map[string]any, 0, len(items))
	for index, item := range items {
		if item == nil {
			enriched = append(enriched, nil)
			continue
		}
		if !resolveItemCreatedAt(item).IsZero() {
			stats.NativeTimestampCount++
		}

		preparedInput := cloneItemMap(item)
		cacheKey := codexHistoryItemTimestampKey(preparedInput)
		usedCache := false
		if cacheKey != "" {
			if cached := parsePersistedTimestamp(cache.Items[cacheKey]); !cached.IsZero() {
				preparedInput["created_at"] = cached.UTC().Format(time.RFC3339Nano)
				usedCache = true
			}
		}

		prepared, classification := prepareItemForPersistenceWithClassification(preparedInput, now.Add(time.Duration(index)*time.Nanosecond))
		if prepared == nil {
			enriched = append(enriched, nil)
			continue
		}
		if usedCache {
			stats.CacheHitCount++
		}
		if classification.UsedDaemonTimestamp {
			stats.DaemonFilledCount++
		}
		if cacheKey != "" {
			if createdAt := strings.TrimSpace(asString(prepared["created_at"])); createdAt != "" && cache.Items[cacheKey] != createdAt {
				cache.Items[cacheKey] = createdAt
				updated = true
			}
		}
		enriched = append(enriched, prepared)
	}

	if updated && pathErr == nil {
		if err := writeCodexHistoryTimestampCache(cachePath, cache); err != nil {
			pathErr = err
		}
	}
	return enriched, stats, pathErr
}

func (s *SessionService) codexTimestampCachePath(sessionID string) (string, error) {
	baseDir, err := s.sessionsBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, strings.TrimSpace(sessionID), codexTimestampCacheFileName), nil
}

func readCodexHistoryTimestampCache(path string) (*codexHistoryTimestampCache, error) {
	if strings.TrimSpace(path) == "" {
		return &codexHistoryTimestampCache{Items: map[string]string{}}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &codexHistoryTimestampCache{Items: map[string]string{}}, nil
		}
		return nil, err
	}
	var cache codexHistoryTimestampCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	if cache.Items == nil {
		cache.Items = map[string]string{}
	}
	return &cache, nil
}

func writeCodexHistoryTimestampCache(path string, cache codexHistoryTimestampCache) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if cache.Items == nil {
		cache.Items = map[string]string{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(path), "codex-item-ts-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func codexHistoryItemTimestampKey(item map[string]any) string {
	if item == nil {
		return ""
	}
	for _, key := range []string{"id", "item_id", "provider_message_id"} {
		if value := strings.TrimSpace(asString(item[key])); value != "" {
			return key + ":" + value
		}
	}
	if message, ok := item["message"].(map[string]any); ok && message != nil {
		if value := strings.TrimSpace(asString(message["id"])); value != "" {
			return "message.id:" + value
		}
	}
	if info, ok := item["info"].(map[string]any); ok && info != nil {
		if value := strings.TrimSpace(asString(info["id"])); value != "" {
			return "info.id:" + value
		}
	}

	sanitized, ok := removeTimestampFields(item).(map[string]any)
	if !ok || sanitized == nil {
		return ""
	}
	data, err := json.Marshal(sanitized)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return "hash:" + hex.EncodeToString(sum[:])
}

func removeTimestampFields(raw any) any {
	switch typed := raw.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			if isTimestampField(key) {
				continue
			}
			out[key] = removeTimestampFields(value)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, value := range typed {
			out = append(out, removeTimestampFields(value))
		}
		return out
	default:
		return typed
	}
}

func isTimestampField(key string) bool {
	switch strings.TrimSpace(key) {
	case "provider_created_at", "created_at", "createdAt", "created", "ts", "timestamp":
		return true
	default:
		return false
	}
}
