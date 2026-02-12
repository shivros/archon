package app

import (
	"sort"
	"strings"

	"control/internal/types"
)

func (m *Model) syncAppStateInputDrafts() {
	if m == nil {
		return
	}
	m.appState.ComposeDrafts = exportDraftMap(m.composeDrafts, composeHistoryMaxSessions)
	m.appState.NoteDrafts = exportDraftMap(m.noteDrafts, composeHistoryMaxSessions)
}

func (m *Model) saveCurrentComposeDraft() bool {
	if m == nil || m.chatInput == nil {
		return false
	}
	return m.setComposeDraft(m.composeSessionID(), m.chatInput.Value())
}

func (m *Model) restoreComposeDraft(sessionID string) bool {
	if m == nil || m.chatInput == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	value := ""
	if sessionID != "" && m.composeDrafts != nil {
		value = m.composeDrafts[sessionID]
	}
	m.chatInput.SetValue(value)
	return strings.TrimSpace(value) != ""
}

func (m *Model) clearComposeDraft(sessionID string) bool {
	return m.setComposeDraft(sessionID, "")
}

func (m *Model) setComposeDraft(sessionID, value string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	next, changed := setDraftValue(m.composeDrafts, sessionID, value, composeHistoryMaxSessions)
	if !changed {
		return false
	}
	m.composeDrafts = next
	m.hasAppState = true
	m.syncAppStateInputDrafts()
	return true
}

func (m *Model) saveCurrentNoteDraft() bool {
	if m == nil || m.noteInput == nil {
		return false
	}
	return m.setNoteDraft(m.notesScope, m.noteInput.Value())
}

func (m *Model) restoreNoteDraft(scope noteScopeTarget) bool {
	if m == nil || m.noteInput == nil {
		return false
	}
	key := scope.DraftKey()
	value := ""
	if key != "" && m.noteDrafts != nil {
		value = m.noteDrafts[key]
	}
	m.noteInput.SetValue(value)
	return strings.TrimSpace(value) != ""
}

func (m *Model) clearNoteDraft(scope noteScopeTarget) bool {
	return m.setNoteDraft(scope, "")
}

func (m *Model) setNoteDraft(scope noteScopeTarget, value string) bool {
	key := scope.DraftKey()
	if key == "" {
		return false
	}
	next, changed := setDraftValue(m.noteDrafts, key, value, composeHistoryMaxSessions)
	if !changed {
		return false
	}
	m.noteDrafts = next
	m.hasAppState = true
	m.syncAppStateInputDrafts()
	return true
}

func (s noteScopeTarget) DraftKey() string {
	scope := strings.TrimSpace(string(s.Scope))
	workspaceID := strings.TrimSpace(s.WorkspaceID)
	worktreeID := strings.TrimSpace(s.WorktreeID)
	sessionID := strings.TrimSpace(s.SessionID)

	switch s.Scope {
	case types.NoteScopeWorkspace:
		if workspaceID == "" {
			return ""
		}
		return "workspace:" + workspaceID
	case types.NoteScopeWorktree:
		if workspaceID == "" || worktreeID == "" {
			return ""
		}
		return "worktree:" + workspaceID + ":" + worktreeID
	case types.NoteScopeSession:
		if sessionID == "" {
			return ""
		}
		return "session:" + sessionID
	default:
		if scope == "" {
			return ""
		}
	}
	return ""
}

func normalizeDraftValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}

func setDraftValue(raw map[string]string, key, value string, limit int) (map[string]string, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return raw, false
	}
	value = normalizeDraftValue(value)
	if raw == nil {
		raw = map[string]string{}
	}
	if value == "" {
		if _, ok := raw[key]; !ok {
			return raw, false
		}
		delete(raw, key)
		return raw, true
	}
	if current, ok := raw[key]; ok && current == value {
		return raw, false
	}
	raw[key] = value
	trimDraftMap(raw, limit)
	return raw, true
}

func trimDraftMap(raw map[string]string, limit int) {
	if len(raw) == 0 || limit <= 0 || len(raw) <= limit {
		return
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	excess := len(raw) - limit
	for i := 0; i < excess && i < len(keys); i++ {
		delete(raw, keys[i])
	}
}

func importDraftMap(raw map[string]string, limit int) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if limit > 0 && len(keys) > limit {
		keys = keys[len(keys)-limit:]
	}
	for _, key := range keys {
		value := normalizeDraftValue(raw[key])
		if value == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func exportDraftMap(raw map[string]string, limit int) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	keys := make([]string, 0, len(raw))
	for key := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if limit > 0 && len(keys) > limit {
		keys = keys[len(keys)-limit:]
	}
	for _, key := range keys {
		value := normalizeDraftValue(raw[key])
		if value == "" {
			continue
		}
		out[key] = value
	}
	return out
}
