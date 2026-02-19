package app

import "strings"

const defaultSelectionHistoryLimit = 256

type SelectionHistory interface {
	Visit(key string)
	SyncCurrent(key string)
	Back(valid func(string) bool) (string, bool)
	Forward(valid func(string) bool) (string, bool)
}

type boundedSelectionHistory struct {
	entries []string
	index   int
	limit   int
}

func NewSelectionHistory(limit int) SelectionHistory {
	if limit <= 0 {
		limit = defaultSelectionHistoryLimit
	}
	return &boundedSelectionHistory{
		entries: nil,
		index:   -1,
		limit:   limit,
	}
}

func (h *boundedSelectionHistory) Visit(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if h == nil {
		return
	}
	if h.index >= 0 && h.index < len(h.entries) && h.entries[h.index] == key {
		return
	}
	if h.index >= 0 && h.index+1 < len(h.entries) {
		h.entries = append([]string(nil), h.entries[:h.index+1]...)
	}
	h.entries = append(h.entries, key)
	if len(h.entries) > h.limit {
		trim := len(h.entries) - h.limit
		h.entries = append([]string(nil), h.entries[trim:]...)
		h.index -= trim
		if h.index < 0 {
			h.index = 0
		}
	}
	h.index = len(h.entries) - 1
}

func (h *boundedSelectionHistory) SyncCurrent(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if h == nil {
		return
	}
	if h.index < 0 || h.index >= len(h.entries) {
		h.entries = []string{key}
		h.index = 0
		return
	}
	h.entries[h.index] = key
}

func (h *boundedSelectionHistory) Back(valid func(string) bool) (string, bool) {
	if h == nil || h.index <= 0 {
		return "", false
	}
	if valid == nil {
		valid = alwaysValidSelectionHistoryKey
	}
	for i := h.index - 1; i >= 0; i-- {
		key := strings.TrimSpace(h.entries[i])
		if key == "" || !valid(key) {
			continue
		}
		h.index = i
		return key, true
	}
	return "", false
}

func (h *boundedSelectionHistory) Forward(valid func(string) bool) (string, bool) {
	if h == nil || h.index < 0 || h.index+1 >= len(h.entries) {
		return "", false
	}
	if valid == nil {
		valid = alwaysValidSelectionHistoryKey
	}
	for i := h.index + 1; i < len(h.entries); i++ {
		key := strings.TrimSpace(h.entries[i])
		if key == "" || !valid(key) {
			continue
		}
		h.index = i
		return key, true
	}
	return "", false
}

func alwaysValidSelectionHistoryKey(string) bool { return true }
